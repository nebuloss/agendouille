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
| `internal/surface` | the `Surface` type (`dav`, `app`, `admin`), listeners, per-surface muxes, the `404` for surfaces a listener does not serve, each surface's public URL | config |
| `internal/members` | members, roles, password hashing, the last-admin rule | store |
| `internal/access` | calendars' metadata and the access table; `Access(member, calendar)`; the activity log of admin actions | store |
| `internal/auth` | sessions per surface, sign-in links, approval codes, devices, subscription links, Basic auth, rate limits, client IP, same-origin | store, members, surface |
| `internal/ical` | parse, validate, index, expand, patch; pure (no store) | go-ical, rrule-go |
| `internal/calendar` | the calendar service (1.3): objects, changes, tokens, import/export, backup | store, ical, hub, access |
| `internal/hub` | in-process fan-out of `calendar.Change` | — |
| `internal/dav` | surface `dav`: `/dav/`, `/.well-known/caldav`, `/ics/` | calendar, access, auth, ical |
| `internal/devices` | QR payload, `.mobileconfig`, the devices page data | auth |
| `internal/dto` | every type of section 3 and 6 | — |
| `internal/web/webx` | layouts, JSON errors, same-origin middleware, static serving per surface | surface, dto |
| `internal/web/app` | surface `app`: family pages and `/api/*` but `/api/live` | calendar, access, members, auth, devices, ical, dto, webx |
| `internal/web/sse` | surface `app`: `/api/live` | hub, access, auth, dto |
| `internal/web/admin` | surface `admin`: admin pages and `/api/admin/*` | calendar, access, members, auth, devices, dto, webx |

`internal/dav`, `internal/web/app` and `internal/web/admin` never import
each other (a test walks the imports, as claudication's `arch` test). Nothing below
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
    Access   *access.Service
    Surfaces *surface.Set
    Clock    func() time.Time
}

// Every HTTP feature package registers on its own surface's mux only:
func Register(mux *http.ServeMux, d *deps.Deps) // mux = d.Surfaces.Mux(surface.App), ...
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
    Seq      int64
    Calendar string // slug
    Name     string
    Summary  string // at the time of the change, for the activity log
    Deleted  bool
    By       int64 // member id
    DeviceID int64
    Via      string
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

// cal is a calendar slug. Every call checks the actor's access on it
// (none -> ErrNotFound, read -> ErrForbidden on writes) with access.Service;
// the CLI's Actor is an admin with readwrite everywhere.
func (s *Service) Info(ctx, cal string) (Info, error)          // name, colour, zone, Seq
func (s *Service) Get(ctx, a Actor, cal, name string) (*Object, error)
func (s *Service) List(ctx, a Actor, cal string) ([]Object, error) // ETag and Name only
func (s *Service) GetMany(ctx, a Actor, cal string, names []string) ([]Object, error)
func (s *Service) Range(ctx, a Actor, cals []string, from, to time.Time) ([]Object, error)
func (s *Service) ChangesSince(ctx, a Actor, cal string, seq int64, limit int) ([]Change, int64, error)
func (s *Service) Put(ctx, a Actor, cal, name string, data []byte, p Precondition) (obj *Object, created bool, err error)
func (s *Service) Delete(ctx, a Actor, cal, name string, p Precondition) error
func (s *Service) Move(ctx, a Actor, from, name, to string, p Precondition) (*Object, error) // one transaction
func (s *Service) Activity(ctx, f ActivityFilter) ([]Change, error) // admin UI
```

`Put`, `Delete` and `Move` are the only functions that write `objects` and
`changes`. After commit they call `hub.Publish(Change)`. `ErrForbidden`
joins the errors above (`403 D:need-privileges` / `forbidden`).

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
    Member    members.Member
    DeviceID  int64           // 0 for a session
    DeviceCap string          // "readwrite" | "read"; "readwrite" for a session
    Surface   surface.Surface // where it was authenticated
}

func (s *Service) Basic(r *http.Request) (*Principal, error)                     // surface dav only
func (s *Service) Session(r *http.Request, sf surface.Surface) (*Principal, error) // agd_app or agd_admin; admin requires the admin role
func (s *Service) RedeemSigninLink(ctx, token string, r *http.Request) (*http.Cookie, error)
func (s *Service) StartApproval(ctx, r *http.Request) (Approval, error) // code + QR for the app's /signin
func (s *Service) Approve(ctx, p *Principal, code string) error                              // from a signed-in phone
func (s *Service) SameOrigin(r *http.Request) bool
func (s *Service) ClientIP(r *http.Request) netip.Addr
func ContextPrincipal(ctx context.Context) *Principal
```

The effective access on a calendar is `min(access.Access(member, cal),
p.DeviceCap)`, computed in one place, `access.Effective`.

---

## 2. Routes

Every route belongs to one surface (PLAN 9.1). On a listener that does not
serve its surface, it answers `404` with an empty body. `/healthz` is on
every listener.

### 2.1 Surface `dav`

| Method | Path | Auth |
|---|---|---|
| GET | `/.well-known/caldav` | none (301 to `/dav/`) |
| any DAV | `/dav/...` | Basic, device password |
| GET | `/ics/{secret}.ics` | the secret |

### 2.2 Surface `app` (the family app)

| Method | Path | Auth |
|---|---|---|
| GET, POST | `/signin` | none; rate limited. GET also starts an approval (QR) |
| GET | `/signin/wait/{id}` | none; long-poll until the approval is used or expires, then sets the cookie |
| GET, POST | `/login/{token}` | the sign-in link (POST redeems it; GET only shows "Continue", so link previews do not burn it) |
| GET, POST | `/approve/{code}` | `agd_app` session, same-origin |
| POST | `/signout` | `agd_app`, same-origin |
| GET | `/`, `/devices`, `/me` | `agd_app` (else redirect to `/signin`) |
| GET | `/static/app/...`, `/static/shared/...` | none |
| GET | `/api/me` | `agd_app` |
| PATCH | `/api/me` | `agd_app`, same-origin; `display_name`, `color` |
| PUT, DELETE | `/api/me/password` | `agd_app`, same-origin; `current` required if one is set |
| GET | `/api/me/sessions` ; DELETE `/api/me/sessions/{id}` | `agd_app` (DELETE same-origin) |
| POST | `/api/me/signin-link` | `agd_app`, same-origin; a 2-minute link for another device |
| GET | `/api/members` | `agd_app`; names, display names, colours only |
| GET | `/api/calendars` | `agd_app`; the readable ones with `access` and `shown` |
| PATCH | `/api/calendars/{slug}/prefs` | `agd_app`, same-origin; `shown` |
| GET | `/api/occurrences?from=&to=&calendars=` | `agd_app` |
| GET | `/api/calendars/{slug}/events/{name}` | `agd_app` |
| POST | `/api/calendars/{slug}/events` | `agd_app`, same-origin, readwrite |
| PUT, DELETE | `/api/calendars/{slug}/events/{name}` | `agd_app`, same-origin, readwrite, `If-Match` required |
| POST | `/api/calendars/{slug}/events/{name}/move` | `agd_app`, same-origin, readwrite on both, `If-Match`; body `{"to":"<slug>"}` |
| GET, POST | `/api/devices` | `agd_app` (POST same-origin); own devices |
| DELETE | `/api/devices/{id}` | `agd_app`, same-origin, own device |
| GET | `/api/devices/{id}/mobileconfig` | `agd_app`, own device, within 10 minutes of creation |
| GET, POST | `/api/links` ; DELETE `/api/links/{id}` | `agd_app` (writes same-origin); own links |
| GET | `/api/live` | `agd_app` |

### 2.3 Surface `admin`

| Method | Path | Auth |
|---|---|---|
| GET, POST | `/admin/signin` | none; rate limited; admins, password only |
| POST | `/admin/signout` | `agd_admin`, same-origin |
| GET | `/admin`, `/admin/members[/{name}]`, `/admin/calendars[/{slug}]`, `/admin/activity`, `/admin/system` | `agd_admin` |
| GET | `/static/admin/...`, `/static/shared/...` | none |
| | `/api/admin/*` | `agd_admin`; writes same-origin (3.5) |

A session request that is not same-origin and changes state: `403
{"error":"cross_origin"}`. Without a session: `401` for `/api/*`, a redirect
to the surface's sign-in for pages. An `agd_app` cookie on the admin
surface is ignored, never upgraded.

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

`GET /api/calendars`:

```json
{ "family_timezone": "Europe/Paris",
  "calendars": [
    { "slug": "famille", "name": "Famille", "color": "#3b6ea5", "timezone": "Europe/Paris",
      "description": "", "access": "readwrite", "shown": true, "seq": 812 },
    { "slug": "ecole", "name": "École", "color": "#2e7d32", "timezone": "Europe/Paris",
      "description": "", "access": "read", "shown": true, "seq": 790 } ] }
```

`GET /api/occurrences?from=2026-11-23&to=2026-12-01&calendars=famille,ecole`
(dates in the family zone, `to` exclusive, at most 62 days; `calendars`
defaults to the shown ones):

```json
{
  "seq": 812,
  "occurrences": [
    {
      "calendar": "famille",
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

`GET /api/calendars/{slug}/events/{name}` returns the event as the editor needs it, with
`ETag` as a header and in the body:

```json
{
  "calendar": "famille",
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

`PUT /api/calendars/{slug}/events/{name}?scope=this|following|all&occurrence=<local time or date>`:
`scope` and `occurrence` are required when the event is recurring.
`DELETE` takes the same query. Both need `If-Match` with the etag the form
started from.

Answers: `201` / `200` with the event (3.2 shape) and its `ETag`; for
`following`, also `"created": "<new name>"`.

### 3.4 Me, members, devices, links (surface `app`)

```json
// GET /api/me
{ "name": "lea", "display_name": "Léa", "color": "#c2185b", "admin": false,
  "has_password": false }

// GET /api/members items (what every member may know about the others)
{ "name": "lea", "display_name": "Léa", "color": "#c2185b", "former": false }

// POST /api/devices  { "label": "Pixel de Léa", "cap": "readwrite" }
// answer: the only time the password is ever sent
{ "id": 7, "label": "Pixel de Léa", "cap": "readwrite",
  "password": "agd_k3jd-…",
  "davx5_uri": "davx5://lea:agd_k3jd-…@agenda.example.org/dav/",
  "qr_svg": "<svg …>",
  "mobileconfig_url": "/api/devices/7/mobileconfig",
  "manual": { "server": "https://agenda.example.org/dav/", "username": "lea" } }

// GET /api/devices items
{ "id": 7, "label": "Pixel de Léa", "cap": "readwrite",
  "created_at": "…", "last_used_at": "…", "last_ip": "203.0.113.4", "client": "DAVx5/4.5" }

// POST /api/links { "label": "Télé", "calendars": ["famille","ecole"] }
// -> { "id": 2, "label": "Télé", "calendars": [...], "url": "https://…/ics/<secret>.ics" }

// POST /api/me/signin-link -> { "url": "https://…/login/<token>", "qr_svg": "<svg …>", "expires_at": "…" }
```

`qr_svg` is an SVG built by Go from `rsc.io/qr` with no text nodes; the
bundles insert it as an `<img src="data:image/svg+xml,…">`, never as markup.

### 3.5 Admin API (surface `admin`, `/api/admin`)

| Method | Path | Body / answer |
|---|---|---|
| GET | `/overview` | counts, stale devices, last backup, version, warnings |
| GET, POST | `/members` | list (3.5.1) ; create `{name, display_name, color, admin, access: {slug: access}}` -> member + a sign-in link |
| GET, PATCH | `/members/{name}` | detail ; `display_name`, `color`, `admin`, `disabled` |
| DELETE | `/members/{name}` | remove (former member) |
| PUT | `/members/{name}/access` | `{slug: "none"|"read"|"readwrite"}` (only the slugs given change) |
| POST | `/members/{name}/signin-links` | -> `{id, url, qr_svg, expires_at}` (72 h) |
| DELETE | `/members/{name}/signin-links/{id}` | revoke |
| DELETE | `/members/{name}/devices/{id}`, `/members/{name}/sessions/{id}`, `/members/{name}/links/{id}` | revoke |
| GET, POST | `/calendars` | list ; create `{slug, name, color, timezone, description, default_access, access: {member: access}}` |
| GET, PATCH | `/calendars/{slug}` | detail ; `name`, `color`, `timezone`, `description`, `default_access`, `archived` |
| DELETE | `/calendars/{slug}?confirm=<slug>` | only when archived |
| PUT | `/calendars/{slug}/access` | `{member: access}` |
| POST | `/calendars/{slug}/import` | `text/calendar` body -> `{added, skipped, replaced, errors:[…]}` (`?replace=1`) |
| GET | `/calendars/{slug}/export` | `text/calendar` |
| GET | `/activity?member=&calendar=&device=&before=` | 3.5.2, 100 per page |
| GET, POST | `/backups` | list ; make one now |
| GET | `/backups/{file}` | download |
| GET | `/system` | effective listeners and public URLs, version |

#### 3.5.1 Member (admin view)

```json
{ "name": "lea", "display_name": "Léa", "color": "#c2185b", "admin": false,
  "has_password": false, "disabled": false, "former": false,
  "created_at": "…", "last_seen_at": "…",
  "access": { "famille": "readwrite", "ecole": "read" },
  "devices": [ { "id": 7, "label": "Pixel de Léa", "cap": "readwrite", "last_used_at": "…", "client": "DAVx5/4.5" } ],
  "sessions": [ { "id": 3, "surface": "app", "user_agent": "…", "seen_at": "…" } ],
  "signin_links": [ { "id": 4, "created_at": "…", "expires_at": "…", "by": "papa" } ],
  "links": [ { "id": 2, "label": "Télé", "calendars": ["famille"] } ] }
```

#### 3.5.2 Activity item

```json
{ "at": "…", "kind": "event",  "action": "changed", "by": "lea",
  "calendar": "famille", "name": "9f1c….ics", "summary": "Judo",
  "via": "caldav", "device": "Pixel de Léa" }
{ "at": "…", "kind": "admin", "action": "access", "by": "papa",
  "detail": "lea: ecole read" }
```

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
| `D:displayname` | | member's display name | | the calendar's name (or the member's PROPPATCH) | |
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

`https://<dav public host>/ns/sync/<slug>/<n>`. Another calendar's token,
any other string, a host that is not
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
  password_hash TEXT,                 -- optional for members, required for admins
  disabled      INTEGER NOT NULL DEFAULT 0,
  removed_at    TEXT,                 -- "former member": events kept, no access
  created_at    TEXT NOT NULL
);

CREATE TABLE signin_links (            -- admin-made (72 h) or self-made (2 min)
  id          INTEGER PRIMARY KEY,
  member_id   INTEGER NOT NULL REFERENCES members(id) ON DELETE CASCADE,
  token_hash  BLOB NOT NULL UNIQUE,
  created_by  INTEGER NOT NULL REFERENCES members(id),
  created_at  TEXT NOT NULL,
  expires_at  TEXT NOT NULL,
  used_at     TEXT,
  revoked_at  TEXT
);

CREATE TABLE approvals (               -- the app's /signin QR, approved from a signed-in phone
  id          INTEGER PRIMARY KEY,
  code_hash   BLOB NOT NULL UNIQUE,
  user_agent  TEXT NOT NULL,
  ip          TEXT NOT NULL,
  created_at  TEXT NOT NULL,
  expires_at  TEXT NOT NULL,
  approved_by INTEGER REFERENCES members(id),
  used_at     TEXT
);

CREATE TABLE sessions (
  id          INTEGER PRIMARY KEY,
  member_id   INTEGER NOT NULL REFERENCES members(id) ON DELETE CASCADE,
  surface     TEXT NOT NULL CHECK (surface IN ('app','admin')),
  token_hash  BLOB NOT NULL UNIQUE,
  created_at  TEXT NOT NULL,
  seen_at     TEXT NOT NULL,
  user_agent  TEXT NOT NULL DEFAULT ''
);

CREATE TABLE devices (
  id            INTEGER PRIMARY KEY,
  member_id     INTEGER NOT NULL REFERENCES members(id) ON DELETE CASCADE,
  label         TEXT NOT NULL,
  cap           TEXT NOT NULL CHECK (cap IN ('readwrite','read')),
  password_hash BLOB NOT NULL UNIQUE, -- sha256
  client        TEXT NOT NULL DEFAULT '', -- last User-Agent seen
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
  id             INTEGER PRIMARY KEY,
  slug           TEXT NOT NULL UNIQUE,      -- 'famille'; never changes
  name           TEXT NOT NULL,
  color          TEXT NOT NULL,
  timezone       TEXT NOT NULL,
  description    TEXT NOT NULL DEFAULT '',
  default_access TEXT NOT NULL DEFAULT 'readwrite' CHECK (default_access IN ('none','read','readwrite')),
  archived_at    TEXT,
  deleted_at     TEXT,
  seq            INTEGER NOT NULL DEFAULT 0, -- last change in this calendar
  created_at     TEXT NOT NULL
);

CREATE TABLE link_calendars (
  link_id     INTEGER NOT NULL REFERENCES links(id) ON DELETE CASCADE,
  calendar_id INTEGER NOT NULL REFERENCES calendars(id) ON DELETE CASCADE,
  PRIMARY KEY (link_id, calendar_id)
);

CREATE TABLE calendar_access (          -- no row = 'none'
  calendar_id INTEGER NOT NULL REFERENCES calendars(id) ON DELETE CASCADE,
  member_id   INTEGER NOT NULL REFERENCES members(id) ON DELETE CASCADE,
  access      TEXT NOT NULL CHECK (access IN ('read','readwrite')),
  PRIMARY KEY (calendar_id, member_id)
);

CREATE TABLE calendar_prefs (           -- per member: shown in the app, PROPPATCH results
  calendar_id INTEGER NOT NULL REFERENCES calendars(id) ON DELETE CASCADE,
  member_id   INTEGER NOT NULL REFERENCES members(id) ON DELETE CASCADE,
  shown       INTEGER NOT NULL DEFAULT 1,
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
  seq         INTEGER PRIMARY KEY,     -- global; = calendars.seq of its calendar after the write
  calendar_id INTEGER NOT NULL REFERENCES calendars(id),
  name        TEXT NOT NULL,
  summary     TEXT NOT NULL DEFAULT '',
  action      TEXT NOT NULL CHECK (action IN ('created','changed','deleted')),
  member_id   INTEGER NOT NULL REFERENCES members(id),
  device_id   INTEGER REFERENCES devices(id) ON DELETE SET NULL,
  via         TEXT NOT NULL,           -- caldav | web | import
  at          TEXT NOT NULL
);
CREATE INDEX changes_cal ON changes (calendar_id, seq);

CREATE TABLE admin_log (               -- admin actions, for /admin/activity
  id        INTEGER PRIMARY KEY,
  at        TEXT NOT NULL,
  member_id INTEGER NOT NULL REFERENCES members(id),
  action    TEXT NOT NULL,             -- member_add, access, signin_link, calendar_add, ...
  detail    TEXT NOT NULL
);

CREATE TABLE settings (key TEXT PRIMARY KEY, value TEXT NOT NULL); -- VAPID keys (M6), ...
```

Deleting a calendar (only once archived) deletes its objects and sets
`deleted_at`; the row and its `changes` stay, so the activity log keeps its
names and an old sync token gets `404` rather than a different calendar.
Its slug is never reused.

Members are never deleted from `members` (only `removed_at` is set), so
`created_by` keeps pointing somewhere.

---

## 6. SSE (`/api/live`)

```
event: hello
data: {"seq":812}

event: changed
data: {"seq":813,"calendar":"famille","name":"9f1c….ics","action":"changed","by":"lea"}

event: access          (the member's access or the calendars list changed)
data: {}

event: ping            (every 25 s)
data: {}
```

Only changes on calendars the member may read are sent. A client that
reconnects with `Last-Event-ID: <seq>` gets `changed` for every such change
after it, or a single `resync` event if they are pruned or
more than 500. The app bundle reloads the visible range on `changed` and
`resync`, and the calendars list on `access`. The admin UI has no SSE in v1
(Default).
