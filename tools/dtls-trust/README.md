# Among Us DTLS trust patch

> **Test-only build. Do not ship it.** The patch in this directory accepts *any* server
> certificate. The owner requires the released patch to accept only OpenPak's fixed DTLS
> certificate (2026-10-24). No patcher or Harbor built-in may carry this version. Release plan:
> replace the NOP with a check against OpenPak's certificate (an embedded key hash compared in the
> same branch, or a swap of the pinned-set entry), then regenerate per build.

## The Among Us exception to "no game patches"

OpenPak does not patch games. It changes where the console connects and puts its CA in the
console's certificate store. This patch is a documented exception, approved by the owner on
2026-10-24, in the same class as the D2R trust patch: it changes only how the title trusts a
server, never what the game does.

Why no console-side fix reaches it: after the matchmaker names a game server, every online path
of the console client opens a Hazel DTLS 1.2 session to it (Impostor's port + 3, or an auth
handshake on port + 2). The client accepts only the server certificates its build pins
(`DtlsUnityConnection.serverCertificates.Contains(certificate)`), and the pinned certificate is
self-signed, so no server without its private key can complete the handshake. Neither nn::ssl
nor the console certificate store is consulted.

What the patch changes: one instruction. The branch that rejects a certificate the pinned set
does not contain becomes a NOP. Everything else stays as shipped. The handshake still verifies
the ServerKeyExchange signature against the certificate the server sent, and the session is
still encrypted (ECDHE X25519, AES-128-GCM). The pinned set is no longer consulted, so this
test build accepts any server certificate; the release build must accept only OpenPak's (above).
The game server side is `dtls-front/`. It serves OpenPak's fixed DTLS certificate, kept on the
box (`AMONG_US_DTLS_CERT` / `AMONG_US_DTLS_KEY`); the private key never leaves the box.

| Build | What | Site |
|---|---|---|
| `19A77A3165441065F8B2B43A54A4F2AB00000000` | update v3997696, 2026.18.0 (game version 2026.7.15, build 7239), the build consoles run | text + 0x35d7e4c |
| `5D3446A443A5054EA4886722DA8DF13D00000000` | base v0 NSP | no DTLS client of this shape; not patched (the base build is too old to play online anyway) |

`make-ips.py <main.nso> [out-dir]` writes the IPS32 exefs patch for a build. It finds the site
by instruction shape (Contains -> tbz -> reload -> cbz -> PublicKey) and refuses unless exactly
one site matches, so a new update needs only a re-run. The file carries one 4-byte NOP and no
game bytes. Offsets include the NSO header (+0x100).

## Install

The file name is the build id. The loader picks the file that matches.

| Where | Path |
|---|---|
| Switch (Atmosphere) | `sd:/atmosphere/exefs_patches/openpak_among_us_dtls/<build id>.ips` |
| Harbor (Eden-based) | `<user>/load/0100B0C013912000/openpak-among-us-dtls/exefs/<build id>.ips` |

Delivery to players will go the same way as D2R's (the OpenPak Switch Patcher and Harbor's
built-in patches), and only with the pinned release build. Neither carries anything yet.

Other consoles: the PlayStation and Xbox builds embed the same Hazel DTLS client and would need
the equivalent change in their own executable format. OpenPak has no PS4/PS5 or Xbox One/Series
path today (see the README's platform matrix).
