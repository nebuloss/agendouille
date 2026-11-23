package settings

import (
	"context"
	"strings"
	"testing"

	"github.com/nebuloss/agendouille/internal/store"
)

func open(t *testing.T) (*store.Store, *Service) {
	t.Helper()
	st, err := store.OpenDir(context.Background(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	return st, New(Default())
}

func TestDefaultsAreValid(t *testing.T) {
	if err := Default().Validate(); err != nil {
		t.Fatal(err)
	}
	if len(DefaultPalette) != 12 {
		t.Errorf("palette has %d colours, want 12", len(DefaultPalette))
	}
}

func TestApplyNotifies(t *testing.T) {
	_, s := open(t)
	var heard []Change
	s.Subscribe(func(c Change) { heard = append(heard, c) })
	_, err := s.Apply(map[string]string{
		"public_listen":   "0.0.0.0:8791",
		"public_url":      "https://agenda.example.org",
		"trusted_proxies": "10.0.40.0/24, 192.168.1.1",
		"backup_keep":     "30",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(heard) != 1 || heard[0].Old.PublicListen != "" || heard[0].New.PublicListen != "0.0.0.0:8791" || len(heard[0].Keys) != 4 {
		t.Fatalf("heard %+v", heard)
	}
	got := s.Get()
	if got.BackupKeep != 30 || len(got.TrustedProxies) != 2 || got.TrustedPrefixes()[1].String() != "192.168.1.1/32" {
		t.Fatalf("got %+v", got)
	}
	// The same values again: nothing changed, nobody is told.
	if keys, err := s.Replace(s.Get()); err != nil || len(keys) != 0 || len(heard) != 1 {
		t.Fatalf("a non-change: %v %v %d", keys, err, len(heard))
	}
}

// The table an older version kept its settings in is read once, for the
// migration into the file, and then emptied.
func TestStoredAndForgotten(t *testing.T) {
	st, _ := open(t)
	ctx := context.Background()
	st.DB().Exec(`INSERT INTO settings(key, value, updated_at) VALUES ('public_url', '"https://a.example"', 'x'), ('gone_key', '1', 'x')`)
	got, err := Stored(ctx, st.DB())
	if err != nil || len(got) != 1 || got["public_url"] != `"https://a.example"` {
		t.Fatalf("stored: %v %v", got, err)
	}
	if err := ForgetStored(ctx, st.DB()); err != nil {
		t.Fatal(err)
	}
	if got, _ := Stored(ctx, st.DB()); len(got) != 0 {
		t.Fatalf("not forgotten: %v", got)
	}
}

func TestApplyIsAllOrNothing(t *testing.T) {
	_, s := open(t)
	if _, err := s.Apply(map[string]string{"public_listen": "0.0.0.0:8791"}); err == nil ||
		!strings.Contains(err.Error(), "public_url") {
		t.Fatalf("public_listen without public_url: %v", err)
	}
	if s.Get().PublicListen != "" {
		t.Fatal("a refused change was applied")
	}
}

func TestRules(t *testing.T) {
	cases := map[string]map[string]string{
		"plain http off loopback":    {"public_listen": "0.0.0.0:1", "public_url": "http://agenda.lan"},
		"a URL with a path":          {"public_url": "https://agenda.example.org/cal"},
		"a bad proxy":                {"trusted_proxies": "the proxy"},
		"a bad zone":                 {"family_timezone": "Mars/Olympus"},
		"a bad colour":               {"palette": "red"},
		"a bad time":                 {"backup_schedule": "25:00"},
		"zero devices":               {"max_devices_per_user": "0"},
		"sync faster than Android":   {"app_sync_interval_minutes": "5"},
		"an unknown key":             {"rate_limit_failures": "10"},
		"a number that is not":       {"backup_keep": "many"},
		"tiny events":                {"max_object_bytes": "100"},
		"a listen without a port":    {"public_listen": "0.0.0.0", "public_url": "https://a"},
		"a URL with credentials":     {"public_url": "https://u:p@agenda.example.org"},
		"http with the insecure off": {"public_url": "http://10.0.0.1:8791"},
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			_, s := open(t)
			if _, err := s.Apply(change); err == nil {
				t.Fatalf("%v was accepted", change)
			}
		})
	}
	_, s := open(t)
	for name, change := range map[string]map[string]string{
		"http on loopback":       {"public_listen": "127.0.0.1:0", "public_url": "http://127.0.0.1:8791"},
		"http accepted":          {"public_url": "http://agenda.lan:8791", "public_allow_insecure": "true"},
		"a JSON list":            {"palette": `["#000000","#ffffff"]`},
		"a JSON string":          {"family_timezone": `"America/Montreal"`},
		"no proxies at all":      {"trusted_proxies": ""},
		"the public side off":    {"public_listen": ""},
		"a trailing slash is ok": {"public_url": "https://agenda.example.org/"},
	} {
		if _, err := s.Apply(change); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func TestKeysCoverEveryField(t *testing.T) {
	keys := Keys()
	if len(keys) != 16 {
		t.Fatalf("%d keys", len(keys))
	}
	for _, k := range keys {
		if k.Group == "" || k.Type == "" {
			t.Errorf("%+v has no group or type", k)
		}
	}
}
