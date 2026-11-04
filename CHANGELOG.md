# Changelog — among-us

Generated from git history on 2026-09-15. `git log` stays the source
of truth; this file is the readable summary.

## v0.6.1 - 2026-11-04

- The erase endpoint forwards to Impostor (`openpak-v1.10.12`, which erases its rewards state, PUID map and outbox) on loopback, `IMPOSTOR_ERASE_URL` default `http://127.0.0.1:22601`, and answers with its row count; Impostor down = 502 so the step is retried. Impostor listens on loopback only, so this is the container's one door.

## v0.6.0 - 2026-11-04

**Account erasure endpoint (openpak/prds#120), a no-op.** `DELETE /internal/accounts/{account_id}/data` with
`Authorization: Bearer $ACCOUNT_ERASE_KEY` (unset: 503) answers `{"service":"among-us","rows":0}`. The front holds no
per-account data: its only state is the room code -> region node table (no player ids) and a routing cache that no
code path fills. The player data lives in Impostor (its own repo/endpoint).

## v0.5.1 — 2026-11-03

- Impostor `openpak-v1.10.11`: one log line per HTTP request (`http <method> <path> -> <status> <ms>ms`) and
  per Hazel message (`cmd <Message> -> ok|error <ms>ms`), plus connect and disconnect lines. No query,
  headers, bodies, nicknames or message bytes; the per-packet game-data relay is not logged.

## v0.5.0 — 2026-11-03
- Per-request logging: one slog line per HTTP request (method, path, status, ms) on stderr via internal/httplog. Query strings, headers and bodies are never logged; /healthz and /metrics are skipped.

## v0.4.0 — 2026-11-03

- Impostor `openpak-v1.10.10`: rewards state persisted in Postgres. Set `IMPOSTOR_DATABASE_URL` on the
  unit (docs/deploy.md).

## v0.3.2 — 2026-11-03

- Release workflow IMPOSTOR_REF fallback now openpak-v1.10.9 (v0.3.1 still built 1.10.8). Impostor reports `game_sessions.among-us` to rewards (needs
  `REWARDS_EVENTS_*` and `EOS_INTERNAL_*` on the unit; docs/deploy.md).

## v0.3.0 — 2026-11-02

- Release from main; replaces the hand-built production image.


- Region nodes: with `OPENPAK_ROUTING_URL` set, a host's `PUT /api/games` gets the Impostor of the
  node for the region hostname they picked, and a join by a code a node reported
  (`POST /internal/v1/amongus/rooms`, `AMONG_US_ROOM_REPORT_KEY`) gets that node. Unset URL or no
  nodes: unchanged.

## v0.2.2 — 2026-10-10

- The log carries no personal data: request paths keep their shape but any segment that is not a
  plain route word (a game code, an id, a name) is logged as a `~xxxxxx` tag, proxy and server
  errors lose the addresses and URLs they spell out, and the client address never reaches a line.
  Behaviour is unchanged. `TestLogsCarryNoPersonalData` drives the front with a sentinel game code,
  token, query, name and address and fails if any reaches the log.

## v0.2.1 — 2026-09-25

- First tagged build since the build moved to the self-hosted runner and the private Impostor fork (openpak-v1.10.8) is cloned through a netrc build secret; game code unchanged since v0.2.0
- ci: release builds on the self-hosted runner with podman; builds only on `v*.*.*` tags;
  build scratch in the runner temp [4c0854b, 07f4bf0, 51169ea]
- build: the private impostor fork is cloned through a netrc build secret
  (`OPENPAK_MODULE_TOKEN`) [0dcdd60]

## v0.2.0 — 2026-09-11

- One image: Impostor built from the openpak fork at build time, the front beside it [594ef56]

## v0.1.1 — 2026-09-11

- Dockerfile: no dependencies, so no go.sum to copy [2ae8ad1]

## v0.1.0 — 2026-09-11

- Container, release image and the deployed status [a59c287]
- Go 1.26, the house version [52c57e2]
- design: depend on Impostor, build only the console redirect [32c93aa]
- seed: empty among-us, AGPL-3.0-only, clean-room gate [128d3a8]

