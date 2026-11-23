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
	a := x.do("POST", x.admin+"/admin/signin", body, edit)
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
	for _, p := range []string{"/admin", "/admin/users", "/admin/users/lea", "/admin/groups", "/admin/calendars", "/admin/activity", "/admin/settings", "/admin/backups"} {
		if a := x.do("GET", x.admin+p, nil, cookie(auth.AdminCookie, A.session)); a.status != 200 || !strings.Contains(a.body, "admin-content") {
			t.Errorf("page %s: %d", p, a.status)
		}
	}
	ov := A.json("GET", "/overview", nil, 200)
	if !strings.Contains(asJSON(ov["warnings"]), "no_public_listener") {
		t.Errorf("the overview does not warn the public side is off: %v", ov["warnings"])
	}

	// Set the public listener from the settings page.
	addr := freePort(t)
	A.json("PATCH", "/settings", map[string]any{"public_listen": addr, "public_url": "http://" + addr}, 200)
	deadline := time.Now().Add(5 * time.Second)
	for a := x.srv.PublicAddr(); a == nil || a.String() != addr; a = x.srv.PublicAddr() {
		if time.Now().After(deadline) {
			t.Fatal("the public listener did not start from the settings page")
		}
		time.Sleep(20 * time.Millisecond)
	}
	x.public = "http://" + addr
	bad := A.call("PATCH", "/settings", map[string]any{"public_listen": "0.0.0.0:1", "public_url": "http://lan"})
	if bad.status != http.StatusBadRequest || !strings.Contains(bad.body, "plain http") {
		t.Errorf("a bad setting: %d %s", bad.status, bad.body)
	}
	// A cross-site request cannot change a setting.
	evil := x.do("PATCH", x.admin+"/api/admin/settings", strings.NewReader(`{"backup_keep":1}`), func(r *http.Request) {
		r.AddCookie(&http.Cookie{Name: auth.AdminCookie, Value: A.session})
		r.Header.Set("Sec-Fetch-Site", "cross-site")
	})
	if evil.status != http.StatusForbidden {
		t.Errorf("a cross-site PATCH: %d", evil.status)
	}

	// A group and two users; lea gets her first sign-in QR.
	A.json("POST", "/groups", map[string]any{"name": "famille", "display_name": "Famille"}, 201)
	res := A.json("POST", "/users", map[string]any{"name": "lea", "display_name": "Léa", "groups": []string{"famille"}, "enrol": true}, 201)
	enrol, _ := res["enrolment"].(map[string]any)
	link, _ := enrol["url"].(string)
	if qr, _ := enrol["qr_svg"].(string); !strings.HasPrefix(link, x.public+"/enrol/") || !strings.HasPrefix(qr, "<svg") {
		t.Fatalf("the enrolment: %v", enrol)
	}
	A.json("POST", "/users", map[string]any{"name": "maman", "groups": []string{"famille"}}, 201)

	// The phone's browser opens the link, signed in; the admin sees it used.
	if a := x.do("POST", link, nil, nil); a.status != http.StatusSeeOther || cookieOf(a, auth.AppCookie) == "" {
		t.Fatalf("opening the enrolment link: %d", a.status)
	}
	lea := A.json("GET", "/users/lea", nil, 200)
	if es, _ := lea["enrolments"].([]any); len(es) != 1 || es[0].(map[string]any)["used_at"] == nil {
		t.Errorf("the enrolment is not marked used: %v", lea["enrolments"])
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
		"/activity?user=lea", "/activity?kind=event", "/settings", "/backups", "/system"} {
		a := A.call("GET", p, nil)
		if a.status != 200 {
			t.Errorf("GET %s: %d", p, a.status)
		}
		if strings.Contains(a.body, "SECRET") {
			t.Errorf("GET /api/admin%s carries event data: %s", p, a.body)
		}
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
	A.json("POST", "/calendars/lea~perso/transfer", map[string]any{"to": "maman", "keep": true}, 200)
	if cs := A.json("GET", "/calendars", nil, 200); !strings.Contains(asJSON(cs), "maman~perso") {
		t.Errorf("after the transfer: %v", cs)
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
