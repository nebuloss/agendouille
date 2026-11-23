package i18n

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// Every "error.…" key the web handlers name is in the catalog: a missing
// one would reach the reader as the key itself.
func TestErrorKeys(t *testing.T) {
	re := regexp.MustCompile(`"(error\.[A-Za-z0-9.]+)"`)
	root := filepath.Join("..", "web")
	seen := 0
	err := filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(p, ".go") {
			return err
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		for _, m := range re.FindAllStringSubmatch(string(b), -1) {
			seen++
			if !Has(m[1]) {
				t.Errorf("%s: %q is not in the catalog", p, m[1])
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if seen == 0 {
		t.Fatal("no error key found: the walk looks in the wrong place")
	}
}

// The French messages follow docs/i18n.md: a typographic apostrophe, and a
// non-breaking space before the high punctuation.
func TestErrorTypography(t *testing.T) {
	for key, m := range catalog {
		if !strings.HasPrefix(key, "error.") {
			continue
		}
		fr := m[FR]
		if strings.Contains(fr, "'") {
			t.Errorf("%s: a straight apostrophe: %q", key, fr)
		}
		for _, p := range []string{" :", " ;", " ?", " !"} {
			if strings.Contains(fr, p) {
				t.Errorf("%s: a breaking space before %q: %q", key, p[1:], fr)
			}
		}
		if strings.HasPrefix(fr, "Erreur") {
			t.Errorf("%s: starts with Erreur: %q", key, fr)
		}
	}
}
