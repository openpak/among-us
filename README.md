# Among Us

OpenPak's Among Us Server.

| | |
| --- | --- |
| Game | [Among Us](https://openpak.org/titles#t-19) |
| Network | Nintendo Switch |
| Title ID | `0100B0C013912000` |
| Image | `ghcr.io/openpak/among-us` |
| Licence | AGPL-3.0-only |

## What it serves

| Host or port | What | Served by |
| --- | --- | --- |
| `matchmaker[-eu\|-as].among.us`, `na\|eu\|as.mm.among.us` | Matchmaker (`/api/user`, `/api/games`) and region routing | `cmd/among-us` front, proxying to Impostor |
| UDP 22603 | Plain game traffic | Impostor |
| UDP 22606 | DTLS game traffic from consoles | `dtls-front`, relayed to Impostor |

## Run

```sh
go run ./cmd/among-us   # :22600, proxying to http://127.0.0.1:22601
```

| Variable | Default | Meaning |
| --- | --- | --- |
| `AMONG_US_HTTP_ADDR` | `:22600` | Front listener |
| `AMONG_US_IMPOSTOR_URL` | `http://127.0.0.1:22601` | Impostor the front proxies to |

## Docs

- [Protocol](docs/protocol.md)
- [Evidence](docs/evidence.md)
- [Deploy](docs/deploy.md)
- [Notes](docs/notes.md)
- [Design](docs/design.md)
- [Evidence needed](docs/evidence-needed.md)
- [Provenance](docs/provenance.md)

Live compatibility status: [openpak.org/titles#t-19](https://openpak.org/titles#t-19).

## Licence

AGPL-3.0-only. See [LICENSE](LICENSE).
