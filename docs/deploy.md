# Deploy: among-us

## What runs

| Piece | What it is |
| --- | --- |
| Impostor | Built into this image at build time from OpenPak's unmodified fork (`openpak/impostor`, `IMPOSTOR_REF` build arg, default `openpak-v1.10.8`), run as its own process next to the front. Not vendored here — GPL-3.0 stays at arm's length from AGPL-3.0-only OpenPak; sharing a container is aggregation. |
| `cmd/among-us` | This repository. `/healthz`, a reverse proxy to Impostor, and a log of the paths the console asks for. |
| Traefik + DNS | Existing OpenPak plumbing: the console resolves the matchmaker name to us and trusts our CA. |

```sh
go run ./cmd/among-us   # :22600, proxying to http://127.0.0.1:22601
```

`AMONG_US_HTTP_ADDR` and `AMONG_US_IMPOSTOR_URL` override those defaults.

## Region nodes: running a node

A node runs this same image with `IMPOSTOR_Server__PublicIp` set to its public address, no
`OPENPAK_ROUTING_URL`, and UDP 22603 open. **Left to do:** the node-side room reporter (an
Impostor plugin, or a poller in the node's front, posting on game create/close); a node's
public lobbies are not merged into home's `/api/games/filtered`; node and home Impostors pick
codes independently, so a collision is possible until codes are allocated at home. Unverified:
whether the Switch client accepts a returned `Ip`/`Port` that is not the matchmaker's host
(`docs/design.md` §5 item 6).

With `OPENPAK_ROUTING_URL` unset, or with no node in the table, the front behaves exactly as
before (`TestHomeOnlyIsUnchanged`). Ports 22600–22609 are
claimed in [`ports.md`](https://docs.openpak.org/contributors/ports/).

## Image and ports (from the code)

- Image `ghcr.io/openpak/among-us`. A release tag `v1.2.3` publishes `:1.2.3`, `:1.2` and `:latest` (`.github/workflows/release.yml`).
- One container, three processes, started by `entrypoint.sh`: Impostor (`/app`), the front (`/usr/local/bin/among-us`) and the DTLS front (`/opt/dtls-front`). If any of them exits, the container exits and the runtime restarts all three.
- Ports (from the `Dockerfile`): `22600/tcp` is the front, behind Traefik; `22603/udp` is plain game traffic; `22606/udp` is DTLS game traffic from consoles (Impostor's port + 3). Publish both UDP ports and set `IMPOSTOR_Server__PublicIp`.
- Environment defaults baked into the image: `IMPOSTOR_Server__ListenPort=22603`, `IMPOSTOR_Server__PublicPort=22603`, `IMPOSTOR_HttpServer__ListenIp=127.0.0.1`, `IMPOSTOR_HttpServer__ListenPort=22601`, `AMONG_US_HTTP_ADDR=:22600`, `AMONG_US_IMPOSTOR_URL=http://127.0.0.1:22601`.
