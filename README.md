<p align="center">
  <img src="res/images/agendouille.svg" alt="agendouille" width="520">
</p>

*agendouille*, n. — an *agenda* with the *-ouille* of *andouille*. The
family's calendars, not serious ones.

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

**Status: M4.** CalDAV works, read and write, with calendars owned by
users and shared with users or groups. The admin UI manages users,
groups, sign-in QR codes, calendars (their metadata only), settings and
backups at `http://127.0.0.1:8792/`. The family web app shows the
calendars by agenda, week and month, edits events (one, this and
following, or all of a series), creates and shares calendars, and sets
up phones (DAVx5 by QR, iPhone by profile, Thunderbird by hand) and
read-only `.ics` links. `deploy/install.sh` installs and upgrades it on
Debian or Alpine (see `deploy/README.md`). See `PLAN.md` for the design,
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
agendouille admin passwd --random     # printed once; serve refuses to start without it
agendouille serve                     # the admin UI on 127.0.0.1:8792
curl http://127.0.0.1:8792/healthz    # ok dev

# The public side (CalDAV and the family web app), for the DMZ proxy, in
# /etc/agendouille.yaml (applied within seconds):
#   public_listen: 0.0.0.0:8791
#   public_url: https://agenda.example.org

agendouille group add famille
agendouille user add lea --display-name Léa --group famille
agendouille user enroll lea            # a one-time sign-in link and QR code, 72 hours

agendouille calendar add lea perso --name "Perso"
agendouille calendar share lea~perso g:famille read
agendouille import lea~perso google-export.ics
agendouille device add lea --label "Pixel de Léa"   # DAVx5's QR; or --kind ios|thunderbird
```

Any CalDAV client then syncs: DAVx5 from the QR, an iPhone (Settings,
Calendar, Accounts, Add, Other, CalDAV, with the server, username and
password printed), Thunderbird (New calendar, On the network, the same).
A calendar made in DAVx5 or on an iPhone belongs to its user.

Users have no password and no email. The enrollment QR signs in their first
device; after that, any browser shows a QR code at `/` that a
signed-in phone approves (`/approve`, or the agendouille app's scanner),
picking the number the browser shows. `agendouille config` prints every
setting and where it came from.

The configuration is `/etc/agendouille.yaml` (or `-config`, or
`AGENDOUILLE_CONFIG`), every setting included, as cafard's and
claudication's (`deploy/agendouille.yaml.example` lists them); a running
server applies an edit of a setting within seconds, and the admin UI shows
them without editing them. Every key is also a flag and an `AGENDOUILLE_*`
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
