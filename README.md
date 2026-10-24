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

## Platforms

Among Us ships on Switch, PlayStation 4/5, Xbox One/Series, PC (Steam, Epic, Microsoft Store,
itch.io) and mobile, and its rooms are cross-play. This service is platform-neutral: one Impostor,
one set of room codes, one public game list. A console differs only in how its player proves who
they are and in the platform record it sends inside the game protocol, and that identity is
handled in exactly one place: the EOS Connect login in
[`platforms/switch/games/eos`](../eos) (`POST /auth/v1/oauth/token`, `external_auth`). Each platform's
credential type becomes one more case there, and every later service (`backend.innersloth.com`,
this matchmaker, Impostor) only ever sees the EOS product user id (PUID) that the login mints.

What every platform's client does, in order: EOS SDK config and client token at
`api.epicgames.dev`, then EOS Connect login with the platform's own credential, then Innersloth's
account backend at `backend.innersloth.com` (friend code, friends, blocks, lobby invites), then
this matchmaker (`POST /api/user` with `{Puid, Username, ClientVersion, Language}`, then
`/api/games`), then Hazel over UDP to the address the matchmaker names. In the Hazel hello the
client sends its platform record: a tag (Impostor's `Platforms`), a display name, and for Xbox a
64-bit XUID and for PlayStation a 64-bit PSN account id.

| Platform | Platform tag | Connect credential (`external_auth_type`) | Redirect on OpenPak | Status |
| --- | --- | --- | --- | --- |
| Switch | `Switch` (8) | `nintendo_nsa_id_token`, measured; OpenPak mints the NSA id_token (nx-baas) and eosd verifies it | DNS + CA via the signed ceiling: `.among.us`, `.epicgames.dev`, `.innersloth.com` (all in it) | Harbor 2026-10-24: sign-in, account backend and matchmaker work; the game connection stops at DTLS (below) |
| PlayStation 4/5 | `Playstation` (10), PSN account id | `psn_id_token` (EOS SDK naming; not measured) | none: OpenPak has no PS4/PS5 network (PS3 only) | Blocked on a PS4/PS5 redirect; needs an OpenPak PSN identity that mints the id_token, then one eosd case |
| Xbox One/Series | `Xbox` (9), XUID | `xbl_xsts_token` (EOS SDK naming; not measured) | none: OpenPak's Xbox work covers the original Xbox and the 360 | Blocked on an Xbox One/Series redirect; needs an XSTS issuer, then one eosd case |
| PC (Steam, Epic, Microsoft Store, itch.io) | `StandaloneSteamPC` (2), `StandaloneEpicPC` (1), `StandaloneWin10` (4), `StandaloneItch` (5); `StandaloneMac` (3) | Steam ticket, Epic id token, Xbox (Microsoft Store) or device id | not redirected. A PC client given an Impostor region file still signs in to Innersloth's real EOS and backend, and could reach this matchmaker only if it were published under an OpenPak hostname (it is not today) | Out of scope to redirect; same protocol, so useful as a test client |
| Mobile | `IPhone` (6), `Android` (7) | Apple / Google id token or device id | not redirected (same as PC) | Out of scope to redirect |

**Blocker (2026-10-24): the game server connection is DTLS with a pinned certificate.** After the
matchmaker names the server, every online path of the console client opens a Hazel DTLS 1.2
session (game port + 3, or an auth handshake on port + 2), and it accepts only one compiled-in,
self-signed server certificate. No server without that certificate's private key can complete
the handshake, and DNS, the console CA store or the signed ceiling cannot change it. Going further
needs a console-side change to the client's certificate check plus a DTLS front for Impostor;
that is an owner decision under the no-game-patches rule. The same client code ships on the
other consoles.

Rules that hold for every platform:

- Cross-play is Impostor's: every platform shares one `CompatibilityManager` version group and the
  in-game cross-play filter. Nothing here filters by platform.
- The PUID is derived per account, not per console (eosd: HMAC of the deployment and the account),
  so a player keeps one friend code everywhere. When a second platform arrives, it maps to the
  same OpenPak account and therefore to the same PUID, which is what makes cross-platform
  friends work.
- Purchases have one seam too, in eosd's `user/validate-receipt[-batch]`: a receipt that is a
  console's own item list (Switch on-disk DLC, `entitlement_type` `dlc` or `innersloth-receipt`)
  is accepted as sent, because the console checked ownership itself; a receipt that only the
  real store can verify (Apple/Google, Microsoft Store XSTS, Epic Games Store) is refused with 403.
  A new console platform whose receipts are console-checked lists needs no change there.
- Game version: the Switch 2026.18.0 build reports `2026.7.15` revision 0 (build 7239), which
  Impostor `openpak-v1.10.8` already lists in its 18.0 compatibility group with PC and mobile,
  so Switch rooms and PC/mobile rooms of the same release are one pool.
- The matchmaker can name the account behind a request: the `Puid` in `/api/user` is eosd's. A
  lookup from PUID to account (for country refinement and test nodes, below) belongs in eosd,
  next to the derivation, not in this front.

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
claimed in [`ports.md`](https://docs.openpak.org/contributors/ports/).

## Ground rules

- Clean room. Read [clean-room policy](https://docs.openpak.org/contributors/clean-room/) before writing
  a line. Public protocol documentation, our own captures, and compatibly licensed code are the
  only inputs.
- AGPL-3.0-only, English only, Go 1.25, no secrets in the repository.

This project is not affiliated with or endorsed by Nintendo, Among Us's publisher, or any
original service provider. It contains no proprietary game code, assets, SDK material,
binaries, certificates, keys, firmware, extracted files, credentials, or raw packet captures.
