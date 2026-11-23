# Decisions

Deviations from `PLAN.md` and answers to its open questions, newest first.
Each entry gives the date, the decision and the reason.

## 2026-11-23: two UIs on separate surfaces, several calendars, QR sign-in

Asked how the frontend splits the admin UI from the family's, and how
members and calendars are managed, the user answered:

- **"Since caldav will be exposed on dmz we need to support splitting
  ui."** agendouille serves three surfaces, `dav`, `app` (the family app)
  and `admin`, and the config's `listeners` say which listener serves
  which (PLAN 9.1). The intended layout: `dav` and `app` on the port the
  DMZ proxy forwards, `admin` on a LAN-only port. Off its listener a
  surface answers `404`. Two Vite bundles (`app.js`, `admin.js`) share
  `src/shared`, so the admin code is never served to the DMZ, and the two
  surfaces have separate session cookies. The admin UI signs in with a
  password only, no QR approval, so a stolen phone session on the
  internet side never reaches it.
- **Several shared calendars** with per-member access (`none`, `read`,
  `readwrite`), made and managed by admins. This replaces "one shared
  calendar" from the starting brief; the `famille` calendar is made at
  first start. Events are coloured by their calendar now, and the creator
  is shown by an initial (PLAN 9.4).
- **Roles: admin and member.** A read-only child is a matter of access per
  calendar, not a role.
- **"qrcode login"** for lost passwords, read as QR sign-in in general
  (PLAN 8.1): an admin makes a one-time sign-in link shown as a QR (the
  invitation and the reset path), and a signed-in phone can approve a new
  device by scanning the QR on its `/signin`. Member passwords become
  optional; admins must have one.

Milestones were renumbered: M3 is the admin UI, M4 the family app, M5
devices, M6 deployment, M7 instant sync, M8 the Android app. Entries
below this one use the old numbers.

## 2026-11-23: our own server, not an existing one

Asked whether existing software already does this: DAVx5 covers Android
(and stays the client, PLAN 6). For the server, Radicale, Baïkal, Davis,
Nextcloud, SOGo, RustiCal and EteSync were weighed. The user keeps a server
of our own: "i hate php and python for server so i would like to keep
server side implementation to be free to manage properly users and
calendar + have an ui view", and "it would allow to have a modern stack"
(Go 1.26, TypeScript, one static binary). So no PHP or Python server, members and the
calendar are managed in agendouille itself (the web UI's Members and
Calendar settings pages, and the CLI), and the web UI has a real calendar
view (PLAN 9.3). RustiCal (Rust, SQLite, WebDAV-Push) is the closest
existing server; it has no calendar view and says it is not
production-ready, and it is worth reading when writing `internal/dav`.

## 2026-11-23: the starting brief

The user's brief: a shared calendar for the family; every member reads and
creates events; Go for the server, TypeScript for a web UI; at first,
CalDAV read-only and a custom Android client for writing. Asked before
planning, the user answered:

- **CalDAV read and write, no protocol of our own.** "If caldav supports
  write let's use it instead of reinventing our own protocol." It does
  (RFC 4791 `PUT`/`DELETE` with etag preconditions), so every client
  writes through CalDAV, and the JSON API serves the web UI only. The
  first idea, read-only access for some devices, survives as a per-device
  `read` access (PLAN 5.3).
- **One shared calendar**, every member writes all of it. (Replaced the
  same day by several calendars with access per member; see above.)
- **Reachable from the internet behind a reverse proxy.** Hence the whole
  of PLAN section 10 in v1: argon2id, device passwords for CalDAV, lockout,
  trusted proxies, the exposure guard.
- **A sync adapter on Android, without building Kotlin or Java.** These
  meet in DAVx5, which is a CalDAV sync adapter already. PLAN section 6
  makes DAVx5 the Android client, adds QR pairing (M4) and WebDAV-Push
  (M6), and leaves a custom app as optional M7, to decide once DAVx5 has
  been used; if built, it builds in a container on dev-build and in CI,
  never with Android Studio on the editing machine.

## 2026-11-23: CalDAV written in-house, not emersion/go-webdav (Default)

`github.com/emersion/go-webdav` v0.8.0 has a CalDAV server, but its
`caldav.Backend` (checked on master on 2026-11-23) has no
`sync-collection` REPORT and no `CS:getctag`. Without one of them, DAVx5,
iOS and Thunderbird re-list every etag on every sync, and the server has no
cheap way to tell them nothing changed. The backend interface also gives
no place for per-member homes that alias one calendar, per-device
read-only privileges in `current-user-privilege-set`, or the per-member
`PROPPATCH` iOS sends. The surface agendouille needs (PLAN 5.2) is small
and fixed: one calendar, `VEVENT` only, three reports. `internal/dav`
implements it on `encoding/xml`, and go-webdav's **client** stays as a
test-only dependency, an independent implementation to test against.

Revisit if go-webdav gains sync-collection and the in-house package turns
out to be the larger cost.

## 2026-11-23: first start needs a member made at the shell (Default)

cafard prints a random admin password at first start. Here the first
member is always the person at the server's shell, and a printed password
would end up in the journal of an internet-facing host. `serve` refuses to
start until `agendouille member add <name> --admin` has made one, and
that command prints a one-time invitation link instead of a password.
