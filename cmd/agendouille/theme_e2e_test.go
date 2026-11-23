package main

import (
	"net/http"
	"strings"
	"testing"
)

// Pages carry Dark Reader's lock (they have their own dark theme), and the
// reader's theme cookie puts data-theme on <html> before the first paint.
func TestThemeAndDarkReader(t *testing.T) {
	x := startServer(t)
	x.publicOn()
	page := func(cookie string) string {
		t.Helper()
		a := x.do("GET", x.public+"/signin", nil, func(r *http.Request) {
			if cookie != "" {
				r.AddCookie(&http.Cookie{Name: "agd_theme", Value: cookie})
			}
		})
		if a.status != 200 {
			t.Fatalf("/signin: %d", a.status)
		}
		return a.body
	}
	plain := page("")
	if !strings.Contains(plain, `<meta name="darkreader-lock">`) || !strings.Contains(plain, `<meta name="color-scheme" content="light dark">`) {
		t.Error("no Dark Reader lock or color-scheme on the page")
	}
	if strings.Contains(plain, "data-theme") {
		t.Error("data-theme without a choice")
	}
	if !strings.Contains(page("dark"), `<html lang="fr" data-theme="dark">`) {
		t.Error("the dark choice is not on <html>")
	}
	if strings.Contains(page("purple"), "data-theme") {
		t.Error("an unknown theme reached the page")
	}
	if a := x.do("GET", x.admin+"/signin", nil, func(r *http.Request) {
		r.AddCookie(&http.Cookie{Name: "agd_theme", Value: "light"})
	}); !strings.Contains(a.body, `data-theme="light"`) {
		t.Error("the admin pages ignore the theme")
	}
}
