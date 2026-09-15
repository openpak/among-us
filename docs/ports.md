# Ports — among-us (trimmed)

> Trimmed copy for this repository; the canonical document lives at
> `Openpak/ports.md` and governs. Last synchronised 2026-09-15.

One block per concern, nothing below 20000, nothing at or above 27000 (Photon-Reference and
Steam live there). Every service reads its listeners from `<SVC>_HTTP_ADDR`, `<SVC>_GRPC_ADDR`,
`<SVC>_METRICS_ADDR`; the values below are the defaults and the local-run convention. Each
service owns a block of ten: +0 HTTP, +1 gRPC, +2 metrics/pprof, +3..+9 spare.

## Third-party-network titles

| Port(s) | Service |
| --- | --- |
| 22600–22699 | third-party-network titles (Among Us, Fall Guys, PvZ, Minecraft Dungeons, CTR), one block of ten each |
| 22600–22609 | Among Us (deployed 2026-09-11): 22600 front (HTTP, behind Traefik), 22601 Impostor HTTP matchmaker (internal network), 22603/udp Impostor game traffic (published) |

## Health

| Service | Endpoint |
| --- | --- |
| `among-us` (Impostor front) | `/healthz` |

Other services' rows live in the canonical ports.md.
