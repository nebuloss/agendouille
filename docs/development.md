# Development

Read `PLAN.md` (the specification), then `CONTRACTS.md` (the interfaces
between the parts, which win on any contract the two disagree on).
`DECISIONS.md` records every deviation and why; `AGENTS.md` holds the
conventions and the traps.

## Layout

| Path | What |
|---|---|
| `cmd/agendouille` | The binary: `serve` and the operator's commands; end-to-end tests over HTTP. |
| `internal/` | `store` (SQLite, migrations), `config`, `settings`, `directory` (users, groups), `auth`, `sharing`, `calendar` (the one write path), `ical` (our own RFC 5545 parser and RRULE engine), `dav` (CalDAV), `web/app` (the family web app), `web/admin`, `backup`, `live`, `i18n`. |
| `src/app`, `src/admin`, `src/android` | The three TypeScript bundles (plain DOM, Tailwind). They never import from each other's pages, except that `src/android` reuses the web app's calendar code. |
| `src/shared` | What they share: the API client, components (dialogs, the picker, segmented controls), i18n, the design. |
| `android/` | The Android app: plain Java, a Makefile over `aapt2`, `javac`, `d8`, `apksigner`. |
| `deploy/` | `install.sh`, the service units, proxy examples. |
| `scripts/` | Build, check, deploy and screenshot scripts (below). |
| `testdata/ical` | iCalendar fixtures shared by the Go and the Android tests. |

## Building

The frontend is three bundles the Go binary embeds (the Android one goes
into the APK); build them first:

```sh
npm ci
npm run typecheck && npm test && npm run build
go build ./cmd/agendouille
```

Go 1.26, no C compiler (`CGO_ENABLED=0`, always). A binary built without
the bundles still runs.

Our editing machine builds nothing: everything runs on the build machine
through `scripts/remote.sh`, which syncs the tree there first.

```sh
scripts/remote.sh          # gofmt -l, go build, go vet, go test -race
scripts/remote.sh web      # npm ci, typecheck, vitest, the bundles, the HTML-sink check
scripts/remote.sh all      # web, then the Go checks
scripts/remote.sh '<cmd>'  # anything in the remote tree
```

## The Android app

```sh
scripts/android.sh test    # the JVM tests of the pure-Java parts
scripts/android.sh apk     # android/build/agendouille.apk, on the build machine
scripts/android.sh cert    # the signing key's SHA-256 (for assetlinks.json)
scripts/remote.sh 'bash scripts/android-live.sh'   # the sync code against a real server
```

No Gradle, no Kotlin, no AndroidX, no Google Play services (the build
fails if the dex names them). The APK is signed with the release key when
the build machine has it (`scripts/conf/build.conf`), else a debug key.

## Screenshots

Tests in happy-dom do not catch layout: look at the pages.

```sh
scripts/remote.sh 'bash scripts/shots/run.sh'       # the admin UI and the web app
scripts/remote.sh 'npm run build && node scripts/shots/shoot-android.mjs android/assets/web /tmp/shots fr'
```

`run.sh` seeds a server from `scripts/fixtures` (the family, this week's
events); the Android screens run against a fake bridge
(`scripts/shots/fixtures/android.json`).

## Scripts

The entry points in `scripts/` are thin. Settings go in `scripts/conf`
(`build.conf`: the build machine and its trees; `targets/*.conf`: where to
deploy), data in `scripts/fixtures`, shared functions in `scripts/lib`,
the steps that run on the build machine in `scripts/build`. CI runs
shellcheck over all of them.

## Deploying

```sh
scripts/devdeploy.sh home          # a build of the working tree, on scripts/conf/targets/home.conf
scripts/deploy.sh home v0.1.0      # a GitHub release
TARGET=root@host scripts/devdeploy.sh
```

There is no default target, on purpose. Both run `deploy/install.sh`
there, which never touches the configuration, the data or the admin
password.

## Releasing

Push a tag `vX.Y.Z`: `.github/workflows/release.yml` builds and checks the
bundles, builds and signs the APK (secrets `ANDROID_KEYSTORE_B64`,
`ANDROID_KEYSTORE_PASSWORD`, `ANDROID_KEY_ALIAS`), runs the Go tests with
the APK embedded, cross-builds `linux-amd64`, `linux-arm64` and
`linux-armv7`, smoke-tests one, and publishes them with `SHA256SUMS`.
Installed apps from a release update themselves from it.

CI (`.github/workflows/ci.yml`) runs on every push to `main` and `dev`:
frontend, server, the cross-builds, the Android tests and APK, shellcheck.

## Translations

[i18n](i18n.md): French and English, the style guide and the glossary.
