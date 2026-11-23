package main

import (
	"context"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nebuloss/agendouille/internal/config"
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
	cfg := config.Default()
	cfg.DataDir = t.TempDir()
	cfg.AdminListen = "127.0.0.1:0"
	log := slog.New(slog.NewTextHandler(io.Discard, nil))

	ctx, cancel := context.WithCancel(context.Background())
	addr := make(chan net.Addr, 1)
	done := make(chan error, 1)
	go func() { done <- serve(ctx, &cfg, log, func(a net.Addr) { addr <- a }) }()

	var base string
	select {
	case a := <-addr:
		base = "http://" + a.String()
	case err := <-done:
		t.Fatalf("serve returned early: %v", err)
	case <-time.After(5 * time.Second):
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

	// The family app's bundle is never on the admin listener.
	resp, err = http.Get(base + "/static/app/app.js")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("/static/app/app.js on the admin listener: %d, want 404", resp.StatusCode)
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

func TestServeFailsOnATakenPort(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	cfg := config.Default()
	cfg.AdminListen = ln.Addr().String()
	err = serve(context.Background(), &cfg, slog.New(slog.NewTextHandler(io.Discard, nil)), nil)
	if err == nil || !strings.Contains(err.Error(), "admin listener") {
		t.Fatalf("err = %v", err)
	}
}
