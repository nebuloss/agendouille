package main

import (
	"context"
	"flag"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nebuloss/agendouille/internal/config"
	"github.com/nebuloss/agendouille/internal/surface"
)

func runArgs(t *testing.T, vars map[string]string, args ...string) (string, string, error) {
	t.Helper()
	var out, errOut strings.Builder
	err := run(context.Background(), args, strings.NewReader(""), &out, &errOut, func(k string) string { return vars[k] })
	return out.String(), errOut.String(), err
}

func TestVersionPrintsDev(t *testing.T) {
	for _, arg := range []string{"version", "--version"} {
		out, _, err := runArgs(t, nil, arg)
		if err != nil {
			t.Fatal(err)
		}
		if out != "agendouille dev\n" {
			t.Errorf("%s: out = %q", arg, out)
		}
	}
}

func TestUnknownCommandFails(t *testing.T) {
	if _, _, err := runArgs(t, nil, "frobnicate"); err == nil {
		t.Fatal("want an error")
	}
	if _, _, err := runArgs(t, nil); err == nil {
		t.Fatal("want an error with no command")
	}
}

func TestHelpIsNotAnError(t *testing.T) {
	out, _, err := runArgs(t, nil, "help")
	if err != nil || !strings.Contains(out, "Commands:") {
		t.Fatalf("help: out = %q, err = %v", out, err)
	}
	if _, _, err := runArgs(t, nil, "serve", "-h"); err != nil {
		t.Fatalf("serve -h: %v", err)
	}
}

func TestConfigPrintsTheEffectiveConfiguration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agendouille.yaml")
	if err := os.WriteFile(path, []byte("data_dir: /srv/agendouille\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	out, _, err := runArgs(t, map[string]string{"AGENDOUILLE_LOG_LEVEL": "debug"}, "config", "-config", path, "-admin-listen", "127.0.0.1:9")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"# loaded from " + path, "data_dir: /srv/agendouille", "admin_listen: 127.0.0.1:9", "log_level: debug"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in\n%s", want, out)
		}
	}
}

func TestConfigFailsOnAnInvalidConfiguration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agendouille.yaml")
	if err := os.WriteFile(path, []byte("admin_listen: 0.0.0.0:8792\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := runArgs(t, nil, "config", "-config", path); err == nil || !strings.Contains(err.Error(), "admin_public_url") {
		t.Fatalf("an admin UI on every interface without a URL: err = %v", err)
	}
}

func TestServeAnswersHealthzAndStopsCleanly(t *testing.T) {
	cfg := testConfig(t)
	withAdminPassword(t, cfg)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))

	ctx, cancel := context.WithCancel(context.Background())
	up := make(chan *surface.Server, 1)
	done := make(chan error, 1)
	go func() { done <- serve(ctx, cfg, log, func(s *surface.Server) { up <- s }) }()

	var base string
	select {
	case s := <-up:
		base = "http://" + s.AdminAddr().String()
	case err := <-done:
		t.Fatalf("serve returned early: %v", err)
	case <-time.After(10 * time.Second):
		t.Fatal("serve did not start")
	}

	resp, err := http.Get(base + "/healthz")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK || string(body) != "ok dev\n" {
		t.Errorf("/healthz: %d %q", resp.StatusCode, body)
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("serve: %v", err)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("serve did not stop")
	}
}

func TestServeRefusesToStartWithoutAnAdminPassword(t *testing.T) {
	cfg := testConfig(t)
	err := serve(context.Background(), cfg, slog.New(slog.NewTextHandler(io.Discard, nil)), nil)
	if err == nil || !strings.Contains(err.Error(), "admin passwd") {
		t.Fatalf("err = %v", err)
	}
}

func TestServeFailsOnATakenPort(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	cfg := testConfig(t)
	withAdminPassword(t, cfg)
	cfg.AdminListen = ln.Addr().String()
	err = serve(context.Background(), cfg, slog.New(slog.NewTextHandler(io.Discard, nil)), nil)
	if err == nil || !strings.Contains(err.Error(), "admin listener") {
		t.Fatalf("err = %v", err)
	}
}

func testConfig(t *testing.T) *config.Config {
	t.Helper()
	cfg := config.Default()
	cfg.DataDir = filepath.Join(t.TempDir(), "data")
	cfg.AdminListen = "127.0.0.1:0"
	return &cfg
}

const testAdminPassword = "correct horse battery staple"

func withAdminPassword(t *testing.T, cfg *config.Config) {
	t.Helper()
	svc, err := openServices(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer svc.store.Close()
	if err := svc.auth.SetAdminPassword(context.Background(), "test", testAdminPassword); err != nil {
		t.Fatal(err)
	}
}

// install.sh runs `admin passwd --if-unset` on every install: the first
// sets the password, an upgrade leaves it alone.
func TestAdminPasswdIfUnset(t *testing.T) {
	vars := map[string]string{"AGENDOUILLE_DATA_DIR": t.TempDir()}
	passwd := func(pw string) string {
		t.Helper()
		var out strings.Builder
		err := run(context.Background(), []string{"admin", "passwd", "--if-unset"}, strings.NewReader(pw+"\n"), &out, io.Discard,
			func(k string) string { return vars[k] })
		if err != nil {
			t.Fatal(err)
		}
		return out.String()
	}
	if out := passwd("first password here"); !strings.HasPrefix(out, "Admin password set.") {
		t.Errorf("first: %q", out)
	}
	if out := passwd("second password here"); !strings.HasPrefix(out, "An admin password is already set") {
		t.Errorf("second: %q", out)
	}
}

func TestFaviconIsTheAndouille(t *testing.T) {
	rec := httptest.NewRecorder()
	favicon(rec, httptest.NewRequest("GET", "/favicon.ico", nil))
	if rec.Code != 200 || rec.Header().Get("Content-Type") != "image/svg+xml" || !strings.Contains(rec.Body.String(), "andouille") {
		t.Errorf("favicon: %d %q", rec.Code, rec.Header().Get("Content-Type"))
	}
}

// config migrate moves the settings an older version kept in the database
// into the file, then empties the table; config set says where to edit.
func TestConfigMigrate(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "agendouille.yaml")
	if err := os.WriteFile(path, []byte("data_dir: "+filepath.Join(dir, "data")+"\nbackup_keep: 20\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	vars := map[string]string{"AGENDOUILLE_CONFIG": path}
	cfg, _, err := config.NewLoader(flag.NewFlagSet("t", flag.ContinueOnError)).Load(func(k string) string { return vars[k] })
	if err != nil {
		t.Fatal(err)
	}
	svc, err := openServices(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	svc.store.DB().Exec(`INSERT INTO settings(key, value, updated_at) VALUES
		('public_listen', '"0.0.0.0:8791"', 'x'), ('public_url', '"https://agenda.example.org"', 'x'), ('backup_keep', '3', 'x')`)
	svc.store.Close()

	out, _, err := runArgs(t, vars, "config", "migrate")
	if err != nil || !strings.Contains(out, "Moved 2 setting(s)") {
		t.Fatalf("migrate: %q %v", out, err)
	}
	body, _ := os.ReadFile(path)
	if !strings.Contains(string(body), `public_url: "https://agenda.example.org"`) || !strings.Contains(string(body), "backup_keep: 20") ||
		strings.Contains(string(body), "backup_keep: 3") {
		t.Errorf("the file: %s", body)
	}
	if info, _ := os.Stat(path); info.Mode().Perm() != 0o640 {
		t.Errorf("mode %v", info.Mode().Perm())
	}
	if out, _, _ := runArgs(t, vars, "config", "migrate"); !strings.Contains(out, "nothing to move") {
		t.Errorf("twice: %q", out)
	}
	if _, _, err := runArgs(t, vars, "config", "set", "backup_keep", "4"); err == nil || !strings.Contains(err.Error(), path) {
		t.Errorf("config set: %v", err)
	}
}
