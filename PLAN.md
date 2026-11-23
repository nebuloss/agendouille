# agendouille: implementation plan

> *agendouille* (French, made up): an *agenda* with the *-ouille* of
> *andouille* and *fripouille*. The family's calendars, not serious ones.

This is the full brief for the agent implementing agendouille. Read it end to
end before writing code.

- **Decided**: not up for debate. Most of these are the user's own answers
  (`DECISIONS.md` quotes them).
- **Default**: may change for a concrete reason, written in `DECISIONS.md`.

`CONTRACTS.md` holds the interfaces between parts (Go packages, routes, the
CalDAV surface, JSON, the app bridge, the schema). When this file and
`CONTRACTS.md` disagree on a contract, `CONTRACTS.md` wins; fix the other one
in the same change.

Conventions (layout, build, CI, release, deployment, frontend style) come
from **cafard** (`~/misc/cafard`); the operator model (one admin, settings
in the UI, a password printed once at install) from **claudication**
(`~/misc/claudication`). When this plan is silent on a convention, open those
and do what they do. Section 14 lists the files to copy from.

---

## 1. What it is

A self-hosted calendar server for a family, at home, with no outside
service (Decided, user: "the goal is to not rely on external provider and
self host at home"). Three parts:

1. **The server** (Go, one binary, SQLite). Serves CalDAV, the family web
   app and the admin UI.
2. **The family web app** (TypeScript). Users see and edit their calendars,
   create calendars, share them, manage their devices and sign-ins.
3. **The Android app** (Java shell + the web app in a WebView). A **sync
   adapter** (events appear in any calendar app on the phone), the family web
   app, and the **scanner that signs a browser in** with a QR code.

```
  Android: agendouille app ── sync adapter ── CalendarContract ── any calendar app
           DAVx5 or any other CalDAV app ──┐
  iPhone / macOS Calendar ─────────────────┤ CalDAV (RFC 4791) + sync-collection (RFC 6578)
  Thunderbird ─────────────────────────────┤   = the only sync protocol
  TV / dashboard (read-only .ics) ─────────┤
                                           ▼
            DMZ: reverse proxy (TLS) ──► agendouille :8791 [dav, app] ──► SQLite
                                                          ▲
  browser / app WebView ── additional JSON API + SSE ─────┘   (sign-in, sharing, devices)
  LAN only: admin browser ──► agendouille :8792 [admin]
```

### One protocol, plus an additional API (Decided, user)

- **Sync is CalDAV, and only CalDAV**, for every client, our app included.
  Any CalDAV app keeps working against the same server with the same
  calendars: DAVx5 and other Android CalDAV apps, iOS, Thunderbird (user:
  "we stay compatible with other android app", "we use same protocol with
  additional api").
- **What CalDAV has no standard for** goes through an **additional JSON
  API** on the same server: sign-in (enrolment, approvals), sharing,
  devices, profile. The family web app uses it, and so does our app (its UI
  is the web app; its Java side uses the few `/api/app/*` calls).
- Nothing in the additional API is needed to sync. A user with only DAVx5
  gets every calendar they may read, read-only where they may only read;
  they use the web app for sharing and sign-ins.

### Who does what (Decided, user)

- **The admin** is the operator of the server, as in claudication: one
  account, not a family member, with a password. In the admin UI (LAN only)
  it creates and removes **users**, puts them in **groups**, hands each one a
  **one-time sign-in QR code**, and manages the server: ports and public
  URLs, storage and backups, limits and quotas, defaults. It sees
  **metadata only**, never an event.
- **Users** manage themselves in the family web app (in a browser or in the
  Android app): their profile, their **calendars** (create, rename, colour,
  delete), **sharing** them with people of their groups, their devices and
  their sign-ins. Users have **no password and no email**.
- **Groups** decide who can see whom: a user can belong to several groups
  and sees, and can share with, anyone who shares at least one group with
  them.

### Typical uses

- **A new user.** The admin adds Léa to the group "Famille", and the admin
  UI shows a QR code. Léa scans it with her phone: the agendouille app (or
  the browser) opens signed in, and the app sets up sync. Her calendars and
  the ones shared with her are in her phone's calendar app.
- **Sign in on the laptop.** The laptop's browser shows a QR code; Léa
  taps "Scan" in the app, picks the number shown on the laptop, and the
  laptop is signed in. No password, nothing typed.
- **A shared calendar.** Maman creates "École" in the web app and shares it
  read-write with Papa and read-only with the group "Enfants". It appears on
  everyone's phone at their next sync, whether they use our app or DAVx5.
- **Weekly activity.** "Judo, every Wednesday 14:00 until the end of June",
  with one Wednesday cancelled and one moved. Recurrence and its exceptions
  are kept as iCalendar has them, so every client shows the same thing.
- **A calendar from the phone.** An iPhone user, or a DAVx5 user, creates a
  calendar in their calendar settings; it is theirs, private until shared.
- **Lost phone.** The admin (or the user, from another signed-in device)
  revokes it; the admin shows a new one-time QR for the new phone.
- **Migrate.** An existing Google Calendar is exported to `.ics` and
  imported into a calendar from the web app.

### Why build it

Radicale, Baïkal, Davis, Nextcloud, SOGo, RustiCal and EteSync were weighed
(DECISIONS 2026-11-23). The user wants: no PHP or Python on the server, one
Go binary deployed like cafard; users, groups and calendars managed the way
a family needs; a real calendar view in the browser; passwordless sign-in
with QR codes; nothing outside the home.

### Non-goals

- **A sync protocol of our own.** Sync is CalDAV (above).
- **Email** of any kind: no sign-in links, no invitations, no scheduling
  (RFC 6638, iTIP). `ATTENDEE` and `ORGANIZER` are stored as sent and shown.
- **Passwords for users.** Only the admin has one.
- **Tasks and journals** (`VTODO`, `VJOURNAL`) and **contacts** (CardDAV).
- **An iPhone app.** iPhones use CalDAV and the web app.
- **Google Play.** The APK is downloaded from the server (section 7.6).
- **Outside services** at run time: no push through Google, no email
  provider, no analytics. The only outside contacts are Let's Encrypt (at
  the proxy, to renew certificates) and dynamic DNS if the home IP changes.

---

## 2. Technology (Decided unless marked)

| Concern | Choice | Notes |
|---|---|---|
| Server language | Go, `go 1.26.0` in `go.mod` (as cafard) | |
| cgo | **Disabled everywhere** (`CGO_ENABLED=0`) | Cross-compiles to linux amd64/arm64/armv7 |
| Database | `modernc.org/sqlite` (pure Go), WAL, foreign keys on | As cafard |
| iCalendar | **In-house**, `internal/ical` (Decided, user: no outside library in the calendar core) | Parse and write RFC 5545; the stored text is never rewritten |
| Recurrence | **In-house** RRULE engine, `internal/ical/rrule.go` (Decided, same) | Every example of RFC 5545 3.8.5.3 is a test |
| CalDAV/WebDAV server | **In-house**, `internal/dav` (Default) | go-webdav's server has no `sync-collection`, no `getctag` (DECISIONS) |
| Time zones | `time` + `time/tzdata` embedded | |
| Admin password | `golang.org/x/crypto/argon2` (argon2id) | The only password |
| QR codes | `rsc.io/qr` (as cafard) | |
| Config | YAML bootstrap file + settings in the database (section 11) | `gopkg.in/yaml.v3` |
| HTTP | `net/http`, no router library | `PROPFIND`, `REPORT`, `MKCALENDAR` go through one handler under `/dav/` |
| Logging | `log/slog` | |
| Web frontend | TypeScript (strict), **plain DOM in classes, no framework**, Vite, embedded by Go | As cafard |
| QR scanning in the browser | `BarcodeDetector` where present, else `jsqr` (Apache-2.0) (Default) | Same code in a browser and in the app's WebView |
| Frontend tests | `vitest` + `happy-dom` | |
| Android app | **Java**, Android framework APIs only: no Kotlin, no AndroidX, no Gradle (Decided, user dislikes the Android build) | Built by a Makefile in a container (section 13.5) |
| Android tests | JUnit 5 on the JVM, for the pure Java parts | Same container |
| Passkeys (optional, M10) | `github.com/go-webauthn/webauthn` (BSD-3) | Only if M10 happens |

Ask in `DECISIONS.md` before adding any other dependency, on any side.

### 2.1 Protocols (Decided)

| Link | Protocol | Notes |
|---|---|---|
| **Sync, every client** | **CalDAV** (RFC 4791) on WebDAV (RFC 4918), read and write, `MKCALENDAR` | Our app, DAVx5 and other CalDAV apps, iOS, Thunderbird |
| Incremental sync | **WebDAV sync** (RFC 6578) + `CS:getctag` | |
| Discovery | `/.well-known/caldav` (RFC 6764), `current-user-principal` (RFC 5397) | |
| Data | **iCalendar** (RFC 5545) | |
| **The additional API** | JSON over HTTPS (CONTRACTS 2, 3) | Sign-in, sharing, devices, profile; the web app and our app |
| Read-only subscription | A secret `.ics` URL | TVs, dashboards |
| Browser sign-in | **Approval** by a signed-in device (section 8.2), session cookie | In-house, no outside service |
| Sync credentials | HTTP Basic (RFC 7617) over TLS, **one random credential per device** | Section 8.4 |
| Android app links | Android **App Links** (`/.well-known/assetlinks.json`) | Opens sign-in links in the app when installed |
| Web app live updates | **Server-Sent Events** | |
| Push (optional, M9) | **UnifiedPush** with a self-hosted **ntfy**; **WebDAV-Push** draft for DAVx5 | No Google |
| iOS setup | Apple configuration profile (`.mobileconfig`) | |
| DAVx5 setup | DAVx5 login URI `davx5://user:secret@host/dav/` | DAVx5 manual, "Integration" |

---

## 3. Repository layout

```
agendouille/
  cmd/agendouille/main.go        serve, config, version, admin, user, group, backup,
                                 restore, import, export
  internal/config/               the bootstrap YAML + env + flags (section 11.1)
  internal/settings/             settings kept in the database, typed, live (11.2)
  internal/deps/                 the Deps struct (CONTRACTS 1.2)
  internal/dto/                  every JSON type of the APIs and SSE
  internal/store/                SQLite handle, migrations, queries
  internal/directory/            users, groups, visibility ("who can see whom")
  internal/sharing/              calendar ownership, shares, effective access
  internal/auth/                 admin password, sessions, enrolment links, approvals,
                                 devices (credentials), subscription links, Basic auth,
                                 client IP, same-origin
  internal/calendar/             calendars and objects: the one write path, changes,
                                 sync tokens, quotas, import/export
  internal/ical/                 parse, validate, index, expand, patch (pure)
  internal/dav/                  WebDAV + CalDAV server, well-known, /ics/
  internal/dav/testdata/clients/ recorded requests from our app, DAVx5, iOS, Thunderbird
  internal/hub/                  in-memory fan-out of changes (SSE, push)
  internal/surface/              surfaces (dav, app, admin), listeners, live rebinding
  internal/backup/               scheduled snapshots, download, restore
  internal/web/webx/             shared web plumbing: layouts, JSON errors, same-origin
  internal/web/app/              the family web app: pages, /api/*, /api/app/* (the APK's)
  internal/web/admin/            the admin UI: pages, /api/admin/*
  internal/web/app/live.go       /api/live (DECISIONS, M4)
  internal/web/assets/           embed: templates, static/{shared,app,admin}/
  testdata/ical/                 iCalendar fixtures shared by Go and the Android tests
  src/shared/ src/app/ src/admin/  TypeScript (section 9)
  android/                       the Android app (section 7)
    Makefile  AndroidManifest.xml  res/  src/  test/  libs/  docker/Dockerfile
  deploy/                        install.sh, agendouille.service, .openrc, container
                                 drop-in, agendouille.yaml.example, Caddyfile.example,
                                 nginx.conf.example, README.md
  scripts/remote.sh              rsync to dev-build, run checks there
  scripts/android.sh             build/test the APK in its container on dev-build
  scripts/devdeploy.sh           build on dev-build, push to the home server
  .github/workflows/ci.yml       frontend, Go, Android, shellcheck
  .github/workflows/release.yml  tag v* -> binaries, APK, SHA256SUMS, GitHub release
  package.json tsconfig.json vite.config.ts vitest.config.ts go.mod
  README.md AGENTS.md CLAUDE.md PLAN.md CONTRACTS.md DECISIONS.md
```

The project is **MIT** (Decided, user), server, web app and Android app
alike (`LICENSE`). Module path `github.com/nebuloss/agendouille`, remote
`git@github.com:nebuloss/agendouille.git` (Default). The Android package is
`io.github.nebuloss.agendouille` (Default, open question 6).

---

## 4. Data model

### 4.1 Concepts

- **Admin**: one operator account (name `admin`), argon2id password. Not a
  user: it has no calendars and cannot sign in to the family web app.
- **User**: a login name (`[a-z][a-z0-9-]{0,31}`, fixed), a display name, a
  colour, groups. No password, no email. Created, disabled and removed by
  the admin only.
- **Group**: a name and a colour, made by the admin. A user can be in several
  groups. **Visibility**: user A *sees* user B when they share a group. Only
  seen users appear in A's pickers, and A can share only with them (and with
  A's own groups).
- **Calendar**: **owned by one user**. A slug (`[a-z0-9][a-z0-9-]{0,62}`,
  unique per owner, fixed once made: it is in every phone's URL), a name, a
  colour, a time zone, a description. Made by its owner in the web app, in
  our app, or by any CalDAV client (`MKCALENDAR`).
- **Share**: a calendar shared by its owner with a **user** or a **group**,
  with `read` or `readwrite`. A group share covers its current and future
  members. A user's access to a calendar is the highest of: owner (all
  rights), their user share, the shares of their groups. Only users and
  groups the owner sees can be targets. Sharees can **hide** a calendar for
  themselves (it leaves their phones too) but cannot change or delete it.
- **Device**: a credential for sync. One per phone, computer or TV: a label,
  a kind (`app`, `davx5`, `ios`, `thunderbird`, `other`), a random secret
  shown once (or handed straight to the app), a cap `readwrite` or `read`.
  On a calendar, a device gets the lower of its cap and its user's access.
  The agendouille app's device also opens web sessions for its WebView
  (section 8.3).
- **Web session**: a browser (or the app's WebView) signed in to the family
  web app. 90 days, renewed on use (Default, a setting).
- **Enrolment link**: a one-time URL made by the admin for a user, shown as
  a QR code; it signs in the first device (section 8.1).
- **Approval**: a short-lived request from a browser that wants to sign in,
  approved from a device already signed in (section 8.2).
- **Subscription link**: a secret URL serving chosen calendars as one
  `.ics`, read-only. Belongs to a user, revocable, filtered by their access
  at each fetch.
- **Calendar object**: one CalDAV resource `<name>.ics` in one calendar,
  holding all the `VEVENT`s of one `UID` (master and overrides), as RFC 4791
  4.1 requires. The original text is stored, so what a client sent is what a
  client gets back, including properties agendouille does not understand.
- **Change**: one row per write, strictly increasing across the server.
  Each calendar's sync token and ctag come from its last change.

### 4.2 Who did what

Every object records `created_by`, `modified_by` (users) and the device or
session. The web app marks an event with its creator's initial in the
creator's colour and shows "added by Maman, changed by Léa". These live in
columns, **not** in the iCalendar text.

### 4.3 The write path (Decided)

One write path, `calendar.Service` (CONTRACTS 1.3): CalDAV, the additional
API, import, and the app all go through it. In one SQLite transaction it:

1. checks access (section 4.1) and the device cap;
2. checks the precondition (`If-Match`, `If-None-Match: *`), else `412`;
3. parses and validates the object (4.5) and the quotas (11.2);
4. checks `UID` uniqueness in the calendar (`no-uid-conflict`);
5. extracts the index columns (summary, first start, last end or open,
   recurring, all-day);
6. writes the object with a new etag, appends a `changes` row, sets the
   calendar's `seq`;
7. after commit, publishes the change on the hub (SSE, push).

Calendar-level writes (create, rename, recolour, share, unshare, delete)
go through the same service and also bump the calendar's `seq` when
clients must see them (name, colour).

The **etag** is the hex SHA-256 of the stored text, 16 bytes, quoted. The
**sync token** is `https://<dav host>/ns/sync/<calendar id>/<n>`; the
**ctag** is `<n>`.

Tombstones (the `changes` rows of deletions) older than
`sync_token_retention_days` (Default 180) are pruned; an older token gets
`403 DAV:valid-sync-token` and the client syncs in full (RFC 6578 3.2).

Gaining access to a calendar (a new share, joining a group) makes it appear
in the user's home; losing it makes it disappear. Clients pick that up from
the home listing at their next sync.

### 4.4 Time

- Instants are computed with Go's embedded tz database by `TZID`; an unknown
  `TZID` (Outlook's Windows names) goes through the CLDR Windows map, then
  the calendar's zone with a warning. The object's `VTIMEZONE` is kept.
- Floating times are read in the calendar's zone. All-day events are dates.
- The web app shows times in the **family zone** (a setting, Default
  `Europe/Paris`), not the browser's (Default: one family, one place).

### 4.5 What is accepted

- One `VCALENDAR` with `VEVENT`s sharing a `UID`, plus `VTIMEZONE`s.
  `VTODO`, `VJOURNAL`: `403 CALDAV:supported-calendar-component`.
- At most `max_object_bytes` (setting, Default 256 KiB).
- Must parse; must have `UID` and `DTSTART`; `DTEND` after `DTSTART`.
- Open-ended `RRULE` is fine; expansion is capped (Default 5000 instances
  per query).
- **No rewriting** of what a CalDAV client `PUT`: stored as received.

### 4.6 Edits from the web app

The additional API takes a simple event shape (CONTRACTS 3.3).
`internal/ical` **patches** the stored object: it changes only the
properties the form owns (`SUMMARY`, `DTSTART`, `DTEND`, `LOCATION`,
`DESCRIPTION`, `RRULE`, `EXDATE`), bumps `SEQUENCE`, sets
`DTSTAMP`/`LAST-MODIFIED`, and keeps the rest (alarms, attendees, `X-`
properties).

Recurring events take a **scope**:

| Scope | Effect |
|---|---|
| `this` | Change: add or replace an override with `RECURRENCE-ID`. Delete: add `EXDATE`. |
| `following` | Split: `UNTIL` on the master before this occurrence; a new object (new `UID`) from here on. |
| `all` | Change or delete the master and its overrides. |

### 4.7 Alarms (Default, open question 1)

A `VALARM` set on a shared calendar reaches everyone who syncs it. v1 keeps
it as sent.

---

## 5. CalDAV

Full contract in CONTRACTS section 4. Everything here is standard CalDAV;
our app gets nothing over it that DAVx5 does not.

### 5.1 URLs

```
/.well-known/caldav                        301 -> /dav/
/dav/                                      current-user-principal
/dav/principals/<user>/                    the principal
/dav/calendars/<user>/                     calendar-home-set: every calendar <user> may read
/dav/calendars/<user>/<owner>~<slug>/      a calendar, as seen from <user>'s home
/dav/calendars/<user>/<owner>~<slug>/<name>.ics   an object
/ics/<secret>.ics                          read-only subscription
```

Every calendar a user may read appears in their home under
`<owner>~<slug>` (`maman~ecole`), their own ones included (`lea~perso`).
The same calendar has the same objects in every home. A user reaching into
another user's home, or into a calendar they cannot read, gets `404`.

Object names are kept as the client chose them, unique in the calendar,
`[A-Za-z0-9._@+-]{1,200}\.ics` (Default).

### 5.2 Methods

| Method | Where | Notes |
|---|---|---|
| `OPTIONS` | anywhere under `/dav/` | `DAV: 1, 3, calendar-access, extended-mkcol` |
| `PROPFIND` | everything | Depth 0 and 1; `infinity` is `403 propfind-finite-depth` |
| `REPORT calendar-multiget`, `calendar-query`, `sync-collection` | calendar | `calendar-query`: `comp-filter` VEVENT, `time-range`, `prop-filter` on `UID`/`SUMMARY` |
| `GET`, `HEAD` | object; calendar (the whole `.ics`) | |
| `PUT`, `DELETE` | object | `If-Match`, `If-None-Match: *` |
| `MKCALENDAR`, extended `MKCOL` | `/dav/calendars/<user>/<slug>/` in one's **own** home | Creates a calendar owned by the user (display name from the body). Quota and component checks. It then lives at `<user>~<slug>` |
| `PROPPATCH` | calendar | Owner: `displayname`, `calendar-color`, `calendar-description`, `calendar-timezone` change the calendar for everyone. Sharee: stored as their own view. `calendar-order` per user |
| `DELETE` | calendar | Owner: deletes it for everyone. Sharee: `403` (they hide it in the web app) |
| `MOVE`, `COPY`, `LOCK`, `ACL` | | `403` / `405` |

`time-range` queries use the index, then expand recurrence in Go (RFC 4791
9.9). `<C:expand>` is not supported (Default).

The URL a client `MKCALENDAR`s at (`/dav/calendars/lea/<uuid>/`) and where
the calendar then lives (`lea~<uuid>`) differ. That is allowed (a `201` with
`Location`), but it must be checked against our app, DAVx5 and iOS in M2;
if one of them insists on the URL it asked for, the home also answers on
the bare `<slug>` for the owner's own calendars (Default fallback).

### 5.3 Access over CalDAV

On each calendar: the lower of the device cap and the user's access. With
`read`, `PUT`/`DELETE` get `403 DAV:need-privileges`, and
`current-user-privilege-set` lists `read` only, so clients show it
read-only.

### 5.4 Clients to work with (Decided)

| Client | Notes |
|---|---|
| **Our app** (M6, M7) | Its traffic is recorded first |
| **DAVx5** | Must keep working throughout (Decided, user) |
| Other Android CalDAV apps (OpenSync, …) | Should work: nothing non-standard is needed to sync |
| **iOS / macOS Calendar** | Picky about `current-user-privilege-set`, `supported-report-set`, `calendar-color` |
| **Thunderbird** | ctag + etag |
| GNOME Calendar / Evolution | Nice to have |

Captured requests from each go in `internal/dav/testdata/clients/`.

---

## 6. Sharing

Sharing is part of the additional API (CalDAV has no standard for it): the
web app, in a browser or in our app, shows it; every CalDAV client sees its
result.

- In the web app, a calendar's **Share** panel lists its shares and offers
  the users and groups the owner sees, each with `read` or `read-write`.
- Sharing, changing or removing a share takes effect at once on the server;
  clients see the calendar appear, turn read-only or disappear at their next
  sync (or at once with push, M9).
- Sharees see "shared by Maman" on the calendar and can **hide** it; hiding
  removes it from their CalDAV home too (so from their phones), and they can
  show it again.
- The owner can **transfer** a calendar to another user they see (Default);
  the new owner gets it, the old one keeps `readwrite` unless they remove it.
- Removing a user (admin): their calendars must be **transferred** to a user
  or **deleted**; the admin UI asks, calendar by calendar (it sees names and
  sizes, not events). Default: there is no "orphan" calendar.
- Leaving a group (admin removes a user from it): group shares stop
  applying; user shares to people they no longer see stay (Default: a share
  once made is the owner's decision; the admin can see and remove it).

---

## 7. The Android app (Decided, user: "a sync app with share capabilities + web QR code login")

### 7.1 What it is

```
┌──────────────────── agendouille.apk ─────────────────────┐
│ WebView: the family web app, loaded from the server      │
│          (same TypeScript as in a browser; uses the      │
│          additional API for sharing, sign-in, devices)   │
│      ▲  bridge: WebMessagePort, our origin only (7.4)    │
│      ▼                                                   │
│ Java, Android framework APIs only:                       │
│  • Account authenticator (AccountManager)                │
│  • Device credential in the Android Keystore             │
│  • Sync adapter: standard CalDAV <-> CalendarContract    │
│  • Enrolment by App Link / QR (/api/app/*)               │
│  • Camera permission for the scanner (the scanner is TS) │
│  • Notifications (M9, UnifiedPush)                       │
└──────────────────────────────────────────────────────────┘
```

- **Sync is the same CalDAV as everyone's.** The app is one CalDAV client
  among others; DAVx5 or another CalDAV app on the same phone, or on the
  next one, sees exactly the same calendars.
- **The extras are the additional API**: the WebView shows the family web
  app (sharing, devices, sign-ins); the Java side only calls `/api/app/*`
  to enrol and to open the WebView's session.
- **The server serves the UI**, so app and server cannot drift. The Java
  side keeps a small, versioned bridge contract (CONTRACTS 6).

### 7.2 Enrolment (first sign-in on the phone)

1. The admin makes an enrolment link for Léa: `https://<host>/enrol/<token>`,
   shown as a QR code in the admin UI (and as text, to send by SMS).
2. Léa scans it with the phone camera. With the app installed, Android's
   App Link opens it in the app (the server serves
   `/.well-known/assetlinks.json` with the APK's signing key fingerprint).
   Without the app, the browser opens a page that offers the APK download
   (7.6), "set up DAVx5 instead", and "continue in the browser" (a plain
   web session; iPhone users take this path).
3. The app `POST`s the token to `/api/app/enrol` with the phone's model; the
   server makes a device of kind `app` and returns its credential once.
4. The app stores it in the Keystore, adds the Android account
   "agendouille (Léa)", turns sync on, and opens the web app signed in.

The link is single use, valid 72 hours (setting), revocable, and the admin
UI shows whether it has been used.

### 7.3 Sync adapter (M7)

- **Scope**: `VEVENT` only, the calendars in the user's CalDAV home. Each
  becomes an Android calendar (name, colour, read-only flag from
  `current-user-privilege-set`). The user can untick calendars on the phone
  (stored locally) or hide them for all devices in the web app.
- **Protocol**: plain CalDAV with `sync-collection`, then `calendar-multiget`
  for changed objects; `PUT`/`DELETE` with `If-Match` for local changes;
  `MKCALENDAR` when the user creates a calendar from the app. Exactly what
  DAVx5 does; nothing the server offers only to us.
  `HttpURLConnection` and `XmlPullParser` (both in the framework).
- **Mapping** iCalendar <-> `CalendarContract` (the hard part, and the part
  most tests go to):
  - `Events`: `DTSTART`, `DTEND`/`DURATION`, `EVENT_TIMEZONE`, `ALL_DAY`,
    `RRULE`, `RDATE`, `EXDATE`, `TITLE`, `EVENT_LOCATION`, `DESCRIPTION`,
    `STATUS`, `AVAILABILITY`.
  - Overrides: rows with `ORIGINAL_SYNC_ID` and `ORIGINAL_INSTANCE_TIME`.
  - `Reminders` from `VALARM` (display alarms, relative triggers).
  - `Attendees` are read-only on the phone (no scheduling).
  - **The original iCalendar text is kept** (in a sync column) and patched
    on upload, so properties the phone does not know survive, the same rule
    as the web app's (4.6).
- **iCalendar parsing in Java**: an in-house parser for the subset above
  (Default; ical4j needs Java 11 APIs, desugaring and several libraries,
  which the no-Gradle build cannot take). Pure Java, no Android types, so it
  is unit-tested on the JVM against the same fixtures as `internal/ical`
  (`testdata/ical/`).
- **When**: on local change (Android's content-change trigger), every
  `sync_interval` (Default 30 min, the user can change it), on "Sync now",
  and on push (M9).
- **Conflicts**: on `412` the server's version wins; the local change is
  re-applied on top when it touched different fields, else dropped with a
  notification (Default).
- **DAVx5 is the reference, read but not copied** (Decided, user). Before
  M7 starts, read `bitfireAT/synctools` (the iCalendar <-> CalendarContract
  mapping) and `bitfireAT/davx5-ose` (the sync adapter and its sync logic),
  and write down every rule and quirk they handle (recurrence exceptions,
  all-day, time zones, reminders, vendor calendar apps' oddities) in
  `android/docs/mapping-notes.md`, each as a case in `testdata/ical/` with
  the expected Android rows. Then our own Java is written against those
  tests. Their code is GPLv3 and the project is MIT, so **no code is copied
  or translated line by line** from them; ideas, rules and test cases are
  fine, and each note cites the file it came from. Their code is Kotlin on
  AndroidX and ical4j anyway, which the no-Gradle build cannot take.
- The sync-adapter boilerplate (account, authenticator, service, XML
  metadata) starts from Google's `SampleSyncAdapter` (Apache-2.0), with its
  notice kept.

### 7.4 The bridge

- The WebView loads only the family web app's origin; any other navigation
  opens the system browser (`shouldOverrideUrlLoading`).
- The page and Java talk over a `WebMessagePort` the app posts with
  `postWebMessage` targeted at our origin (framework API, Android 6+), never
  `addJavascriptInterface`. Messages are JSON (CONTRACTS 6).
- Calls: `hello` (versions), `scan` (opens the in-page scanner with the
  camera permission granted), `sync.status`, `sync.now`, `davx5.handoff`,
  `account.remove`.

### 7.5 Requirements (Default)

`minSdkVersion` 26 (Android 8), `targetSdkVersion` the current one. No
Google Play services.

### 7.6 Distribution

- CI builds and signs the APK on a `v*` tag and attaches it to the release;
  the release key is a CI secret and a backup on the home server, never in
  git.
- The server embeds the APK of its own version and serves it at
  `/app/agendouille.apk`, with its SHA-256 on the download page. The app
  compares its version with the server's at start and offers the update
  (Android asks the user to allow installs from the app once).
- F-Droid and Play: not now.

---

## 8. Signing in (Decided, user: no passwords, no email, any platform)

### 8.1 First device: the admin's enrolment link

Section 7.2. Opened in a browser instead of the app, it signs that browser
in. It is the only step that needs the admin: once per user, and after
losing every device.

### 8.2 Any browser: approval from a signed-in device

Works in every browser on every system, with nothing installed and nothing
outside.

1. The browser opens `/signin`. It shows a **QR code**, a **short code**
   (`KQ4-7XM`) and a **number** (`42`), and waits.
2. On any device already signed in (usually the phone, in the app) the user
   taps **Scan**, or opens "Approve a sign-in" in the web app and types the
   short code (for screens a camera cannot read, and to approve from a
   computer).
3. The approving device shows: "Sign in **Firefox on Linux**, from
   192.168.1.20, as Léa?" and three numbers; the user picks the one the
   browser shows (`42`). A wrong pick cancels the request.
4. The waiting browser is signed in within a second.

Codes live **2 minutes** and are single use; at most 200 wait at once
(the only cap: `/signin` is open to the internet and stores a row). The
short code is 6 characters (about 28 bits), enough because only a
signed-in user can try one, for two minutes, and a wrong number cancels it.
Number matching stops an attacker who shows our QR on their own page from
getting a blind "yes". The scanner is TypeScript (`BarcodeDetector`, else
`jsqr`), so the same approval works from an iPhone's browser.

### 8.3 The app's WebView

The app trades its device credential for a web session
(`POST /api/app/session`, CONTRACTS 2.2), so the WebView is always signed
in while the device is valid. Revoking the device ends that session.

### 8.4 Sync credentials

- One per device, 20 random bytes, `agd_` + base32 in groups, stored as
  SHA-256. `last_used_at`, `last_ip`, client user agent kept and shown.
- Never typed: our app receives it at enrolment; DAVx5 gets it through the
  `davx5://` link (a button in the web app on the phone, or a QR on a
  computer); iPhones through a `.mobileconfig`; Thunderbird through a copy
  button.
- They open CalDAV only (and, for kind `app`, `/api/app/session`).

### 8.5 Sessions and recovery

- Web sessions: 90 days, renewed on use (setting). Each user sees their
  devices and browsers in the web app ("Signed in") and can revoke any; the
  admin sees and revokes them in the admin UI.
- Lost phone, another device still signed in: approve the new phone's
  browser from it, then enrol the app with a link the user makes for
  themselves (Default: a signed-in user can make an enrolment link for
  themselves, 10 minutes, shown as a QR), revoke the old phone.
- Every device lost: the admin makes a new enrolment link.

### 8.6 Passkeys (optional, M10)

After signing in, a device may add a passkey (fingerprint, Face ID, Windows
Hello, a security key) to sign in again on that device without approval.
Optional because Linux browsers and TVs often have no authenticator, and
because the browsers' own cross-device passkey flow goes through Google's
or Apple's relays; approval (8.2) is what always works.

### 8.7 The admin

The admin signs in at `/signin` on the admin listener with its
password (argon2id), nothing else: no approval, so a phone session on the
internet side can never reach the admin UI. Admin sessions last 12 hours
(Default).

---

## 9. Web frontend

### 9.1 Two UIs, three surfaces (Decided, user: the DMZ split)

| Surface | What | Where (Default layout) |
|---|---|---|
| `dav` | `/.well-known/caldav`, `/dav/`, `/ics/` | public listener, behind the DMZ proxy |
| `app` | the family web app and the additional API: pages, `/api/*`, `/api/app/*`, `/api/live`, `/enrol/`, `/app/agendouille.apk`, `/.well-known/assetlinks.json`, `static/app/` | public listener |
| `admin` | the admin UI: `/admin/*`, `/api/admin/*`, `static/admin/` | admin listener, LAN only |

- Each surface is served by exactly one listener; elsewhere its paths are
  `404`. `/healthz` is on every listener.
- **Two bundles** (`app.js`, `admin.js`) sharing `src/shared/`; the admin
  code is never served on the public listener.
- **Separate cookies**: `agd_app` and `agd_admin`.
- Absolute URLs (DAV hrefs, QR codes, profiles, App Links) come from the
  serving listener's `public_url` (a setting), never from `Host`.

### 9.2 Principles (Decided, as cafard)

- Plain TypeScript classes on the DOM, no framework. **No HTML sinks** in
  `src/` (cafard's `scripts/check-sinks.sh` in CI): titles come from every
  family member's phone.
- Go serves an HTML shell per page; the bundle renders the rest. The
  sign-in, enrolment and approval pages work without the bundle; the admin
  UI's pages beyond its sign-in need it (DECISIONS 2026-11-23, M3 notes).
- The family web app is **phone first** (it is also the Android app's UI).
  The admin UI is desktop first.

### 9.3 The family web app (surface `app`)

| Path | Page |
|---|---|
| `/signin` | The approval screen (8.2) |
| `/enrol/<token>` | First sign-in (7.2): opens the app, else offers the APK, DAVx5 or the browser |
| `/` | The calendar: agenda (phone default), week, month |
| `/calendars` | My calendars and the ones shared with me: create, rename, colour, share, transfer, hide, delete, import, export |
| `/approve` | Approve a sign-in: scanner and short-code field |
| `/devices` | Sync devices: add (our app, DAVx5, iPhone, Thunderbird, subscription link), revoke; signed-in browsers: revoke |
| `/me` | Display name, colour, groups (read only), "Enrol another phone" |

**The calendar views** (Default):
- **Calendars list** (side panel on wide screens, a sheet on phones): own
  and shared calendars, colours, show/hide, a lock on read-only ones,
  "shared by …".
- **Agenda** (default on phones), **week** (default on wide screens),
  **month**. Event colour: its calendar's; creator's initial on the event.
- **Detail sheet**: calendar, when, where, notes, recurrence in words,
  added by, changed by; Edit and Delete if writable.
- **Editor**: calendar (writable ones), title, all-day, start, end,
  location, notes, repeat (none, daily, weekly on days, monthly by date,
  yearly; until a date or a count); scope dialog for recurring events;
  changing the calendar moves the event.
- **Live**: SSE `changed` reloads the visible range; `access` reloads the
  calendars list.
- **Conflicts**: `412` reloads the event, says "changed meanwhile by
  <user>", keeps the form.

### 9.4 The admin UI (surface `admin`)

| Path | Page |
|---|---|
| `/signin` | Password |
| `/` | Overview: users, groups, calendars, devices not synced for a week, storage used, last backup, version, warnings (no public listener yet, the admin surface on a public listener, a backup older than two days) |
| `/users` | List: name, groups, calendars owned, devices, last seen. "Add user" |
| `/users/<name>` | Profile, groups; **enrolment QR** (make, show, revoke, used or not); devices and browsers (revoke); calendars owned and shared with them (names, counts, sizes); disable, enable; remove (with the transfer-or-delete step, 6) |
| `/groups` | Groups: name, colour, members; add, rename, delete (a group with shares asks first) |
| (on `/users/<name>`) | Every calendar: owner, name, events, size, shares. **No events.** The admin can remove a share, transfer a calendar, delete one (typing its name) |
| `/activity` | Who did what, without event titles: "Léa created an event in maman~ecole from Pixel 8", "Maman shared École with Enfants", admin actions |
| `/settings` | Every setting of 11.2, grouped: Network, Storage and backups, Limits, Defaults. Applied live |
| `/backups` | List, download, back up now, restore (13.6) |

### 9.5 TypeScript layout (Default, as cafard)

```
src/
  shared/  core/ (Api, Live, Dates, Bridge), render/, components/ (Dialog,
           Sheet, QrImage, Scanner, ColorPicker, UserChip), model/
  app/     main.ts, pages/ (Calendar, Calendars, Approve, Devices, Me,
           Signin, Enrol), components/ (CalendarList, AgendaView, WeekView,
           MonthView, EventSheet, EventEditor, RecurrencePicker, ScopeDialog,
           SharePanel), model/ (Event, Occurrence, Recurrence)
  admin/   main.ts, pages/ (Overview, Users, User, Groups, Calendars,
           Activity, Settings, Backups), components/
```

`src/app` and `src/admin` import `src/shared`, never each other (a test
checks). `shared/core/Bridge` is the page side of CONTRACTS 6: it detects the
app (a port arrives) and degrades to plain web features without it.

### 9.6 Look (Default)

Material 3 roles and the dark block from cafard's `app.css`; the admin UI
has a distinct top bar colour. User, group and calendar colours come from a
fixed palette of twelve checked for contrast (cafard's
`scripts/palette.py`).

---

## 10. Security (Decided: the public side faces the internet)

- **Admin password**: argon2id (time 2, memory 64 MiB), at least 12
  characters. Set by `agendouille admin passwd` (install.sh runs it with
  `--random` and prints the password once in the installer's terminal,
  never in a log). `serve` refuses to start without one.
- **Secrets** (enrolment tokens, approval codes, device credentials,
  session ids, subscription secrets): from `crypto/rand`, stored as SHA-256,
  compared in constant time, logged by id only. Access logs redact
  `/enrol/…`, `/ics/…` and `/api/app/*` bodies.
- **Cookies**: `HttpOnly`, `Secure` when the listener's URL is https,
  `SameSite=Lax`, one per surface.
- **CSRF**: state-changing web requests need `Sec-Fetch-Site: same-origin`
  or an `Origin` equal to the listener's `public_url`. `/dav/` and
  `/api/app/*` (bearer credentials, no cookies) are exempt.
- **No rate limits** (Decided, user: "rate limit is useless"): every
  secret but the admin password is random and at least 128 bits, so there
  is nothing to guess, and an approval needs a signed-in approver. The
  admin password sits on the LAN-only listener and behind argon2, with at
  most two derivations at once so the sign-in form cannot exhaust memory.
  The pending approvals are capped (8.2).
- **Client IP**: the TCP peer, or the last hop of `X-Forwarded-For` from a
  peer in `trusted_proxies`.
- **Exposure guard**: a non-loopback listener needs a `public_url`; an
  `http://` `public_url` needs `allow_insecure` on that listener (meant for
  the LAN-only admin listener). `serve` warns when `admin` shares a listener
  with `dav`.
- **Privacy from the admin** (Decided, user): no admin route returns
  `objects.data`, a summary or a location; the activity log keeps event
  titles only for the users' own history, and the admin's view of it drops
  them. A test pins every `/api/admin` answer against that. The admin can
  still read the database file on the server; this is a promise of the UI,
  not encryption (said in the README).
- **Headers**: `Content-Security-Policy: default-src 'self'` (no inline
  script), `nosniff`, `Referrer-Policy: same-origin`, `frame-ancestors
  'none'`.
- **XML** on `/dav/`: bodies capped (1 MiB), `encoding/xml` (no external
  entities).
- **The app**: the WebView loads only our origin, the bridge is a
  `WebMessagePort` bound to it, the credential is in the Keystore, cleartext
  traffic is off.

---

## 11. Configuration

### 11.1 Bootstrap file (Decided: only what the server needs before the database)

`/etc/agendouille.yaml` (or `-config`, `AGENDOUILLE_CONFIG`); keys also as
flags and `AGENDOUILLE_*` variables; unknown keys are errors (as cafard).

| Key | Default | Notes |
|---|---|---|
| `data_dir` | `/var/lib/agendouille` | database, backups |
| `admin_listen` | `127.0.0.1:8792` | The admin UI. Never changed from the UI, so a mistake there cannot lock the admin out |
| `admin_public_url` | `http://<admin_listen>` | |
| `admin_allow_insecure` | `false` | Must be `true` for a plain `http://` admin URL off loopback (a LAN-only admin UI); off loopback, `admin_public_url` is required |
| `log_level` | `info` | |

### 11.2 Settings (in the database, edited in the admin UI, applied live)

| Group | Setting | Default |
|---|---|---|
| Network | `public_listen` (CalDAV + family web app) | none: the public surfaces are off until set |
| | `public_url` | none; required with `public_listen` |
| | `public_allow_insecure` | `false` |
| | `trusted_proxies` | `[]` |
| Storage and backups | `backup_schedule` (daily at) | `03:30` |
| | `backup_keep` | 14 |
| | `sync_token_retention_days` | 180 |
| | `max_object_bytes` | 256 KiB |
| Limits | `max_calendars_per_user` | 50 |
| | `max_events_per_calendar` | 50 000 |
| | `max_devices_per_user` | 20 |
| Defaults | `family_timezone` | `Europe/Paris` |
| | `palette` | the twelve colours |
| | `enrolment_link_ttl_hours` | 72 |
| | `session_ttl_days` | 90 |
| | `app_sync_interval_minutes` | 30 (at least 15: Android's floor) |

- Setting names carry their unit (`_days`, `_hours`, `_minutes`), so a
  value typed in a shell cannot be read in the wrong one.
- Changing `public_listen` or `public_url` rebinds the public listener
  without a restart (the old one drains for 10 s). A change made with
  `agendouille config set` while the server runs reaches it within five
  seconds (it watches the table). The page says that
  phones keep their URL, so changing `public_url` means re-enrolling them
  (and App Links follow the new host only with an APK that knows it: open
  question 3).
- Every change is in the admin activity log with old and new values.
- `agendouille config` prints the effective bootstrap and settings;
  `agendouille config set <key> <value>` changes a setting from a shell.

---

## 12. Commands

| Command | Does |
|---|---|
| `serve` | Run |
| `config [set k v]` | Print or change (11) |
| `admin passwd [--random]` | Set the admin password |
| `user add <name> [--group g]...`, `user enrol <name>` (prints a link and a QR in the terminal), `user list`, `user disable\|enable\|remove` | |
| `group add\|list\|remove`, `group member add\|remove` | |
| `backup [file]`, `restore <file>` (server stopped) | |
| `calendar add <user> <slug>`, `calendar list|share|unshare|transfer|delete` | The operator's view until M3 and M4 |
| `device add <user> [--kind] [--label] [--read]`, `device list|revoke` | Sync credentials by hand until M4 (DAVx5's QR in the terminal) |
| `import <user>~<slug> <file.ics>`, `export <user>~<slug>` | |
| `version` | |

---

## 13. Build, CI, release, deployment (Decided: as cafard)

### 13.1 Where things run

- dev-code edits, runs `npm` checks, commits, pushes. **It never builds Go
  binaries or the APK.**
- dev-build (`guillaume@10.0.50.21`) runs Go checks through
  `scripts/remote.sh`, and the Android build through `scripts/android.sh`,
  both through `rtk`.
- CI builds releases on `v*` tags.

### 13.2 `scripts/remote.sh`

From cafard, `REMOTE_DIR=agendouille`: `gofmt -l`, `go build`, `go vet`,
`go test -race`.

### 13.3 CI and release

cafard's workflows minus Windows, plus an `android` job (the same container
as 13.5) that runs the JVM tests and builds the APK; the release signs it
with the CI secret key and the Go build embeds it.

### 13.4 Deployment (`deploy/`)

cafard's `install.sh`, hardened systemd unit, OpenRC, LXC drop-in, renamed;
`install.sh` sets the admin password (`--random`, printed once). Plus:

- `Caddyfile.example`, `nginx.conf.example` for the DMZ proxy: forward only
  the public listener; pass `PROPFIND`, `REPORT`, `PROPPATCH`, `MKCALENDAR`,
  `MKCOL`, `OPTIONS` and the `Depth`, `If-Match`, `If-None-Match`,
  `Authorization` headers; do not buffer `/api/live`; answer
  `/.well-known/caldav` and `/.well-known/assetlinks.json` on the bare host.
- `deploy/README.md`: DMZ and LAN layout, DNS, certificates, first start,
  backups, upgrading, the per-milestone client checklist.

### 13.5 The Android build (Decided: no Gradle, no Kotlin, no Android Studio)

- On dev-build: the JDK and the Android SDK pieces already in
  `~/android-build` (`jdk-17…`, `sdk/`); dev-build has no Docker (found in
  M0). In CI: the same pieces in a container.
- `android/docker/Dockerfile` (CI): a JDK, Android `build-tools` (`aapt2`, `d8`,
  `zipalign`, `apksigner`) and one `platforms;android-NN` (`android.jar`),
  fetched by `sdkmanager` at image build. A few hundred MB, on dev-build
  and in CI only.
- `android/Makefile`: `aapt2 compile/link` -> `javac --release 17 -cp
  android.jar` -> `d8` -> zip -> `zipalign` -> `apksigner`. `make test` runs
  JUnit on the pure Java packages. Plain `.jar` libraries only, in
  `android/libs/` with their checksums (JUnit for tests; none at run time,
  Default).
- `scripts/android.sh`: rsync `android/` to dev-build and run `rtk docker
  run … make <target>` there. Debug builds are signed with a debug key kept
  on dev-build.

### 13.6 Backups

`serve` writes `VACUUM INTO data_dir/backups/agendouille-<UTC time>.db` on
`backup_schedule` and keeps `backup_keep`. The admin UI lists, downloads and
makes backups, and **restores** one: it checks the file (SQLite's
integrity check), stages it as `restore-pending.db`, and the server exits
with code 75; the service manager starts it again (`Restart=on-failure`),
and the start puts the staged file in place before opening the database,
keeping the old one as `backups/before-restore-<time>.db`. `agendouille
restore <file>` does the same at once with the server stopped.

### 13.7 `scripts/devdeploy.sh`

cafard's: build on dev-build, copy the binary to the home server, restart.

---

## 14. Reference files to copy from

| Need | Copy from |
|---|---|
| `remote.sh`, `rsync-excludes`, `devdeploy.sh` | `~/misc/cafard/scripts/` |
| `install.sh`, unit, OpenRC, container drop-in | `~/misc/cafard/deploy/` |
| Password printed once at install, settings in the DB, admin UI tone | `~/misc/claudication/` (`deploy/install.sh`, `internal/service/settings`) |
| CI, release | `~/misc/cafard/.github/workflows/` (drop Windows) |
| Vite, tsconfig, vitest, embed placeholder, `bundleBuilt` | `~/misc/cafard/` |
| `main.go` shape, bootstrap config loading | `~/misc/cafard/cmd/cafard/`, `internal/config/` |
| Migrations runner | `~/misc/cafard/internal/store/` |
| Same-origin check | `~/misc/cafard/internal/auth/` |
| SSE | `~/misc/cafard/internal/web/`, `src/core/` |
| Package import rules test | `~/misc/claudication/internal/arch/` |
| TS class style, `render/`, sinks check | `~/misc/cafard/src/`, `scripts/check-sinks.sh` |
| CSS, palette check | `~/misc/cafard/internal/web/assets/static/app.css`, `scripts/palette.py` |

---

## 15. Milestones

Each milestone ends with code, tests, updated `README.md` and `AGENTS.md`,
and its "Done when" passing on dev-build (and in the Android container for
M6, M7).

**M0. Skeleton**
- `cmd/agendouille` (`serve`, `config`, `version`), bootstrap config, slog,
  `/healthz`, graceful shutdown; `package.json`, Vite with two entries,
  vitest, `src/shared/core` with tests, embed placeholders, `bundleBuilt`;
  `remote.sh`, CI, release, `.gitignore`.
- *Done when:* `scripts/remote.sh` passes, `npm run typecheck && npm test &&
  npm run build` passes, `agendouille version` prints `dev`.

**M1. Store, settings, surfaces, directory, auth**
- Migrations (CONTRACTS 5); settings with live apply; listeners and
  surfaces (the public listener from settings, rebinding); the admin
  password, admin sessions, `/admin/signin`; users, groups, visibility;
  enrolment links; approvals; devices and Basic auth; web sessions; rate
  limits; trusted proxies; same-origin.
- CLI: `admin passwd`, `user …`, `group …`, `config set`.
- *Done when:* tests show: an enrolment link works once; an approval with
  the wrong number fails and the right one signs the waiting browser in;
  a device credential opens a protected test route and nothing else;
  setting `public_listen` from a test starts the public listener without a
  restart; `/admin/` is `404` on it and `/dav/` is `404` on the admin
  listener; an `agd_app` cookie does not open `/admin`.

**M2. CalDAV, read and write, with ownership and shares**
- `internal/ical`, `internal/calendar`, `internal/sharing`, `internal/dav`
  (section 5, CONTRACTS 4), the hub, quotas; import/export.
- *Done when:* a test sending CalDAV requests by hand makes a calendar with `MKCALENDAR`,
  shares it read-only with a second user, and checks: the second user sees
  it and cannot write; time-range queries with a weekly rule and an
  `EXDATE`; `If-Match` and `412`; `sync-collection` through creates,
  changes and deletes; recorded client requests answer as recorded; **by
  hand**: DAVx5, iOS and Thunderbird each add, edit and delete an event the
  other two see, and DAVx5 and iOS create a calendar (checking 5.2's URL
  question).

**M3. Admin UI**
- `/api/admin/*`, the admin bundle, every page of 9.4, the activity log,
  settings with live apply, backups and restore.
- *Done when:* from the admin UI alone: set the public listener, add a
  group and two users, make an enrolment QR that a phone's browser opens
  signed in, see it marked used, restore yesterday's backup; a test proves
  no `/api/admin` answer carries event data.

**M4. Family web app and the additional API**
- `/api/*`, SSE, the app bundle: calendar views, editor with scopes,
  calendars page with sharing and transfer, approval page with short code
  (no in-page scanner: DECISIONS, M4), devices (DAVx5 link and QR, `.mobileconfig`, Thunderbird,
  subscription links), signed-in browsers, `/me`.
- *Done when:* vitest covers `Dates` across DST, `Recurrence` both ways,
  each view's renderer; a Go test drives every
  scope and checks a `VALARM` and an `X-` property survive a web edit; a
  laptop signs in by approval from a phone's browser; a calendar shared in
  the web app appears on a DAVx5 phone.

**M5. Deployment**
- `deploy/` (13.4), backups schedule (13.6), `devdeploy.sh`, proxy examples.
- *Done when:* `install.sh` installs and upgrades on Debian and Alpine, and
  the home server serves the family through its DMZ proxy with the admin UI
  on the LAN only. From here the family can use the web app and DAVx5.

**M6. Android app: shell and sign-in**
- First a **spike**: a "hello WebView + WebMessagePort" APK built by the
  Makefile in the container on dev-build, installed on a phone. Then: the
  account authenticator, Keystore storage, App Links and `/api/app/enrol`,
  `/api/app/session`, the bridge (`hello`, `scan`, `davx5.handoff`), the
  APK served by the server, the update check.
- *Done when:* a phone with the APK scans the admin's enrolment QR and
  lands in the web app signed in; "Scan to sign in a browser" signs a
  laptop in; revoking the device in the web app signs the WebView out; the
  whole APK builds with `scripts/android.sh` and in CI, with no Gradle.

**M7. Android app: sync adapter**
- First: the reading of synctools and davx5-ose (7.3) into
  `android/docs/mapping-notes.md` and test cases in `testdata/ical/`.
- The Java iCalendar subset parser, the CalDAV client, the mapping (7.3),
  local changes, conflicts, calendars appearing and disappearing, creating
  a calendar, "Sync now" and status over the bridge.
- *Done when:* the JVM tests run the mapping over the shared fixtures
  (recurrence, overrides, all-day, DST, alarms, unknown properties kept);
  on a phone, with Google Calendar, Fossify Calendar and the Samsung
  calendar app: create, edit one occurrence, delete, all seen on a DAVx5
  phone and in the web app, and the other way round; a read-only shared
  calendar cannot be edited on the phone; the app's recorded requests
  contain nothing DAVx5 does not also send (only standard CalDAV).

**M8. Hardening pass**
- Fuzzing `internal/ical` and the DAV XML parser, a review of every
  `/api/admin` answer, load test with 10 years of a busy family's events,
  the client checklist again.

**M9. Push (optional, Default: yes)**
- A self-hosted **ntfy** on the home server (documented, not embedded);
  UnifiedPush in the app (implemented by hand on its intent API, no
  library); WebDAV-Push for DAVx5; the server pushes on each change. Read
  the WebDAV-Push draft again before starting.
- *Done when:* a change in the web app reaches the app and a DAVx5 phone in
  under ten seconds, with no Google service involved.

**M10. Passkeys (optional)**
- Section 8.6.

---

## 16. Testing

### Go

- **`internal/ical`**: expansion against known outputs (weekly `BYDAY`,
  monthly `BYMONTHDAY=31`, yearly 29 Feb, `EXDATE`, overrides in and out of
  a range, DST in `Europe/Paris`, all-day, floating, unknown `TZID`). The
  fixtures live in `testdata/ical/` and are **shared with the Android JVM
  tests**.
- **`internal/dav`** against recorded client requests.
- **End to end** with CalDAV requests written out by hand, through the real auth
  (`cmd/agendouille/caldav_e2e_test.go`).
- **Write path**: concurrent puts (one `412`), increasing tokens,
  tombstones, pruning.
- **Access**: owner, user share, group share, highest wins, device cap,
  hidden calendars leave the home, leaving a group, transfer, removal of a
  user with transfer.
- **Security**: CSRF, `X-Forwarded-For` only from
  trusted proxies, surfaces `404` off their listener, cookies do not cross
  surfaces, single-use links and codes, number matching, no event data in
  admin answers.

### TypeScript

vitest for `shared/core/Dates`, `shared/core/Api`, `shared/core/Bridge`,
`app/model/Recurrence`, the scanner's payload parsing, renderers under
happy-dom, the import rule between `src/app` and `src/admin`.

### Android

JUnit on the JVM for the parser and the mapping (with a fake of the
`CalendarContract` rows as plain Java objects), the CalDAV XML handling,
and the bridge's message codec. On-device checks by hand, per the M7
checklist.

### By hand

The client checklist in `deploy/README.md` (M2, M4, M7): our app, DAVx5,
iOS, Thunderbird, each step seen on the others.

---

## 17. Open questions (ask the user, do not guess)

1. **Alarms.** Keep a `VALARM` on a shared calendar for everyone (v1), or
   strip alarms so each person sets their own?
2. **"Who is it for."** Should an event name the users it concerns beyond
   its calendar (`CATEGORIES` with names)?
3. **Host and domain.** Which machine and container, which domain, and is
   it fixed? App Links and every phone's URL depend on it.
4. **Holidays.** Should the server fetch an outside `.ics` (school holidays)
   into a read-only calendar? That is an outside contact, so off by default.
5. **Removed users' calendars.** Transfer or delete, asked each time
   (Default), or a fixed rule?
6. **The app's identity.** Package name
   (`io.github.nebuloss.agendouille`?). The licence is decided: MIT.

Until they are answered, implement with the defaults written here.
