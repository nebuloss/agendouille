# AGENTS.md: agendouille

agendouille is a self-hosted calendar server for a family, at home, with no
outside service: a Go server (CalDAV, the family web app, the admin UI), a
TypeScript web app, and an Android app (a Java sync adapter around the web
app in a WebView).

**Read `PLAN.md`, then `CONTRACTS.md`.** `PLAN.md` is the specification;
`CONTRACTS.md` holds the interfaces between parts and wins on any contract
the two disagree on. Record every deviation, with the reason, in
`DECISIONS.md`. Open questions are in PLAN section 17: ask, do not guess.

## Status

M0 is done: `agendouille serve|config|version`, the bootstrap config
(`internal/config`), the admin listener with `/healthz` and the admin
bundle (`internal/web/assets`, one bundle per surface), the two Vite
bundles, `src/shared/core` (Dom, Disposable, Clock, Backoff, Pages) with
tests, the import-rule test, CI, release, `scripts/remote.sh`. Next is M1
(PLAN section 15).

## Conventions come from cafard and claudication

Layout, config loading, embedded Vite bundles, plain-DOM TypeScript
classes, CSS, CI, release, installer and `remote.sh` come from
`~/misc/cafard`; the operator model (one admin, settings in the UI, a
password printed once at install) from `~/misc/claudication`. PLAN section
14 lists the files to copy from.

## Build and test

The editing machine has neither Go nor Node: everything runs on dev-build
through `scripts/remote.sh`, which rsyncs the tree there first.

```sh
scripts/remote.sh        # gofmt -l, go build, go vet, go test -race
scripts/remote.sh web    # npm ci, typecheck, vitest, both bundles, the sinks check
scripts/remote.sh all    # web, then the Go checks (so they see the bundles)
scripts/remote.sh <cmd>  # anything in the remote tree, e.g. 'rtk go test ./internal/config'
scripts/check-sinks.sh   # runs here too: no HTML sinks in src/

scripts/android.sh test  # (M6) JVM tests of the Android app, on dev-build
scripts/android.sh apk   # (M6) the debug APK, same place
```

A new npm dependency: change `package.json`, run `scripts/remote.sh 'rtk npm
install'`, and copy `package-lock.json` back with `scp`.

- **Go and the APK are built on dev-build only** (through `rtk`). Never
  build on the editing machine. A pass on dev-build is the only pass that
  counts.
- **No cgo, ever.** Releases cross-compile with `CGO_ENABLED=0`.
- **No Gradle, no Kotlin, no AndroidX.** The APK is a Makefile over
  `aapt2`, `javac`, `d8`, `apksigner` and one `android.jar`. Plain `.jar`
  libraries only.

## Gotchas that will cost you an afternoon

- **Sync is standard CalDAV, for our app too.** Anything the app needs
  beyond it goes in the additional API (CONTRACTS 2.2, 3). If DAVx5 cannot
  get something our app gets over `/dav/`, it is a bug.
- **One write path.** Every change to a calendar or an event goes through
  `calendar.Service` (CONTRACTS 1.3). Writing `calendars`, `objects` or
  `changes` anywhere else breaks sync tokens, and clients silently miss
  changes.
- **The admin never sees events.** `internal/web/admin` calls only the
  metadata functions; a test checks every `/api/admin` answer.
- **Never rewrite what a CalDAV client PUT.** Store its bytes. The web app
  and the Android app patch, they do not regenerate (PLAN 4.5, 4.6, 7.3).
- **All the VEVENTs of one UID live in one object**, overrides included.
- **Times are in the family zone**, not the browser's nor the host's.
- **Three surfaces, maybe on different listeners** (`dav`, `app`,
  `admin`). Absolute URLs come from the surface's `public_url` setting,
  never from `Host`. The public listener can be rebound live from the
  admin UI; the admin listener only from the bootstrap file.
- **`src/app` and `src/admin` never import each other.** Shared code goes
  in `src/shared`; the admin bundle is never served on the public listener.
- **DAVx5's code is GPLv3; we are MIT.** Read `synctools` and
  `davx5-ose` for rules and quirks, write them down as tests, never copy or
  translate their code (PLAN 7.3).
- **iCalendar fixtures are shared** between Go and the Android JVM tests
  (`testdata/ical/`). Change one side, run both.
- **Templates and static files live under `internal/web/assets/`**:
  `go:embed` cannot reach a parent directory.
