package config

import (
	"flag"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func load(t *testing.T, file string, env map[string]string, args ...string) (*Config, error) {
	t.Helper()
	set := flag.NewFlagSet("test", flag.ContinueOnError)
	set.SetOutput(io.Discard)
	l := NewLoader(set)
	if file != "" {
		path := filepath.Join(t.TempDir(), "agendouille.yaml")
		if err := os.WriteFile(path, []byte(file), 0o600); err != nil {
			t.Fatal(err)
		}
		args = append([]string{"-config", path}, args...)
	}
	if err := set.Parse(args); err != nil {
		t.Fatal(err)
	}
	cfg, _, err := l.Load(func(k string) string { return env[k] })
	return cfg, err
}

func TestDefaultsAreValid(t *testing.T) {
	c := Default()
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	if c.AdminURL() != "http://127.0.0.1:8792" {
		t.Errorf("AdminURL = %q", c.AdminURL())
	}
}

func TestFileThenEnvironmentThenFlags(t *testing.T) {
	cfg, err := load(t, "log_level: warn\ndata_dir: /srv/a\n",
		map[string]string{"AGENDOUILLE_LOG_LEVEL": "error", "AGENDOUILLE_DATA_DIR": "/srv/b"},
		"-data-dir", "/srv/c")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.LogLevel != "error" {
		t.Errorf("log_level = %q, want the environment's", cfg.LogLevel)
	}
	if cfg.DataDir != "/srv/c" {
		t.Errorf("data_dir = %q, want the flag's", cfg.DataDir)
	}
}

func TestUnknownKeysAreErrors(t *testing.T) {
	if _, err := load(t, "public_listen: 0.0.0.0:8791\n", nil); err == nil {
		t.Fatal("public_listen is a setting, not a bootstrap key: want an error")
	}
}

func TestAdminExposure(t *testing.T) {
	cases := []struct {
		name, file string
		wantErr    string
	}{
		{"loopback without a URL", "admin_listen: 127.0.0.1:9000\n", ""},
		{"localhost without a URL", "admin_listen: localhost:9000\n", ""},
		{"LAN without a URL", "admin_listen: 10.0.50.30:8792\n", "must be set"},
		{"every interface without a URL", "admin_listen: :8792\n", "must be set"},
		{"LAN over plain http", "admin_listen: 10.0.50.30:8792\nadmin_public_url: http://agenda.lan:8792\n", "plain http"},
		{"LAN over http, accepted", "admin_listen: 10.0.50.30:8792\nadmin_public_url: http://agenda.lan:8792\nadmin_allow_insecure: true\n", ""},
		{"LAN over https", "admin_listen: 10.0.50.30:8792\nadmin_public_url: https://admin.agenda.lan\n", ""},
		{"a URL with a query", "admin_public_url: https://x/?a=1\n", "plain scheme"},
		{"a bad level", "log_level: loud\n", "log_level"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := load(t, c.file, nil)
			switch {
			case c.wantErr == "" && err != nil:
				t.Fatalf("unexpected error: %v", err)
			case c.wantErr != "" && (err == nil || !strings.Contains(err.Error(), c.wantErr)):
				t.Fatalf("err = %v, want one containing %q", err, c.wantErr)
			}
		})
	}
}

func TestAdminURLDropsTheTrailingSlash(t *testing.T) {
	c := Default()
	c.AdminPublicURL = "https://admin.agenda.lan/"
	if c.AdminURL() != "https://admin.agenda.lan" {
		t.Errorf("AdminURL = %q", c.AdminURL())
	}
}

func TestEveryScalarKeyHasAFlagAndAVariable(t *testing.T) {
	names := map[string]bool{}
	for _, k := range scalarKeys() {
		names[k.flagName()+" "+k.envName()] = true
	}
	for _, want := range []string{
		"data-dir AGENDOUILLE_DATA_DIR",
		"admin-listen AGENDOUILLE_ADMIN_LISTEN",
		"admin-public-url AGENDOUILLE_ADMIN_PUBLIC_URL",
		"admin-allow-insecure AGENDOUILLE_ADMIN_ALLOW_INSECURE",
		"log-level AGENDOUILLE_LOG_LEVEL",
	} {
		if !names[want] {
			t.Errorf("missing %s in %v", want, names)
		}
	}
}
