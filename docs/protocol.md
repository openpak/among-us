# Protocol: among-us

## Platforms

Among Us ships on Switch, PlayStation 4/5, Xbox One/Series, PC (Steam, Epic, Microsoft Store,
itch.io) and mobile, and its rooms are cross-play. This service is platform-neutral: one Impostor,
one set of room codes, one public game list. A console differs only in how its player proves who
they are and in the platform record it sends inside the game protocol, and that identity is
handled in exactly one place: the EOS Connect login in
[`platforms/switch/games/eos`](../../eos) (`POST /auth/v1/oauth/token`, `external_auth`). Each platform's
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
