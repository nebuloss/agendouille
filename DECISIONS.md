# Decisions

Deviations from `PLAN.md` and answers to its open questions, newest first.
Each entry gives the date, the decision and the reason.

## 2026-11-23: M3 notes, and the fixes of the M2 review

**An independent review of M2** (a separate agent reading the code cold)
found nine defects; all are fixed, each with a test:

- **Critical, one PUT could take the server down.** An `INTERVAL` of 2^55
  overflowed to a step of zero and looped forever; repeated `BYHOUR` and
  `BYMINUTE` values multiplied into billions of instances. `INTERVAL` is
  now at most 1000, `COUNT` at most 100 000, BY lists are deduplicated,
  and a rule whose BY parts allow more than 50 000 instances in one period
  is refused as invalid data.
- **Expansion is budgeted by periods**, skips whole days and hours a
  sub-daily rule rules out, and starts near the asked range when the rule
  has no COUNT: an hourly event made twelve years ago has instances today,
  and `FREQ=SECONDLY;BYHOUR=9` no longer stops after its first day.
- **Quadratic work on hostile text** (thousands of continuation lines, an
  XML body of many small chunks, a TZID of a hundred thousand segments)
  is linear now; TZIDs are capped at 128 characters and cached.
- **The write path re-checks access inside its transaction**
  (`calendar.recheck`, `sharing.AccessTx`), as PLAN 4.3 said: a share
  removed, or a calendar deleted, between the check and the write no
  longer lets the write through.
- **PROPPATCH** refuses a read-only device, answers 403/424 for what it
  does not set (a sharee's description, an owner removing the name) rather
  than 200, and the owner can set the calendar's zone with
  `calendar-timezone`.
- **Index times are clamped to the year 9999** (they are stored as text);
  durations over a century are invalid.
- **A request at the bare-slug URL gets hrefs under it**, so a client that
  insists on the URL it MKCALENDARed at finds its objects.

**M3:**

- **The admin UI needs JavaScript** beyond its sign-in (PLAN said its forms
  would work without the bundle). It is used by one person, on the LAN,
  from a recent browser; one code path (the JSON API the bundle calls) is
  worth more than server-rendered duplicates of eight pages.
- **A restore restarts the server** instead of swapping the database
  while it runs: every service holds the open handle, and a swap under
  them is a class of bugs for a feature used once a year. The backup is
  checked (`PRAGMA integrity_check`) and staged; the server exits with 75;
  systemd's `Restart=on-failure` (cafard's unit) starts it again, and the
  start applies the staged file before opening anything.
- **Backup files are named by UTC time to the second**, not by day:
  `VACUUM INTO` refuses an existing file, and "back up now" may run twice
  a day. Backups kept from before a restore are never pruned.
- **Admin answers are in French**, the UI's language; the API's own error
  codes stay English.
- **Found by looking at screenshots, not by tests:** the admin sign-in
  page rendered no form at all (an empty `map` is false in
  `{{with .Data}}`), and so did `/approve`. Page templates now get their
  data as their dot, and a test checks each form page has its form.
- **The QR SVG draws runs of modules** as one rectangle each, a fifth of
  the size.

## 2026-11-23: M2 notes

- **No iCalendar or recurrence library** (the user: "i am not sure it is
  good idea to rely on external go libraries for that", then "own both").
  `internal/ical` has its own parser and encoder (RFC 5545 3.1: folding,
  quoted parameters, TEXT escapes) and its own RRULE engine, run against
  every example of RFC 5545 3.8.5.3 (38 cases) plus DST, all-day, floating
  and Windows-zone cases. go-ical and rrule-go are gone from the plan, and
  so is go-webdav as a test client: the end-to-end test writes CalDAV
  requests by hand, which is closer to what phones send anyway.
- **The client fixtures are hand-written**, from what DAVx5, iOS and
  Thunderbird are known to ask, not captured: no client can be run from
  here. The by-hand checks of M2 replace them with captures.
- **Two shell commands the plan did not list**, needed before the admin UI
  and the web app exist: `calendar add|list|share|unshare|transfer|delete`
  and `device add|list|revoke` (with DAVx5's login QR in the terminal).
  `user remove` refuses while the user owns calendars (PLAN 6).
- **A sync token names the calendar by id**
  (`<public_url>/ns/sync/<id>/<seq>`), so transferring a calendar, which
  changes its ref, does not invalidate its clients' tokens for nothing.
- **The URL a client MKCALENDARs at keeps working** for its owner: the
  calendar lives at `<user>~<slug>` (said in `Location`) and the bare slug
  resolves to it in the owner's home (PLAN 5.2's fallback, built in at
  once since it costs a line).
- **`calendar-timezone` is not reported** (404): writing a correct
  VTIMEZONE needs transition rules Go's zone database does not expose, and
  every event carries its own TZID. A PROPPATCH of it is accepted and
  ignored; MKCALENDAR takes the TZID from it.
- **A missing `Depth` on PROPFIND is 0**, not infinity: every target client
  sends one, and infinity is refused anyway. **OPTIONS needs no
  credential**, as clients probe it first.
- **A sharee's PROPPATCH changes their own view** (name, colour, order), a
  read-only sharee's included; the description stays the owner's.
- **The settings watcher reloads on its first tick**: it took its first
  snapshot inside its goroutine, so a change written between the server's
  start and that snapshot was missed (found by a flaky test).

## 2026-11-23: M1 notes

- **No rate limits** (the user: "rate limit is useless"). Every secret
  agendouille checks but one is random and at least 128 bits (device
  credentials 160, sessions and enrolment tokens 256), and an approval
  needs a signed-in approver and dies on a wrong number. The exception,
  the admin password, sits on the LAN-only listener behind argon2id.
  What stays is resource protection, not guessing protection: at most two
  argon2 derivations at once (64 MiB each), and at most 200 approvals
  waiting, since `/signin` is open to anyone and stores a row. PLAN 10,
  11.2 and CONTRACTS 3.1 lose their rate-limit entries.
- **The short code is 6 characters**, not 7: "KQ4-7XM" as PLAN shows it,
  about 28 bits from a 27-letter alphabet without look-alikes. Enough for
  a code that lives two minutes and only signed-in users can try.
- **Setting names carry their unit**: `session_ttl_days`,
  `enrolment_link_ttl_hours`, `app_sync_interval_minutes`,
  `sync_token_retention_days`. `config set session_ttl 90` cannot then mean
  90 seconds to one reader and 90 days to another.
- **Admin sessions are in the database** (cafard keeps them in memory), so
  a restart does not sign the admin out; they still last 12 hours.
- **`serve` watches the settings table** every five seconds, so `config
  set` from a shell reaches a running server, as the admin UI's changes
  will.
- **The approval page works without the bundle**: `/approve` is a plain
  form, and `/signin` has a "continue" button that collects the session
  with `?then=redirect`. The bundle only reads a scanned QR's fragment
  (which never reaches the server) and waits on the long-poll by itself.

## 2026-11-23: M0 notes

- **No Node or Go on the editing machine.** AGENTS.md said npm checks run
  on dev-code; they cannot. `scripts/remote.sh web|all` runs them on
  dev-build (Node 20, Go 1.26 through the auto toolchain), and
  `package-lock.json` is copied back from there.
- **Two separate Vite builds**, not one build with two entries: each bundle
  carries its own copy of `src/shared`, so no shared chunk exists that the
  public listener would have to serve next to the admin one. The shared
  code is small; the duplication costs bytes, not correctness.
- **`admin_allow_insecure` defaults to `false`** (PLAN said "true on a
  private address"): off loopback, plain http for the admin UI is accepted
  only when the bootstrap file says so in so many words. Fewer guesses
  about what "private" means behind a NAT.
- **dev-build has no Docker**, but has a JDK 17 and an Android SDK in
  `~/android-build`. M6 uses those there, and a container only in CI
  (PLAN 13.5 updated).

## 2026-11-23: MIT, and DAVx5 read but not copied

The user chose MIT for the whole project, and agreed to the way DAVx5 is
used: its sync code (`synctools`, `davx5-ose`) is GPLv3, so it is read to
learn the mapping rules and quirks, which become notes and test cases, and
our Java is written against those tests without copying or translating
their code (PLAN 7.3). Google's `SampleSyncAdapter` (Apache-2.0) is the
starting point for the sync-adapter boilerplate, its notice kept.

## 2026-11-23: the model, settled

After a long exchange the user settled the model; it replaces the earlier
entries of today where they disagree (marked below). In the user's words
and what follows from them:

- **Admin is the operator, as in claudication.** "a single admin ui that
  allow to create / remove users, manage server storage port use, ... give
  them one time login qrcode". One admin account with a password, not a
  family member, LAN only. It manages users, groups, enrolment QR codes and
  every setting (network, storage and backups, limits, defaults: all four
  groups the user picked), and sees **metadata only** ("No, metadata
  only"). Settings live in the database and apply live; a small bootstrap
  file keeps only `data_dir` and the admin listener, so a mistake in the UI
  cannot lock the admin out. *Replaces* "admins are members with an admin
  flag".
- **Users manage themselves and own their calendars.** "from webui they can
  create and share calendar to other". Calendars belong to a user, are
  shared with users or groups (read or read-write), and can be made by any
  CalDAV client (`MKCALENDAR`; DAVx5 and iOS can). *Replaces* "several
  shared calendars made by admins with access per member".
- **Groups decide visibility.** "when creating user we assign them to group
  of user so each user in same group can see one other". Several groups per
  user (the user's pick); you see, and share with, anyone in a group of
  yours.
- **No passwords, no email, nothing outside.** "the goal is to not rely on
  external provider and self host at home": email was dropped (home mail
  servers land in spam; providers are outside). Sign-in is an admin's
  one-time enrolment QR for the first device, then **approval** of any
  browser from a signed-in device (QR or short code, with number matching),
  long sessions, optional passkeys later. *Replaces* "member passwords,
  optional" and the email-link idea.
- **Our own Android app: "a sync app with share capabilities + web qr code
  login".** A Java shell (sync adapter, account, Keystore, App Links,
  camera) around the family web app in a WebView (TypeScript), which
  carries sharing, devices and sign-in. Java, not Kotlin; framework APIs
  only, no AndroidX, **no Gradle**: a Makefile over `aapt2`, `javac`,
  `d8`, `apksigner` and one `android.jar`, in a container on dev-build and
  in CI (the user: "i hate kotlin/java because it is a mess to compile
  apk", "is there a way to compile native android app without having the
  whole android sdk"). *Replaces* "DAVx5 is the Android client; a custom
  app is optional".
- **One protocol, plus an additional API.** "we stay compatible with other
  android app", "we use same protocol with additional api". Sync is
  standard CalDAV for every client, our app included; DAVx5 and any other
  CalDAV app keep working. What CalDAV has no standard for (sign-in,
  sharing, devices) is the additional JSON API, used by the web app and
  the app. CONTRACTS states it as a rule: sync never depends on section 3.

Milestones are now M0 skeleton, M1 store/settings/surfaces/directory/auth,
M2 CalDAV, M3 admin UI, M4 family web app, M5 deployment, M6 app shell and
sign-in, M7 app sync adapter, M8 hardening, M9 push (ntfy, UnifiedPush,
WebDAV-Push), M10 passkeys. Entries below use older numbers.

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

## 2026-11-23: first start needs a member made at the shell (Default; replaced)

*Replaced by "the model, settled": the admin is an operator whose password
`install.sh` sets with `agendouille admin passwd --random`, printed once in
the installer's terminal, never in a log; `serve` refuses to start without
it.*

cafard prints a random admin password at first start. Here the first
member is always the person at the server's shell, and a printed password
would end up in the journal of an internet-facing host. `serve` refuses to
start until `agendouille member add <name> --admin` has made one, and
that command prints a one-time invitation link instead of a password.
