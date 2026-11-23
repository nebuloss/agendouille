# agendouille

> *agendouille*: an *agenda* with the *-ouille* of *andouille*. The
> family's calendars, not serious ones.

A self-hosted calendar server for a family, at home, with nothing outside.

- **Users** own calendars and share them with the people of their groups,
  read-only or read-write.
- **Android**: the agendouille app syncs into the phone's calendar (any
  calendar app shows the events) and signs browsers in with a QR code.
  DAVx5 and other CalDAV apps work too.
- **iPhone and macOS**, natively over CalDAV; **Thunderbird** too.
- **A web app**, good on a phone. No passwords and no email: sign in by
  approving from a device already signed in.
- **An admin UI** on the LAN only: users, groups, sign-in QR codes, ports,
  storage, backups, limits. It never shows events (a promise of the UI:
  whoever runs the server can still read its database).

Sync is standard CalDAV for everyone; sharing and sign-in go through an
additional API. One Go binary, SQLite, behind a reverse proxy in the DMZ.

**Status: M0, the skeleton.** It builds, runs and answers `/healthz` on
the admin listener; nothing else yet. See `PLAN.md` for the design,
`CONTRACTS.md` for the interfaces and `DECISIONS.md` for what was decided
and why.

## Build

The frontend is two TypeScript bundles (the family web app and the admin
UI) that the Go binary embeds, so build them first:

```sh
npm ci
npm run typecheck && npm test && npm run build
go build ./cmd/agendouille
```

Go 1.26 and no C compiler (`CGO_ENABLED=0`). A binary built without the
bundles still runs.

## Run

```sh
agendouille version
agendouille config          # validate and print the bootstrap configuration
agendouille serve -data-dir ./data
curl http://127.0.0.1:8792/healthz   # ok dev
```

The bootstrap file is `/etc/agendouille.yaml` (or `-config`, or
`AGENDOUILLE_CONFIG`); every key is also a flag and an `AGENDOUILLE_*`
variable, which win in that order:

| File key | Flag | Default |
|---|---|---|
| `data_dir` | `-data-dir` | `/var/lib/agendouille` |
| `admin_listen` | `-admin-listen` | `127.0.0.1:8792` |
| `admin_public_url` | `-admin-public-url` | `http://<admin_listen>`; required off loopback |
| `admin_allow_insecure` | `-admin-allow-insecure` | `false`; needed for a plain-http admin URL off loopback |
| `log_level` | `-log-level` | `info` |

Everything else (the public listener for CalDAV and the family web app,
limits, backups) will be a setting edited in the admin UI, from M1.
