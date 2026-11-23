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

M0 and M1 are done. What exists:
- `cmd/agendouille`: `serve`, `config [set k v ...]`, `admin passwd
  [--random]`, `user add|enrol|list|disable|enable|remove`, `group
  add|list|remove|member`, `version`. `serve` refuses to start without an
  admin password.
- `internal/store` (migrations in `migrations/*.sql`, times as text in
  `store.TimeFormat`), `internal/settings` (typed, validated together,
  live: `Subscribe`, and `Watch` for changes another process made),
  `internal/directory` (users, groups, `Sees`), `internal/auth` (admin
  password with argon2id, sessions per surface, enrolment links,
  approvals with number matching, device credentials and Basic auth,
  `ClientIP`, `SameOrigin`), `internal/surface` (the admin listener from
  the bootstrap file, the public one from `public_listen`, rebound live).
- `internal/web/app`: `/signin` (QR, short code, number), `/signin/wait`,
  `/enrol/{token}`, `/approve`, `/api/me`, `/api/approvals/{code}`,
  `/api/app/enrol|session|config`. `internal/web/admin`: `/admin/signin`
  and a placeholder home. `internal/dav`: only the Basic check (`501`
  once signed in). `src/app/pages`: `SigninPage` (waits on the long-poll),
  `ApprovePage` (reads the code from a scanned QR's fragment).
- `cmd/agendouille/e2e_test.go` drives M1's "done when" over HTTP.

- **M2, CalDAV:** `internal/ical` (our own RFC 5545 parser and encoder,
  and our own RRULE engine: `rrule_test.go` runs every example of RFC 5545
  3.8.5.3), `internal/sharing` (access: owner, user and group shares,
  device cap; views), `internal/calendar` (the one write path, change log,
  tokens, quotas, import/export, prune), `internal/hub`, `internal/dav`
  (PROPFIND, the three reports, GET/PUT/DELETE with ETags, MKCALENDAR and
  extended MKCOL, PROPPATCH). CLI: `calendar`, `device`, `import`,
  `export`. Tests: `caldav_e2e_test.go` (M2's "done when" with raw CalDAV
  requests) and `clients_test.go` (replays `internal/dav/testdata/clients`).

- **M3, the admin UI:** `internal/web/admin/api.go` (CONTRACTS 3.5),
  `internal/backup` (scheduled `VACUUM INTO`, list, download, restore by
  staging and restarting), the admin bundle (`src/admin/pages`: Overview,
  Users and User, Groups, Calendars, Activity, Settings, Backups), the
  shared stylesheet (`static/shared/app.css`), `src/shared/core/Api.ts`,
  `src/shared/components/Ui.ts`. CLI: `backup`, `restore`. Tests:
  `admin_e2e_test.go` (M3's "done when", the privacy walk over every
  `/api/admin` answer, a real restore). `scripts/shots/run.sh` takes
  screenshots of the admin UI on dev-build.

- **M4, the family web app:** `internal/ical/web.go` (the web edits, by
  patching, every scope), `internal/web/app/family.go` (CONTRACTS 3.2 to
  3.4) and `live.go` (SSE, the `.mobileconfig`), `calendar.Service.Move`,
  subscription links (`auth/links.go`, `/ics/` in `internal/dav`). The app
  bundle: `src/shared/core/Dates.ts`, `src/app/model/Recurrence.ts`,
  `src/app/components/views.ts` (agenda, week, month), `Editor.ts`,
  `src/app/pages/CalendarPage.ts` and `Manage.ts` (Calendriers, Appareils,
  Moi). Tests: `family_e2e_test.go` (M4's "done when"),
  `src/app/**/*.test.ts`. `scripts/shots/run.sh` shoots the app too.

- **M5, deployment:** `deploy/` (install.sh for Debian and Alpine, the
  hardened unit, OpenRC under supervise-daemon, the container drop-in,
  nginx and Caddy examples, README with the client checklist),
  `scripts/devdeploy.sh`, `admin passwd --if-unset`, the andouille icon.
  Deployed to CT 408 `dmz-agendouille`:
  `PVE=root@10.0.0.2 CT=408 scripts/devdeploy.sh` upgrades it.

M2's by-hand checks with real DAVx5, iOS and Thunderbird are still to do,
now with the web app's Appareils page to set them up.

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
- **No iCalendar or CalDAV library, on purpose** (DECISIONS 2026-11-23). A
  recurrence bug is fixed in `internal/ical/rrule.go` with a test, not by
  adding a dependency.
- **The client fixtures in `internal/dav/testdata/clients` are hand-written**
  until the by-hand checks capture real ones; a captured request that
  fails is a server bug, never a fixture to edit.
- **iCalendar fixtures are shared** between Go and the Android JVM tests
  (`testdata/ical/`). Change one side, run both.
- **No rate limits, on purpose** (DECISIONS 2026-11-23). Do not add one to
  a route that checks a random secret; keep secrets random and long
  instead.
- **A setting changed from the CLI reaches a running server through
  `settings.Watch`** (every 5 s), not through its own `Set`. Tests that
  change settings from a second `services` lower `settingsPoll`.
- **The remote tree is rsynced with `--delete`:** a `go get` or `npm
  install` run there is undone by the next `scripts/remote.sh` unless
  `go.mod`/`go.sum`/`package-lock.json` are copied back with `scp` first.
- **A page template's dot is its Data**, not the `webx.Page`: never wrap a
  page in `{{with .Data}}` (an empty map is false and the page vanished;
  it happened to the admin sign-in).
- **A restore restarts the server** (`ErrRestart`, exit 75): the service
  must run under `Restart=on-failure` or a supervisor, which `deploy/`
  sets up (systemd, and OpenRC's `supervise-daemon`).
- **The rsync in `remote.sh` deletes `scripts/shots/node_modules`** (the
  link to puppeteer): `ln -sfn $HOME/shots/node_modules scripts/shots/node_modules`
  in the same remote command before any `node scripts/shots/*.mjs`.
- **Screenshots before saying a page works:** `scripts/remote.sh 'bash
  scripts/shots/run.sh'`, then look at them. Happy-dom tests do not catch
  layout. The app pages hold `/api/live` open: wait for `networkidle2`,
  never `networkidle0`.
- **The web app sends and gets naive local times in the family zone.**
  Never `new Date("2026-11-25T14:00")` in the browser (that is the
  browser's zone): use `Dates.ts`.
- **Templates and static files live under `internal/web/assets/`**:
  `go:embed` cannot reach a parent directory.
