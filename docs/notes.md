# Notes: among-us

Getting a Nintendo Switch onto a self-hosted Among Us server.

**This repository is not a game server, and should not become one.** Among Us does not use any
Nintendo online service — it talks to Innersloth's own servers with its own protocol — and
[Impostor](https://github.com/Impostor/Impostor) (GPL-3.0, C#) already reimplements that server
completely and keeps up with the game's releases. What Impostor does not do is get a console
onto it: its FAQ says consoles "don't allow adding custom servers", which is true of the client's
region-select screen and not true of a Switch, where the hostname can be redirected the same way
every other OpenPak title is.

So the whole of OpenPak's contribution here is a redirect, a TLS front, and about a hundred lines
of Go. See [`docs/design.md`](design.md) for the reasoning and the sources.

**Status: deployed 2026-09-11, unproven on a console.** one container (`ghcr.io/openpak/among-us`: Impostor built from our fork plus this front) runs on production; Traefik terminates `matchmaker.among.us`, `matchmaker-eu.among.us` and `matchmaker-as.among.us` with an OpenPak-CA leaf and `openpak.nro` 0.2.2 redirects them. Game traffic: UDP 22603. Nothing has been tested against the game yet. The design rests on assumptions
about the Switch build that only a capture can settle;
[`docs/evidence-needed.md`](evidence-needed.md) lists them in the order that matters, and
two of them can kill the approach outright.

## Ground rules

- Clean room. Read [clean-room policy](https://docs.openpak.org/contributors/clean-room/) before writing
  a line. Public protocol documentation, our own captures, and compatibly licensed code are the
  only inputs.
- AGPL-3.0-only, English only, Go 1.25, no secrets in the repository.

This project is not affiliated with or endorsed by Nintendo, Among Us's publisher, or any
original service provider. It contains no proprietary game code, assets, SDK material,
binaries, certificates, keys, firmware, extracted files, credentials, or raw packet captures.
