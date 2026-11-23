package i18n

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestFromRequest(t *testing.T) {
	for _, c := range []struct {
		cookie, accept string
		want           Lang
	}{
		{"", "", FR},
		{"", "en-US,en;q=0.9", EN},
		{"", "de-DE,en;q=0.8,fr;q=0.5", EN},
		{"", "de-DE", FR},
		{"fr", "en-US", FR},
		{"en", "fr-FR", EN},
		{"xx", "en", EN},
	} {
		r := httptest.NewRequest("GET", "/", nil)
		if c.accept != "" {
			r.Header.Set("Accept-Language", c.accept)
		}
		if c.cookie != "" {
			r.Header.Set("Cookie", Cookie+"="+c.cookie)
		}
		if got := FromRequest(r); got != c.want {
			t.Errorf("cookie %q, accept %q: %s, want %s", c.cookie, c.accept, got, c.want)
		}
	}
}

// Every message has every language, and the same placeholders in each.
func TestCatalogComplete(t *testing.T) {
	ph := regexp.MustCompile(`\{\w+\}`)
	for key, m := range catalog {
		for _, l := range Langs {
			if strings.TrimSpace(m[l]) == "" {
				t.Errorf("%s has no %s", key, l)
			}
		}
		fr, en := ph.FindAllString(m[FR], -1), ph.FindAllString(m[EN], -1)
		if strings.Join(sorted(fr), ",") != strings.Join(sorted(en), ",") {
			t.Errorf("%s: placeholders differ: %v %v", key, fr, en)
		}
	}
}

// Every {{t "key"}} in the templates is in the catalog.
func TestTemplatesKeys(t *testing.T) {
	re := regexp.MustCompile(`\{\{-?\s*t\s+"([^"]+)"`)
	root := filepath.Join("..", "web", "assets", "templates")
	err := filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(p, ".html") {
			return err
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		for _, m := range re.FindAllStringSubmatch(string(b), -1) {
			if !Has(m[1]) {
				t.Errorf("%s: %q is not in the catalog", p, m[1])
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestT(t *testing.T) {
	catalog["test.hello"] = map[Lang]string{FR: "Bonjour {name}", EN: "Hello {name}"}
	defer delete(catalog, "test.hello")
	if got := T(EN, "test.hello", "name", "Léa"); got != "Hello Léa" {
		t.Error(got)
	}
	if got := T(FR, "nope"); got != "nope" {
		t.Error(got)
	}
}

func sorted(s []string) []string {
	out := append([]string(nil), s...)
	for i := range out {
		for j := i + 1; j < len(out); j++ {
			if out[j] < out[i] {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	return out
}
