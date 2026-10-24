#!/usr/bin/env python3
"""make-ips.py <main.nso> [out-dir]: write the Among Us DTLS trust patch for that build.

The console client opens every online game connection with Hazel DTLS and accepts only the
server certificates its build pins (Hazel DtlsUnityConnection, `serverCertificates.Contains`).
This patch removes that one branch: the `tbz w0, #0, <reject>` after the Contains call becomes a
NOP. Everything else stays as shipped: the handshake still verifies the ServerKeyExchange
signature against the certificate the server sent, and the session is still encrypted.

The site is found by its instruction shape, not by address, so the script works for any build
that has the same code: Contains(cert) -> tbz -> reload cert -> cbz -> PublicKey -> ...
It refuses unless exactly one site matches. The IPS32 file is named by the NSO build id and
carries one 4-byte NOP, no game bytes. Offsets include the NSO header (+0x100), the convention
Atmosphere and the emulators share.
"""
import os
import struct
import sys

NOP = 0xD503201F


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


def text_segment(nso):
    if nso[:4] != b"NSO0":
        raise ValueError("not an NSO")
    flags = struct.unpack_from("<I", nso, 0xC)[0]
    fo, mo, sz = struct.unpack_from("<III", nso, 0x10)
    csz = struct.unpack_from("<I", nso, 0x60)[0]
    raw = nso[fo:fo + csz]
    return mo, (lz4_block(raw, sz) if flags & 1 else raw[:sz])


def find_sites(text):
    w = struct.unpack("<%dI" % (len(text) // 4), text[: len(text) // 4 * 4])
    sites = []
    for k in range(len(w) - 10):
        a = w[k]
        if (a & 0xFFE00FFF) != 0xF84003A1:  # ldur x1, [x29, #s]
            continue
        if w[k + 1] != 0xAA1F03E2:  # mov x2, xzr
            continue
        if (w[k + 2] & 0xFC000000) != 0x94000000:  # bl Contains
            continue
        if (w[k + 3] & 0xFFF8001F) != 0x36000000:  # tbz w0, #0, <reject>
            continue
        if w[k + 4] != (a & ~0x1F):  # ldur x0, [x29, #s]
            continue
        if (w[k + 5] & 0xFF00001F) != 0xB4000000:  # cbz x0
            continue
        if w[k + 6] != 0xAA1F03E1 or (w[k + 7] & 0xFC000000) != 0x94000000:  # mov x1, xzr; bl
            continue
        if (w[k + 8] & 0xFF00001F) != 0xB4000000 or w[k + 9] != 0xAA1F03E1:  # cbz x0; mov x1, xzr
            continue
        sites.append((k + 3) * 4)
    return sites


def main():
    if len(sys.argv) < 2:
        sys.exit(__doc__)
    nso = open(sys.argv[1], "rb").read()
    out_dir = sys.argv[2] if len(sys.argv) > 2 else "."
    build_id = nso[0x40:0x54].hex().upper()
    base, text = text_segment(nso)
    sites = find_sites(text)
    if len(sites) != 1:
        sys.exit(f"{build_id}: {len(sites)} candidate sites, refusing (need exactly 1)")
    va = base + sites[0]
    ips = b"IPS32" + struct.pack(">IH", va + 0x100, 4) + struct.pack("<I", NOP) + b"EEOF"
    path = os.path.join(out_dir, build_id + ".ips")
    with open(path, "wb") as f:
        f.write(ips)
    print(f"{path}: NOP at text+{va:#x}")


if __name__ == "__main__":
    main()
