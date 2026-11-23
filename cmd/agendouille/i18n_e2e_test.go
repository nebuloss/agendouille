package main

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/nebuloss/agendouille/internal/i18n"
)

// An API error's message is in the reader's language, French by default;
// its code never changes (CONTRACTS.md 3.1, DECISIONS: languages).
func TestAPIErrorLanguage(t *testing.T) {
	x := startServer(t)
	x.publicOn()
	if _, err := x.svc.dir.AddUser(context.Background(), "cli", "lea", "Léa", "", nil); err != nil {
		t.Fatal(err)
	}
	L := x.browser("lea")

	check := func(a answer, status int, code, key string, lang i18n.Lang, args ...string) {
		t.Helper()
		var body struct{ Error, Message string }
		_ = json.Unmarshal([]byte(a.body), &body)
		if a.status != status || body.Error != code {
			t.Fatalf("%d %q, want %d %q: %s", a.status, body.Error, status, code, a.body)
		}
		if want := i18n.T(lang, key, args...); body.Message != want || strings.HasPrefix(want, "error.") {
			t.Errorf("%s message %q, want %q", lang, body.Message, want)
		}
	}
	if i18n.T(i18n.FR, "error.calendar.nameRequired") == i18n.T(i18n.EN, "error.calendar.nameRequired") {
		t.Fatal("the two languages say the same")
	}

	noName := map[string]any{"name": " "}
	check(L.call("POST", "/api/calendars", noName), 400, "bad_request", "error.calendar.nameRequired", i18n.FR)
	check(L.call("POST", "/api/calendars", noName, "Accept-Language", "en-GB,en;q=0.9"), 400, "bad_request", "error.calendar.nameRequired", i18n.EN)
	check(L.call("POST", "/api/calendars", noName, "Accept-Language", "de-DE,fr;q=0.5"), 400, "bad_request", "error.calendar.nameRequired", i18n.FR)

	// A placeholder is filled in either language.
	check(L.call("GET", "/api/occurrences?from=x&to=y", nil, "Accept-Language", "en"), 400, "bad_request", "error.occurrences.range", i18n.EN, "days", "62")
	if a := L.call("GET", "/api/occurrences?from=x&to=y", nil); !strings.Contains(a.body, "62") {
		t.Errorf("the placeholder is not filled: %s", a.body)
	}

	// The shared plumbing (webx) and the service errors mapped at the
	// boundary.
	check(L.call("POST", "/api/calendars", "{not json", "Accept-Language", "en"), 400, "bad_request", "error.request.invalid", i18n.EN)
	check(L.call("GET", "/api/calendars/lea~nope", nil, "Accept-Language", "en"), 404, "not_found", "error.notFound", i18n.EN)
	check(L.call("GET", "/api/calendars/lea~nope", nil), 404, "not_found", "error.notFound", i18n.FR)
	check(x.do("GET", x.public+"/api/me", nil, func(r *http.Request) { r.Header.Set("Accept-Language", "en") }),
		401, "unauthenticated", "error.signin.required", i18n.EN)
	check(L.call("PATCH", "/api/me", map[string]any{"color": "red"}, "Accept-Language", "en"), 422, "invalid_event", "error.color.invalid", i18n.EN)
}
