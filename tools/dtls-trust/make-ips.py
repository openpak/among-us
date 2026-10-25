#!/usr/bin/env python3
"""make-ips.py <main.nso> <global-metadata.dat> [out-dir]: write the Among Us DTLS trust patch.

RELEASE build (default). The console client opens every online game connection with Hazel DTLS
and accepts only the server certificates its build pins (Hazel DtlsUnityConnection,
`serverCertificates.Contains(presented)`). The pinned set is the game's own self-signed
certificate, so no server without that private key can complete the handshake, and neither
nn::ssl nor the console certificate store is consulted.

This patch re-pins the client to OpenPak's own fixed DTLS certificate (O=OpenPak,
CN=OpenPak Among Us DTLS), and only that certificate. It does NOT weaken trust: the rejection
branch stays, and the comparison stays an exact DER match -- it just compares against OpenPak's
certificate instead of Innersloth's. The original certificate, and any other certificate, are
still rejected. The handshake still verifies the ServerKeyExchange signature against the
certificate the server sent, and the session is still encrypted (ECDHE X25519, AES-128-GCM).

How: at the one `serverCertificates.Contains(presented)` site the build's result (a bool that
gates accept/reject) is produced by a tiny routine placed in the executable padding at the end
of .text (a code cave). The routine fetches the presented certificate's raw DER
(X509Certificate2.get_RawData), requires its length and all bytes to equal OpenPak's DER (the
843 bytes of openpak-dtls-cert.pem, embedded read-only in a free .rodata gap), and returns the
same bool the original Contains returned (1 = present/accept, 0 = reject). No managed allocation
is added; the original parse, signature check and encryption are untouched.

Everything is found by shape / metadata / segment layout, not by hard-coded address, so a new
game update needs only a re-run with that update's main.nso + global-metadata.dat. Offsets in
the IPS include the NSO header (+0x100), the convention Atmosphere and the emulators share.

TEST build (do NOT ship): `make-ips.py --test <main.nso> [out-dir]` writes the historic
accept-any patch (one NOP over the reject branch -- accepts ANY server certificate). It needs
only the NSO. It exists for local bring-up and must never reach a patcher or Harbor built-in.
"""
import os
import re
import struct
import sys

NOP = 0xD503201F
HDR = 0x100            # NSO header, added to every IPS offset
CERT_PEM = "openpak-dtls-cert.pem"   # OpenPak's PUBLIC DTLS certificate, next to this script


# ---------------------------------------------------------------- NSO decode
def lz4_block(src, size):
    out = bytearray()
    i = 0
    while i < len(src):
        tok = src[i]
        i += 1
        lit = tok >> 4
        if lit == 15:
            while True:
                b = src[i]
                i += 1
                lit += b
                if b != 255:
                    break
        out += src[i:i + lit]
        i += lit
        if i >= len(src):
            break
        off = src[i] | (src[i + 1] << 8)
        i += 2
        ml = tok & 15
        if ml == 15:
            while True:
                b = src[i]
                i += 1
                ml += b
                if b != 255:
                    break
        ml += 4
        st = len(out) - off
        for k in range(ml):
            out.append(out[st + k])
    if len(out) != size:
        raise ValueError("bad LZ4 segment")
    return bytes(out)


def segments(nso):
    """Return [(vaddr, bytes), ...] for text, rodata, data, LZ4 undone."""
    if nso[:4] != b"NSO0":
        raise ValueError("not an NSO")
    flags = struct.unpack_from("<I", nso, 0xC)[0]
    segs = []
    for n, (ho, co) in enumerate(((0x10, 0x60), (0x20, 0x64), (0x30, 0x68))):
        fo, mo, sz = struct.unpack_from("<III", nso, ho)
        csz = struct.unpack_from("<I", nso, co)[0]
        raw = nso[fo:fo + csz]
        segs.append((mo, lz4_block(raw, sz) if flags & (1 << n) else raw[:sz]))
    return segs


def flat_image(segs):
    """Flat vaddr-indexed image (file offset == vaddr), bss left as zero."""
    end = max(mo + len(b) for mo, b in segs)
    img = bytearray(end)
    for mo, b in segs:
        img[mo:mo + len(b)] = b
    return img


# ------------------------------------------------- find the pinning site
def find_contains_bl(text):
    """Address (text-relative) of the `bl Contains` whose result gates accept/reject.

    Shape: ldur x1,[x29,#s]; mov x2,xzr; bl Contains; tbz w0,#0,<reject>;
    ldur x0,[x29,#s]; cbz x0; mov x1,xzr; bl; cbz x0; mov x1,xzr.
    (Same shape the accept-any patch keyed on; the bl is the instruction before the tbz.)
    """
    w = struct.unpack("<%dI" % (len(text) // 4), text[: len(text) // 4 * 4])
    sites = []
    for k in range(len(w) - 10):
        a = w[k]
        if (a & 0xFFE00FFF) != 0xF84003A1:         # ldur x1, [x29, #s]
            continue
        if w[k + 1] != 0xAA1F03E2:                  # mov x2, xzr
            continue
        if (w[k + 2] & 0xFC000000) != 0x94000000:   # bl Contains
            continue
        if (w[k + 3] & 0xFFF8001F) != 0x36000000:   # tbz w0, #0, <reject>
            continue
        if w[k + 4] != (a & ~0x1F):                 # ldur x0, [x29, #s]
            continue
        if (w[k + 5] & 0xFF00001F) != 0xB4000000:   # cbz x0
            continue
        if w[k + 6] != 0xAA1F03E1 or (w[k + 7] & 0xFC000000) != 0x94000000:
            continue
        if (w[k + 8] & 0xFF00001F) != 0xB4000000 or w[k + 9] != 0xAA1F03E1:
            continue
        sites.append((k + 2) * 4)                   # the bl, not the tbz
    return sites


def find_tbz(text):
    """Address of the reject-branch tbz (the accept-any / test target)."""
    return [s + 4 for s in find_contains_bl(text)]


# ------------------------------------- resolve get_RawData from il2cpp metadata
def resolve_method(flat, meta, assembly, want):
    """Native vaddr of `want` (Type::Method) in `assembly`, via il2cpp v29/31 metadata."""
    def h(o):
        return struct.unpack_from("<II", meta, o)
    STR = h(0x18)[0]
    METH = h(0x30)
    TYPES = h(0xA0)
    IMAGES = h(0xA8)
    msz = 36 if METH[1] % 36 == 0 else 32

    def s(i):
        return meta[STR + i: meta.index(b"\0", STR + i)].decode("latin1")

    # locate the assembly's Il2CppCodeGenModule (moduleName ptr -> count, ptrs)
    i = flat.find(b"\x00" + assembly.encode() + b"\x00")
    if i < 0:
        raise ValueError("assembly name %r not in image" % assembly)
    name_va = i + 1
    j = flat.find(struct.pack("<Q", name_va))
    if j < 0:
        raise ValueError("code-gen module for %r not found" % assembly)
    _, count, ptrs = struct.unpack_from("<QQQ", flat, j)

    for k in range(IMAGES[1] // 40):
        v = struct.unpack_from("<10I", meta, IMAGES[0] + k * 40)
        if s(v[0]) != assembly:
            continue
        for t in range(v[2], v[2] + v[3]):
            o = TYPES[0] + t * 88
            tn, ns = struct.unpack_from("<II", meta, o)
            mstart = struct.unpack_from("<i", meta, o + 36)[0]
            mcount = struct.unpack_from("<H", meta, o + 64)[0]
            for m in range(mstart, mstart + mcount):
                mo = METH[0] + m * msz
                mn = struct.unpack_from("<I", meta, mo)[0]
                tok = struct.unpack_from("<I", meta, mo + (24 if msz == 36 else 20))[0]
                rid = tok & 0xFFFFFF
                if not (0 < rid <= count):
                    continue
                full = f'{s(ns) + "." if s(ns) else ""}{s(tn)}::{s(mn)}'
                if full == want:
                    return struct.unpack_from("<Q", flat, ptrs + (rid - 1) * 8)[0]
    raise ValueError("method %r not found in %s" % (want, assembly))


# ----------------------------------------------- free space (cave + rodata)
def text_code_cave(segs, need):
    """(vaddr, size) of the executable zero padding at the end of .text."""
    tvaddr, text = segs[0]
    end = tvaddr + len(text)
    cave = (end + 0xFFF) & ~0xFFF          # mapped up to the next page (RX, zero-filled)
    size = cave - end
    if size < need:
        raise ValueError("text cave too small: have %#x need %#x" % (size, need))
    return end, size


def refs_into(flat, lo, hi, text_end):
    """True if any .text adrp+add / adrp+ldr, or any stored 8-byte pointer, lands in [lo,hi)."""
    for base in range(0x8, len(flat) - 8, 8):
        v = struct.unpack_from("<Q", flat, base)[0]
        if lo <= v < hi:
            return True
    w = struct.unpack("<%dI" % (text_end // 4), bytes(flat[:text_end]))
    page = {}
    for k, x in enumerate(w):
        a = k * 4
        if (x & 0x9F000000) == 0x90000000:                      # ADRP
            rd = x & 0x1F
            immlo = (x >> 29) & 3
            immhi = (x >> 5) & 0x7FFFF
            imm = (immhi << 2) | immlo
            if imm & (1 << 20):
                imm -= (1 << 21)
            page[rd] = ((a & ~0xFFF) + (imm << 12), k)
        elif (x & 0xFF800000) == 0x91000000:                    # ADD imm64
            rn = (x >> 5) & 0x1F
            if rn in page and page[rn][1] >= k - 6:
                t = page[rn][0] + ((x >> 10) & 0xFFF)
                if lo <= t < hi:
                    return True
        elif (x & 0xFFC00000) == 0xF9400000:                    # LDR unsigned imm64
            rn = (x >> 5) & 0x1F
            if rn in page and page[rn][1] >= k - 6:
                t = page[rn][0] + (((x >> 10) & 0xFFF) << 3)
                if lo <= t < hi:
                    return True
    return False


def rodata_slot(segs, flat, need):
    """A free, unreferenced 16-aligned vaddr in .rodata with room for `need` bytes."""
    rvaddr, rod = segs[1]
    text_end = segs[0][0] + len(segs[0][1])
    runs = []
    i = 0
    n = len(rod)
    while i < n:
        if rod[i] == 0:
            j = i
            while j < n and rod[j] == 0:
                j += 1
            runs.append((j - i, rvaddr + i, rvaddr + j))
            i = j
        else:
            i += 1
    runs.sort(reverse=True)
    for _, a, b in runs:
        slot = (a + 15) & ~15
        if slot + need <= b and not refs_into(flat, slot, slot + need, text_end):
            return slot
    raise ValueError("no free unreferenced .rodata gap for %#x bytes" % need)


# ------------------------------------------------------------- cave assembly
def assemble_cave(cave_va, get_rawdata, der_va, der_len):
    """Bytes of the pinning routine. x1 = presented X509Certificate2; returns w0 in {0,1}."""
    try:
        import keystone
    except ImportError:
        sys.exit("need the 'keystone-engine' python package to build the release patch")
    ks = keystone.Ks(keystone.KS_ARCH_ARM64, keystone.KS_MODE_LITTLE_ENDIAN)
    n = 22
    loop_a = cave_va + 11 * 4
    rej_a = cave_va + 19 * 4
    done_a = cave_va + 20 * 4
    prog = [
        "stp x29, x30, [sp, #-0x10]!",      # 0  save lr (and keep sp 16-aligned)
        "mov x0, x1",                        # 1  this = presented certificate
        f"bl #{get_rawdata}",                # 2  x0 = presented.RawData (byte[] DER)
        f"cbz x0, #{rej_a}",                 # 3  null -> reject
        "ldr w1, [x0, #0x18]",               # 4  byte[].Length
        f"cmp w1, #{der_len}",               # 5  must equal OpenPak DER length
        f"b.ne #{rej_a}",                    # 6
        "add x0, x0, #0x20",                 # 7  -> byte[] data
        f"adrp x2, #{der_va & ~0xfff}",      # 8  -> embedded OpenPak DER
        f"add x2, x2, #{der_va & 0xfff}",    # 9
        f"mov w3, #{der_len}",               # 10 byte counter
        "ldrb w4, [x0], #1",                 # 11 loop: presented byte
        "ldrb w5, [x2], #1",                 # 12       OpenPak byte
        "cmp w4, w5",                        # 13
        f"b.ne #{rej_a}",                    # 14      mismatch -> reject
        "subs w3, w3, #1",                   # 15
        f"b.ne #{loop_a}",                   # 16
        "mov w0, #1",                        # 17 all bytes equal -> accept
        f"b #{done_a}",                      # 18
        "mov w0, #0",                        # 19 reject:
        "ldp x29, x30, [sp], #0x10",         # 20 done:
        "ret",                               # 21
    ]
    assert len(prog) == n
    blob = b""
    for k, line in enumerate(prog):
        code, cnt = ks.asm(line, cave_va + k * 4)
        if cnt != 1:
            raise ValueError("assembly failed: %r" % line)
        blob += bytes(code)
    return blob


# ------------------------------------------------------------------- PEM -> DER
def load_der():
    pem = open(os.path.join(os.path.dirname(os.path.abspath(__file__)), CERT_PEM)).read()
    import base64
    m = re.search(r"-----BEGIN CERTIFICATE-----(.+?)-----END CERTIFICATE-----", pem, re.S)
    if not m:
        sys.exit("%s: no PEM certificate" % CERT_PEM)
    return base64.b64decode("".join(m.group(1).split()))


# --------------------------------------------------------------------- IPS32
def write_ips(path, records):
    """records: list of (vaddr, bytes). Offsets get +HDR."""
    out = bytearray(b"IPS32")
    for va, data in records:
        if not 0 < len(data) <= 0xFFFF:
            raise ValueError("bad record length")
        out += struct.pack(">IH", va + HDR, len(data)) + data
    out += b"EEOF"
    with open(path, "wb") as f:
        f.write(out)


def main():
    args = sys.argv[1:]
    test = False
    if args and args[0] == "--test":
        test = True
        args = args[1:]
    if not args or (not test and len(args) < 2):
        sys.exit(__doc__)

    nso = open(args[0], "rb").read()
    build_id = nso[0x40:0x54].hex().upper()
    segs = segments(nso)
    tvaddr, text = segs[0]
    out_dir = None

    if test:
        out_dir = args[1] if len(args) > 1 else "."
        tbz = find_tbz(text)
        if len(tbz) != 1:
            sys.exit(f"{build_id}: {len(tbz)} reject sites, refusing (need exactly 1)")
        va = tvaddr + tbz[0]
        write_ips(os.path.join(out_dir, build_id + ".test-accept-any.ips"),
                  [(va, struct.pack("<I", NOP))])
        print(f"{build_id}: TEST accept-any NOP at text+{va:#x} (do NOT ship)")
        return

    meta = open(args[1], "rb").read()
    out_dir = args[2] if len(args) > 2 else "."
    der = load_der()

    bls = find_contains_bl(text)
    if len(bls) != 1:
        sys.exit(f"{build_id}: {len(bls)} Contains sites, refusing (need exactly 1)")
    bl_va = tvaddr + bls[0]

    flat = flat_image(segs)
    get_rawdata = resolve_method(
        flat, meta, "System.dll",
        "System.Security.Cryptography.X509Certificates.X509Certificate2::get_RawData")

    der_va = rodata_slot(segs, flat, len(der))
    # size the cave: assemble once against a provisional cave base to learn its length
    cave_va, _ = text_code_cave(segs, 0x100)
    cave = assemble_cave(cave_va, get_rawdata, der_va, len(der))
    cave_va, _ = text_code_cave(segs, len(cave))
    cave = assemble_cave(cave_va, get_rawdata, der_va, len(der))

    # redirect the Contains call to the cave: bl (cave_va - bl_va)
    import keystone
    ks = keystone.Ks(keystone.KS_ARCH_ARM64, keystone.KS_MODE_LITTLE_ENDIAN)
    bl_bytes, _ = ks.asm(f"bl #{cave_va}", bl_va)

    path = os.path.join(out_dir, build_id + ".ips")
    write_ips(path, [
        (der_va, der),                          # OpenPak DER into free .rodata
        (cave_va, cave),                         # pinning routine into the .text cave
        (bl_va, bytes(bl_bytes)),                # Contains -> cave
    ])
    print(f"{path}")
    print(f"  pin to OpenPak DER ({len(der)} bytes, sha-256 of cert in {CERT_PEM})")
    print(f"  cave  text+{cave_va:#x} ({len(cave)} bytes)")
    print(f"  der   rodata+{der_va:#x}")
    print(f"  bl    text+{bl_va:#x} -> cave  (was Contains {struct.unpack('<I', text[bls[0]:bls[0]+4])[0]:#010x})")


if __name__ == "__main__":
    main()
