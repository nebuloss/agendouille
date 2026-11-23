# AGENTS.md: agendouille

agendouille is a family's shared calendars: a Go server with an
embedded TypeScript web UI, read and written over CalDAV by phones
(Android through DAVx5, iOS natively) and Thunderbird, reached from the
internet through a reverse proxy.

**Read `PLAN.md`, then `CONTRACTS.md`.** `PLAN.md` is the specification:
data model, CalDAV, Android, web UI, security, build, milestones.
`CONTRACTS.md` holds the interfaces between parts and wins on any contract
the two disagree on. Record every deviation, with the reason, in
`DECISIONS.md`. Open questions are in PLAN section 16: ask, do not guess.

## Status

Planning. Nothing is built yet; next is M0 (PLAN section 14).

## Conventions come from cafard

This project follows `~/misc/cafard`: the layout, config loading, embedded
Vite bundle, plain-DOM TypeScript classes, CSS, CI, release, installer and
`remote.sh`. PLAN section 13 lists the files to copy from. When unsure, do
what cafard does.

## Build and test

```sh
npm ci
npm run typecheck        # tsc; vite does not typecheck
npm test                 # vitest
npm run build            # src/ -> internal/web/assets/static/app/main.js
scripts/check-sinks.sh   # no HTML sinks in src/

scripts/remote.sh        # sync to dev-build: gofmt -l, build, vet, test -race
scripts/remote.sh <cmd>  # run anything in the remote tree
```

- **Go is built and tested on dev-build only**, through `scripts/remote.sh`
  (it runs there through `rtk`). Never build on the editing machine. A pass
  on dev-build is the only pass that counts.
- **No cgo, ever.** Releases cross-compile with `CGO_ENABLED=0`.
- **No Android build on the editing machine.** If M8 happens, the APK is
  built in a container on dev-build and in CI.

## Gotchas that will cost you an afternoon

- **One write path.** Every change to an event goes through
  `calendar.Service.Put`/`Delete` (CONTRACTS 1.3). Writing `objects` or
  `changes` anywhere else breaks sync tokens, and clients silently miss
  changes.
- **Never rewrite what a CalDAV client PUT.** Store its bytes. Clients
  compare what they get back; the web UI patches, it does not regenerate
  (PLAN 4.5, 4.6).
- **All the VEVENTs of one UID live in one object**, overrides included
  (RFC 4791 4.1). An override is not an event of its own.
- **Times are in the calendar's zone**, not the browser's and not the
  server host's (PLAN 4.4). Test across the DST change.
- **Devices, not passwords, on CalDAV.** Basic auth takes device passwords
  only; the member's web password is refused there on purpose.
- **Three surfaces, maybe on different listeners** (`dav`, `app`, `admin`;
  PLAN 9.1). A handler belongs to exactly one, and its absolute URLs come
  from that surface's listener, never from the request's `Host`. The admin
  UI may be on a LAN address that the DMZ never sees.
- **`src/app` and `src/admin` never import each other**; shared code goes
  in `src/shared`. The admin bundle must not be served on the DMZ.
- **Templates and static files live under `internal/web/assets/`**:
  `go:embed` cannot reach a parent directory.
