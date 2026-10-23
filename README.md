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

**Status: deployed 2026-09-11, unproven on a console.** one container (`ghcr.io/openpak/among-us`: Impostor built from our fork plus this front) runs on production; Traefik terminates `matchmaker.among.us`, `matchmaker-eu.among.us` and `matchmaker-as.among.us` with an OpenPak-CA leaf and `openpak.nro` 0.2.2 redirects them. Game traffic: UDP 22603. Nothing has been tested against the game yet. The design rests on assumptions
about the Switch build that only a capture can settle;
[`docs/evidence-needed.md`](docs/evidence-needed.md) lists them in the order that matters, and
two of them can kill the approach outright.

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

## Region nodes (federation)

Every region hostname (`matchmaker[-eu|-as].among.us`, `na|eu|as.mm.among.us`) reaches this one
home front, so room codes stay global. With `OPENPAK_ROUTING_URL` (account core, e.g.
`http://openpak-account:8000`) and `OPENPAK_ROUTING_KEY` set, the front reads the routing table
(`internal/routing`, copied from the account core) and changes two replies; everything else
still proxies to home's Impostor:

- `PUT /api/games` (host): the hostname is the host's region choice (`na`/`eu`/`as`), resolved
  with service `impostor`; a node's reply names that node's Impostor (`impostor` role, IPv4
  `ip:port`). Home, an unknown hostname or an unusable address proxies as before. The front
  cannot name a Switch account (Impostor's token carries the game's product user id), so it
  resolves by token alone: no country refinement, no test nodes.
- `GET /api/games/{code}` and the legacy `POST /api/games?gameId=` (join): a code a node reported
  answers with that node's current address, whatever hostname the joiner used.

The code does not exist when the host asks where to host (the node's Impostor makes it over
UDP), so a node reports its rooms to `POST /internal/v1/amongus/rooms` with header `X-Room-Key:
$AMONG_US_ROOM_REPORT_KEY` and body `{"node":"<id>","code":<int>,"game":{<its own
/api/games/{code} Game>},"closed":false}`. Reports live in memory for 6 h (re-report to keep a
room). The endpoint exists only when `AMONG_US_ROOM_REPORT_KEY` is set.

A node runs this same image with `IMPOSTOR_Server__PublicIp` set to its public address, no
`OPENPAK_ROUTING_URL`, and UDP 22603 open. **Left to do:** the node-side room reporter (an
Impostor plugin, or a poller in the node's front, posting on game create/close); a node's
public lobbies are not merged into home's `/api/games/filtered`; node and home Impostors pick
codes independently, so a collision is possible until codes are allocated at home. Unverified:
whether the Switch client accepts a returned `Ip`/`Port` that is not the matchmaker's host
(`docs/design.md` §5 item 6).

With `OPENPAK_ROUTING_URL` unset, or with no node in the table, the front behaves exactly as
before (`TestHomeOnlyIsUnchanged`). Ports 22600–22609 are
claimed in [`ports.md`](https://github.com/openpak/docs/blob/main/ports.md).

## Ground rules

- Clean room. Read [`docs/clean-room-policy.md`](https://github.com/openpak/docs/blob/main/clean-room-policy.md) before writing
  a line. Public protocol documentation, our own captures, and compatibly licensed code are the
  only inputs.
- AGPL-3.0-only, English only, Go 1.25, no secrets in the repository.

This project is not affiliated with or endorsed by Nintendo, Among Us's publisher, or any
original service provider. It contains no proprietary game code, assets, SDK material,
binaries, certificates, keys, firmware, extracted files, credentials, or raw packet captures.
