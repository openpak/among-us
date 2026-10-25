# Among Us DTLS trust patch

> **Release build: pins the console client to OpenPak's own fixed DTLS certificate, and only
> that certificate.** This is strictly narrower than trusting Innersloth's certificate, not
> wider: the rejection branch stays and the comparison stays an exact DER match. It is the
> artifact to ship (approved by the owner 2026-10-24, reconfirmed 2026-10-25).
>
> A separate **test-only** patch (`--test`, file `*.test-accept-any.ips`) accepts *any* server
> certificate. It exists for local bring-up only and must never reach a patcher or Harbor
> built-in.

## The Among Us exception to "no game patches"

OpenPak does not patch games. It changes where the console connects and puts its CA in the
console's certificate store. This patch is a documented exception, approved by the owner on
2026-10-24, in the same class as the D2R trust patch: it changes only how the title trusts a
server, never what the game does.

Why no console-side fix reaches it: after the matchmaker names a game server, every online path
of the console client opens a Hazel DTLS 1.2 session to it (Impostor's port + 3, or an auth
handshake on port + 2). The client accepts only the server certificates its build pins
(`DtlsUnityConnection.serverCertificates.Contains(presented)`), and the pinned certificate is
self-signed, so no server without its private key can complete the handshake. Neither nn::ssl
nor the console certificate store is consulted. The game server side is `dtls-front/`; it serves
OpenPak's fixed DTLS certificate, kept on the box (`AMONG_US_DTLS_CERT` / `AMONG_US_DTLS_KEY`);
the private key never leaves the box.

## What the release patch does

It re-points the pin at OpenPak's certificate. Trust is not weakened: the handshake still
verifies the ServerKeyExchange signature against the certificate the server sent, the session is
still encrypted (ECDHE X25519, AES-128-GCM), and a certificate that is not OpenPak's is still
rejected.

Three IPS records, no game bytes changed in place beyond one redirected call:

1. **OpenPak's DER** (`openpak-dtls-cert.pem`, 843 bytes, SHA-256 `F6:9C:E6:…:CC:84`) is written
   into a free, unreferenced zero gap in `.rodata` (read-only data).
2. A **small routine** is placed in the executable zero padding at the end of `.text` (a code
   cave). It receives the presented `X509Certificate2`, fetches its raw DER
   (`X509Certificate2.get_RawData`), and returns the same bool the original `Contains` returned:
   `1` (present → accept) only if the presented DER's length is 843 and every byte equals
   OpenPak's embedded DER, otherwise `0` (reject).
3. The one `serverCertificates.Contains(presented)` **call is redirected** to that routine. The
   reject branch (`tbz w0, #0, <reject>`) and everything downstream are untouched.

No managed allocation is added. The original PEM-decode / pinned-set construction in
`AuthManager::CreateDtlsConnection` is left as shipped and simply no longer decides the outcome.

Why an exact-DER compare rather than a hash: it needs no crypto call and is exactly the equality
the stock client already used for pinning (`X509Certificate2.Equals`, i.e. raw-data equality) —
re-aimed from Innersloth's certificate to OpenPak's. The original certificate is rejected on the
length gate alone (its DER is 881 bytes, not 843).

| Build | What | Pinning site (text) |
|---|---|---|
| `19A77A3165441065F8B2B43A54A4F2AB00000000` | update v3997696, 2026.18.0 (game version 2026.7.15, build 7239), the build consoles run | `Contains` bl at 0x35d7e48 → cave 0x48e7e38; DER at rodata 0x4a8c9b0 |
| `5D3446A443A5054EA4886722DA8DF13D00000000` | base v0 NSP | no DTLS client of this shape; not patched (the base build is too old to play online anyway) |

## Regenerating for a new update

```
make-ips.py <main.nso> <global-metadata.dat> [out-dir]      # RELEASE (pin to OpenPak), default
make-ips.py --test <main.nso> [out-dir]                      # TEST accept-any NOP (do NOT ship)
```

Nothing is hard-coded to an address. The release run finds the `Contains` call by instruction
shape, resolves `X509Certificate2::get_RawData` from `global-metadata.dat` by name, finds an
unreferenced `.rodata` gap for the DER, and uses the `.text` tail padding for the cave; it
refuses unless exactly one pinning site matches. The certificate comes from
`openpak-dtls-cert.pem` next to the script (the PUBLIC certificate only — the private key stays
on the box). Offsets in the IPS include the NSO header (+0x100), the convention Atmosphere and
the emulators share. The release run needs the `keystone-engine` Python package.

## Install

The file name is the build id. The loader picks the file that matches. Ship the `<build id>.ips`
(release); never ship `<build id>.test-accept-any.ips`.

| Where | Path |
|---|---|
| Switch (Atmosphere) | `sd:/atmosphere/exefs_patches/openpak_among_us_dtls/<build id>.ips` |
| Harbor (Eden-based) | `<user>/load/0100B0C013912000/openpak-among-us-dtls/exefs/<build id>.ips` |

Delivery to players goes the same way as D2R's (the OpenPak Switch Patcher and Harbor's built-in
patches), with the pinned release build only.

Other consoles: the PlayStation and Xbox builds embed the same Hazel DTLS client and would need
the equivalent change in their own executable format. OpenPak has no PS4/PS5 or Xbox One/Series
path today (see the repo README's platform matrix).
