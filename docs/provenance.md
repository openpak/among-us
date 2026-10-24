# Provenance

| Piece | Source | Licence | How it is used |
|---|---|---|---|
| `dtls-front/` | written for OpenPak | AGPL-3.0-only | the DTLS front for Impostor |
| Hazel Networking | github.com/willardf/Hazel-Networking, commit `HAZEL_REF` in the Dockerfile | MIT (Innersloth LLC, DarkRift Networking) | compiled into `dtls-front` at image build time from a pinned checkout; not vendored. Its server-side `DtlsConnectionListener` terminates the consoles' DTLS sessions. |
| Impostor | OpenPak's unmodified fork, `IMPOSTOR_REF` | GPL-3.0 | built into the image as its own process |
| `tools/dtls-trust/` | written for OpenPak | AGPL-3.0-only | generates the console patch |

Protocol facts (the DTLS-on-port+3 rule, the pinned-certificate check, the instruction shape the
patch targets) come from OpenPak's own analysis of the game client the owner dumped from his
console: the IL2CPP metadata and main executable, read for facts only. No game code, assets or
certificates are in this repository.
