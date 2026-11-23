package auth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nebuloss/agendouille/internal/directory"
	"github.com/nebuloss/agendouille/internal/settings"
	"github.com/nebuloss/agendouille/internal/store"
)

type clock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *clock) Now() time.Time      { c.mu.Lock(); defer c.mu.Unlock(); return c.t }
func (c *clock) Add(d time.Duration) { c.mu.Lock(); c.t = c.t.Add(d); c.mu.Unlock() }

type fixture struct {
	auth  *Service
	dir   *directory.Service
	clock *clock
	lea   directory.User
	db    *store.Store
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	ctx := context.Background()
	st, err := store.OpenDir(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	c := &clock{t: time.Date(2026, 11, 23, 9, 0, 0, 0, time.UTC)}
	set := settings.New(settings.Default())
	dir := directory.New(st.DB(), c.Now, func() []string { return settings.DefaultPalette })
	lea, err := dir.AddUser(ctx, "admin", "lea", "Léa", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	return &fixture{auth: New(st.DB(), c.Now, set, dir), dir: dir, clock: c, lea: lea, db: st}
}

func withCookie(name, value string) *http.Request {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.AddCookie(&http.Cookie{Name: name, Value: value})
	return r
}

func TestEnrolmentWorksOnce(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	e, err := f.auth.NewEnrolment(ctx, "admin", f.lea.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	if !e.ExpiresAt.Equal(f.clock.Now().Add(72 * time.Hour)) {
		t.Errorf("expires %v", e.ExpiresAt)
	}
	secret, err := f.auth.RedeemForBrowser(ctx, e.Token, "Firefox")
	if err != nil {
		t.Fatal(err)
	}
	p, err := f.auth.Session(withCookie(AppCookie, secret), SurfaceApp)
	if err != nil || p.User.Name != "lea" {
		t.Fatalf("the session: %+v, %v", p, err)
	}
	if _, err := f.auth.RedeemForBrowser(ctx, e.Token, "Firefox"); !errors.Is(err, ErrUsed) {
		t.Errorf("a second use: %v, want ErrUsed", err)
	}
	if _, err := f.auth.RedeemForApp(ctx, e.Token, "Pixel 8"); !errors.Is(err, ErrUsed) {
		t.Errorf("a second use by the app: %v, want ErrUsed", err)
	}
	if _, err := f.auth.RedeemForBrowser(ctx, "not-a-token", "Firefox"); !errors.Is(err, ErrInvalid) {
		t.Errorf("an unknown token: %v", err)
	}
}

func TestEnrolmentExpiresAndCanBeRevoked(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	e, _ := f.auth.NewEnrolment(ctx, "admin", f.lea.ID, time.Hour)
	f.clock.Add(time.Hour)
	if _, err := f.auth.RedeemForBrowser(ctx, e.Token, ""); !errors.Is(err, ErrExpired) {
		t.Errorf("after its hour: %v", err)
	}
	e2, _ := f.auth.NewEnrolment(ctx, "admin", f.lea.ID, 0)
	if err := f.auth.RevokeEnrolment(ctx, f.lea.ID, e2.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.auth.RedeemForBrowser(ctx, e2.Token, ""); !errors.Is(err, ErrInvalid) {
		t.Errorf("a revoked link: %v", err)
	}
}

func TestTheAppEnrolsAsADeviceAndOpensItsWebView(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	e, _ := f.auth.NewEnrolment(ctx, "admin", f.lea.ID, 0)
	d, err := f.auth.RedeemForApp(ctx, e.Token, "Pixel 8")
	if err != nil {
		t.Fatal(err)
	}
	if d.Kind != "app" || d.Username != "lea" || !strings.HasPrefix(d.Secret, "agd_") {
		t.Fatalf("device %+v", d)
	}
	r := httptest.NewRequest(http.MethodPost, "/api/app/session", nil)
	r.SetBasicAuth("lea", d.Secret)
	p, err := f.auth.Basic(r, "192.0.2.1")
	if err != nil {
		t.Fatal(err)
	}
	secret, err := f.auth.AppSession(ctx, p, "WebView")
	if err != nil {
		t.Fatal(err)
	}
	web, err := f.auth.Session(withCookie(AppCookie, secret), SurfaceApp)
	if err != nil || web.DeviceID != d.ID {
		t.Fatalf("WebView session: %+v %v", web, err)
	}
	// Revoking the device ends its WebView session too.
	if err := f.auth.RevokeDevice(ctx, "lea", f.lea.ID, d.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.auth.Session(withCookie(AppCookie, secret), SurfaceApp); !errors.Is(err, ErrInvalid) {
		t.Errorf("after revoking the device: %v", err)
	}
	if _, err := f.auth.Basic(r, "192.0.2.1"); !errors.Is(err, ErrInvalid) {
		t.Errorf("a revoked device's credential: %v", err)
	}
}

func TestOnlyAnAppDeviceOpensAWebSession(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	d, _ := f.auth.NewDevice(ctx, f.lea.ID, "davx5", "Pixel de Léa", "readwrite")
	r := httptest.NewRequest(http.MethodPost, "/", nil)
	r.SetBasicAuth("lea", d.Secret)
	p, err := f.auth.Basic(r, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.auth.AppSession(ctx, p, ""); !errors.Is(err, ErrInvalid) {
		t.Errorf("a DAVx5 credential opened a web session: %v", err)
	}
}

func TestBasicAuth(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	d, _ := f.auth.NewDevice(ctx, f.lea.ID, "thunderbird", "Laptop", "read")
	try := func(user, pass string) error {
		r := httptest.NewRequest(http.MethodGet, "/dav/", nil)
		r.SetBasicAuth(user, pass)
		_, err := f.auth.Basic(r, "")
		return err
	}
	if err := try("lea", d.Secret); err != nil {
		t.Fatal(err)
	}
	// Typed by hand: any case, without the dashes.
	if err := try("lea", strings.ToUpper(strings.ReplaceAll(d.Secret, "-", ""))); err != nil {
		t.Errorf("a retyped secret: %v", err)
	}
	f.dir.AddUser(ctx, "admin", "paul", "", "", nil)
	for _, c := range [][2]string{{"paul", d.Secret}, {"lea", "agd_wrong"}, {"lea", ""}} {
		if err := try(c[0], c[1]); !errors.Is(err, ErrInvalid) {
			t.Errorf("%v: %v", c, err)
		}
	}
	f.dir.SetDisabled(ctx, "admin", "lea", true)
	if err := try("lea", d.Secret); !errors.Is(err, ErrInvalid) {
		t.Errorf("a disabled user's device: %v", err)
	}
}

func TestDeviceLimit(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	for i := range 20 {
		if _, err := f.auth.NewDevice(ctx, f.lea.ID, "other", "d", "read"); err != nil {
			t.Fatalf("device %d: %v", i, err)
		}
	}
	if _, err := f.auth.NewDevice(ctx, f.lea.ID, "other", "d", "read"); !errors.Is(err, ErrTooMany) {
		t.Errorf("the 21st device: %v", err)
	}
}

func TestApprovalWithTheRightNumber(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	phone := &Principal{User: f.lea, Surface: SurfaceApp}
	a, err := f.auth.StartApproval(ctx, "Firefox on Linux", "192.168.1.20")
	if err != nil {
		t.Fatal(err)
	}
	if len(a.ShortCode) != 7 || a.ShortCode[3] != '-' || a.Number < 10 || a.Number > 99 {
		t.Fatalf("approval %+v", a)
	}
	view, err := f.auth.LookupApproval(ctx, phone, strings.ToLower(strings.ReplaceAll(a.ShortCode, "-", " ")))
	if err != nil {
		t.Fatal(err)
	}
	if len(view.Numbers) != 3 || view.UserAgent != "Firefox on Linux" || view.IP != "192.168.1.20" {
		t.Fatalf("view %+v", view)
	}
	found := false
	for _, n := range view.Numbers {
		found = found || n == a.Number
	}
	if !found {
		t.Fatalf("the right number %d is not among %v", a.Number, view.Numbers)
	}

	got := make(chan string, 1)
	errs := make(chan error, 1)
	go func() {
		s, err := f.auth.WaitApproval(ctx, a.ID, "Firefox on Linux")
		got <- s
		errs <- err
	}()
	if err := f.auth.Approve(ctx, phone, a.Code, a.Number); err != nil {
		t.Fatal(err)
	}
	secret := <-got
	if err := <-errs; err != nil {
		t.Fatal(err)
	}
	p, err := f.auth.Session(withCookie(AppCookie, secret), SurfaceApp)
	if err != nil || p.User.Name != "lea" {
		t.Fatalf("the approved browser: %+v %v", p, err)
	}
	if _, err := f.auth.WaitApproval(ctx, a.ID, ""); !errors.Is(err, ErrUsed) {
		t.Errorf("collected twice: %v", err)
	}
	if err := f.auth.Approve(ctx, phone, a.Code, a.Number); err == nil {
		t.Error("approved twice")
	}
}

func TestApprovalWithTheWrongNumberIsCancelled(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	phone := &Principal{User: f.lea, Surface: SurfaceApp}
	a, _ := f.auth.StartApproval(ctx, "x", "y")
	if err := f.auth.Approve(ctx, phone, a.Code, a.Number+1); !errors.Is(err, ErrWrongNumber) {
		t.Fatalf("wrong number: %v", err)
	}
	if err := f.auth.Approve(ctx, phone, a.Code, a.Number); !errors.Is(err, ErrInvalid) {
		t.Errorf("the right number after a wrong one: %v, want the request gone", err)
	}
	if _, err := f.auth.WaitApproval(ctx, a.ID, ""); !errors.Is(err, ErrWrongNumber) {
		t.Errorf("the waiting browser hears: %v", err)
	}
}

func TestApprovalNeedsASignedInUserAndExpires(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	a, _ := f.auth.StartApproval(ctx, "x", "y")
	if err := f.auth.Approve(ctx, &Principal{Admin: true, Surface: SurfaceAdmin}, a.Code, a.Number); !errors.Is(err, ErrInvalid) {
		t.Errorf("the admin approved a family sign-in: %v", err)
	}
	if err := f.auth.Approve(ctx, nil, a.Code, a.Number); !errors.Is(err, ErrInvalid) {
		t.Errorf("nobody approved: %v", err)
	}
	f.clock.Add(ApprovalTTL)
	if err := f.auth.Approve(ctx, &Principal{User: f.lea, Surface: SurfaceApp}, a.Code, a.Number); !errors.Is(err, ErrExpired) {
		t.Errorf("after two minutes: %v", err)
	}
	if _, err := f.auth.WaitApproval(ctx, a.ID, ""); !errors.Is(err, ErrExpired) {
		t.Errorf("the waiting browser: %v", err)
	}
}

func TestPendingApprovalsAreCapped(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	for range maxPending {
		if _, err := f.auth.StartApproval(ctx, "", ""); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := f.auth.StartApproval(ctx, "", ""); !errors.Is(err, ErrBusy) {
		t.Fatalf("over the cap: %v", err)
	}
	f.clock.Add(ApprovalTTL + time.Second)
	if _, err := f.auth.StartApproval(ctx, "", ""); err != nil {
		t.Errorf("once they expired: %v", err)
	}
}

func TestAdminPasswordAndSessionsStayOnTheirSurface(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	if ok, _ := f.auth.HasAdminPassword(ctx); ok {
		t.Fatal("a password before one was set")
	}
	if _, err := f.auth.AdminSignIn(ctx, "anything at all", ""); !errors.Is(err, ErrWrongPassword) {
		t.Errorf("no password set: %v", err)
	}
	if err := f.auth.SetAdminPassword(ctx, "cli", "short"); !errors.Is(err, ErrShortPassword) {
		t.Errorf("a short password: %v", err)
	}
	pw := RandomPassword()
	if len(pw) != 29 {
		t.Errorf("random password %q", pw)
	}
	if err := f.auth.SetAdminPassword(ctx, "cli", pw); err != nil {
		t.Fatal(err)
	}
	if _, err := f.auth.AdminSignIn(ctx, pw+"x", ""); !errors.Is(err, ErrWrongPassword) {
		t.Errorf("a wrong password: %v", err)
	}
	secret, err := f.auth.AdminSignIn(ctx, pw, "Firefox")
	if err != nil {
		t.Fatal(err)
	}
	if p, err := f.auth.Session(withCookie(AdminCookie, secret), SurfaceAdmin); err != nil || !p.Admin {
		t.Fatalf("admin session: %+v %v", p, err)
	}
	// The admin's secret under the app's cookie name opens nothing.
	if _, err := f.auth.Session(withCookie(AppCookie, secret), SurfaceApp); !errors.Is(err, ErrInvalid) {
		t.Errorf("an admin session on the app surface: %v", err)
	}
	// And an app session never opens the admin surface.
	e, _ := f.auth.NewEnrolment(ctx, "admin", f.lea.ID, 0)
	app, _ := f.auth.RedeemForBrowser(ctx, e.Token, "")
	if _, err := f.auth.Session(withCookie(AdminCookie, app), SurfaceAdmin); !errors.Is(err, ErrInvalid) {
		t.Errorf("an app session on the admin surface: %v", err)
	}
	// Admin sessions last 12 hours, used or not.
	f.clock.Add(AdminSessionTTL)
	if _, err := f.auth.Session(withCookie(AdminCookie, secret), SurfaceAdmin); !errors.Is(err, ErrInvalid) {
		t.Errorf("after 12 hours: %v", err)
	}
	// A new password ends admin sessions.
	s2, _ := f.auth.AdminSignIn(ctx, pw, "")
	f.auth.SetAdminPassword(ctx, "cli", pw+"-new")
	if _, err := f.auth.Session(withCookie(AdminCookie, s2), SurfaceAdmin); !errors.Is(err, ErrInvalid) {
		t.Errorf("after a new password: %v", err)
	}
}

func TestAppSessionsRenewWhileUsed(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	e, _ := f.auth.NewEnrolment(ctx, "admin", f.lea.ID, 0)
	secret, _ := f.auth.RedeemForBrowser(ctx, e.Token, "")
	for range 4 { // 4 x 60 days, used each time: still alive at 240 days
		f.clock.Add(60 * 24 * time.Hour)
		if _, err := f.auth.Session(withCookie(AppCookie, secret), SurfaceApp); err != nil {
			t.Fatalf("a used session expired: %v", err)
		}
	}
	f.clock.Add(91 * 24 * time.Hour)
	if _, err := f.auth.Session(withCookie(AppCookie, secret), SurfaceApp); !errors.Is(err, ErrInvalid) {
		t.Errorf("after 91 idle days: %v", err)
	}
	// Removing the user ends their sessions at once.
	e2, _ := f.auth.NewEnrolment(ctx, "admin", f.lea.ID, 0)
	s2, _ := f.auth.RedeemForBrowser(ctx, e2.Token, "")
	f.dir.Remove(ctx, "admin", "lea")
	if _, err := f.auth.Session(withCookie(AppCookie, s2), SurfaceApp); !errors.Is(err, ErrInvalid) {
		t.Errorf("a removed user's session: %v", err)
	}
}

func TestClientIP(t *testing.T) {
	trusted := []netip.Prefix{netip.MustParsePrefix("10.0.40.0/24")}
	cases := []struct{ peer, xff, want string }{
		{"203.0.113.9:1234", "", "203.0.113.9"},
		{"203.0.113.9:1234", "1.2.3.4", "203.0.113.9"},              // not from the proxy: ignored
		{"10.0.40.2:1234", "198.51.100.7", "198.51.100.7"},          // from the proxy
		{"10.0.40.2:1234", "6.6.6.6, 198.51.100.7", "198.51.100.7"}, // the client cannot add hops on the right
		{"10.0.40.2:1234", "198.51.100.7, 10.0.40.3", "198.51.100.7"},
		{"10.0.40.2:1234", "garbage", "10.0.40.2"},
		{"10.0.40.2:1234", "", "10.0.40.2"},
	}
	for _, c := range cases {
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.RemoteAddr = c.peer
		if c.xff != "" {
			r.Header.Set("X-Forwarded-For", c.xff)
		}
		if got := ClientIP(r, trusted).String(); got != c.want {
			t.Errorf("%s via %q: %s, want %s", c.peer, c.xff, got, c.want)
		}
	}
}

func TestSameOrigin(t *testing.T) {
	cases := []struct {
		site, origin string
		want         bool
	}{
		{"same-origin", "", true},
		{"cross-site", "https://agenda.example.org", false},
		{"same-site", "", false},
		{"", "https://agenda.example.org", true},
		{"", "https://evil.example", false},
		{"", "http://agenda.example.org", false}, // the scheme counts
		{"", "", true},
	}
	for _, c := range cases {
		r := httptest.NewRequest(http.MethodPost, "/", nil)
		if c.site != "" {
			r.Header.Set("Sec-Fetch-Site", c.site)
		}
		if c.origin != "" {
			r.Header.Set("Origin", c.origin)
		}
		if got := SameOrigin(r, "https://agenda.example.org"); got != c.want {
			t.Errorf("%+v: %v", c, got)
		}
	}
}
