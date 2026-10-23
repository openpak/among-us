# Evidence needed

> **Current Switch client (2026-10-13): Harbor**, at
> `~/REPOS/Openpak/emulators/harbor`. Harbor is an independent
> OpenPak project based on Eden. Citron, Ryujinx (desktop and Android), and
> the former OpenPak Eden checkouts have been removed. References to those
> forks below record historical evidence only: do not run their commands,
> reinstall them, or select them for current work. Use Harbor's README and
> current handoff for builds and testing; old results do not establish Harbor parity.

Ordered so that a failure stops the work early. Each step says what to do, what would confirm
the design, and what would kill it. Raw captures stay out of this repository (`.gitignore`
already refuses `*.pcap`); write findings up here as prose and hostnames.

The cheapest vantage point is the emulator: `emulators/ryujinx` resolves DNS through
`DnsMitmResolver`, reading the Atmosphère hosts file from the virtual SD card at
`/atmosphere/hosts/default.txt`, wildcard `*` included, and it already trusts the OpenPak CA
(`OPENPAK_CA`, `emulators/ryujinx/OPENPAK.md`). Everything below can be done there first and
repeated on hardware only once it works. `emulators/citron` is the second vantage point if
Ryujinx cannot run the title.

## 1. What does the game resolve, and does it start a TLS handshake? (blocks everything)

Run Among Us in `emulators/ryujinx` with no redirect at all and log name resolution and socket
activity. In Citron the equivalent is the log filter already documented in its README
(`*:Info Service:Debug Service.SSL:Debug WebService:Debug`).

Record: every hostname queried when entering online play, in order; whether any of them are
`*.epicgames.dev` or `*.innersloth.com` (evidence for a hard account gate, design §5 item 5);
whether the matchmaker contact is HTTPS on 443 or UDP on 22023 (design §5 item 1); and the exact
matchmaker hostname (item 2).

- **Confirms the design** if the game resolves a matchmaker name and opens TLS to it on 443.
- **Kills the cheap path** if it opens a UDP DTLS session to a hardcoded IP with no DNS lookup:
  then there is no name to hijack and the answer is "needs a game patch".

## 2. Does it trust our CA on that name? (blocks everything)

Add the hostname from step 1 to `default.txt` pointing at the OpenPak host, put the name on
Traefik with an OpenPak-issued certificate, and run `cmd/among-us` with no Impostor behind it.

- **Confirms** if the front logs any request path at all — that means the handshake succeeded,
  which is design §5 item 3 answered.
- **Kills it** if the game reports a network error with nothing in the front's log: the build
  pins its certificate, and this repository stops.

## 3. What does the console actually ask for?

Read `cmd/among-us`'s log from step 2. It classifies each path against the routes Impostor
serves (`/`, `/api/user`, `/api/games`, `/api/games/{id}`, `/api/games/filtered`) and marks
anything else `NEW`.

Record: every `NEW` path, and the response status Impostor gives to the known ones. A `NEW` path
is the only thing that would justify writing handler code in this repository rather than
proxying.

## 4. Does Impostor accept the Switch build's version?

Start Impostor behind the front and let the console reach `/api/user` and `/api/games`.

- **Confirms** if `/api/games` returns an `Ip`/`Port` instead of a `MatchmakerError`.
- **A version rejection** shows up as a disconnect reason naming the client version — design §5
  item 4. Fix order: check whether Impostor's master branch already supports that build; only if
  it does not, consider `AllowFutureGameVersions`, and record that we did.

## 5. Does it go where the matchmaker sends it?

With Impostor's `Server__PublicIp`/`PublicPort` set to the OpenPak host on **22603/udp** (`ufw
allow 22603/udp`), host a lobby from the console.

- **Confirms the whole design** if the console reaches the UDP listener on 22603 and a lobby
  opens — design §5 item 6.
- **If the console instead sends UDP to 22023**, move Impostor to 22023 and note it in
  `ports.md`; the design is otherwise unchanged.

## 6. Two clients

Host on the console, join from a PC Among Us pointed at the same server with a generated region
file, then the reverse. This is the first result worth announcing, and the first that proves the
game is genuinely playable rather than merely connected.

## 7. Hardware

Repeat steps 1–6 on a real Switch with the hosts file and CA patch that every other OpenPak
title already uses. Only after the emulator path works end to end: hardware adds NSO membership
and real DNS to the list of things that can be wrong, and there is no reason to debug all three
at once.
