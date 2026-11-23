# agendouille: contracts between parts

`PLAN.md` says what agendouille does. This file pins the **interfaces
between its parts**:

1. Go packages, ownership and interfaces.
2. HTTP routes, with auth.
3. The JSON API of the web UI.
4. The CalDAV surface: properties, reports, errors.
5. The SQLite schema.
6. SSE events.

**Rules for this file:**

- When `PLAN.md` and this file disagree on a contract, this file wins. Fix
  the other one in the same change.
- Changing a contract is a change to this file first, with a line in
  `DECISIONS.md`.
- Additions that break nothing (a new optional field, a new route) can be
  made by whoever needs them. Renaming or removing anything needs the
  user's agreement.
- JSON uses `snake_case`. Instants are RFC 3339 strings in UTC; local
  times are `YYYY-MM-DDTHH:MM:SS` with no offset and mean the calendar's
  zone; dates are `YYYY-MM-DD`.

---

## 1. Packages and Go interfaces

### 1.1 Ownership

| Package | Owns | Imports (besides stdlib) |
|---|---|---|
| `cmd/agendouille` | subcommands, wiring; one `Register` call per feature | everything |
| `internal/config` | the root config struct; each feature adds **one** field holding its own sub-struct | — |
| `internal/deps` | the `Deps` struct (1.2) | store, calendar, members, auth, hub |
| `internal/store` | the `*sql.DB`, migrations, `Tx` helper | sqlite |
| `internal/members` | members, password hashing, invitations | store |
| `internal/auth` | sessions, devices, links, Basic auth, rate limits, client IP, same-origin | store, members |
| `internal/ical` | parse, validate, index, expand, patch; pure (no store) | go-ical, rrule-go |
| `internal/calendar` | the calendar service (1.3), changes, tokens, backup, import/export | store, ical, hub |
| `internal/hub` | in-process fan-out of `calendar.Change` | — |
| `internal/dav` | `/dav/`, `/.well-known/caldav`, `/ics/` | calendar, auth, ical |
| `internal/devices` | QR payload, `.mobileconfig`, the devices page data | auth |
| `internal/dto` | every type of section 3 and 6 | — |
| `internal/web/api` | `/api/*` but `/api/live` | calendar, members, auth, devices, ical, dto |
| `internal/web/sse` | `/api/live` | hub, auth, dto |
| `internal/web/pages` | HTML shells and forms | auth, members, devices |

`internal/dav` and `internal/web/*` never import each other. Nothing below
`calendar` writes the `objects` or `changes` tables.

### 1.2 Deps

```go
type Deps struct {
    Config   *config.Config
    Log      *slog.Logger
    Store    *store.Store
    Members  *members.Service
    Auth     *auth.Service
    Calendar *calendar.Service
    Hub      *hub.Hub
    Clock    func() time.Time
}

// Every HTTP feature package:
func Register(mux *http.ServeMux, d *deps.Deps)
```

### 1.3 The calendar service (the one write path)

```go
package calendar

type Actor struct {
    MemberID int64
    DeviceID int64 // 0 for a web session or the CLI
    Via      string // "caldav", "web", "import"
}

type Object struct {
    Name       string    // "<x>.ics", unique in the calendar
    UID        string
    ETag       string    // quoted
    Data       []byte    // iCalendar text as stored
    CreatedBy  int64
    ModifiedBy int64
    CreatedAt  time.Time
    ModifiedAt time.Time
}

type Precondition struct {
    IfMatch     string // "" = none; a quoted etag, or "*"
    IfNoneMatch bool   // If-None-Match: *
}

type Change struct {
    Seq     int64
    Name    string
    Deleted bool
    By      int64 // member id
}

var (
    ErrNotFound          = errors.New("not found")
    ErrPrecondition      = errors.New("precondition failed")  // 412
    ErrUIDConflict       = errors.New("uid conflict")         // 403 no-uid-conflict
    ErrInvalidData       = errors.New("invalid calendar data")// 403 valid-calendar-data
    ErrUnsupportedComp   = errors.New("unsupported component")// 403 supported-calendar-component
    ErrTooLarge          = errors.New("too large")            // 403 max-resource-size
    ErrTokenExpired      = errors.New("sync token expired")   // 403 valid-sync-token
)

type Service struct{ /* ... */ }

func (s *Service) Info(ctx) (Info, error)                     // name, colour, zone, Seq
func (s *Service) Get(ctx, name string) (*Object, error)
func (s *Service) List(ctx) ([]Object, error)                 // ETag and Name only, Data nil
func (s *Service) GetMany(ctx, names []string) ([]Object, error)
func (s *Service) Range(ctx, from, to time.Time) ([]Object, error) // candidates by index
func (s *Service) ChangesSince(ctx, seq int64, limit int) ([]Change, int64, error)
func (s *Service) Put(ctx, a Actor, name string, data []byte, p Precondition) (obj *Object, created bool, err error)
func (s *Service) Delete(ctx, a Actor, name string, p Precondition) error
```

`Put` and `Delete` are the only functions that write `objects` and
`changes`. After commit they call `hub.Publish(Change)`.

### 1.4 ical

```go
package ical

type Index struct {
    UID       string
    Summary   string
    Start     time.Time  // first instance start, UTC
    End       *time.Time // last instance end, UTC; nil = open-ended
    Recurring bool
    AllDay    bool
}

func Validate(data []byte, maxBytes int) (*ical.Calendar, Index, error)
func Expand(cal *ical.Calendar, loc *time.Location, from, to time.Time, max int) ([]Occurrence, error)
func Overlaps(cal *ical.Calendar, loc *time.Location, from, to time.Time) (bool, error)
func FromWeb(prev *ical.Calendar, e dto.EventInput, scope Scope, occ *time.Time, loc *time.Location, now time.Time) (Result, error)

type Scope string // "this", "following", "all"

type Result struct { // what FromWeb asks the calendar to write
    Put    []NamedData // the edited object, and for "following" the new one
    Delete []string
}
```

### 1.5 auth

```go
type Principal struct {
    Member   members.Member
    DeviceID int64  // 0 for a session
    Access   string // "readwrite" | "read"
}

func (s *Service) Basic(r *http.Request) (*Principal, error)   // /dav/ only
func (s *Service) Session(r *http.Request) (*Principal, error) // web only
func (s *Service) SameOrigin(r *http.Request) bool
func (s *Service) ClientIP(r *http.Request) netip.Addr
func ContextPrincipal(ctx context.Context) *Principal
```

---

## 2. Routes

| Method | Path | Auth | Owner |
|---|---|---|---|
| GET | `/healthz` | none | cmd |
| GET | `/.well-known/caldav` | none (301 to `/dav/`) | dav |
| any DAV | `/dav/...` | Basic, device password | dav |
| GET | `/ics/{secret}` | the secret | dav |
| GET, POST | `/signin` | none; rate limited | pages |
| POST | `/signout` | session, same-origin | pages |
| GET, POST | `/invite/{token}` | the token | pages |
| GET | `/`, `/devices`, `/members`, `/me` | session (`/members`: admin) | pages |
| GET | `/static/...` | none | web assets |
| GET | `/api/me` | session | api |
| PATCH | `/api/me` | session, same-origin | api |
| POST | `/api/me/password` | session, same-origin | api |
| GET | `/api/calendar` | session | api |
| GET | `/api/occurrences?from=&to=` | session | api |
| GET | `/api/events/{name}` | session | api |
| POST | `/api/events` | session, same-origin | api |
| PUT | `/api/events/{name}` | session, same-origin, `If-Match` required | api |
| DELETE | `/api/events/{name}` | session, same-origin, `If-Match` required | api |
| GET | `/api/members` | session | api |
| POST | `/api/members` | admin, same-origin | api |
| PATCH, DELETE | `/api/members/{name}` | admin, same-origin | api |
| POST | `/api/members/{name}/invitation` | admin, same-origin | api |
| GET, POST | `/api/devices` | session (POST same-origin) | api |
| DELETE | `/api/devices/{id}` | session, same-origin, own device (or admin) | api |
| GET | `/api/devices/{id}/mobileconfig` | session, own device, only within 10 min of creation | api |
| GET, POST | `/api/links` | session (POST same-origin) | api |
| DELETE | `/api/links/{id}` | session, same-origin, own link (or admin) | api |
| GET | `/api/live` | session | sse |

A session request that is not same-origin and changes state: `403
{"error":"cross_origin"}`. A route that needs a session without one:
`401` for `/api/*`, a redirect to `/signin` for pages.

---

## 3. JSON API

### 3.1 Errors

```json
{ "error": "precondition_failed", "message": "changed meanwhile", "by": "lea" }
```

| Status | `error` |
|---|---|
| 400 | `bad_request` (with `field`) |
| 401 | `unauthenticated` |
| 403 | `forbidden`, `cross_origin` |
| 404 | `not_found` |
| 409 | `uid_conflict` |
| 412 | `precondition_failed` (with `by`, the member who last changed it) |
| 413 | `too_large` |
| 422 | `invalid_event` (with `field`) |
| 429 | `rate_limited` (with `retry_after` seconds) |

### 3.2 Reading

`GET /api/calendar`:

```json
{ "name": "Famille", "color": "#3b6ea5", "timezone": "Europe/Paris", "seq": 812 }
```

`GET /api/occurrences?from=2026-11-23&to=2026-12-01` (dates in the
calendar's zone, `to` exclusive, at most 62 days):

```json
{
  "seq": 812,
  "occurrences": [
    {
      "name": "9f1c….ics",
      "etag": "\"a1b2…\"",
      "uid": "9f1c…@agendouille",
      "recurrence_id": "2026-11-25T14:00:00",
      "summary": "Judo",
      "location": "Gymnase",
      "all_day": false,
      "start": "2026-11-25T14:00:00",
      "end": "2026-11-25T15:30:00",
      "recurring": true,
      "overridden": false,
      "created_by": "maman",
      "modified_by": "lea"
    }
  ],
  "truncated": false
}
```

`recurrence_id` is null for a non-recurring event. For all-day events
`start` and `end` are dates (`end` exclusive).

`GET /api/events/{name}` returns the event as the editor needs it, with
`ETag` as a header and in the body:

```json
{
  "name": "9f1c….ics",
  "etag": "\"a1b2…\"",
  "summary": "Judo",
  "location": "Gymnase",
  "description": "",
  "all_day": false,
  "start": "2026-09-02T14:00:00",
  "end": "2026-09-02T15:30:00",
  "rrule": "FREQ=WEEKLY;BYDAY=WE;UNTIL=20270630T215959Z",
  "exdates": ["2026-12-23T14:00:00"],
  "overrides": [ { "recurrence_id": "2027-01-06T14:00:00", "start": "2027-01-06T15:00:00", "end": "2027-01-06T16:30:00", "summary": "Judo" } ],
  "has_alarms": true,
  "has_attendees": false,
  "created_by": "maman", "created_at": "…",
  "modified_by": "lea",  "modified_at": "…",
  "editable_rrule": true
}
```

`editable_rrule` is false when the rule uses parts the editor cannot show
(`BYSETPOS`, `BYWEEKNO`, several `BYMONTH`…); the editor then keeps the rule
and only offers "this" and "all" with no repeat change.

### 3.3 Writing

`EventInput` (POST body, PUT body):

```json
{
  "summary": "Dentiste",
  "location": "",
  "description": "",
  "all_day": false,
  "start": "2026-11-26T17:30:00",
  "end": "2026-11-26T18:00:00",
  "rrule": null
}
```

`PUT /api/events/{name}?scope=this|following|all&occurrence=<local time or date>`:
`scope` and `occurrence` are required when the event is recurring.
`DELETE` takes the same query. Both need `If-Match` with the etag the form
started from.

Answers: `201` / `200` with the event (3.2 shape) and its `ETag`; for
`following`, also `"created": "<new name>"`.

### 3.4 Members, devices, links

```json
// GET /api/me, members list items
{ "name": "lea", "display_name": "Léa", "color": "#c2185b", "admin": false,
  "disabled": false, "former": false }

// POST /api/devices  { "label": "Pixel de Léa", "access": "readwrite" }
// answer: the only time the password is ever sent
{ "id": 7, "label": "Pixel de Léa", "access": "readwrite",
  "password": "agd_k3jd-…",
  "davx5_uri": "davx5://lea:agd_k3jd-…@agenda.example.org/dav/",
  "qr_svg": "<svg …>",
  "mobileconfig_url": "/api/devices/7/mobileconfig",
  "manual": { "server": "https://agenda.example.org/dav/", "username": "lea",
              "calendar_url": "https://agenda.example.org/dav/calendars/lea/family/" } }

// GET /api/devices items
{ "id": 7, "label": "Pixel de Léa", "access": "readwrite",
  "created_at": "…", "last_used_at": "…", "last_ip": "203.0.113.4" }

// POST /api/links { "label": "Télé" } -> { "id": 2, "label": "Télé", "url": "https://…/ics/<secret>.ics" }
```

`qr_svg` is an SVG built by Go from `rsc.io/qr` with no text nodes; the
bundle inserts it as an `<img src="data:image/svg+xml,…">`, never as markup.

---

## 4. CalDAV surface

Namespaces: `D` = `DAV:`, `C` = `urn:ietf:params:xml:ns:caldav`,
`CS` = `http://calendarserver.org/ns/`, `A` = `http://apple.com/ns/ical/`.

### 4.1 Resources and their properties

| Property | `/dav/` | principal | home | calendar | object |
|---|---|---|---|---|---|
| `D:resourcetype` | collection | collection, `D:principal` | collection | collection, `C:calendar` | empty |
| `D:current-user-principal` | ✓ | ✓ | ✓ | ✓ | ✓ |
| `D:principal-URL` | | ✓ | | | |
| `D:displayname` | | member's display name | | calendar name (or the member's PROPPATCH) | |
| `C:calendar-home-set` | | `/dav/calendars/<m>/` | | | |
| `C:calendar-user-address-set` | | `mailto:` none; `/dav/principals/<m>/` | | | |
| `D:owner` | | | | first admin's principal | |
| `D:current-user-privilege-set` | | | `read` | `read` + (`write`, `write-content`, `bind`, `unbind` if readwrite) | same as calendar |
| `D:supported-report-set` | | | | multiget, query, sync-collection | |
| `C:supported-calendar-component-set` | | | | `VEVENT` | |
| `C:supported-calendar-data` | | | | `text/calendar` 2.0 | |
| `C:max-resource-size` | | | | `max_object_bytes` | |
| `C:calendar-timezone` | | | | a `VTIMEZONE` for `timezone` | |
| `CS:getctag` | | | | change counter | |
| `D:sync-token` | | | | the sync token | |
| `A:calendar-color` | | | | calendar colour (or the member's) | |
| `A:calendar-order` | | | | the member's, else 0 | |
| `D:getetag` | | | | | ✓ |
| `D:getcontenttype` | | | | `text/calendar; charset=utf-8; component=vcalendar` | `text/calendar; charset=utf-8; component=vevent` |
| `D:getcontentlength`, `D:getlastmodified` | | | | | ✓ |
| `C:calendar-data` | | | | | in REPORTs, and PROPFIND if asked |

`allprop` returns the above but `C:calendar-data`. Unknown properties are
`404` in the multistatus `propstat`.

### 4.2 Errors

Precondition failures answer with an XML body
`<D:error xmlns:D="DAV:" xmlns:C="…caldav"><C:valid-calendar-data/></D:error>`:

| Status | Element | When |
|---|---|---|
| 401 | (`WWW-Authenticate: Basic realm="agendouille", charset="UTF-8"`) | no or bad credentials |
| 403 | `D:need-privileges` | `read` device writes |
| 403 | `D:propfind-finite-depth` | `Depth: infinity` |
| 403 | `C:supported-calendar-component` | not `VEVENT` |
| 403 | `C:valid-calendar-data` | does not parse or validate |
| 403 | `C:no-uid-conflict` (with `D:href` of the holder) | UID already used by another name |
| 403 | `C:max-resource-size` | too large |
| 403 | `C:supported-filter` | a filter outside 5.2 |
| 403 | `D:valid-sync-token` | token unknown or expired |
| 404 | | outside the member's home, or no such object |
| 412 | | `If-Match` / `If-None-Match` failed |
| 415 | | `Content-Type` not `text/calendar` |
| 507 | (in the multistatus, `D:number-of-matches-within-limits`) | sync-collection `limit` hit |

### 4.3 Sync token

`https://<public host>/ns/sync/<n>`. Any other string, a host that is not
ours, or `<n>` below the oldest kept change: `403 D:valid-sync-token`. An
empty token: every object as a `200` response, then the current token.

---

## 5. Schema (end state of the v1 migrations)

```sql
CREATE TABLE members (
  id            INTEGER PRIMARY KEY,
  name          TEXT NOT NULL UNIQUE,
  display_name  TEXT NOT NULL,
  color         TEXT NOT NULL,
  admin         INTEGER NOT NULL DEFAULT 0,
  password_hash TEXT,                 -- NULL until the invitation is used
  disabled      INTEGER NOT NULL DEFAULT 0,
  removed_at    TEXT,                 -- "former member": events kept, no access
  created_at    TEXT NOT NULL
);

CREATE TABLE invitations (
  id         INTEGER PRIMARY KEY,
  member_id  INTEGER NOT NULL REFERENCES members(id) ON DELETE CASCADE,
  token_hash BLOB NOT NULL UNIQUE,
  expires_at TEXT NOT NULL,
  used_at    TEXT
);

CREATE TABLE sessions (
  id          INTEGER PRIMARY KEY,
  member_id   INTEGER NOT NULL REFERENCES members(id) ON DELETE CASCADE,
  token_hash  BLOB NOT NULL UNIQUE,
  created_at  TEXT NOT NULL,
  seen_at     TEXT NOT NULL,
  user_agent  TEXT NOT NULL DEFAULT ''
);

CREATE TABLE devices (
  id            INTEGER PRIMARY KEY,
  member_id     INTEGER NOT NULL REFERENCES members(id) ON DELETE CASCADE,
  label         TEXT NOT NULL,
  access        TEXT NOT NULL CHECK (access IN ('readwrite','read')),
  password_hash BLOB NOT NULL UNIQUE, -- sha256
  created_at    TEXT NOT NULL,
  last_used_at  TEXT,
  last_ip       TEXT,
  revoked_at    TEXT
);

CREATE TABLE links (
  id          INTEGER PRIMARY KEY,
  member_id   INTEGER NOT NULL REFERENCES members(id) ON DELETE CASCADE,
  label       TEXT NOT NULL,
  secret_hash BLOB NOT NULL UNIQUE,
  created_at  TEXT NOT NULL,
  last_used_at TEXT,
  revoked_at  TEXT
);

CREATE TABLE calendars (
  id        INTEGER PRIMARY KEY,
  slug      TEXT NOT NULL UNIQUE,      -- 'family'
  name      TEXT NOT NULL,
  color     TEXT NOT NULL,
  timezone  TEXT NOT NULL,
  seq       INTEGER NOT NULL DEFAULT 0 -- change counter
);

CREATE TABLE calendar_prefs (           -- per-member PROPPATCH results
  calendar_id INTEGER NOT NULL REFERENCES calendars(id),
  member_id   INTEGER NOT NULL REFERENCES members(id) ON DELETE CASCADE,
  displayname TEXT, color TEXT, sort_order INTEGER,
  PRIMARY KEY (calendar_id, member_id)
);

CREATE TABLE objects (
  id           INTEGER PRIMARY KEY,
  calendar_id  INTEGER NOT NULL REFERENCES calendars(id),
  name         TEXT NOT NULL,
  uid          TEXT NOT NULL,
  etag         TEXT NOT NULL,
  data         BLOB NOT NULL,
  summary      TEXT NOT NULL DEFAULT '',
  start_utc    TEXT NOT NULL,
  end_utc      TEXT,                   -- NULL = open-ended recurrence
  recurring    INTEGER NOT NULL,
  all_day      INTEGER NOT NULL,
  created_by   INTEGER NOT NULL REFERENCES members(id),
  modified_by  INTEGER NOT NULL REFERENCES members(id),
  created_via  TEXT NOT NULL,
  modified_via TEXT NOT NULL,
  device_id    INTEGER REFERENCES devices(id) ON DELETE SET NULL,
  created_at   TEXT NOT NULL,
  modified_at  TEXT NOT NULL,
  UNIQUE (calendar_id, name),
  UNIQUE (calendar_id, uid)
);
CREATE INDEX objects_range ON objects (calendar_id, start_utc, end_utc);

CREATE TABLE changes (
  seq         INTEGER PRIMARY KEY,     -- = calendars.seq after the write
  calendar_id INTEGER NOT NULL REFERENCES calendars(id),
  name        TEXT NOT NULL,
  deleted     INTEGER NOT NULL,
  member_id   INTEGER NOT NULL REFERENCES members(id),
  at          TEXT NOT NULL
);
CREATE INDEX changes_name ON changes (calendar_id, name, seq);

CREATE TABLE settings (key TEXT PRIMARY KEY, value TEXT NOT NULL); -- VAPID keys (M6), ...
```

Members are never deleted from `members` (only `removed_at` is set), so
`created_by` keeps pointing somewhere.

---

## 6. SSE (`/api/live`)

```
event: hello
data: {"seq":812}

event: changed
data: {"seq":813,"name":"9f1c….ics","deleted":false,"by":"lea"}

event: ping            (every 25 s)
data: {}
```

A client that reconnects with `Last-Event-ID: <seq>` gets `changed` for
every change after it, or a single `resync` event if they are pruned or
more than 500. The bundle reloads the visible range on `changed` and
`resync`.
