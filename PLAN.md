# agendouille: implementation plan

> *agendouille* (French, made up): an *agenda* with the *-ouille* of
> *andouille* and *fripouille*. The family's calendar, not a serious one.

This is the full brief for the agent implementing agendouille. Read it end to
end before writing code.

- **Decided**: not up for debate. Most of these are the user's own answers.
- **Default**: may change for a concrete reason, written in `DECISIONS.md`.

`CONTRACTS.md` holds the interfaces between parts (Go packages, routes, the
CalDAV surface, JSON, SSE, the schema). When this file and `CONTRACTS.md`
disagree on a contract, `CONTRACTS.md` wins; fix the other one in the same
change.

Conventions (layout, build, CI, release, deployment, frontend style) are
taken from **cafard** (`~/misc/cafard`), the user's most recent Go +
TypeScript project. When this plan is silent on a convention, open cafard
and do what it does. Section 13 lists the files to copy from.

---

## 1. What it is

One self-hosted server that holds **a family's shared calendars**.

- Admins make calendars ("Famille", "École", "Sport") and decide, per
  member, who reads and who writes each one.
- Members add, change and delete events in the calendars they may write.
- Phones and desktops reach it over **CalDAV, read and write**: Android
  through DAVx5 (a sync adapter, so events appear in any calendar app),
  iPhone and macOS natively, Thunderbird natively.
- A **web UI** (TypeScript) shows and edits the same calendar from any
  browser.
- It runs at home and is reached **from the internet through a reverse
  proxy** that terminates TLS.

```
  Android calendar app ── CalendarContract ── DAVx5 ──┐
  iPhone / macOS Calendar ───────────────────────────┤ CalDAV (RFC 4791)
  Thunderbird ───────────────────────────────────────┤ + sync-collection (RFC 6578)
  a TV / Google Calendar (read-only .ics link) ──────┤
                                                     ▼
                         reverse proxy (TLS) ──► agendouille ──► SQLite
                                                     ▲
  browser (web UI, TypeScript) ── JSON API + SSE ────┘
```

### Typical uses

- **Add from the phone.** A parent adds "Dentist, Léa, Thursday 17:30" in
  the phone's calendar app. DAVx5 pushes it; the other phones have it at
  their next sync (minutes, or seconds with WebDAV-Push, M7).
- **Look at the week.** Someone opens the web UI on a laptop and sees the
  week, each calendar in its colour, and who added what.
- **School calendar.** An admin makes an "École" calendar that the parents
  write and the children only read.
- **Weekly activity.** "Judo, every Wednesday 14:00 until the end of June",
  with one Wednesday cancelled and one moved. Recurrence and its
  exceptions are kept as iCalendar has them, so every client shows the same
  thing.
- **Bring a new phone.** The member opens "Add a device" in the web UI,
  scans a QR code with DAVx5, and the calendar is on the phone. Losing the
  phone means revoking that device, not changing a password.
- **Migrate.** The family's existing Google Calendar is exported to `.ics`
  and imported once (`agendouille import`).

### Why build it

Radicale, Baïkal, Nextcloud and Stalwart already serve CalDAV. agendouille
exists because the user wants:

- a single Go binary with the web UI embedded, deployed like cafard;
- a family-first model (shared calendars, members, QR sign-in and QR device
  pairing) instead of a general groupware server;
- an admin interface that can stay off the DMZ while CalDAV and the family
  app face the internet (section 9.1);
- a web UI that is good on a phone, which Radicale and Baïkal do not have;
- no PHP or Python on the server, and members and the calendar managed in
  agendouille itself rather than through a server's generic admin pages
  (DECISIONS 2026-11-23, "our own server").

### Non-goals

- **Scheduling** (RFC 6638, iTIP, email invitations). `ATTENDEE` and
  `ORGANIZER` are stored as sent and shown, nothing is emailed.
- **Tasks and journals** (`VTODO`, `VJOURNAL`). The calendar accepts
  `VEVENT` only.
- **Contacts** (CardDAV).
- **Personal calendars.** Every calendar is shared; admins make them.
  Members cannot make calendars (Decided, user: roles are admin and member).
- **Subscribing to outside calendars** (school holidays from a URL) on the
  server: open question 4.
- **Multi-family hosting.** One install, one family.
- **Our own sync protocol.** Clients write through CalDAV (Decided, user:
  "if caldav supports write let's use it instead of reinventing our own
  protocol"). The JSON API exists for the web UI only.

---

## 2. Technology (Decided unless marked)

| Concern | Choice | Notes |
|---|---|---|
| Server language | Go, `go 1.26.0` in `go.mod` (as cafard) | |
| cgo | **Disabled everywhere** (`CGO_ENABLED=0`) | Cross-compiles to linux amd64/arm64/armv7 |
| Database | `modernc.org/sqlite` (pure Go), WAL, foreign keys on in the DSN | As cafard |
| iCalendar | `github.com/emersion/go-ical` (MIT) | Parse and write RFC 5545; keeps unknown properties |
| Recurrence | `github.com/teambition/rrule-go` (MIT) | Expands `RRULE`/`RDATE`/`EXDATE` for time-range queries and the web UI |
| CalDAV/WebDAV | **Written in-house**, `internal/dav` (Default) | `emersion/go-webdav`'s server has neither `sync-collection` nor `getctag`; see DECISIONS 2026-11-23 |
| Time zones | `time` + `time/tzdata` embedded | No dependency on the host's zoneinfo; VTIMEZONE in objects is kept but Go's database is used to compute instants (section 4.4) |
| Passwords | `golang.org/x/crypto/argon2` (argon2id) | Member passwords |
| QR codes | `rsc.io/qr` (as cafard) | Device pairing |
| Config | YAML, `gopkg.in/yaml.v3`, plus flags and env (as cafard) | |
| HTTP | `net/http` with Go 1.22+ patterns, no router library | Custom methods (`PROPFIND`, `REPORT`) go through one handler under `/dav/` |
| Own logging | `log/slog` | |
| Frontend | TypeScript (strict), **plain DOM in classes, no framework**, bundled by Vite into `internal/web/assets/static/app/main.js` and embedded by Go | As cafard |
| Frontend tests | `vitest` + `happy-dom` | As cafard |
| CSS | Hand-written, custom properties, Material 3 roles as cafard | |
| Test-only | `github.com/emersion/go-webdav` (client) | An independent CalDAV client in integration tests |

Ask in `DECISIONS.md` before adding any other dependency, on either side.

### 2.1 Protocols (Decided)

| Link | Protocol | Notes |
|---|---|---|
| Calendar clients | **CalDAV** (RFC 4791) on WebDAV (RFC 4918), read and write | |
| Incremental sync | **WebDAV sync** (RFC 6578, `sync-collection`), plus `CS:getctag` | DAVx5, iOS and Thunderbird all use one or the other |
| Discovery | `/.well-known/caldav` (RFC 6764), `current-user-principal` (RFC 5397) | |
| Data | **iCalendar** (RFC 5545), `text/calendar` | |
| Read-only subscription | A secret `.ics` URL (RFC 5545 over plain GET) | TVs, Google Calendar's "add by URL", dashboards |
| Instant sync (M7) | **WebDAV-Push** draft (bitfireAT) over Web Push (RFC 8030) | DAVx5 4.5+ with a UnifiedPush distributor |
| iOS setup | Apple configuration profile (`.mobileconfig`, CalDAV payload) | One tap instead of typing a URL |
| Android setup | DAVx5 login URI (`davx5://user:pass@host/path`) in a QR code | DAVx5 manual, "Integration" |
| Web UI | JSON over HTTP, **Server-Sent Events** for live changes | Same-origin, session cookie |
| Auth, CalDAV | HTTP Basic (RFC 7617) over TLS, **device passwords only** | Section 10 |
| Auth, web | Session cookie after a password sign-in | Section 10 |

---

## 3. Repository layout

```
agendouille/
  cmd/agendouille/main.go        subcommands: serve, config, version, member, device,
                                 import, export, backup
  internal/config/               YAML + env + flags, validation (`agendouille config`)
  internal/deps/                 the Deps struct (breaks import cycles; CONTRACTS 1.2)
  internal/dto/                  every JSON type of the web API and SSE
  internal/store/                SQLite handle, migrations, queries
  internal/members/              members, roles, password hashing, the last-admin rule
  internal/access/               calendars' metadata, the access table, the admin log
  internal/auth/                 sessions per surface, sign-in links, approvals, device
                                 passwords, Basic auth, rate limits,
                                 CSRF (Sec-Fetch-Site / Origin), trusted proxies
  internal/calendar/             the calendar: objects, change log, sync tokens,
                                 validation, the one write path (section 4.3)
  internal/ical/                 parse/patch/serialise helpers over go-ical, time-range
                                 and recurrence expansion, the web event <-> VEVENT mapping
  internal/dav/                  WebDAV + CalDAV server: PROPFIND, REPORT, GET, PUT,
                                 DELETE, OPTIONS, XML, preconditions, well-known
  internal/dav/testdata/clients/ recorded requests from DAVx5, iOS, Thunderbird
  internal/hub/                  in-memory fan-out of calendar changes (SSE, WebDAV-Push)
  internal/devices/              pairing: QR (DAVx5), .mobileconfig, subscription links
  internal/surface/              the three surfaces (dav, app, admin) and which listener
                                 serves each (section 9.1)
  internal/web/webx/             shared web plumbing: layouts, JSON errors, same-origin
  internal/web/app/              the family app: pages and /api/*
  internal/web/admin/            the admin UI: pages and /api/admin/*
  internal/web/sse/              GET /api/live
  internal/web/assets/           embed of templates and static
  internal/web/assets/static/app/    Vite output of src/app (app.js; ignored)
  internal/web/assets/static/admin/  Vite output of src/admin (admin.js; ignored)
  src/                           TypeScript sources (section 9)
  deploy/                        install.sh, agendouille.service, agendouille.openrc,
                                 agendouille-container.conf, agendouille.yaml.example,
                                 Caddyfile.example, nginx.conf.example, README.md
  scripts/remote.sh              rsync to dev-build, run checks there
  scripts/devdeploy.sh           build on dev-build, push the binary to the host
  .github/workflows/ci.yml       frontend -> artifact -> Go checks, cross-build, shellcheck
  .github/workflows/release.yml  tag v* -> binaries + SHA256SUMS + GitHub release
  package.json  tsconfig.json  vite.config.ts  vitest.config.ts
  go.mod  .gitignore  README.md  AGENTS.md  PLAN.md  CONTRACTS.md  DECISIONS.md
```

The Go module path is `github.com/nebuloss/agendouille` and the remote
`git@github.com:nebuloss/agendouille.git` (Default, as the other projects).

An Android app, if M8 happens, goes in `android/` in this repository
(Default), so the server and its client change in one commit.

---

## 4. Data model

### 4.1 Concepts

- **Member**: a person of the family. Has a login name (`[a-z][a-z0-9-]{0,31}`),
  a display name, a colour, and a role: **admin** or **member** (Decided,
  user). A password is **optional**: a member without one signs in by QR
  code only (section 8).
- **Calendar**: a shared calendar. Has a **slug** (`[a-z][a-z0-9-]{0,31}`,
  fixed once made, because it is in every phone's URL), a display name, a
  colour, a time zone, a description, and a **default access** given to
  members added later. Several per install (Decided, user). One,
  `famille`, is made at first start.
- **Access**: per member and calendar, `none`, `read` or `readwrite`. Set by
  admins. Admins have no implicit access to events: they grant it to
  themselves like to anyone (Default; the admin UI manages calendars, the
  family app shows events).
- **Device**: a phone, a laptop's Thunderbird, a TV. Belongs to a member.
  Has a label, a **device password** (random, shown once), and a cap of
  `readwrite` or `read`. On a calendar, a device gets the lower of its cap
  and its member's access. CalDAV accepts device passwords only, never a
  member's password. Revoking a device stops that device and nothing else.
- **Subscription link**: a secret URL that serves chosen calendars as one
  `.ics`, read-only, without a password. Belongs to a member, revocable,
  filtered by the member's access each time it is fetched.
- **Sign-in link**: a one-time URL, shown as a QR code, that signs a member
  in to the family app. Made by an admin (invitation, lost password) or by
  the member from a device already signed in (section 8).
- **Calendar object**: one CalDAV resource, `<name>.ics`, in one calendar,
  holding one iCalendar `VCALENDAR` with all the `VEVENT`s of one `UID`
  (the master and its overrides by `RECURRENCE-ID`), as RFC 4791 4.1
  requires. The original text is stored, so what a client sent is what a
  client gets back, including properties agendouille does not understand
  (`X-APPLE-*`, `VALARM`, attachments, attendees).
- **Change**: one row per write (put or delete) with a strictly increasing
  number across all calendars. Each calendar's sync token and ctag are made
  from the last change in it (section 4.3). Changes also feed the admin's
  activity page (who, when, through which device, which event).

### 4.2 Who did what

Every object records `created_by` and `modified_by` (member ids) and the
device or session that did it. The web UI colours an event by its calendar,
marks it with its creator's initial in the creator's colour, and shows
"added by Maman, changed by Léa". These live in columns, **not**
in the iCalendar text: CalDAV clients would rewrite or drop them, and they
must not travel to other servers when someone exports.

### 4.3 The write path (Decided)

There is **one** write path, `calendar.Service.Put` / `Delete`
(CONTRACTS 1.3). CalDAV `PUT`/`DELETE`, the web API, `agendouille import`
and anything later all go through it. In one SQLite transaction it:

1. checks the precondition (`If-Match` etag, `If-None-Match: *`), and
   answers `412` if it fails;
2. parses and validates the object (section 4.5);
3. checks `UID` uniqueness in the calendar (`no-uid-conflict`);
4. extracts the index columns (summary, first start, last end or open,
   recurring or not, all-day);
5. writes the object with a new etag, appends a `changes` row, bumps the
   calendar's change counter;
6. after commit, publishes the change on the hub (SSE, WebDAV-Push).

The **etag** is the hex SHA-256 of the stored text, truncated to 16 bytes,
in quotes. The change counter is global; each calendar keeps the number of
its last change. The **sync token** is
`https://<public host>/ns/sync/<slug>/<n>` with `<n>` that number; the
**ctag** is `<n>` as a string.

Losing access to a calendar makes it disappear from the member's home (the
client drops it); getting access makes it appear. A change of access is
not a change of the calendar, so it does not move its token.

Deleted objects leave their `changes` row (the tombstone) so
`sync-collection` can report them. Tombstones older than
`sync_token_retention` (Default 180 days) are pruned; a sync token older
than the oldest kept change answers `403 DAV:valid-sync-token`, and the
client does a full sync (RFC 6578 3.2).

### 4.4 Time

- Instants are computed with Go's embedded tz database by `TZID`. A
  `VTIMEZONE` in the object is kept in the text but only consulted when its
  `TZID` is unknown to Go (Outlook's "Romance Standard Time", for one): then
  the Windows-name map from CLDR is tried, and after that the calendar's
  zone is assumed and a warning logged.
- Floating times (no `TZID`, no `Z`) are read in the calendar's time zone.
- All-day events (`VALUE=DATE`) are dates, not instants; the index stores
  them as midnight-to-midnight in the calendar's zone.
- Each calendar has a zone (Default for new ones: `timezone` in the config,
  `Europe/Paris`). The web UI shows times in the **family zone** (the
  config's), not the browser's (Default: one family, one place; a member
  abroad still sees the family's time).

### 4.5 What is accepted

- One `VCALENDAR` with one or more `VEVENT`s that share a `UID`, plus any
  `VTIMEZONE`s. `VTODO` and `VJOURNAL`: `403 CALDAV:supported-calendar-component`.
- Size ≤ `max_object_bytes` (Default 256 KiB): `403 CALDAV:max-resource-size`.
- Must parse; must have `UID`, `DTSTART`; `DTEND` after `DTSTART` if
  present: otherwise `403 CALDAV:valid-calendar-data`.
- `RRULE` with neither `COUNT` nor `UNTIL` is fine (stored as open-ended).
  Expansion is capped at `max_instances` (Default 5000) per query and
  answers what it has.
- **No rewriting** of client data on CalDAV `PUT`: the text is stored as
  received (after the size and validity checks). Clients rely on getting
  their bytes back; DAVx5 and iOS both compare.

### 4.6 Edits from the web UI

The web API takes a simple event shape (CONTRACTS 3.3), not iCalendar.
`internal/ical` maps it onto the stored object by **patching**: it parses
the object, changes only the properties the web form owns (`SUMMARY`,
`DTSTART`, `DTEND`, `LOCATION`, `DESCRIPTION`, `RRULE`, `EXDATE`), bumps
`SEQUENCE`, sets `DTSTAMP` and `LAST-MODIFIED`, and serialises it. Alarms,
attendees and `X-` properties set from a phone survive an edit in the
browser.

Recurring events from the web UI take a **scope**:

| Scope | Effect |
|---|---|
| `this` | Change: add or replace an override `VEVENT` with `RECURRENCE-ID`. Delete: add an `EXDATE` (and drop the override if any). |
| `following` | Split: set `UNTIL` on the master to just before this occurrence; create a new object (new `UID`) from this occurrence on, with the overrides that fall after it. |
| `all` | Change or delete the master (and all its overrides). |

This is what Google Calendar and iOS do, so the result looks the same on
every client.

### 4.7 Alarms (Default, open question 1)

A shared calendar means a `VALARM` set by one member's phone reaches
everyone's phone. v1 keeps `VALARM` as sent (the iCalendar way, and what
Nextcloud does for a shared calendar). Clients add their own default
reminders locally anyway.

---

## 5. CalDAV (the main interface)

Full contract in CONTRACTS section 4. The shape:

### 5.1 URLs

```
/.well-known/caldav                 301 -> /dav/
/dav/                               PROPFIND: current-user-principal
/dav/principals/<member>/           the principal
/dav/calendars/<member>/            calendar-home-set of <member>: the calendars they may read
/dav/calendars/<member>/<slug>/     a calendar (the same one under every home)
/dav/calendars/<member>/<slug>/<name>.ics   an object
/ics/<secret>.ics                   read-only subscription
```

Each member has their own home, listing the calendars their access allows.
The objects are the same in every home; only the path prefix differs. A
member reaching into another member's home, or into a calendar they have
`none` on, gets `404`, not `403`.

Object names are what the client chose on `PUT` (DAVx5 uses `<uuid>.ics`,
iOS `<UID>.ics`); the web UI creates `<uuid>.ics`. They are kept as sent,
unique in their calendar, `[A-Za-z0-9._@+-]{1,200}\.ics` (Default; others
`403`). Moving an event to another calendar is a `DELETE` and a `PUT`, as
clients already do it; the web UI does the same in one transaction.

### 5.2 Methods

| Method | Where | Notes |
|---|---|---|
| `OPTIONS` | anywhere under `/dav/` | `DAV: 1, 3, calendar-access`, `Allow` list |
| `PROPFIND` | everything | Depth 0 and 1. `Depth: infinity` is `403 DAV:propfind-finite-depth` |
| `REPORT calendar-multiget` | calendar | |
| `REPORT calendar-query` | calendar | `comp-filter` VEVENT with optional `time-range`; `prop-filter`/`text-match` on `UID` and `SUMMARY`. Others: `403 CALDAV:supported-filter` |
| `REPORT sync-collection` | calendar | RFC 6578, depth 1, `limit` honoured with `507` + a new token |
| `GET`, `HEAD` | object | `ETag`, `Content-Type: text/calendar; charset=utf-8` |
| `GET` | calendar | the whole calendar as one `.ics` (export) |
| `PUT` | object | `If-Match`, `If-None-Match: *`. `201` or `204`, with `ETag` |
| `DELETE` | object | `If-Match` honoured. `204` |
| `PROPPATCH` | calendar | `displayname`, `calendar-color`, `calendar-order` stored **per member** (each one's view); others `403` in the multistatus |
| `MKCALENDAR`, `MKCOL`, `MOVE`, `COPY`, `LOCK` | | `403` / `405`. Calendars are made in the admin UI |

`calendar-query` with a `time-range` uses the index columns to pick
candidates, then expands recurrence in Go to decide (RFC 4791 9.9).
`<C:expand>` and `<C:limit-recurrence-set>` in `calendar-data` are not
supported (Default; no target client sends them); the full object is
returned.

### 5.3 Access over CalDAV

On each calendar the principal gets the lower of the device's cap and the
member's access. With `read`, `PUT` and `DELETE` get `403
DAV:need-privileges` and `current-user-privilege-set` lists `read` only, so
clients show that calendar as read-only. The children can read "École"
while the parents write it, and the user's first idea (read-only CalDAV for
some devices) is still there as a device cap.

### 5.4 Clients to work with (Decided)

The target list, each tested by hand at the end of M2 and M5 (section 15):

| Client | Discovery | Sync | Notes |
|---|---|---|---|
| DAVx5 (Android) | well-known, principal | `sync-collection`, falls back to ctag | Sync adapter into CalendarContract; any calendar app reads and writes it |
| iOS / macOS Calendar | well-known, principal | ctag + etag PROPFIND, `sync-collection` | Picky about `current-user-privilege-set`, `supported-report-set`, `calendar-color` |
| Thunderbird | direct calendar URL or well-known | ctag, etag | |
| GNOME Calendar / Evolution | well-known | ctag | Nice to have |

Captured request bodies from each go in `internal/dav/testdata/clients/`
and are replayed by tests.

---

## 6. Android (Decided: a sync adapter; Default: DAVx5 first)

The user wants a sync adapter on Android and does not want to build
Kotlin/Java. CalDAV read-write (2.1) makes these one answer: **DAVx5**
(GPLv3, on F-Droid and Play) *is* a CalDAV sync adapter. It registers an
account and syncs into Android's `CalendarContract`, so the system calendar,
Google Calendar, Etar, Fossify Calendar or any other app reads and writes
the family's calendars. No APK is built here.

What agendouille adds on top:

- **QR pairing (M5).** "Add a device" makes a device password and shows a
  QR code with `davx5://<member>:<device-password>@<host>/dav/` (the DAVx5
  manual documents this URI). Scanning it opens DAVx5's login with
  everything filled in.
- **Instant sync (M7).** DAVx5 4.5+ supports the WebDAV-Push draft over
  UnifiedPush. agendouille implements the server side, so a change made on
  one phone reaches the others in seconds instead of at the next periodic
  sync.

**M8, a custom app, is optional and decided after M5 is in use.** It would
only add what DAVx5 lacks: one install instead of two, family-specific
notifications ("Papa added Judo"). If it is built:

- Kotlin is unavoidable for a sync adapter (`AbstractThreadedSyncAdapter`,
  `AccountAuthenticator`); the code stays small, and the CalDAV side uses
  `dav4jvm` and `ical4j` (the libraries DAVx5 itself uses).
- It is **never built on the editing machine and never needs Android
  Studio**: a Gradle wrapper builds it in a container on dev-build
  (`rtk docker run ... ./gradlew assembleRelease`) and in GitHub Actions,
  which signs it and attaches the APK to the release.
- It speaks CalDAV, like any other client. No private protocol.

---

## 7. iPhone, macOS, Thunderbird, read-only links

- **iOS / macOS**: "Add a device" offers a `.mobileconfig` profile with a
  CalDAV payload (host, port 443, principal URL, member name, device
  password). Unsigned (Default), so iOS says "unverified", which is fine for
  a family. The page also lists the manual settings.
- **Thunderbird**: the page shows the calendar URL, the member name and the
  device password, and says which menu to use.
- **Read-only subscription links**: `/ics/<secret>.ics`, the chosen
  calendars as one `.ics` with `REFRESH-INTERVAL` and `X-PUBLISHED-TTL` of 15 minutes.
  The secret is 32 random bytes in base64url. Google Calendar polls these
  links slowly (hours), which the page says.

---

## 8. Members, sign-in, first start

### 8.1 Signing in (Decided, user: "qrcode login")

There is no email. A member signs in to the family app in one of three
ways:

1. **A sign-in link from an admin.** The admin UI's "Sign-in QR" on a
   member makes a one-time link, shown as a QR code (and as text to send by
   message). Scanning it on the member's phone signs that browser in. This
   is both the **invitation** of a new member and the **lost password**
   path. Valid 72 hours (Default), single use, revocable, listed on the
   member's admin page until used.
2. **A QR from a device already signed in.** `/signin` on a new device (a
   laptop) shows a QR code; the member scans it with a phone that is signed
   in to the family app, which asks "Sign in *Firefox on Linux* as Léa?";
   on yes, the laptop is signed in. The code lives 2 minutes (Default). The
   same exists the other way round from `/me` ("Sign in another device"),
   for a phone that cannot scan.
3. **A password**, if the member chose one (on `/me`, after a QR sign-in).
   Optional (Default): a child can live without one.

Using a sign-in link from an admin does not touch the member's password or
devices. A member who forgot their password signs in by QR and sets a new
one, or removes it.

### 8.2 The admin

The admin UI has its own sign-in (`/admin/signin`), with its own session
cookie, on whichever listener serves it (9.1). Admins sign in there with
their **password**, which every admin must have, and nothing else
(Default). No QR approval there: a family-app session on a phone that
faces the internet must never be enough to open the admin UI.

### 8.3 Lifecycle

- **Add** (admin UI or `agendouille member add <name> [--admin]`): name,
  display name, colour, role, and the access on each calendar (pre-filled
  from each calendar's default access). Shows the first sign-in QR.
- **Disable / enable**: a disabled member's sessions, devices and links stop
  working until enabled; nothing is deleted.
- **Remove**: keeps their events (the calendars are shared) and shows them
  as "former member"; revokes devices, sessions and links. The name stays
  taken.
- **Make admin / remove admin**: the last admin cannot be demoted, disabled
  or removed.
- `agendouille member passwd <name>` sets a password from a shell on the
  server, no old password needed, and `agendouille member signin <name>`
  prints a sign-in link: the recovery paths when no admin can sign in.

### 8.4 First start

With no member at all, `serve` refuses to start on any address and says to
run `agendouille member add <you> --admin`, which asks for a password (the
admin needs one, 8.2) and makes the `famille` calendar with read-write
access for that admin.

## 9. Web frontend

### 9.1 Two UIs and three surfaces (Decided, user: the DMZ split)

CalDAV faces the internet from a DMZ, and the admin UI must be able to stay
off it. agendouille serves three **surfaces**, and each listener in the
config (section 11) serves some of them:

| Surface | What | Typically |
|---|---|---|
| `dav` | `/.well-known/caldav`, `/dav/`, `/ics/` | DMZ, through the proxy |
| `app` | the **family app**: calendar, my devices, me, sign-in, `/api/*`, `/api/live`, `static/app/` | DMZ, through the proxy |
| `admin` | the **admin UI**: `/admin/*`, `/api/admin/*`, `static/admin/` | LAN only |

- A surface is served by **exactly one** listener (its absolute URLs come
  from that listener's `public_url`). Paths of a surface that a listener
  does not serve answer `404`, so the proxy can forward a whole port without
  path rules.
- With one listener (the default, `127.0.0.1:8791`) all three are on it.
  `serve` logs a warning when `admin` shares a non-loopback listener with
  `dav` (Default: allowed, for a LAN-only install).
- **Two bundles.** Vite builds `src/app/main.ts` into `static/app/app.js` and
  `src/admin/main.ts` into `static/admin/admin.js`, sharing `src/shared/`.
  The admin code is never served on the DMZ.
- **Separate sessions.** The family app's cookie is `agd_app`, the admin
  UI's `agd_admin`, each bound to its surface and checked against its
  listener's origin. Being signed in to one never signs in to the other.
- The admin UI needs the family app's and CalDAV's public URLs (for QR
  codes and profiles); it reads them from the config, not from the request.

### 9.2 Principles (Decided, as cafard)

- Plain TypeScript classes on the DOM, no framework. No HTML sinks
  (`innerHTML` and friends) in `src/`; the CI grep from cafard
  (`scripts/check-sinks.sh`) runs here too. Event titles come from every
  family member's phone, so all text goes through `textContent`.
- Go serves an HTML shell per page (`layout_app.html`, `layout_admin.html`);
  the bundle renders the rest. Sign-in, choosing a password and the admin's
  forms work without the bundle; the calendar needs it.
- The family app is phone first. The admin UI is desktop first but usable
  on a phone.

### 9.3 The family app (surface `app`, every member)

| Path | Page |
|---|---|
| `/signin` | Password form, and the QR to scan from a signed-in phone (8.1) |
| `/login/<token>` | Landing of a sign-in link: confirm, then signed in |
| `/approve/<code>` | On the signed-in phone: approve another device's sign-in |
| `/` | The calendar: agenda (phone default), week, month |
| `/devices` | My devices and subscription links; add one (QR, profile, manual settings); revoke |
| `/me` | Display name, colour, password (set, change, remove), my sessions (sign out others), "sign in another device" |

What a member does **not** see: other members' devices, sessions or links;
calendars they have `none` on; anything about access rules beyond "read
only" on a calendar they cannot write.

### 9.4 The calendar views (Default)

- **Calendars list** (side panel on wide screens, a sheet on phones): every
  calendar the member may read, with its colour, a show/hide switch (kept
  per member, on the server) and a lock on read-only ones.
- **Agenda**: a scrolling list of days with their events, today on top,
  infinite scroll both ways. Default on narrow screens.
- **Week**: seven columns, an all-day strip, hours on the side, the current
  time line. Default on wide screens.
- **Month**: a grid with up to three events per day and "+n".
- Event colour: **its calendar's**; the creator's initial in the creator's
  colour on the event.
- Tap an event: a detail sheet (calendar, when, where, notes, recurrence in
  words, "added by", "changed by"), with Edit and Delete if the calendar is
  writable.
- Tap an empty slot (week) or a day (month, agenda's "+"): the editor.
- **Editor**: calendar (writable ones only; the last used is preselected),
  title, all-day, start, end, location, notes, repeat (none, daily, weekly
  on chosen days, monthly by date, yearly; until a date or a count).
  Recurring events ask for the scope (4.6) on save and delete. Changing the
  calendar moves the event (all occurrences).
- **Live**: the bundle listens on `/api/live` (SSE). A `changed` event on a
  shown calendar reloads the visible range; an `access` event reloads the
  calendars list.
- **Conflicts**: saves send the etag they started from; a `412` reloads the
  event and says "changed meanwhile by <member>", and keeps the form.

### 9.5 The admin UI (surface `admin`, admins only)

| Path | Page |
|---|---|
| `/admin/signin` | Password only (8.2) |
| `/admin` | Overview: members, calendars, devices that have not synced for a week, last backup, version, warnings (an admin without a password, the admin surface on the DMZ) |
| `/admin/members` | The list: name, role, calendars, devices, last seen; "Add member" |
| `/admin/members/<name>` | Profile and role; **access on each calendar** (none / read / read-write); devices (revoke); sessions (end); sign-in links (make a QR, revoke); disable, enable, remove |
| `/admin/calendars` | The list: colour, name, slug, events, members with access; "New calendar" |
| `/admin/calendars/<slug>` | Name, colour, zone, description, default access; **the access of every member** on it; import an `.ics`; export; archive (hidden from every client, kept) and delete (after archive, typing the slug) |
| `/admin/activity` | Every change, newest first: when, who, through which device or the web, calendar, event title, created / changed / deleted. Filter by member, calendar, device |
| `/admin/system` | Backups (list, download, back up now), the effective listeners and URLs, version |

The two access editors (by member, by calendar) are two views of the same
table, so the admin can think "what can Léa do" or "who sees École".
Every admin action is in the activity log too (`admin` kind rows).

### 9.6 TypeScript layout (Default, as cafard)

```
src/
  shared/
    core/          Api (fetch, JSON, errors, etag), Live (SSE, reconnect),
                   Clock, Dates (zone-aware arithmetic in the family zone)
    render/        DOM helpers (h(), text(); no HTML strings)
    components/    Dialog, Sheet, QrPanel, ColorPicker, MemberChip, Toast
    model/         Member, Calendar, Access
  app/
    main.ts        boot: read the page id, mount the page class
    pages/         CalendarPage, DevicesPage, MePage, SigninPage, ApprovePage
    components/    CalendarList, AgendaView, WeekView, MonthView, EventSheet,
                   EventEditor, RecurrencePicker, ScopeDialog
    model/         Event, Occurrence, Range, Recurrence (rule <-> words)
  admin/
    main.ts
    pages/         OverviewPage, MembersPage, MemberPage, CalendarsPage,
                   CalendarPage, ActivityPage, SystemPage
    components/    AccessMatrix, ActivityTable, SigninLinkPanel
```

`src/app` and `src/admin` may import `src/shared`, never each other (a
vitest test walks the imports). Time arithmetic in the browser is in the
family zone; `Intl` gives offsets for a named zone and `shared/core/Dates`
wraps that (no date library, Default). The server sends occurrences already
expanded (CONTRACTS 3.2), so the browser never expands recurrence; the
`Recurrence` model only turns a rule into words and the editor's fields
back into a rule.

### 9.7 Look (Default)

Material 3 roles and the dark block from cafard's `app.css`, shared by both
UIs; the admin UI has a distinct top bar colour so nobody mistakes it for
the family app. Member and calendar colours are picked from a fixed palette
of twelve that read in light and dark (cafard's `scripts/palette.py`
checks contrast; reuse it).

## 10. Security (Decided: reachable from the internet)

The server sits behind a reverse proxy on the internet, holds a family's
whereabouts, and has children among its users. Everything here is in v1.

### Passwords, sign-in links, devices

- Member passwords: argon2id (time 2, memory 64 MiB, threads 1; Default),
  at least 10 characters, checked against a short embedded list of common
  passwords. Optional for members, required for admins (8.2). Used only by
  `/signin` and `/admin/signin`.
- Sign-in links: 32 random bytes in base64url, stored hashed, single use,
  72 hours. Approval codes (8.1.2): 32 random bytes, 2 minutes, family app
  only, and the approving phone is shown the new device's user agent and IP
  before it says yes.
- Device passwords: 20 random bytes, shown once as `agd_` + base32 in
  groups (`agd_xxxx-xxxx-...`), easy to type into Thunderbird. Stored as
  SHA-256 (enough for a 160-bit random secret; no argon2 on every CalDAV
  request). `last_used_at` and `last_ip` are kept and shown.
- CalDAV accepts `Basic <member>:<device password>` only. A member's
  password on CalDAV is refused with a log line that says to add a device.

### Sessions and CSRF

- Cookies `agd_app` and `agd_admin` (9.1): 32 random bytes, stored hashed
  with their surface, `HttpOnly`, `Secure` when the listener's `public_url`
  is https, `SameSite=Lax`. The app's last 30 days sliding, the admin's 12
  hours (Default). Changing or removing a password ends the member's other
  sessions.
- State-changing web requests need `Sec-Fetch-Site: same-origin` or an
  `Origin` equal to the listener's `public_url` (cafard's check). `/dav/`
  is exempt (Basic auth, no cookies are accepted there).

### Rate limits and lockout

- Failed sign-ins and failed Basic auths are counted per client IP and per
  member name: 10 per 15 minutes, then `429` with `Retry-After` (Default).
  A CalDAV client that keeps retrying a revoked password does not lock out
  the member's web sign-in: device failures and web failures are counted
  apart.
- The client IP is the TCP peer, or the last hop of `X-Forwarded-For` when
  the peer is in `trusted_proxies`. With no trusted proxies, forwarded
  headers are ignored.

### Exposure guard

- `serve` refuses a non-loopback listener without `public_url`, and a
  `public_url` that is not `https://` unless that listener has
  `allow_insecure: true` (for a LAN-only admin listener).
- Each surface takes its absolute URLs (DAV hrefs, sync token namespace, QR
  codes, profiles) from the `public_url` of the listener that serves it.

### Other rules

- Responses carry `Content-Security-Policy: default-src 'self'` (no inline
  script), `X-Content-Type-Options: nosniff`, `Referrer-Policy: same-origin`,
  `frame-ancestors 'none'`.
- Subscription links and invitation tokens are compared in constant time
  and logged by id, never by value. Access logs redact `/ics/<secret>`.
- XML bodies on `/dav/` are capped (Default 1 MiB) and parsed with
  `encoding/xml`, which does not resolve external entities.

---

## 11. Configuration

YAML at `/etc/agendouille.yaml` (or `-config`, or `AGENDOUILLE_CONFIG`);
scalar keys also as flags and environment variables, file < env < flag,
lists are file-only, unknown keys are errors (as cafard).

| Key | Default | Notes |
|---|---|---|
| `data_dir` | `/var/lib/agendouille` | `agendouille.db`, `backups/` |
| `listeners` | one: `127.0.0.1:8791`, all surfaces | File only; see below |
| `trusted_proxies` | `[]` | CIDRs whose `X-Forwarded-For` is believed (file only) |
| `timezone` | `Europe/Paris` | The family zone; default zone of new calendars |
| `log_level` | `info` | |
| `max_object_bytes` | `262144` | |
| `sync_token_retention` | `180d` | |
| `backup_keep` | `14` | Daily snapshots kept (section 12.5) |

The DMZ layout:

```yaml
listeners:
  - listen: 0.0.0.0:8791             # the proxy in the DMZ forwards here
    public_url: https://agenda.example.org
    serve: [dav, app]
  - listen: 10.0.50.30:8792          # LAN only, never forwarded
    public_url: http://agenda.lan:8792
    allow_insecure: true
    serve: [admin]
```

`http_listen` and `public_url` as flags or environment variables
(`AGENDOUILLE_HTTP_LISTEN`, `AGENDOUILLE_PUBLIC_URL`) set a single listener
serving all surfaces, for tests and quick starts; with `listeners` in the
file they are an error.

## 12. Build, CI, release, deployment (Decided: as cafard)

### 12.1 Where things run

- The editing machine (dev-code) edits, runs `npm` checks, commits and
  pushes. **It never builds Go binaries or Android apps.**
- dev-build (`guillaume@10.0.50.21`) runs `go build`, `go vet`,
  `go test -race`, through `scripts/remote.sh` and `rtk`.
- CI builds the releases on a `v*` tag.

### 12.2 `scripts/remote.sh`

Copied from cafard, `REMOTE_DIR=agendouille`. With no argument: `gofmt -l`,
`go build ./...`, `go vet ./...`, `go test -race ./...`.

### 12.3 CI and release

cafard's two workflows, minus the Windows targets: linux amd64, arm64,
armv7.

### 12.4 Deployment (`deploy/`)

cafard's `install.sh`, hardened systemd unit, OpenRC script and LXC
drop-in, renamed. Plus:

- `Caddyfile.example` and `nginx.conf.example`. The nginx one must pass
  `PROPFIND`, `REPORT`, `PROPPATCH`, `OPTIONS` and the `Depth`, `If-Match`,
  `If-None-Match`, `Authorization` headers, must not buffer `/api/live`,
  and must answer `/.well-known/caldav` on the bare host. Both forward
  only the DMZ listener's port; the examples say so in a comment.
- `deploy/README.md`: proxy, DNS, first member, backups, upgrading.

### 12.5 Backups

`serve` writes `VACUUM INTO data_dir/backups/agendouille-YYYY-MM-DD.db`
once a day and keeps `backup_keep` of them. `agendouille backup <file>` does
one now; `agendouille export <calendar> > famille.ics` writes a calendar as one
`.ics` that any calendar program imports.

### 12.6 `scripts/devdeploy.sh`

cafard's: build on dev-build, copy the binary to the host that runs it,
restart the service.

---

## 13. Reference files to copy from

| Need | Copy from |
|---|---|
| `remote.sh`, `rsync-excludes`, `devdeploy.sh` | `~/misc/cafard/scripts/` |
| `install.sh`, unit, OpenRC, container drop-in | `~/misc/cafard/deploy/` |
| CI, release | `~/misc/cafard/.github/workflows/` (drop Windows) |
| Vite, tsconfig, vitest, embed placeholder, `bundleBuilt` | `~/misc/cafard/{vite,vitest}.config.ts`, `tsconfig.json`, `internal/web/assets/` |
| `main.go` shape, config loading (file < env < flag, unknown keys fail) | `~/misc/cafard/cmd/cafard/`, `internal/config/` |
| Migrations runner | `~/misc/cafard/internal/store/` |
| Same-origin check, rate limiter | `~/misc/cafard/internal/auth/`, `internal/ratelimit/` |
| SSE | `~/misc/cafard/internal/web/` (its SSE handler), `src/core/` |
| TS class style, `render/` helpers, sinks check | `~/misc/cafard/src/`, `scripts/check-sinks.sh` |
| CSS (roles, dark block), palette check | `~/misc/cafard/internal/web/assets/static/app.css`, `scripts/palette.py` |
| AGENTS.md tone | `~/misc/cafard/AGENTS.md` |

---

## 14. Milestones

Each milestone ends with code, tests, an updated `README.md` and
`AGENTS.md`, and its "Done when" check passing on dev-build.

**M0. Skeleton and pipeline**
- `go mod init github.com/nebuloss/agendouille`; `cmd/agendouille` with
  `serve`, `config`, `version`; config loading; slog; `/healthz`; graceful
  shutdown; the exposure guard.
- `package.json`, `tsconfig.json`, Vite and vitest configs, `src/main.ts`,
  `App.ts`, `core/` with tests, the embed placeholder, `bundleBuilt`.
- `scripts/remote.sh`, CI and release workflows, `.gitignore`.
- *Done when:* `scripts/remote.sh` passes, `npm run typecheck && npm test &&
  npm run build` passes, `agendouille version` prints `dev`.

**M1. Store, members, sign-in, surfaces**
- Store and migrations (CONTRACTS 5), members, argon2id, calendars and
  access (the `famille` calendar), sessions per surface, sign-in links,
  approval codes, device passwords, Basic auth, rate limits, trusted
  proxies, same-origin check.
- Listeners and surfaces (9.1): `listeners` config, `404` for surfaces a
  listener does not serve, the warnings and the exposure guard.
- `member add|passwd|signin|list|disable|enable|remove`,
  `calendar add|list|access`, `device add|list|revoke` (shell fallbacks).
- `/signin` (password), `/login/<token>`, `/admin/signin`, sign-out; the
  "no member" start refusal.
- *Done when:* a test signs in with a sign-in link, which then fails a
  second time; locks out after 10 failed passwords; a device password works
  on a protected test route while a member's password does not; with two
  listeners, `/admin/` is `404` on the first and `/dav/` is `404` on the
  second; an `agd_app` cookie does not open `/admin`.

**M2. CalDAV, read and write**
- `internal/ical` (parse, validate, index columns, expansion), `internal/
  calendar` (the write path, changes, tokens, access), `internal/dav`
  (section 5, CONTRACTS 4), the hub.
- `agendouille import <calendar> <file.ics>` (one object per UID, skips UIDs
  that exist unless `--replace`) and `export <calendar>`.
- *Done when:* the go-webdav client integration test creates, lists,
  queries by time range (with a weekly rule and an `EXDATE`), updates with
  `If-Match`, gets `412` with a stale etag, deletes, and follows the
  changes through `sync-collection`, on two calendars, one of them
  read-only for the member; every recorded client request in
  `testdata/clients/` gets the recorded kind of answer; **and by hand**:
  DAVx5, iOS and Thunderbird each add, edit and delete an event that the
  other two see, and show the read-only calendar as read-only.

**M3. Admin UI**
- `/api/admin/*` (CONTRACTS 3.5), the `admin` bundle, every page of 9.5,
  the activity log, backups from the UI.
- *Done when:* from the admin UI alone, on the LAN listener, an admin makes
  a calendar, adds a member with read on it, hands them a sign-in QR that a
  phone scans to get in, and sees each of those steps in the activity log;
  a vitest test proves `src/admin` and `src/app` do not import each other.

**M4. Family app**
- `/api/*` (CONTRACTS 3.2 to 3.4), SSE, the `app` bundle: the calendar page
  and its views, the calendars list, the sheet and the editor with
  recurrence scopes and the calendar picker, `/me` with passwords and the
  "sign in another device" QR, `/approve/<code>`.
- *Done when:* the vitest suite covers `Dates` across DST changes,
  `Recurrence` both ways and each view's renderer; a Go test drives the
  API through every scope on a recurring event, moves an event between
  calendars, and checks the stored iCalendar keeps a `VALARM` and an `X-`
  property it did not set; an event made in the browser appears on a
  DAVx5 phone, and one made on the phone appears in an open browser
  without reload; a laptop is signed in by scanning its `/signin` QR with
  a signed-in phone.

**M5. Devices**
- `/devices`: add (label, read or read-write cap) with the DAVx5 QR, the
  `.mobileconfig`, the manual settings; subscription links over chosen
  calendars; revoke; last used. The admin's view of every device (M3)
  gains last sync and client.
- *Done when:* a phone is added by scanning the QR alone; an iPhone by the
  profile alone; a `read` device cannot write (and its client shows every
  calendar read-only); a revoked device gets `401` at its next sync.

**M6. Deployment**
- `deploy/` (12.4), backups (12.5), `devdeploy.sh`, the proxy examples,
  `deploy/README.md`.
- *Done when:* `install.sh` installs and upgrades on Debian and Alpine
  (as cafard's test), and the user's host serves the family through its
  proxy.

**M7. Instant sync (optional, Default: yes)**
- WebDAV-Push server side: the push properties on the calendar, the
  subscription registration, Web Push (VAPID) delivery on each change from
  the hub. Read the current draft before starting; it is still moving.
- *Done when:* with ntfy as UnifiedPush distributor, a change from the web
  UI reaches a DAVx5 phone in under ten seconds.

**M8. Own Android app (optional, decided after M5 is in use)**
- Section 6. A separate plan section is written first, with the user.

---

## 15. Testing

### Go

- **Table tests on `internal/ical`**: recurrence expansion against known
  outputs (weekly with `BYDAY`, monthly `BYMONTHDAY=31`, yearly 29 Feb,
  `EXDATE`, overrides moving an instance in and out of a range, DST
  transitions in `Europe/Paris`, all-day events, floating times, unknown
  `TZID` with a `VTIMEZONE`).
- **`internal/dav` against recorded requests**: each file in
  `testdata/clients/<client>/<step>.http` is a request as captured; its
  `.want` names the status and the hrefs/props expected. New clients and
  new bugs add a file.
- **End to end** with `go-webdav`'s client against a test server over
  `httptest`, through the real auth.
- **Write path**: concurrent puts on one object (one wins, the other gets
  `412`), sync tokens strictly increase, tombstones appear in
  `sync-collection`, pruning produces `valid-sync-token`.
- **Security**: CSRF refusal, lockout split between device and web,
  `X-Forwarded-For` ignored from untrusted peers, a member cannot reach
  another member's home or a calendar they have `none` on, a `read` device
  or `read` access cannot write, surfaces answer `404` off their listener,
  sessions do not cross surfaces, sign-in links are single use, the last
  admin cannot be demoted.

### TypeScript

vitest for `shared/core/Dates`, `app/model/Recurrence`, `shared/core/Api`
(etag, 412), the renderers under happy-dom (both bundles), and the import
rule between `src/app` and `src/admin`, as cafard.

### By hand, per milestone

A checklist in `deploy/README.md` (M2, M5): the three target clients, each
step done and seen on the other two.

---

## 16. Open questions (ask the user, do not guess)

1. **Alarms.** Keep a `VALARM` set by one phone for everyone (v1 default),
   or strip alarms so each person sets their own reminders locally?
2. **"Who is it for."** Should an event say which member(s) it concerns
   ("Dentist, for Léa") beyond its calendar? The interoperable way is
   `CATEGORIES` with member names (Thunderbird and DAVx5 show it).
3. **Host.** Which machine and container runs it, and which domain name
   does the proxy serve? Decides `public_url` and `devdeploy.sh`.
4. **Holidays and birthdays.** Should the server subscribe a calendar to an
   outside `.ics` URL (French school holidays for the right zone, public
   holidays), read-only for everyone? v1 does not; an admin can import an
   `.ics` into a calendar by hand.
5. **M8.** Once the family has used DAVx5 for a while, is a custom Android
   app still wanted, and for what?

Until they are answered, implement with the defaults written here.
