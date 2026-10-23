# Design: Among Us on Nintendo Switch

> **Current Switch client (2026-10-13): Harbor**, at
> `~/REPOS/Openpak/emulators/harbor`. Harbor is an independent
> OpenPak project based on Eden. Citron, Ryujinx (desktop and Android), and
> the former OpenPak Eden checkouts have been removed. References to those
> forks below record historical evidence only: do not run their commands,
> reinstall them, or select them for current work. Use Harbor's README and
> current handoff for builds and testing; old results do not establish Harbor parity.

Written 2026-09-09 from public sources only. No source from a non-free implementation was opened; see
[`clean-room-policy.md`](https://github.com/openpak/docs/blob/main/clean-room-policy.md).

**Recommendation: depend. Do not build a game server.** Run
[Impostor](https://github.com/Impostor/Impostor) (GPL-3.0, C#/.NET 8) as a separate process and
have OpenPak supply the two things Impostor deliberately does not: the hostname redirect that
gets a console onto it, and the TLS front that terminates Innersloth's names with our CA. That
is roughly 100 lines of Go plus deployment config.

## 1. What network the game uses

Among Us does not use NEX, NPLN, or any Nintendo online service for gameplay. It talks to
Innersloth's own servers with its own protocol on every platform, so the Switch build is a
third-party-network title in the sense of [`ports.md`](https://github.com/openpak/docs/blob/main/ports.md), like Outbound's Photon
or Fall Guys.

The modern flow, as implemented by Impostor's HTTP server (public GPL-3.0 source, read for
protocol facts only):

1. The client holds a **region list**. Each region names a matchmaker host.
   - Legacy form (`DnsRegionInfo`): host plus UDP port, e.g. `na.mm.among.us`, `eu.mm.among.us`,
     `as.mm.among.us` on **22023**
     ([example regionInfo.json](https://github.com/onotera-com/amongus/blob/gh-pages/regionInfo.json)).
   - Current form (since Among Us 16.0.5 / Impostor 1.9.0): an **HTTPS matchmaker URL**. The
     public evidence for the live names is Innersloth's own client error text,
     `Error: 0 from authenticate at https://matchmaker-eu.among.us/api/user`
     ([Steam discussion](https://steamcommunity.com/app/945360/discussions/0/3176733459033026557/)).
2. `POST /api/user` → a **matchmaker token** (Impostor
   [`TokenController`](https://github.com/Impostor/Impostor/blob/master/src/Impostor.Server/Http/TokenController.cs),
   `[Route("/api/user")]`).
3. `GET`/`POST` `/api/games` with that token in `Authorization` → the **address of a game
   server**, an `Ip`/`Port` pair the server chooses (Impostor
   [`GamesController`](https://github.com/Impostor/Impostor/blob/master/src/Impostor.Server/Http/GamesController.cs),
   `[Route("/api/games")]`, plus `/api/games/{gameId}` and `/api/games/filtered`).
4. The client then speaks **Hazel over UDP** to that address for the whole session — lobby,
   game state, everything. Impostor's default is 22023/UDP, and its HTTP service listens on
   22023/TCP ([Running the server](https://github.com/Impostor/Impostor/blob/master/docs/Running-the-server.md)).

So there are exactly two console-facing surfaces: **one HTTPS endpoint** and **one UDP endpoint
whose address the HTTPS endpoint chooses**. That second fact is what makes this cheap: we never
have to make the console's UDP traffic land on port 22023, or on any particular host — the
matchmaker tells it where to go.

### What is *not* in scope

- **NSO membership.** The console, not Innersloth, gates online play behind a Nintendo Switch
  Online subscription. `nx-baas` already serves that surface for every title; nothing here.
  (Unknown whether Among Us actually checks it — see evidence item 1.)
- **Innersloth accounts / cosmetics.** Innersloth's own account linking
  ([their page](https://www.innersloth.com/account-linking-on-among-us/)) lists PC, Android, iOS
  and Xbox and says Switch is "being worked on". Cosmetics and progression are Innersloth-side
  and are not something OpenPak reproduces.

## 2. The existing implementations, and whether we may use them

| Project | Language | Licence | Verdict |
| --- | --- | --- | --- |
| [Impostor](https://github.com/Impostor/Impostor) | C#, .NET 8 | **GPL-3.0** (repository `LICENSE` is the plain GPLv3 text; no `-only`/`-or-later` declaration anywhere, so treat as GPL-3.0-only) | **Depend, as a separate process.** Complete, maintained, tracks Among Us releases, has an anticheat and a plugin API. |
| [SkeldJS](https://skeld.js.org/) | TypeScript | GPL-3.0 | Not needed. Protocol reference only. |
| [NodePolus](https://github.com/topics/amongus), [among-js](https://among-js-docs.vercel.app/) | JS/TS | mixed | Not needed. |
| [among-us-protocol](https://github.com/roobscoob/among-us-protocol) | prose | docs | Useful if we ever have to debug Hazel frames ourselves. |

**Licence position.** OpenPak is AGPL-3.0-only. GPL-3.0-only and AGPL-3.0-only are *not* freely
interchangeable: AGPLv3 §13 lets an AGPL work be combined with a GPLv3 work, but the combined
result then carries both sets of terms, and mixing them inside one binary or one Go module is
a mess nobody here wants to audit. We avoid the question entirely: **Impostor runs as its own
process, in its own container, unmodified.** Our Go code talks to it over a socket. That is mere
aggregation, exactly how the fleet already treats a separate service.

**Language exception.** The house preference is Go. Impostor is C#. That is the same exception
already recorded for `servers/photon`, and it is the right trade here: reimplementing Hazel, the
full Among Us game-object model, the anticheat, and then chasing every Among Us patch forever is
a permanent maintenance tax with no product value. OpenPak's value in this title is the console
redirect, which Impostor explicitly does not do — its own FAQ says
["Can this be used with the console version of the game? Sadly not, as these don't allow adding
custom servers."](https://github.com/Impostor/Impostor/blob/master/docs/FAQ.md)

That FAQ answer is about the *client UI*: PC and mobile players edit a region file, and consoles
have no such screen. It is not a statement that the console cannot be redirected. On a Switch we
do not need the game's cooperation — Atmosphère `DNS.mitm` plus a trusted CA is how every other
OpenPak title is already redirected, and the emulator side does the same
(`emulators/ryujinx/OPENPAK.md`). **This is the whole bet of this repository**, and it is
unproven: see `evidence-needed.md`.

## 3. What OpenPak actually builds

```
Switch ──DNS.mitm──> Traefik :443 (SNI passthrough, OpenPak CA)
                        │
                        ├─ HTTPS  ─> among-us front :22600 ─> Impostor HTTP :22601 (loopback)
                        └─ (the front's proxied /api/games reply names)
Switch ──UDP───────────────────────> Impostor game traffic :22603/udp
```

- **`cmd/among-us`** — one Go binary. A `/healthz` for the fleet convention, and a reverse proxy
  to Impostor for everything else. Its second job is the reason it exists at all: it logs the
  *paths* a real console asks for, which is the evidence the next round needs. It never logs
  query strings, headers, or bodies — the matchmaker token lives in all three.
- **Impostor**, unmodified, from its published container image, with `PublicIp` set to the host
  the console can reach and `HttpServer.ListenIp` on loopback.
- **Config**: a hosts line for the redirect, a Traefik route, a ufw rule for the UDP port.

No identity glue. Impostor mints its own matchmaker token in `/api/user` and the game asks for
nothing else; there is no OpenPak account to map onto until we see a console request that carries
one. If a capture shows the Switch build sending a platform identity we will add the mapping
then, in the front, where the request already passes through.

## 4. Ports

Claimed in [`ports.md`](https://github.com/openpak/docs/blob/main/ports.md): **22600–22609**.

| Port | Use |
| --- | --- |
| 22600 | `among-us` front, HTTP, behind Traefik |
| 22601 | Impostor HTTP matchmaker, loopback only |
| 22602 | metrics/pprof, spare |
| 22603/udp | Impostor game traffic (`Server__PublicPort`/`ListenPort`) |

22603 rather than Impostor's default 22023 because the matchmaker reply chooses the port. If a
capture shows the Switch build ignoring the returned port and hardcoding 22023, move Impostor
back to 22023 — nothing else changes.

## 5. Unknowns

Every one of these is a capture, not a guess. Ordered in `evidence-needed.md`.

1. **Does the Switch build's region list use the HTTPS matchmaker or the legacy DTLS/UDP region
   kind?** Unknown, needs a capture of the console's DNS queries and first TLS handshake.
2. **Which hostnames?** `matchmaker.among.us` / `matchmaker-eu.among.us` / `*.mm.among.us` are
   the PC names. The console build may use its own. Unknown, needs a DNS capture.
3. **Does the Switch build validate certificates through the system trust store** (so an
   Atmosphère CA patch is enough), or does it pin? Unity/IL2CPP titles usually use the system
   `ssl` service, which the CA patch covers — but "usually" is not evidence. Unknown, needs the
   handshake result from item 1.
4. **What client version integer does the Switch build send**, and does Impostor accept it?
   Console builds lag PC. Unknown, needs the front's log of a real `/api/user` or the Hazel
   handshake byte.
5. **Does the console require an Innersloth/EOS login** (`api.epicgames.dev`, `*.innersloth.com`)
   before it will reach the matchmaker at all? Unknown, needs the DNS capture from item 1.
6. **Does the game honour the `Ip`/`Port` the matchmaker returns**, including a port that is not
   22023 and an address that is not the matchmaker's? Unknown, needs a full connect attempt.

If item 1 or item 3 comes back badly — a pinned certificate, or a region kind we cannot reach —
this repository's answer is "not possible without patching the game", and it should say so and
stop, not grow a server.
