# among-us

Getting a Nintendo Switch onto a self-hosted Among Us server.

**This repository is not a game server, and should not become one.** Among Us does not use any
Nintendo online service — it talks to Innersloth's own servers with its own protocol — and
[Impostor](https://github.com/Impostor/Impostor) (GPL-3.0, C#) already reimplements that server
completely and keeps up with the game's releases. What Impostor does not do is get a console
onto it: its FAQ says consoles "don't allow adding custom servers", which is true of the client's
region-select screen and not true of a Switch, where the hostname can be redirected the same way
every other OpenPak title is.

So the whole of OpenPak's contribution here is a redirect, a TLS front, and about a hundred lines
of Go. See [`docs/design.md`](docs/design.md) for the reasoning and the sources.

**Status: unproven.** Nothing has been tested against the game. The design rests on assumptions
about the Switch build that only a capture can settle;
[`docs/evidence-needed.md`](docs/evidence-needed.md) lists them in the order that matters, and
two of them can kill the approach outright.

## What runs

| Piece | What it is |
| --- | --- |
| Impostor | Upstream container image, unmodified, its own process. Not vendored here — GPL-3.0 stays at arm's length from AGPL-3.0-only OpenPak. |
| `cmd/among-us` | This repository. `/healthz`, a reverse proxy to Impostor, and a log of the paths the console asks for. |
| Traefik + DNS | Existing OpenPak plumbing: the console resolves the matchmaker name to us and trusts our CA. |

```sh
go run ./cmd/among-us   # :22600, proxying to http://127.0.0.1:22601
```

`AMONG_US_HTTP_ADDR` and `AMONG_US_IMPOSTOR_URL` override those defaults. Ports 22600–22609 are
claimed in [`ports.md`](../../ports.md).

## Ground rules

- Clean room. Read [`docs/clean-room-policy.md`](../../docs/clean-room-policy.md) before writing
  a line. Public protocol documentation, our own captures, and compatibly licensed code are the
  only inputs.
- AGPL-3.0-only, English only, Go 1.25, no secrets in the repository.

This project is not affiliated with or endorsed by Nintendo, Among Us's publisher, or any
original service provider. It contains no proprietary game code, assets, SDK material,
binaries, certificates, keys, firmware, extracted files, credentials, or raw packet captures.
