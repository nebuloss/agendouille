package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/nebuloss/agendouille/internal/auth"
	"github.com/nebuloss/agendouille/internal/config"
	"github.com/nebuloss/agendouille/internal/surface"
)

// The M1 "done when" (PLAN.md 15), against a real server with both
// listeners, driven over HTTP the way a browser, a phone and the CLI do.

type e2e struct {
	t      *testing.T
	admin  string // http://host:port of the admin listener
	public string // of the public listener, once up
	svc    *services
	srv    *surface.Server
	// The server's configuration file, and its keys before any setting:
	// configure rewrites the file, as an operator edits it.
	configPath, configBase string
}

// configure writes the configuration file with these settings (YAML lines)
// after its bootstrap keys, as an operator edits it; the server applies it
// within settingsPoll.
func (x *e2e) configure(settingsYAML string) {
	x.t.Helper()
	if err := os.WriteFile(x.configPath, []byte(x.configBase+settingsYAML), 0o600); err != nil {
		x.t.Fatal(err)
	}
}

func startServer(t *testing.T) *e2e {
	t.Helper()
	old := settingsPoll
	settingsPoll = 50 * time.Millisecond
	t.Cleanup(func() { settingsPoll = old })

	// A configuration file, so serve watches it and a test edits it.
	dir := t.TempDir()
	configPath := filepath.Join(dir, "agendouille.yaml")
	configBase := "data_dir: " + filepath.Join(dir, "data") + "\nadmin_listen: 127.0.0.1:0\n"
	if err := os.WriteFile(configPath, []byte(configBase), 0o600); err != nil {
		t.Fatal(err)
	}
	loaded, _, err := config.NewLoader(flag.NewFlagSet("t", flag.ContinueOnError)).Load(func(k string) string {
		if k == config.EnvPrefix+"CONFIG" {
			return configPath
		}
		return ""
	})
	if err != nil {
		t.Fatal(err)
	}
	cfg := loaded
	withAdminPassword(t, cfg)
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
	t.Cleanup(func() {
		cancel()
		if err := <-done; err != nil {
			t.Errorf("serve: %v", err)
		}
	})
	// A second set of services on the same database, as the CLI would
	// open while the server runs.
	svc, err := openServices(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { svc.store.Close() })
	return &e2e{t: t, admin: "http://" + srv.AdminAddr().String(), svc: svc, srv: srv, configPath: configPath, configBase: configBase}
}

var noRedirect = &http.Client{
	CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	Timeout:       30 * time.Second,
}

type answer struct {
	status  int
	body    string
	cookies []*http.Cookie
	header  http.Header
}

func (x *e2e) do(method, u string, body io.Reader, edit func(*http.Request)) answer {
	x.t.Helper()
	req, err := http.NewRequest(method, u, body)
	if err != nil {
		x.t.Fatal(err)
	}
	if edit != nil {
		edit(req)
	}
	resp, err := noRedirect.Do(req)
	if err != nil {
		x.t.Fatalf("%s %s: %v", method, u, err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return answer{status: resp.StatusCode, body: string(b), cookies: resp.Cookies(), header: resp.Header}
}

func cookie(name, value string) func(*http.Request) {
	return func(r *http.Request) { r.AddCookie(&http.Cookie{Name: name, Value: value}) }
}

func cookieOf(a answer, name string) string {
	for _, c := range a.cookies {
		if c.Name == name {
			return c.Value
		}
	}
	return ""
}

func form(values url.Values, edit ...func(*http.Request)) (io.Reader, func(*http.Request)) {
	return strings.NewReader(values.Encode()), func(r *http.Request) {
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		for _, e := range edit {
			e(r)
		}
	}
}

func freePort(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	return ln.Addr().String()
}

// publicOn sets public_listen and public_url in the configuration file,
// as an operator would, and waits for the running server to apply it.
func (x *e2e) publicOn() {
	x.t.Helper()
	addr := freePort(x.t)
	x.configure("public_listen: " + addr + "\npublic_url: http://" + addr + "\n")
	deadline := time.Now().Add(5 * time.Second)
	for a := x.srv.PublicAddr(); a == nil || a.String() != addr; a = x.srv.PublicAddr() {
		if time.Now().After(deadline) {
			x.t.Fatal("the public listener did not start after the setting changed")
		}
		time.Sleep(20 * time.Millisecond)
	}
	x.public = "http://" + addr
}

func TestM1(t *testing.T) {
	x := startServer(t)
	ctx := context.Background()

	// --- the admin listener serves the admin surface and nothing else
	if a := x.do("GET", x.admin+"/healthz", nil, nil); a.status != 200 {
		t.Fatalf("admin /healthz: %d", a.status)
	}
	for _, p := range []string{"/dav/", "/.well-known/caldav", "/approve", "/enroll/x", "/api/me", "/static/app/app.js"} {
		if a := x.do("GET", x.admin+p, nil, nil); a.status != http.StatusNotFound {
			t.Errorf("admin listener %s: %d, want 404", p, a.status)
		}
	}
	// Its /signin is the admin's (no /admin prefix on its own listener).
	if a := x.do("GET", x.admin+"/signin", nil, nil); a.status != 200 || !strings.Contains(a.body, "Mot de passe administrateur") {
		t.Errorf("admin listener /signin: %d", a.status)
	}
	if x.srv.PublicAddr() != nil {
		t.Fatal("the public listener is up before public_listen is set")
	}

	// --- admin sign-in
	if a := x.do("GET", x.admin+"/", nil, nil); a.status != http.StatusSeeOther || a.header.Get("Location") != "/signin" {
		t.Errorf("a page signed out: %d %s", a.status, a.header.Get("Location"))
	}
	body, edit := form(url.Values{"password": {"wrong password!"}})
	if a := x.do("POST", x.admin+"/signin", body, edit); a.status != http.StatusUnauthorized {
		t.Errorf("a wrong admin password: %d", a.status)
	}
	body, edit = form(url.Values{"password": {testAdminPassword}})
	a := x.do("POST", x.admin+"/signin", body, edit)
	adminSession := cookieOf(a, auth.AdminCookie)
	if a.status != http.StatusSeeOther || adminSession == "" {
		t.Fatalf("admin sign-in: %d, cookie %q", a.status, adminSession)
	}
	if a := x.do("GET", x.admin+"/", nil, cookie(auth.AdminCookie, adminSession)); a.status != 200 {
		t.Errorf("/ signed in: %d", a.status)
	}
	// A cross-site form cannot sign the admin out (or in).
	body, edit = form(url.Values{}, func(r *http.Request) {
		r.Header.Set("Sec-Fetch-Site", "cross-site")
		r.AddCookie(&http.Cookie{Name: auth.AdminCookie, Value: adminSession})
	})
	if a := x.do("POST", x.admin+"/signout", body, edit); a.status != http.StatusForbidden {
		t.Errorf("a cross-site sign-out: %d, want 403", a.status)
	}

	// --- every form page shows its form (an empty page once passed this test)
	if a := x.do("GET", x.admin+"/signin", nil, nil); !strings.Contains(a.body, `name="password"`) {
		t.Errorf("the admin sign-in page has no password field:\n%s", a.body)
	}

	// --- setting public_listen starts the public listener without a restart
	x.publicOn()
	if a := x.do("GET", x.public+"/healthz", nil, nil); a.status != 200 {
		t.Fatalf("public /healthz: %d", a.status)
	}
	for _, p := range []string{"/admin", "/admin/signin", "/static/admin/admin.js"} {
		if a := x.do("GET", x.public+p, nil, nil); a.status != http.StatusNotFound {
			t.Errorf("public listener %s: %d, want 404", p, a.status)
		}
	}
	if a := x.do("GET", x.public+"/signin", nil, nil); a.status != http.StatusMovedPermanently || a.header.Get("Location") != "/" {
		t.Errorf("the old sign-in address: %d %q", a.status, a.header.Get("Location"))
	}
	if a := x.do("GET", x.public+"/calendars", nil, nil); a.status != http.StatusSeeOther || a.header.Get("Location") != "/" {
		t.Errorf("a page signed out: %d %s", a.status, a.header.Get("Location"))
	}
	if !strings.Contains(x.do("GET", x.public+"/", nil, nil).header.Get("Content-Security-Policy"), "frame-ancestors 'none'") {
		t.Error("no CSP on the public listener")
	}

	// --- an enrollment link works once
	if _, err := x.svc.dir.AddGroup(ctx, "cli", "famille", "Famille", ""); err != nil {
		t.Fatal(err)
	}
	lea, err := x.svc.dir.AddUser(ctx, "cli", "lea", "Léa", "", []string{"famille"})
	if err != nil {
		t.Fatal(err)
	}
	en, err := x.svc.auth.NewEnrollment(ctx, "admin", lea.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	link := x.public + "/enroll/" + en.Token
	if a := x.do("GET", link, nil, nil); a.status != 200 || cookieOf(a, auth.AppCookie) != "" {
		t.Fatalf("opening the link: %d (a GET must not use it)", a.status)
	}
	a = x.do("POST", link, nil, nil)
	leaPhone := cookieOf(a, auth.AppCookie)
	if a.status != http.StatusSeeOther || leaPhone == "" {
		t.Fatalf("using the link: %d, cookie %q", a.status, leaPhone)
	}
	if a := x.do("POST", link, nil, nil); a.status != http.StatusGone {
		t.Errorf("using the link again: %d, want 410", a.status)
	}
	if a := x.do("GET", x.public+"/approve", nil, cookie(auth.AppCookie, leaPhone)); !strings.Contains(a.body, `name="code"`) {
		t.Errorf("the approve page has no code field:\n%s", a.body)
	}
	if a := x.do("GET", link, nil, nil); !strings.Contains(a.body, `action="/enroll/`) {
		t.Errorf("the enrollment page has no button:\n%s", a.body)
	}
	if a := x.do("GET", x.public+"/", nil, cookie(auth.AppCookie, leaPhone)); a.status != 200 || !strings.Contains(a.body, `data-user="lea"`) {
		t.Errorf("home signed in: %d", a.status)
	}

	// --- an agd_app session never opens the admin UI
	for _, name := range []string{auth.AppCookie, auth.AdminCookie} {
		if a := x.do("GET", x.admin+"/", nil, cookie(name, leaPhone)); a.status != http.StatusSeeOther {
			t.Errorf("an app session as %s on /admin: %d, want a redirect to sign-in", name, a.status)
		}
	}

	// --- approval with the wrong number fails, and cancels the request
	shortCode, number, waitURL := x.startSignin()
	numbers := x.approvalNumbers(shortCode, leaPhone)
	wrong := numbers[0]
	if wrong == number {
		wrong = numbers[1]
	}
	if a := x.approve(shortCode, wrong, leaPhone); a.status != http.StatusGone || !strings.Contains(a.body, "cancelled") {
		t.Errorf("the wrong number: %d %s", a.status, a.body)
	}
	if a := x.do("GET", waitURL, nil, nil); a.status != http.StatusGone || cookieOf(a, auth.AppCookie) != "" {
		t.Errorf("the waiting browser after a wrong number: %d %s", a.status, a.body)
	}

	// --- and the right one signs the waiting browser in
	shortCode, number, waitURL = x.startSignin()
	waited := make(chan answer, 1)
	go func() { waited <- x.do("GET", waitURL, nil, nil) }()
	time.Sleep(100 * time.Millisecond) // the browser is waiting before the phone approves
	// From a foreign site, the same approval is refused.
	evil := func(r *http.Request) {
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Origin", "https://evil.example")
		r.AddCookie(&http.Cookie{Name: auth.AppCookie, Value: leaPhone})
	}
	if a := x.do("POST", x.public+"/api/approvals/"+url.PathEscape(shortCode), bytes.NewReader([]byte(`{"number":`+strconv.Itoa(number)+`}`)), evil); a.status != http.StatusForbidden {
		t.Errorf("a cross-origin approval: %d, want 403", a.status)
	}
	if a := x.approve(shortCode, number, leaPhone); a.status != http.StatusNoContent {
		t.Fatalf("the right number: %d %s", a.status, a.body)
	}
	w := <-waited
	laptop := cookieOf(w, auth.AppCookie)
	if w.status != 200 || !strings.Contains(w.body, "approved") || laptop == "" {
		t.Fatalf("the waiting browser: %d %s, cookie %q", w.status, w.body, laptop)
	}
	if a := x.do("GET", x.public+"/api/me", nil, cookie(auth.AppCookie, laptop)); a.status != 200 || !strings.Contains(a.body, `"lea"`) {
		t.Errorf("the laptop's /api/me: %d %s", a.status, a.body)
	}

	// --- a device credential opens its test route (/dav/) and nothing else
	dev, err := x.svc.auth.NewDevice(ctx, lea.ID, "davx5", "Pixel de Léa", "readwrite")
	if err != nil {
		t.Fatal(err)
	}
	basic := func(r *http.Request) { r.SetBasicAuth("lea", dev.Secret) }
	if a := x.do("PROPFIND", x.public+"/dav/", nil, basic); a.status != http.StatusMultiStatus {
		t.Errorf("/dav/ with the device: %d, want 207", a.status)
	}
	if a := x.do("PROPFIND", x.public+"/dav/", nil, nil); a.status != http.StatusUnauthorized || a.header.Get("WWW-Authenticate") == "" {
		t.Errorf("/dav/ without: %d", a.status)
	}
	if a := x.do("GET", x.public+"/api/me", nil, basic); a.status != http.StatusUnauthorized {
		t.Errorf("/api/me with a device credential: %d, want 401", a.status)
	}
	if a := x.do("POST", x.public+"/api/app/session", nil, basic); a.status != http.StatusForbidden {
		t.Errorf("a DAVx5 credential asking for a WebView session: %d, want 403", a.status)
	}
	if a := x.do("GET", x.admin+"/", nil, basic); a.status != http.StatusSeeOther {
		t.Errorf("the admin UI with a device credential: %d", a.status)
	}

	// --- the Android app enrolls and opens its WebView
	en2, _ := x.svc.auth.NewEnrollment(ctx, "admin", lea.ID, 0)
	a = x.do("POST", x.public+"/api/app/enroll", strings.NewReader(`{"token":"`+en2.Token+`","model":"Pixel 8","android":15,"app_version":"0.1.0"}`),
		func(r *http.Request) { r.Header.Set("Content-Type", "application/json") })
	var enrolled struct {
		Username string `json:"username"`
		Secret   string `json:"secret"`
		DavURL   string `json:"dav_url"`
	}
	if err := json.Unmarshal([]byte(a.body), &enrolled); err != nil || a.status != 200 {
		t.Fatalf("app enroll: %d %s", a.status, a.body)
	}
	if enrolled.Username != "lea" || !strings.HasPrefix(enrolled.Secret, "agd_") || enrolled.DavURL != x.public+"/dav/" {
		t.Fatalf("app enroll answer: %s", a.body)
	}
	appBasic := func(r *http.Request) { r.SetBasicAuth("lea", enrolled.Secret) }
	a = x.do("POST", x.public+"/api/app/session", nil, appBasic)
	webview := cookieOf(a, auth.AppCookie)
	if a.status != http.StatusNoContent || webview == "" {
		t.Fatalf("app session: %d", a.status)
	}
	if a := x.do("GET", x.public+"/api/me", nil, cookie(auth.AppCookie, webview)); a.status != 200 {
		t.Errorf("the WebView's /api/me: %d", a.status)
	}
	// The app's screens, as its Java sends them: no Origin, no Sec-Fetch-*.
	appJSON := func(r *http.Request) {
		r.Header.Set("Cookie", auth.AppCookie+"="+webview)
		r.Header.Set("Content-Type", "application/json")
	}
	if a := x.do("POST", x.public+"/api/calendars", strings.NewReader(`{"name":"Depuis le téléphone","color":"#43a047"}`), appJSON); a.status != http.StatusCreated {
		t.Errorf("the app creates a calendar: %d %s", a.status, a.body)
	}
	code, number, _ := x.startSignin()
	if a := x.do("GET", x.public+"/api/approvals/"+url.PathEscape(code), nil, appJSON); a.status != 200 || !strings.Contains(a.body, `"numbers"`) {
		t.Errorf("the app looks an approval up: %d %s", a.status, a.body)
	}
	if a := x.do("POST", x.public+"/api/approvals/"+url.PathEscape(code), strings.NewReader(fmt.Sprintf(`{"number":%d}`, number)), appJSON); a.status != http.StatusNoContent {
		t.Errorf("the app approves a browser: %d %s", a.status, a.body)
	}

	// --- moving the public listener, then turning it off, live
	old := x.public
	x.publicOn()
	if a := x.do("GET", x.public+"/healthz", nil, nil); a.status != 200 {
		t.Errorf("after the move: %d", a.status)
	}
	if _, err := http.Get(old + "/healthz"); err == nil {
		// Draining may keep the old listener for a moment; it must not accept new connections.
		time.Sleep(200 * time.Millisecond)
		if _, err := http.Get(old + "/healthz"); err == nil {
			t.Error("the old public address still answers after the move")
		}
	}
}

var (
	shortCodeRe = regexp.MustCompile(`id="signin-short-code"[^>]*>([^<]+)<`)
	numberRe    = regexp.MustCompile(`id="signin-number"[^>]*>([0-9]+)<`)
	waitRe      = regexp.MustCompile(`action="(/signin/wait/[0-9a-f]+)"`)
)

// startSignin opens the root as a new browser would (the sign-in page) and reads its codes.
func (x *e2e) startSignin() (shortCode string, number int, waitURL string) {
	x.t.Helper()
	a := x.do("GET", x.public+"/", nil, nil)
	sc, n, w := shortCodeRe.FindStringSubmatch(a.body), numberRe.FindStringSubmatch(a.body), waitRe.FindStringSubmatch(a.body)
	if a.status != 200 || sc == nil || n == nil || w == nil {
		x.t.Fatalf("/signin: %d\n%s", a.status, a.body)
	}
	// html/template writes the + of svg+xml as &#43;, which browsers decode.
	if !strings.Contains(a.body, `src="data:image/svg&#43;xml;base64,`) {
		i := strings.Index(a.body, `id="signin-qr"`)
		x.t.Errorf("/signin has no QR code: %s", a.body[max(0, i-200):min(len(a.body), i+100)])
	}
	number, _ = strconv.Atoi(n[1])
	return sc[1], number, x.public + w[1]
}

func (x *e2e) approvalNumbers(shortCode, session string) []int {
	x.t.Helper()
	a := x.do("GET", x.public+"/api/approvals/"+url.PathEscape(shortCode), nil, cookie(auth.AppCookie, session))
	var view struct {
		Numbers []int `json:"numbers"`
	}
	if err := json.Unmarshal([]byte(a.body), &view); err != nil || a.status != 200 || len(view.Numbers) != 3 {
		x.t.Fatalf("looking the approval up: %d %s", a.status, a.body)
	}
	return view.Numbers
}

func (x *e2e) approve(shortCode string, number int, session string) answer {
	x.t.Helper()
	return x.do("POST", x.public+"/api/approvals/"+url.PathEscape(shortCode), strings.NewReader(`{"number":`+strconv.Itoa(number)+`}`),
		func(r *http.Request) {
			r.Header.Set("Content-Type", "application/json")
			r.AddCookie(&http.Cookie{Name: auth.AppCookie, Value: session})
		})
}
