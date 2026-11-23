# agendouille: contracts between parts

`PLAN.md` says what agendouille does. This file pins the **interfaces
between its parts**:

1. Go packages, ownership and interfaces.
2. HTTP routes per surface, with auth.
3. The additional JSON API (family web app, app) and the admin API.
4. The CalDAV surface: properties, methods, errors.
5. The SQLite schema.
6. The app bridge (page <-> Java).
7. SSE events.

**Rules for this file:**

- When `PLAN.md` and this file disagree on a contract, this file wins. Fix
  the other one in the same change.
- Changing a contract is a change to this file first, with a line in
  `DECISIONS.md`.
- Additions that break nothing (a new optional field, a new route) can be
  made by whoever needs them. Renaming or removing anything needs the
  user's agreement.
- **Sync never depends on section 3.** A CalDAV client with only section 4
  gets every calendar it may read, with the right access. Section 3 holds
  only what CalDAV has no standard for.
- JSON uses `snake_case`. Instants are RFC 3339 in UTC; local times are
  `YYYY-MM-DDTHH:MM:SS` without offset, in the family zone; dates are
  `YYYY-MM-DD`.
- A calendar is named `<owner>~<slug>` (its **ref**) everywhere outside
  the database.

---

## 1. Packages and Go interfaces

### 1.1 Ownership

| Package | Owns | Imports (besides stdlib) |
|---|---|---|
| `cmd/agendouille` | subcommands, wiring | everything |
| `internal/config` | the bootstrap struct | — |
| `internal/settings` | typed settings in the DB, change notifications | store |
| `internal/deps` | `Deps` (1.2) | the services |
| `internal/store` | `*sql.DB`, migrations, `Tx` | sqlite |
| `internal/directory` | users, groups, membership, `Sees(a, b)` | store |
| `internal/sharing` | ownership, shares, hidden flags, `Access(user, cal)` | store, directory |
| `internal/auth` | admin password and sessions, app sessions, enrolment links, approvals, devices, subscription links, Basic auth, client IP, same-origin | store, directory, settings |
| `internal/ical` | parse, encode, validate, index, RRULE, expand (pure, in-house) | — |
| `internal/calendar` | calendars and objects: the one write path (1.3), changes, tokens, quotas, import/export | store, ical, hub, sharing, settings |
| `internal/hub` | in-process fan-out of `calendar.Change` and access events | — |
| `internal/surface` | `Surface` (`dav`, `app`, `admin`), listeners, per-surface muxes, live rebinding, each surface's public URL | config, settings |
| `internal/backup` | snapshots, schedule, restore | store, settings |
| `internal/dav` | surface `dav` | calendar, sharing, auth, ical, surface |
| `internal/web/webx` | layouts, JSON errors, same-origin middleware, static per surface | surface, dto |
| `internal/web/app` | surface `app`: pages, `/api/*`, `/api/app/*`, `/enrol/`, the APK, `assetlinks.json` | calendar, sharing, directory, auth, ical, dto, webx |
| `internal/web/app` (`live.go`) | surface `app`: `/api/live` | hub, sharing, auth |
| `internal/web/admin` | surface `admin`: pages, `/api/admin/*` | directory, sharing, calendar (metadata calls only), auth, settings, backup, dto, webx |
| `internal/dto` | every JSON type of sections 3 and 7 | — |

`internal/dav`, `internal/web/app` and `internal/web/admin` never import
each other. `internal/web/admin` may call only the `calendar` functions
marked *metadata* (1.3). Both rules are checked by a test that walks the
imports and the calls (as claudication's `internal/arch`).

### 1.2 Deps

```go
type Deps struct {
    Config    *config.Config
    Settings  *settings.Service
    Log       *slog.Logger
    Store     *store.Store
    Directory *directory.Service
    Sharing   *sharing.Service
    Auth      *auth.Service
    Calendar  *calendar.Service
    Hub       *hub.Hub
    Surfaces  *surface.Set
    Backup    *backup.Service
    Clock     func() time.Time
}

// Each HTTP package registers on its own surface's mux only.
func Register(mux *http.ServeMux, d *deps.Deps)
```

### 1.3 The calendar service (the one write path)

```go
package calendar

type Actor struct {
    UserID   int64  // 0 = the admin (metadata calls only)
    DeviceID int64  // 0 for a web session
    Cap      string // "readwrite" | "read" (device cap; "readwrite" for sessions)
    Via      string // "caldav", "web", "app", "import", "admin", "cli"
}

type Ref struct{ Owner, Slug string } // "<owner>~<slug>"

type Object struct {
    Calendar   Ref
    Name       string // "<x>.ics"
    UID        string
    ETag       string // quoted
    Data       []byte // as stored
    CreatedBy, ModifiedBy int64
    CreatedAt, ModifiedAt time.Time
}

type Precondition struct {
    IfMatch     string // "", "*", or a quoted etag
    IfNoneMatch bool
}

type Change struct {
    Seq      int64
    Calendar Ref
    Name     string // "" for a calendar-level change
    Summary  string // never sent to the admin
    Action   string // created, changed, deleted, calendar_created, calendar_changed, calendar_deleted
    By       int64
    DeviceID int64
    Via      string
}

var (
    ErrNotFound        = errors.New("not found")                // 404 (also: no access)
    ErrForbidden       = errors.New("forbidden")                // 403 need-privileges
    ErrPrecondition    = errors.New("precondition failed")      // 412
    ErrUIDConflict     = errors.New("uid conflict")             // 403 no-uid-conflict
    ErrInvalidData     = errors.New("invalid calendar data")    // 403 valid-calendar-data
    ErrUnsupportedComp = errors.New("unsupported component")    // 403 supported-calendar-component
    ErrTooLarge        = errors.New("too large")                // 403 max-resource-size
    ErrQuota           = errors.New("quota exceeded")           // 507 / 403 quota
    ErrTokenExpired    = errors.New("sync token expired")       // 403 valid-sync-token
)

// Calendars. a.UserID 0 is the operator (CLI, admin metadata).
func (s *Service) Home(ctx, a Actor, withHidden bool) ([]Info, error)       // what the actor may read
func (s *Service) Info(ctx, a Actor, r Ref) (Info, error)                   // ErrNotFound without read access
func (s *Service) Create(ctx, a Actor, owner, slug string, p Props) (Info, error)
func (s *Service) Update(ctx, a Actor, r Ref, p Props) (Info, error)        // owner: for all; sharee: own view
func (s *Service) DeleteCalendar(ctx, a Actor, r Ref) error                 // owner or operator
func (s *Service) Transfer(ctx, a Actor, r Ref, to string, keep bool) (Info, error)
func (s *Service) Owned(ctx, userID int64) ([]Info, error)

// Objects
func (s *Service) Get(ctx, a Actor, r Ref, name string) (Info, Object, error)
func (s *Service) List(ctx, i Info) ([]Object, error)
func (s *Service) GetMany(ctx, i Info, names []string) (map[string]Object, error)
func (s *Service) Range(ctx, i Info, from, to time.Time) ([]Object, error)  // index, then expansion
func (s *Service) ChangesSince(ctx, i Info, seq int64, limit int) (Changes, error)
func (s *Service) Put(ctx, a Actor, r Ref, name string, data []byte, p Precondition) (Object, bool, error)
func (s *Service) Delete(ctx, a Actor, r Ref, name string, p Precondition) error
func (s *Service) Import(ctx, a Actor, r Ref, data []byte, replace bool) (ImportResult, error)
func (s *Service) Export(ctx, a Actor, r Ref) ([]byte, error)
func (s *Service) Prune(ctx) (int64, error)

// Tokens: https://<public host>/ns/sync/<calendar id>/<seq>
func Token(base string, i Info) string
func ParseToken(base string, i Info, token string) (int64, error)

// The admin's metadata calls (internal/web/admin may call only these):
func (s *Service) AdminCalendars(ctx, owner string) ([]CalendarMeta, error) // "" for all
func (s *Service) AdminSharedWith(ctx, userID int64) ([]SharedWith, error)
func (s *Service) AdminActivity(ctx, f ActivityFilter) ([]Activity, error)  // never a summary
func (s *Service) AdminCounts(ctx) (Counts, error)

// Not built yet: Move (M4).
```

Only `Create`, `Update`, `DeleteCalendar`, `Transfer`, `Put`, `Delete`,
`Move` and `Import` write `calendars`, `objects` and `changes`. After
commit they publish on the hub.

### 1.4 sharing

```go
type Access string // "none", "read", "readwrite", "owner"

func (s *Service) Access(ctx, userID int64, calID int64) (Access, error) // highest of owner, user share, group shares
func Effective(a Access, deviceCap string) Access                        // the lower of the two
func (s *Service) Share(ctx, owner int64, calID int64, t Target, a Access) error // t.User or t.Group; owner must see t
func (s *Service) Unshare(ctx, owner int64, calID int64, t Target) error
func (s *Service) SetHidden(ctx, userID, calID int64, hidden bool) error
```

### 1.5 auth

```go
type Principal struct {
    User      directory.User // zero for the admin
    Admin     bool
    DeviceID  int64          // 0 for a session
    DeviceCap string         // "readwrite" | "read"
    Surface   surface.Surface
}

func (s *Service) Basic(r *http.Request) (*Principal, error)                          // dav, and /api/app/session
func (s *Service) Session(r *http.Request, sf surface.Surface) (*Principal, error)    // agd_app | agd_admin
func (s *Service) AdminSignIn(ctx, password string, r *http.Request) (*http.Cookie, error)
func (s *Service) NewEnrolment(ctx, by Principal, user string, ttl time.Duration) (Enrolment, error)
func (s *Service) RedeemEnrolmentForApp(ctx, token string, device DeviceInfo) (DeviceSecret, error)
func (s *Service) RedeemEnrolmentForBrowser(ctx, token string, r *http.Request) (*http.Cookie, error)
func (s *Service) StartApproval(ctx, r *http.Request) (Approval, error)  // id, code, short code, number
func (s *Service) LookupApproval(ctx, p *Principal, code string) (ApprovalView, error) // UA, IP, three numbers
func (s *Service) Approve(ctx, p *Principal, code string, number int) error
func (s *Service) WaitApproval(ctx, id string) (*http.Cookie, error)     // long-poll
func (s *Service) AppSession(ctx, p *Principal) (*http.Cookie, error)    // device kind "app" only
func (s *Service) NewDevice(ctx, p *Principal, kind, label, cap string) (DeviceSecret, error)
```

---

## 2. Routes

Every route belongs to one surface (PLAN 9.1); on a listener that does not
serve it, it is `404` with an empty body. `/healthz` is on every listener.

### 2.1 Surface `dav`

| Method | Path | Auth |
|---|---|---|
| GET | `/.well-known/caldav` | none (301 to `/dav/`) |
| DAV methods | `/dav/...` | Basic, device credential |
| GET | `/ics/{secret}.ics` | the secret |

### 2.2 Surface `app`: pages and the additional API

| Method | Path | Auth |
|---|---|---|
| GET | `/signin` | none (starts an approval) |
| GET | `/signin/wait/{id}` | none; long-poll (25 s): `{"status": "approved"}` with the `agd_app` cookie, `pending`, or 410 `cancelled`/`expired`/`used`; `?then=redirect` answers 303 to `/` or back to `/signin` instead, for a browser without the bundle |
| GET, POST | `/enrol/{token}` | the token (GET shows "Continue"; POST redeems for this browser, so link previews do not burn it) |
| GET | `/.well-known/assetlinks.json` | none |
| GET | `/app/agendouille.apk`, `/app/version` | none |
| GET | `/`, `/calendars`, `/approve`, `/devices`, `/me` | `agd_app` (else to `/signin`) |
| POST | `/approve` | `agd_app`, same-origin; the form flow of `/api/approvals/{code}`: `code` alone shows the three numbers, `code` and `number` approves |
| GET | `/static/app/...`, `/static/shared/...` | none |
| POST | `/signout` | `agd_app`, same-origin |
| GET, PATCH | `/api/me` | `agd_app` (PATCH same-origin) |
| POST | `/api/me/enrolment` | `agd_app`, same-origin: a 10-minute enrolment link for one's own new phone |
| GET | `/api/people` | `agd_app`: the users and groups I see |
| GET, POST | `/api/calendars` | `agd_app` (POST same-origin): my home ; create |
| GET, PATCH, DELETE | `/api/calendars/{ref}` | `agd_app` (writes same-origin); PATCH by a sharee changes only `hidden` and their own view |
| GET, PUT, DELETE | `/api/calendars/{ref}/shares[/{target}]` | owner; `target` = `u:<user>` or `g:<group>` |
| POST | `/api/calendars/{ref}/transfer` | owner, same-origin |
| POST | `/api/calendars/{ref}/import` ; GET `/api/calendars/{ref}/export` | readwrite ; read |
| GET | `/api/occurrences?from=&to=&calendars=` | `agd_app` |
| GET | `/api/calendars/{ref}/events/{name}` | read |
| POST | `/api/calendars/{ref}/events` | readwrite, same-origin |
| PUT, DELETE | `/api/calendars/{ref}/events/{name}` | readwrite, same-origin, `If-Match` |
| POST | `/api/calendars/{ref}/events/{name}/move` | readwrite on both, same-origin, `If-Match` |
| GET | `/api/approvals/{code}` | `agd_app`: what is asking (UA, IP, three numbers) |
| POST | `/api/approvals/{code}` | `agd_app`, same-origin: `{"number": 42}` |
| GET, POST | `/api/devices` ; DELETE `/api/devices/{id}` | `agd_app` (writes same-origin); own devices |
| GET | `/api/devices/{id}/mobileconfig?t=` | own device: `agd_app` and the one-time token the POST answered, within 10 minutes |
| GET, DELETE | `/api/sessions[/{id}]` | `agd_app`; own sessions |
| GET, POST, DELETE | `/api/links[/{id}]` | `agd_app`; own subscription links |
| GET | `/api/live` | `agd_app` (section 7) |

**The app's own calls** (Basic with the app's device credential; no cookie,
no same-origin):

| Method | Path | Body / answer |
|---|---|---|
| POST | `/api/app/enrol` | no auth; `{"token", "model", "android", "app_version"}` -> `{"user", "device_id", "username", "secret", "dav_url", "app_url"}` (once) |
| POST | `/api/app/session` | Basic -> sets `agd_app` for the WebView (device kind `app` only) |
| GET | `/api/app/config` | Basic -> `{"server_version", "min_app_version", "apk_sha256", "bridge": 1, "sync_interval_s"}` |

### 2.3 Surface `admin`

| Method | Path | Auth |
|---|---|---|
| GET, POST | `/admin/signin` | none; password (two argon2 derivations at once at most) |
| POST | `/admin/signout` | `agd_admin`, same-origin |
| GET | `/admin`, `/admin/users[/{name}]`, `/admin/groups`, `/admin/calendars`, `/admin/activity`, `/admin/settings`, `/admin/backups` | `agd_admin` |
| GET | `/static/admin/...`, `/static/shared/...` | none |
| | `/api/admin/*` (3.5) | `agd_admin`; writes same-origin |

Without a session: `401` for `/api/*`, a redirect to the surface's sign-in
for pages. Cross-origin writes: `403 {"error":"cross_origin"}`. An `agd_app`
cookie on the admin surface is ignored.

---

## 3. JSON

### 3.1 Errors

```json
{ "error": "precondition_failed", "message": "changed meanwhile", "by": "lea" }
```

| Status | `error` |
|---|---|
| 400 | `bad_request` (`field`) |
| 401 | `unauthenticated` |
| 403 | `forbidden`, `cross_origin`, `not_visible` (share target the owner does not see) |
| 410 | `cancelled` (an approval answered with the wrong number) |
| 503 | `busy` (too many sign-ins waiting) |
| 404 | `not_found` (also: no access) |
| 409 | `uid_conflict`, `slug_taken` |
| 410 | `expired`, `used` (enrolment, approval) |
| 412 | `precondition_failed` (`by`) |
| 413 | `too_large` |
| 422 | `invalid_event` (`field`) |
| 507 | `quota` (`limit`) |

### 3.2 Calendars and occurrences

```json
// GET /api/calendars
{ "family_timezone": "Europe/Paris",
  "calendars": [
    { "ref": "maman~ecole", "owner": "maman", "name": "École", "color": "#2e7d32",
      "timezone": "Europe/Paris", "description": "", "access": "read",
      "shown": true, "hidden": false, "seq": 790,
      "shares": null },
    { "ref": "lea~perso", "owner": "lea", "name": "Perso", "color": "#c2185b",
      "timezone": "Europe/Paris", "description": "", "access": "owner",
      "shown": true, "hidden": false, "seq": 812,
      "shares": [ { "target": "g:famille", "access": "read" } ] } ] }
```

`shown` is the web app's show/hide toggle; `hidden` takes the calendar off
all of the user's CalDAV devices too. `shares` is set only for the owner.

```json
// GET /api/occurrences?from=2026-11-23&to=2026-12-01&calendars=maman~ecole,lea~perso
{ "occurrences": [
    { "calendar": "maman~ecole", "name": "9f1c….ics", "etag": "\"a1b2…\"",
      "uid": "9f1c…", "recurrence_id": "2026-11-25T14:00:00",
      "summary": "Judo", "location": "Gymnase", "all_day": false,
      "start": "2026-11-25T14:00:00", "end": "2026-11-25T15:30:00",
      "recurring": true, "overridden": false,
      "created_by": "maman", "modified_by": "lea" } ],
  "truncated": false }
```

At most 62 days per request; `calendars` defaults to the shown ones.

### 3.3 Events

`GET /api/calendars/{ref}/events/{name}`:

```json
{ "calendar": "maman~ecole", "name": "9f1c….ics", "etag": "\"a1b2…\"",
  "summary": "Judo", "location": "Gymnase", "description": "",
  "all_day": false, "start": "2026-09-02T14:00:00", "end": "2026-09-02T15:30:00",
  "rrule": "FREQ=WEEKLY;BYDAY=WE;UNTIL=20270630T215959Z",
  "exdates": ["2026-12-23T14:00:00"],
  "overrides": [ { "recurrence_id": "2027-01-06T14:00:00",
                   "start": "2027-01-06T15:00:00", "end": "2027-01-06T16:30:00",
                   "summary": "Judo" } ],
  "has_alarms": true, "has_attendees": false, "editable_rrule": true,
  "created_by": "maman", "created_at": "…", "modified_by": "lea", "modified_at": "…" }
```

`EventInput` (POST and PUT bodies):

```json
{ "summary": "Dentiste", "location": "", "description": "",
  "all_day": false, "start": "2026-11-26T17:30:00", "end": "2026-11-26T18:00:00",
  "rrule": null }
```

`PUT` and `DELETE` take `?scope=this|following|all&occurrence=<local time or
date>` (required on recurring events) and `If-Match`. `move` takes
`{"to": "<ref>"}` and moves the whole object.

### 3.4 People, devices, sign-in

```json
// GET /api/me
{ "name": "lea", "display_name": "Léa", "color": "#c2185b",
  "groups": [ { "name": "famille", "display_name": "Famille" } ] }

// GET /api/people
{ "users":  [ { "name": "maman", "display_name": "Maman", "color": "#3b6ea5" } ],
  "groups": [ { "name": "famille", "display_name": "Famille", "color": "#6a1b9a" } ] }

// POST /api/devices { "kind": "davx5", "label": "Pixel de Léa", "cap": "readwrite" }
{ "id": 7, "kind": "davx5", "label": "Pixel de Léa", "cap": "readwrite",
  "username": "lea", "secret": "agd_k3jd-…",
  "davx5_uri": "davx5://lea:agd_k3jd-…@agenda.example.org/dav/",
  "qr_svg": "<svg …>",
  "mobileconfig_url": null,
  "manual": { "server": "https://agenda.example.org/dav/", "username": "lea" } }

// GET /api/devices items
{ "id": 7, "kind": "davx5", "label": "Pixel de Léa", "cap": "readwrite",
  "created_at": "…", "last_used_at": "…", "last_ip": "203.0.113.4", "client": "DAVx5/4.5" }

// GET /signin (data for the page) and the approval as seen by the approver
{ "id": "…", "qr_url": "https://agenda.example.org/approve#c=<code>",
  "short_code": "KQ4-7XM", "number": 42, "expires_at": "…" }
// GET /api/approvals/{code}
{ "user_agent": "Firefox 140 on Linux", "ip": "192.168.1.20",
  "numbers": [17, 42, 85], "expires_at": "…" }
```

The approval QR carries the code in the URL **fragment**, so it never
reaches a server log; the approve page reads it from `location.hash`.
`qr_svg` is built by Go with no text nodes and inserted as an
`<img src="data:image/svg+xml,…">`, never as markup.

### 3.5 Admin API (`/api/admin`)

As built in M3. Every write needs the `agd_admin` session and same-origin;
errors are 3.1's, with messages in French for the UI to show as they are.

| Method | Path | Body / answer |
|---|---|---|
| GET | `/overview` | `users`, `groups`, `calendars`, `events`, `devices`, `stale_devices`, `storage` {database, backups}, `last_backup`, `version`, `public_url`, `warnings` [{code, message}] |
| GET | `/system` | `version`, `data_dir`, `listeners` [{surfaces, listen, public_url, bound, error, from}] |
| GET, POST | `/users` | list ; create `{name, display_name, color, groups, enrol}` -> `{user, enrolment?}` (201) |
| GET, PATCH | `/users/{name}` | 3.5.1 ; `{display_name?, color?, groups?, disabled?}` -> 3.5.1 |
| POST | `/users/{name}/remove` | `{"calendars": {"<ref>": {"transfer_to": "<user>"} or {"delete": true}}}` -> 204; 409 `owns_calendars` with `calendars` when one is missing |
| POST | `/users/{name}/enrolments` | -> `{id, url, qr_svg, created_at, expires_at}` (201); 400 without `public_url` |
| DELETE | `/users/{name}/enrolments/{id}`, `/users/{name}/devices/{id}`, `/users/{name}/sessions/{id}` | revoke -> 204 |
| GET, POST | `/groups` | list with `members` and `shares` ; create `{name, display_name, color}` |
| PATCH | `/groups/{name}` | `{display_name?, color?, members?}` (members: the exact list) -> 204 |
| DELETE | `/groups/{name}` | 409 `has_shares` unless `?confirm=1` |
| GET | `/calendars` | `?owner=`; every calendar: `ref`, `owner`, `name`, `color`, `timezone`, `events`, `bytes`, `shares`. **No events** |
| DELETE | `/calendars/{ref}/shares/{target}` | remove a share |
| POST | `/calendars/{ref}/transfer` | `{to, keep}` -> `{ref}` |
| DELETE | `/calendars/{ref}?confirm=<name>` | the calendar's display name, typed |
| GET | `/activity?user=&calendar=&kind=&before=` | 3.5.2, 100 per page, newest first; `before` is the last line's `at` |
| GET, PATCH | `/settings` | `[{key, group, type, default, value}]` ; `{"<key>": <JSON value>, ...}` applied together -> the list |
| GET, POST | `/backups` | `[{name, size, made, before_restore}]` ; make one now (201) |
| GET | `/backups/{file}` | download |
| POST | `/backups/{file}/restore` | `{"confirm": "<file>"}` -> 202 `{status: "restarting"}`; the server restarts on it (PLAN 13.6) |

#### 3.5.1 User (admin view)

```json
{ "name": "lea", "display_name": "Léa", "color": "#c2185b",
  "groups": ["famille", "enfants"], "disabled": false,
  "created_at": "…", "last_seen_at": "…",
  "enrolments": [ { "id": 4, "created_at": "…", "expires_at": "…", "used_at": null } ],
  "devices":  [ { "id": 7, "kind": "app", "label": "Pixel 8", "cap": "readwrite", "last_used_at": "…", "client": "agendouille/0.3" } ],
  "sessions": [ { "id": 3, "user_agent": "…", "seen_at": "…" } ],
  "calendars_owned":  [ { "ref": "lea~perso", "name": "Perso", "events": 120, "bytes": 81234 } ],
  "calendars_shared": [ { "ref": "maman~ecole", "name": "École", "access": "read", "via": "g:enfants" } ] }
```

#### 3.5.2 Activity item (admin view: never a summary)

```json
{ "at": "…", "kind": "event", "action": "changed", "by": "lea",
  "calendar": "maman~ecole", "via": "app", "device": "Pixel 8" }
{ "at": "…", "kind": "share", "action": "shared", "by": "maman",
  "calendar": "maman~ecole", "detail": "g:enfants read" }
{ "at": "…", "kind": "admin", "action": "setting", "detail": "backup_keep 14 -> 30" }
```

---

## 4. CalDAV surface

Namespaces: `D` = `DAV:`, `C` = `urn:ietf:params:xml:ns:caldav`,
`CS` = `http://calendarserver.org/ns/`, `A` = `http://apple.com/ns/ical/`.

### 4.1 Resources and properties

| Property | `/dav/` | principal | home | calendar | object |
|---|---|---|---|---|---|
| `D:resourcetype` | collection | collection, `D:principal` | collection | collection, `C:calendar` | empty |
| `D:current-user-principal` | ✓ | ✓ | ✓ | ✓ | ✓ |
| `D:principal-URL` | | ✓ | | | |
| `D:displayname` | | user's display name | | calendar name (or the sharee's own) | |
| `C:calendar-home-set` | | `/dav/calendars/<u>/` | | | |
| `D:owner` | | | | the owner's principal | |
| `D:current-user-privilege-set` | | | `read`, `bind` | `read` + (`write`, `write-content`, `write-properties`, `bind`, `unbind` if effective readwrite) | as calendar |
| `D:supported-report-set` | | | | multiget, query, sync-collection | |
| `C:supported-calendar-component-set` | | | | `VEVENT` | |
| `C:calendar-description` | | | | ✓ | |
| `C:calendar-timezone` | | | | not reported (404): Go cannot write a VTIMEZONE; events carry their own | |
| `D:principal-collection-set` | ✓ | ✓ | | | |
| `C:max-resource-size` | | | | `max_object_bytes` | |
| `CS:getctag`, `D:sync-token` | | | | ✓ | |
| `A:calendar-color`, `A:calendar-order` | | | | ✓ (per user view) | |
| `D:getetag`, `D:getcontenttype`, `D:getcontentlength`, `D:getlastmodified` | | | | | ✓ |
| `C:calendar-data` | | | | | in REPORTs, and PROPFIND if asked |

### 4.2 Calendar collections

- `MKCALENDAR` (RFC 4791 5.3.1) or extended `MKCOL` (RFC 5689) at
  `/dav/calendars/<self>/<slug>/`: `201` with `Location:
  /dav/calendars/<self>/<self>~<slug>/`. A slug already used by the user:
  `405`. Over the quota: `507`. A component set without `VEVENT`: `403
  C:supported-calendar-component`.
- `DELETE` on a calendar: owner only (`204`); sharee `403`.
- `PROPPATCH` on a calendar: a multistatus per property; unknown ones
  `403`.

### 4.3 Errors

Bodies `<D:error xmlns:D="DAV:" xmlns:C="…caldav"><C:…/></D:error>`:

| Status | Element | When |
|---|---|---|
| 401 | (`WWW-Authenticate: Basic realm="agendouille", charset="UTF-8"`) | no or bad credential |
| 403 | `D:need-privileges` | effective `read` writes |
| 403 | `D:propfind-finite-depth` | `Depth: infinity` |
| 403 | `C:supported-calendar-component`, `C:valid-calendar-data`, `C:no-uid-conflict` (with `D:href`), `C:max-resource-size`, `C:supported-filter` | |
| 403 | `D:valid-sync-token` | unknown, another calendar's, or expired |
| 404 | | outside the user's home, no access, no such object |
| 412 | | `If-Match` / `If-None-Match` |
| 415 | | not `text/calendar` |
| 507 | | quota; or `sync-collection` `limit` hit (`D:number-of-matches-within-limits`) |

### 4.4 Sync token

`https://<dav host>/ns/sync/<calendar id>/<n>`.

---

## 5. Schema (end state of the v1 migrations)

```sql
CREATE TABLE settings (key TEXT PRIMARY KEY, value TEXT NOT NULL, updated_at TEXT NOT NULL);
CREATE TABLE admin (id INTEGER PRIMARY KEY CHECK (id = 1), password_hash TEXT NOT NULL, updated_at TEXT NOT NULL);

CREATE TABLE users (
  id INTEGER PRIMARY KEY, name TEXT NOT NULL UNIQUE, display_name TEXT NOT NULL,
  color TEXT NOT NULL, disabled INTEGER NOT NULL DEFAULT 0,
  removed_at TEXT, created_at TEXT NOT NULL, last_seen_at TEXT);
CREATE TABLE groups (
  id INTEGER PRIMARY KEY, name TEXT NOT NULL UNIQUE, display_name TEXT NOT NULL,
  color TEXT NOT NULL, created_at TEXT NOT NULL);
CREATE TABLE group_members (
  group_id INTEGER NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
  user_id  INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  PRIMARY KEY (group_id, user_id));

CREATE TABLE enrolments (
  id INTEGER PRIMARY KEY, user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  token_hash BLOB NOT NULL UNIQUE, made_by TEXT NOT NULL,        -- 'admin' or the user's name
  created_at TEXT NOT NULL, expires_at TEXT NOT NULL,
  used_at TEXT, used_by_device INTEGER, revoked_at TEXT);
CREATE TABLE approvals (
  id TEXT PRIMARY KEY, code_hash BLOB NOT NULL UNIQUE, short_code_hash BLOB NOT NULL UNIQUE,
  number INTEGER NOT NULL, user_agent TEXT NOT NULL, ip TEXT NOT NULL,
  created_at TEXT NOT NULL, expires_at TEXT NOT NULL,
  approved_by INTEGER REFERENCES users(id), failed INTEGER NOT NULL DEFAULT 0, used_at TEXT);
CREATE TABLE sessions (
  id INTEGER PRIMARY KEY, surface TEXT NOT NULL CHECK (surface IN ('app','admin')),
  user_id INTEGER REFERENCES users(id) ON DELETE CASCADE,         -- NULL for the admin
  device_id INTEGER REFERENCES devices(id) ON DELETE CASCADE,     -- set for the app's WebView
  token_hash BLOB NOT NULL UNIQUE, user_agent TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL, seen_at TEXT NOT NULL, expires_at TEXT NOT NULL);
CREATE TABLE devices (
  id INTEGER PRIMARY KEY, user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  kind TEXT NOT NULL CHECK (kind IN ('app','davx5','ios','thunderbird','other')),
  label TEXT NOT NULL, cap TEXT NOT NULL CHECK (cap IN ('readwrite','read')),
  secret_hash BLOB NOT NULL UNIQUE, client TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL, last_used_at TEXT, last_ip TEXT, revoked_at TEXT);
CREATE TABLE links (
  id INTEGER PRIMARY KEY, user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  label TEXT NOT NULL, secret_hash BLOB NOT NULL UNIQUE,
  created_at TEXT NOT NULL, last_used_at TEXT, revoked_at TEXT);

CREATE TABLE calendars (
  id INTEGER PRIMARY KEY, owner_id INTEGER NOT NULL REFERENCES users(id),
  slug TEXT NOT NULL, name TEXT NOT NULL, color TEXT NOT NULL, timezone TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  seq INTEGER NOT NULL DEFAULT 0,       -- the last change in this calendar: ctag and token
  pruned_seq INTEGER NOT NULL DEFAULT 0, -- changes at or below it are pruned: older tokens are invalid
  created_at TEXT NOT NULL, deleted_at TEXT,
  UNIQUE (owner_id, slug));            -- a slug is never reused by its owner, even after delete
CREATE TABLE shares (
  calendar_id INTEGER NOT NULL REFERENCES calendars(id) ON DELETE CASCADE,
  user_id  INTEGER REFERENCES users(id) ON DELETE CASCADE,
  group_id INTEGER REFERENCES groups(id) ON DELETE CASCADE,
  access TEXT NOT NULL CHECK (access IN ('read','readwrite')),
  created_at TEXT NOT NULL,
  CHECK ((user_id IS NULL) <> (group_id IS NULL)),
  UNIQUE (calendar_id, user_id), UNIQUE (calendar_id, group_id));
CREATE TABLE calendar_views (          -- per user: shown, hidden, and their own PROPPATCH view
  calendar_id INTEGER NOT NULL REFERENCES calendars(id) ON DELETE CASCADE,
  user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  shown INTEGER NOT NULL DEFAULT 1, hidden INTEGER NOT NULL DEFAULT 0,
  displayname TEXT, color TEXT, sort_order INTEGER,
  PRIMARY KEY (calendar_id, user_id));
CREATE TABLE link_calendars (
  link_id INTEGER NOT NULL REFERENCES links(id) ON DELETE CASCADE,
  calendar_id INTEGER NOT NULL REFERENCES calendars(id) ON DELETE CASCADE,
  PRIMARY KEY (link_id, calendar_id));

CREATE TABLE objects (
  id INTEGER PRIMARY KEY, calendar_id INTEGER NOT NULL REFERENCES calendars(id) ON DELETE CASCADE,
  name TEXT NOT NULL, uid TEXT NOT NULL, etag TEXT NOT NULL, data BLOB NOT NULL,
  summary TEXT NOT NULL DEFAULT '', start_utc TEXT NOT NULL, end_utc TEXT,
  recurring INTEGER NOT NULL, all_day INTEGER NOT NULL,
  created_by INTEGER NOT NULL REFERENCES users(id), modified_by INTEGER NOT NULL REFERENCES users(id),
  created_via TEXT NOT NULL, modified_via TEXT NOT NULL,
  device_id INTEGER REFERENCES devices(id) ON DELETE SET NULL,
  created_at TEXT NOT NULL, modified_at TEXT NOT NULL,
  UNIQUE (calendar_id, name), UNIQUE (calendar_id, uid));
CREATE INDEX objects_range ON objects (calendar_id, start_utc, end_utc);

CREATE TABLE changes (                -- AUTOINCREMENT: a seq is never reused, even after pruning
  seq INTEGER PRIMARY KEY AUTOINCREMENT, calendar_id INTEGER NOT NULL REFERENCES calendars(id),
  name TEXT NOT NULL DEFAULT '', summary TEXT NOT NULL DEFAULT '',
  action TEXT NOT NULL, user_id INTEGER REFERENCES users(id),
  device_id INTEGER REFERENCES devices(id) ON DELETE SET NULL,
  via TEXT NOT NULL, at TEXT NOT NULL);
CREATE INDEX changes_cal ON changes (calendar_id, seq);

CREATE TABLE audit (                   -- shares, admin actions, settings
  id INTEGER PRIMARY KEY, at TEXT NOT NULL, actor TEXT NOT NULL,   -- 'admin' or a user name
  kind TEXT NOT NULL, action TEXT NOT NULL,
  calendar_id INTEGER REFERENCES calendars(id), detail TEXT NOT NULL);
```

- A deleted calendar keeps its row (`deleted_at`) and its `changes` until
  pruned; its objects go. Its slug stays taken for its owner.
- Users are never deleted from `users` (`removed_at`), so `created_by`
  keeps pointing somewhere.

---

## 6. The app bridge (page <-> Java), version 1

The app posts a `MessagePort` to the page with
`postWebMessage(new WebMessage("agendouille-bridge", ports), origin)` after
each page load. `shared/core/Bridge` keeps it; with no port within 500 ms
after load the page is in a plain browser. Messages are JSON strings:

```json
// page -> app
{ "id": 1, "call": "hello", "args": { "page_bridge": 1 } }
{ "id": 2, "call": "scan" }                  // the app grants camera, the page scans
{ "id": 3, "call": "sync.status" }
{ "id": 4, "call": "sync.now" }
{ "id": 5, "call": "davx5.handoff" }          // ask the server for a davx5 device, open DAVx5
{ "id": 6, "call": "account.remove" }

// app -> page
{ "id": 1, "ok": true, "result": { "app_bridge": 1, "app_version": "0.6.0", "android": 14 } }
{ "id": 3, "ok": true, "result": { "last_sync": "…", "state": "idle", "calendars": 4, "error": null } }
{ "id": 4, "ok": false, "error": "offline" }
{ "event": "sync.changed", "data": { "state": "syncing" } }
```

Both sides ignore unknown fields and answer an unknown `call` with
`{"ok": false, "error": "unknown_call"}`. A breaking change raises the
bridge number; the server's page supports the current and the previous
one (`/api/app/config` tells the app the server's).

---

## 7. SSE (`/api/live`)

```
event: hello
data: {}

event: changed
data: {"seq":813,"calendar":"maman~ecole","name":"9f1c….ics","action":"changed","by":"lea"}

event: access          (a share, a group, a hidden flag or a calendar changed for this user)
data: {}

event: ping            (every 25 s)
data: {}
```

Only changes on calendars the user may read are sent. Nothing is replayed:
reconnecting with `Last-Event-ID` gets one `resync`, and so does a listener
that fell behind; the page then reloads what it shows (DECISIONS
2026-11-23, M4). The admin UI has no SSE in v1.
