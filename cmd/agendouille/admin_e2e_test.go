package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nebuloss/agendouille/internal/auth"
	"github.com/nebuloss/agendouille/internal/backup"
	"github.com/nebuloss/agendouille/internal/calendar"
	"github.com/nebuloss/agendouille/internal/surface"
)

// adminClient signs in on the admin listener and calls /api/admin as the
// admin UI does.
type adminClient struct {
	x       *e2e
	session string
}

func signInAdmin(t *testing.T, x *e2e) adminClient {
	t.Helper()
	body, edit := form(url.Values{"password": {testAdminPassword}})
	a := x.do("POST", x.admin+"/signin", body, edit)
	s := cookieOf(a, auth.AdminCookie)
	if s == "" {
		t.Fatalf("admin sign-in: %d", a.status)
	}
	return adminClient{x: x, session: s}
}

func (c adminClient) call(method, path string, body any) answer {
	c.x.t.Helper()
	var r io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		r = strings.NewReader(string(b))
	}
	return c.x.do(method, c.x.admin+"/api/admin"+path, r, func(req *http.Request) {
		req.AddCookie(&http.Cookie{Name: auth.AdminCookie, Value: c.session})
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Sec-Fetch-Site", "same-origin")
	})
}

func (c adminClient) json(method, path string, body any, want int) map[string]any {
	c.x.t.Helper()
	a := c.call(method, path, body)
	if a.status != want {
		c.x.t.Fatalf("%s %s: %d, want %d: %s", method, path, a.status, want, a.body)
	}
	out := map[string]any{}
	if a.body != "" {
		_ = json.Unmarshal([]byte(a.body), &out)
	}
	return out
}

// The M3 "done when" (PLAN.md 15), from the admin UI's API alone.
func TestM3AdminUI(t *testing.T) {
	x := startServer(t)
	ctx := context.Background()

	// Unauthenticated, or with a family-app cookie: nothing.
	if a := x.do("GET", x.admin+"/api/admin/overview", nil, nil); a.status != http.StatusUnauthorized {
		t.Errorf("signed out: %d", a.status)
	}
	A := signInAdmin(t, x)
	// The pages are served (the bundle fills them).
	for _, p := range []string{"/", "/users", "/users/lea", "/groups", "/activity", "/settings", "/backups"} {
		if a := x.do("GET", x.admin+p, nil, cookie(auth.AdminCookie, A.session)); a.status != 200 || !strings.Contains(a.body, "admin-content") {
			t.Errorf("page %s: %d", p, a.status)
		}
	}
	// The old calendars page lands on the users: calendars are under their owner.
	if a := x.do("GET", x.admin+"/calendars", nil, cookie(auth.AdminCookie, A.session)); a.status != 200 || !strings.Contains(a.body, "admin-content") {
		t.Errorf("/calendars: %d", a.status)
	}
	// Links from before, with the /admin prefix, land on the same page.
	for old, now := range map[string]string{"/admin": "/", "/admin/users/lea": "/users/lea", "/admin/calendars": "/calendars"} {
		if a := x.do("GET", x.admin+old, nil, nil); a.status != http.StatusMovedPermanently || a.header.Get("Location") != now {
			t.Errorf("%s: %d %s, want %s", old, a.status, a.header.Get("Location"), now)
		}
	}
	// The notification centre: the public side being off is an error,
	// marked read it stays read, and it is forgotten once fixed.
	nt := A.json("GET", "/notices", nil, 200)
	if !strings.Contains(asJSON(nt), `"id":"server:no_public_listener"`) || !strings.Contains(asJSON(nt), `"severity":"error"`) {
		t.Errorf("notices: %v", nt)
	}
	nt = A.json("POST", "/notices/read", map[string]any{"ids": []string{"server:no_public_listener", "nonsense"}}, 200)
	read := map[string]bool{}
	for _, n := range nt["notices"].([]any) {
		m := n.(map[string]any)
		read[m["id"].(string)] = m["read"] == true
	}
	if !read["server:no_public_listener"] || read["server:backup_old"] {
		t.Errorf("marked read: %v", read)
	}
	ov := A.json("GET", "/overview", nil, 200)
	if !strings.Contains(asJSON(ov["warnings"]), "no_public_listener") {
		t.Errorf("the overview does not warn the public side is off: %v", ov["warnings"])
	}

	// The settings live in the configuration file: the operator edits it,
	// the running server applies it (DECISIONS, settings in the file).
	x.publicOn()
	addr := strings.TrimPrefix(x.public, "http://")
	if nt := A.json("GET", "/notices", nil, 200); strings.Contains(asJSON(nt), "no_public_listener") {
		t.Errorf("the notice outlived its problem: %v", nt)
	}
	var marks int
	_ = x.svc.store.DB().QueryRow(`SELECT count(*) FROM notice_reads WHERE id = 'server:no_public_listener'`).Scan(&marks)
	if marks != 0 {
		t.Error("the read mark outlived its notice")
	}
	// The settings page shows each value and where it came from, and the
	// file; it changes nothing.
	sp := A.json("GET", "/settings", nil, 200)
	origin := map[string]string{}
	for _, it := range sp["settings"].([]any) {
		m := it.(map[string]any)
		origin[m["key"].(string)] = m["origin"].(string)
		if help, _ := m["help"].(string); help == "" {
			t.Errorf("setting %v has no help text", m["key"])
		}
	}
	if sp["path"] != x.configPath || origin["public_url"] != "file" || origin["backup_keep"] != "default" {
		t.Errorf("settings page: %v %v", sp["path"], origin)
	}
	if a := A.call("PATCH", "/settings", map[string]any{"backup_keep": 1}); a.status != http.StatusMethodNotAllowed {
		t.Errorf("the settings page edited: %d", a.status)
	}
	// A file that does not validate is refused: the settings in force stay.
	x.configure("public_listen: 0.0.0.0:1\npublic_url: http://lan\n")
	time.Sleep(300 * time.Millisecond)
	if a := x.srv.PublicAddr(); a == nil || a.String() != addr {
		t.Errorf("a bad file moved the public listener: %v", a)
	}
	x.configure("public_listen: " + addr + "\npublic_url: http://" + addr + "\n")

	// A group and two users; lea gets her first sign-in QR.
	A.json("POST", "/groups", map[string]any{"name": "famille", "display_name": "Famille"}, 201)
	res := A.json("POST", "/users", map[string]any{"name": "lea", "display_name": "Léa", "groups": []string{"famille"}, "enroll": true}, 201)
	enroll, _ := res["enrollment"].(map[string]any)
	link, _ := enroll["url"].(string)
	if qr, _ := enroll["qr_svg"].(string); !strings.HasPrefix(link, x.public+"/enroll/") || !strings.HasPrefix(qr, "<svg") {
		t.Fatalf("the enrollment: %v", enroll)
	}
	A.json("POST", "/users", map[string]any{"name": "maman", "groups": []string{"famille"}}, 201)
	// Per user and per group views: an item's overview and its pages, the
	// rail of its level with the way back, the trail down to the page.
	for _, p := range []string{"/users/lea", "/users/lea/devices", "/users/lea/calendars", "/users/lea/groups",
		"/users/lea/activity", "/groups/famille", "/groups/famille/members", "/groups/famille/calendars"} {
		a := x.do("GET", x.admin+p, nil, cookie(auth.AdminCookie, A.session))
		if a.status != 200 || !strings.Contains(a.body, `aria-label="Pages"`) || !strings.Contains(a.body, `data-item-name`) {
			t.Errorf("item page %s: %d", p, a.status)
		}
	}
	if a := x.do("GET", x.admin+"/users/lea/devices", nil, cookie(auth.AdminCookie, A.session)); !strings.Contains(a.body, `href="/users/lea/devices" class="state-layer`) ||
		!strings.Contains(a.body, `>Appareils</span></a>`) || !strings.Contains(a.body, `href="/users"`) {
		t.Errorf("the rail of a user's page: %s", a.body)
	}
	// The same page in English, the reader's choice: the rail, the trail
	// and the title follow it.
	english := func(r *http.Request) {
		r.AddCookie(&http.Cookie{Name: auth.AdminCookie, Value: A.session})
		r.AddCookie(&http.Cookie{Name: "agd_lang", Value: "en"})
	}
	if a := x.do("GET", x.admin+"/users/lea/devices", nil, english); !strings.Contains(a.body, `>Devices</span></a>`) ||
		!strings.Contains(a.body, `<span>Users</span>`) || !strings.Contains(a.body, `aria-label="Overview"`) ||
		!strings.Contains(a.body, "Sign out") || !strings.Contains(a.body, "Devices · lea") {
		t.Errorf("a user's page in English: %s", a.body)
	}
	// The activity log's CSV: the table's filter, a header, the rows.
	if a := A.call("GET", "/activity/export?user=lea", nil); a.status != 200 || !strings.HasPrefix(a.header.Get("Content-Type"), "text/csv") ||
		!strings.HasPrefix(a.body, "date,auteur,type,action,calendrier,via,appareil,détail\n") || !strings.Contains(a.body, ",user,") {
		t.Errorf("activity export: %d %q", a.status, a.body)
	}
	// A pending sign-in link shows again; a browser link lasts an hour;
	// once used, it is gone.
	pl := A.json("POST", "/users/maman/enrollments", map[string]any{"kind": "browser"}, 201)
	id := fmtID(pl["id"])
	again := A.json("GET", "/users/maman/enrollments/"+id, nil, 200)
	if again["url"] != pl["url"] || pl["kind"] != "browser" {
		t.Errorf("shown again: %v, first %v", again["url"], pl["url"])
	}
	if exp, _ := time.Parse(time.RFC3339Nano, pl["expires_at"].(string)); time.Until(exp) > time.Hour+time.Minute {
		t.Errorf("a browser link lasts until %v", exp)
	}
	token := pl["url"].(string)[strings.LastIndex(pl["url"].(string), "/")+1:]
	if a := x.do("POST", x.public+"/enroll/"+token, nil, nil); cookieOf(a, auth.AppCookie) == "" {
		t.Fatalf("the browser link did not sign in: %d", a.status)
	}
	if a := A.call("GET", "/users/maman/enrollments/"+id, nil); a.status != http.StatusGone {
		t.Errorf("a used link shown again: %d", a.status)
	}
	var kept int
	_ = x.svc.store.DB().QueryRow(`SELECT count(*) FROM enrollments WHERE token IS NOT NULL AND used_at IS NOT NULL`).Scan(&kept)
	if kept != 0 {
		t.Error("a used link's token is still kept")
	}
	if a := x.do("GET", x.admin+"/users/lea/signin", nil, cookie(auth.AdminCookie, A.session)); a.status != http.StatusMovedPermanently || a.header.Get("Location") != "/users/lea/devices" {
		t.Errorf("the old Connexion page: %d %s", a.status, a.header.Get("Location"))
	}
	if a := x.do("GET", x.admin+"/users/lea/nonsense", nil, cookie(auth.AdminCookie, A.session)); a.status != http.StatusSeeOther || a.header.Get("Location") != "/users/lea" {
		t.Errorf("an unknown page of a user: %d %s", a.status, a.header.Get("Location"))
	}

	// The phone's browser opens the link, signed in; the admin sees it used.
	if a := x.do("POST", link, nil, nil); a.status != http.StatusSeeOther || cookieOf(a, auth.AppCookie) == "" {
		t.Fatalf("opening the enrollment link: %d", a.status)
	}
	lea := A.json("GET", "/users/lea", nil, 200)
	if es, _ := lea["enrollments"].([]any); len(es) != 1 || es[0].(map[string]any)["used_at"] == nil {
		t.Errorf("the enrollment is not marked used: %v", lea["enrollments"])
	}
	if !strings.Contains(asJSON(lea["groups"]), "famille") || len(lea["sessions"].([]any)) != 1 {
		t.Errorf("lea's page: %v", lea)
	}

	// Event data never reaches the admin.
	leaU, _ := x.svc.dir.User(ctx, "lea")
	if _, err := x.svc.calendar.Create(ctx, calendar.Actor{UserID: leaU.ID, Name: "lea", Cap: "readwrite", Via: "test"}, "lea", "perso", calendar.Props{}); err != nil {
		t.Fatal(err)
	}
	secret := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nBEGIN:VEVENT\r\nUID:SECRET-UID\r\nSUMMARY:SECRET-DENTIST\r\nLOCATION:SECRET-PLACE\r\n" +
		"DESCRIPTION:SECRET-NOTES\r\nDTSTART:20261126T170000Z\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"
	dev, _ := x.svc.auth.NewDevice(ctx, leaU.ID, "davx5", "Pixel", "readwrite")
	davClient{x, "lea", dev.Secret}.req("PUT", "/dav/calendars/lea/lea~perso/secret.ics", secret)
	for _, p := range []string{"/overview", "/users", "/users/lea", "/users/maman", "/groups", "/calendars", "/activity",
		"/activity?user=lea", "/activity?kind=event", "/settings", "/backups", "/system", "/calendars/lea~perso/stats",
		"/activity/export", "/activity?calendar=lea~perso"} {
		a := A.call("GET", p, nil)
		if a.status != 200 {
			t.Errorf("GET %s: %d", p, a.status)
		}
		if strings.Contains(a.body, "SECRET") {
			t.Errorf("GET /api/admin%s carries event data: %s", p, a.body)
		}
	}
	// The calendar's stats: counts and who, from the change log.
	st := A.json("GET", "/calendars/lea~perso/stats", nil, 200)
	if st["events"] != float64(1) || st["changes"] != float64(1) || len(st["daily"].([]any)) != 90 ||
		!strings.Contains(asJSON(st["by_user"]), `"user":"lea"`) || !strings.Contains(asJSON(st["by_device"]), `"label":"Pixel"`) ||
		!strings.Contains(asJSON(st["by_via"]), `"via":"caldav"`) {
		t.Errorf("stats: %v", st)
	}
	// The admin manages a calendar's members: add, change access, remove.
	if a := A.call("PUT", "/calendars/lea~perso/shares/u:maman", map[string]any{"access": "read"}); a.status != http.StatusNoContent {
		t.Fatalf("admin share: %d %s", a.status, a.body)
	}
	A.call("PUT", "/calendars/lea~perso/shares/u:maman", map[string]any{"access": "readwrite"})
	if st := A.json("GET", "/calendars/lea~perso/stats", nil, 200); !strings.Contains(asJSON(st["shares"]), `{"access":"readwrite","target":"u:maman"}`) {
		t.Errorf("after changing the access: %v", st["shares"])
	}
	if a := A.call("PUT", "/calendars/lea~perso/shares/u:lea", map[string]any{"access": "read"}); a.status != http.StatusBadRequest {
		t.Errorf("sharing with the owner: %d", a.status)
	}
	if a := A.call("PUT", "/calendars/lea~perso/shares/u:nobody", map[string]any{"access": "read"}); a.status != http.StatusNotFound {
		t.Errorf("sharing with no one: %d", a.status)
	}
	A.call("DELETE", "/calendars/lea~perso/shares/u:maman", nil)
	// A calendar as .ics, the admin's backup of it: the events are in it,
	// and the download is in the activity.
	if a := A.call("GET", "/calendars/lea~perso/export", nil); a.status != 200 || !strings.Contains(a.body, "SUMMARY:SECRET-DENTIST") ||
		!strings.HasPrefix(a.header.Get("Content-Type"), "text/calendar") {
		t.Errorf("calendar export: %d %s", a.status, a.header.Get("Content-Type"))
	}
	if act := A.json("GET", "/activity?kind=calendar", nil, 200); !strings.Contains(asJSON(act), `"action":"exported"`) {
		t.Errorf("the export is not in the activity: %v", act)
	}
	if a := A.call("GET", "/calendars/export.zip", nil); a.status != 200 || a.header.Get("Content-Type") != "application/zip" || !strings.Contains(a.body, "lea~perso.ics") {
		t.Errorf("calendars zip: %d %s", a.status, a.header.Get("Content-Type"))
	}
	if a := A.call("GET", "/calendars/lea~nope/stats", nil); a.status != http.StatusNotFound {
		t.Errorf("stats of no calendar: %d", a.status)
	}
	// The admin addresses a calendar by its id, not owner~slug (DECISIONS:
	// calendar ids): the API answers to both, the pages redirect the old form.
	calID, _ := A.json("GET", "/calendars/lea~perso/stats", nil, 200)["id"].(string)
	if len(calID) != 36 {
		t.Fatalf("the calendar has no id: %q", calID)
	}
	if st := A.json("GET", "/calendars/"+calID+"/stats", nil, 200); st["ref"] != "lea~perso" {
		t.Errorf("stats by id: %v", st["ref"])
	}
	for _, sub := range []string{"", "/shares", "/activity"} {
		a := x.do("GET", x.admin+"/calendars/"+calID+sub, nil, cookie(auth.AdminCookie, A.session))
		if a.status != 200 || !strings.Contains(a.body, `<span>Calendriers</span>`) || !strings.Contains(a.body, "data-item-name") {
			t.Errorf("calendar page %s: %d", sub, a.status)
		}
		old := x.do("GET", x.admin+"/calendars/lea~perso"+sub, nil, cookie(auth.AdminCookie, A.session))
		if old.status != http.StatusMovedPermanently || old.header.Get("Location") != "/calendars/"+calID+sub {
			t.Errorf("old address %s: %d %s", sub, old.status, old.header.Get("Location"))
		}
	}
	if cs := A.json("GET", "/calendars", nil, 200); !strings.Contains(asJSON(cs), calID) {
		t.Errorf("the list has no ids: %v", cs)
	}
	act := A.json("GET", "/activity?kind=event", nil, 200)
	if !strings.Contains(asJSON(act["activity"]), `"lea~perso"`) {
		t.Errorf("the event's change is not in the activity log at all: %v", act)
	}

	// Removing a user who owns a calendar asks what becomes of it.
	if a := A.call("POST", "/users/lea/remove", map[string]any{}); a.status != http.StatusConflict || !strings.Contains(a.body, "lea~perso") {
		t.Errorf("removing lea without saying what becomes of her calendar: %d %s", a.status, a.body)
	}
	// Deleting a calendar needs its name typed.
	if a := A.call("DELETE", "/calendars/lea~perso?confirm=nope", nil); a.status != http.StatusBadRequest {
		t.Errorf("deleting without the name: %d", a.status)
	}
	if tr := A.json("POST", "/calendars/"+calID+"/transfer", map[string]any{"to": "maman", "keep": true}, 200); tr["id"] != calID || tr["ref"] != "maman~perso" {
		t.Errorf("a transfer changed the id: %v", tr)
	}
	if cs := A.json("GET", "/calendars", nil, 200); !strings.Contains(asJSON(cs), "maman~perso") {
		t.Errorf("after the transfer: %v", cs)
	}
	if st := A.json("GET", "/calendars/"+calID+"/stats", nil, 200); st["owner"] != "maman" {
		t.Errorf("the id after the transfer: %v", st["owner"])
	}

	// Backups: make one; the list has it.
	b := A.json("POST", "/backups", nil, 201)
	name, _ := b["name"].(string)
	if !strings.HasPrefix(name, "agendouille-") {
		t.Fatalf("backup: %v", b)
	}
	if a := A.call("GET", "/backups/"+name, nil); a.status != 200 || !strings.HasPrefix(a.body, "SQLite format 3") {
		t.Errorf("downloading the backup: %d", a.status)
	}
	if a := A.call("GET", "/backups/..%2F..%2Fetc%2Fpasswd", nil); a.status != http.StatusNotFound {
		t.Errorf("a path outside the backups: %d", a.status)
	}
	// The admin's own session does not open the family app.
	if a := x.do("GET", x.public+"/api/me", nil, cookie(auth.AppCookie, A.session)); a.status != http.StatusUnauthorized {
		t.Errorf("the admin session on the app: %d", a.status)
	}
}

func asJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

// Restoring from the admin UI stages the backup, restarts the server,
// and the next start runs on the backup with the old database kept aside.
func TestM3Restore(t *testing.T) {
	cfg := testConfig(t)
	withAdminPassword(t, cfg)
	old := settingsPoll
	settingsPoll = 50 * time.Millisecond
	defer func() { settingsPoll = old }()

	run := func() (*e2e, chan error, context.CancelFunc) {
		ctx, cancel := context.WithCancel(context.Background())
		up := make(chan *surface.Server, 1)
		done := make(chan error, 1)
		go func() {
			done <- serve(ctx, cfg, slog.New(slog.NewTextHandler(io.Discard, nil)), func(s *surface.Server) { up <- s })
		}()
		var srv *surface.Server
		select {
		case srv = <-up:
		case err := <-done:
			t.Fatalf("serve: %v", err)
		case <-time.After(10 * time.Second):
			t.Fatal("serve did not start")
		}
		return &e2e{t: t, admin: "http://" + srv.AdminAddr().String(), srv: srv}, done, cancel
	}

	x, done, cancel := run()
	A := signInAdmin(t, x)
	A.json("POST", "/groups", map[string]any{"name": "avant"}, 201)
	b := A.json("POST", "/backups", nil, 201)
	name := b["name"].(string)
	A.json("POST", "/groups", map[string]any{"name": "apres"}, 201)
	r := A.json("POST", "/backups/"+name+"/restore", map[string]any{"confirm": name}, 202)
	if r["status"] != "restarting" {
		t.Fatalf("restore: %v", r)
	}
	select {
	case err := <-done:
		if !errors.Is(err, ErrRestart) {
			t.Fatalf("serve returned %v, want ErrRestart", err)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("the server did not restart")
	}
	cancel()

	// The next start applies it: "avant" is there, "apres" is not.
	x, done, cancel = run()
	defer func() { cancel(); <-done }()
	A = signInAdmin(t, x)
	gs := asJSON(A.json("GET", "/groups", nil, 200))
	if !strings.Contains(gs, `"avant"`) || strings.Contains(gs, `"apres"`) {
		t.Errorf("after the restore: %s", gs)
	}
	kept, _ := filepath.Glob(filepath.Join(cfg.DataDir, backup.Dir, "before-restore-*.db"))
	if len(kept) != 1 {
		t.Errorf("the database before the restore is not kept: %v", kept)
	}
	if _, err := os.Stat(filepath.Join(cfg.DataDir, "restore-pending.db")); !os.IsNotExist(err) {
		t.Error("the staged restore is still there")
	}
}
