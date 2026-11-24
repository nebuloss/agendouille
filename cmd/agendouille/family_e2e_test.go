package main

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/nebuloss/agendouille/internal/auth"
)

// webClient is a signed-in browser of the family web app.
type webClient struct {
	x       *e2e
	session string
}

func (c webClient) call(method, path string, body any, headers ...string) answer {
	c.x.t.Helper()
	var r io.Reader
	if body != nil {
		if s, ok := body.(string); ok {
			r = strings.NewReader(s)
		} else {
			b, _ := json.Marshal(body)
			r = strings.NewReader(string(b))
		}
	}
	return c.x.do(method, c.x.public+path, r, func(req *http.Request) {
		req.AddCookie(&http.Cookie{Name: auth.AppCookie, Value: c.session})
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Sec-Fetch-Site", "same-origin")
		for i := 0; i+1 < len(headers); i += 2 {
			req.Header.Set(headers[i], headers[i+1])
		}
	})
}

func (c webClient) json(method, path string, body any, want int, headers ...string) (map[string]any, answer) {
	c.x.t.Helper()
	a := c.call(method, path, body, headers...)
	if a.status != want {
		c.x.t.Fatalf("%s %s: %d, want %d: %s", method, path, a.status, want, a.body)
	}
	out := map[string]any{}
	_ = json.Unmarshal([]byte(a.body), &out)
	return out, a
}

func (x *e2e) browser(user string) webClient {
	x.t.Helper()
	u, err := x.svc.dir.User(context.Background(), user)
	if err != nil {
		x.t.Fatal(err)
	}
	e, _ := x.svc.auth.NewEnrollment(context.Background(), "test", u.ID, 0)
	a := x.do("POST", x.public+"/enroll/"+e.Token, nil, nil)
	return webClient{x: x, session: cookieOf(a, auth.AppCookie)}
}

// « Déconnecter tous les navigateurs »: from a browser (or the app), all the
// others; from the admin, all of them.
func TestSignOutAllBrowsers(t *testing.T) {
	x := startServer(t)
	x.publicOn()
	x.svc.dir.AddUser(context.Background(), "cli", "lea", "Léa", "", nil)
	one, two, three := x.browser("lea"), x.browser("lea"), x.browser("lea")
	out, _ := one.json("DELETE", "/api/sessions", nil, 200)
	if out["signed_out"] != float64(2) {
		t.Errorf("signed out: %v, want 2", out)
	}
	if a := two.call("GET", "/api/me", nil); a.status != http.StatusUnauthorized {
		t.Errorf("another browser after: %d", a.status)
	}
	if a := three.call("GET", "/api/me", nil); a.status != http.StatusUnauthorized {
		t.Errorf("a third browser after: %d", a.status)
	}
	if a := one.call("GET", "/api/me", nil); a.status != 200 {
		t.Errorf("the browser asking was signed out too: %d", a.status)
	}
	A := signInAdmin(t, x)
	all := A.json("DELETE", "/users/lea/sessions", nil, 200)
	if all["signed_out"] != float64(1) {
		t.Errorf("the admin signed out: %v, want 1", all)
	}
	if a := one.call("GET", "/api/me", nil); a.status != http.StatusUnauthorized {
		t.Errorf("after the admin: %d", a.status)
	}
}

func summaries(occ map[string]any) string {
	var out []string
	for _, o := range occ["occurrences"].([]any) {
		m := o.(map[string]any)
		out = append(out, m["start"].(string)[5:16]+" "+m["summary"].(string))
	}
	return strings.Join(out, " | ")
}

// The M4 "done when" (PLAN.md 15) on the API the web app uses.
func TestM4FamilyAPI(t *testing.T) {
	x := startServer(t)
	x.publicOn()
	ctx := context.Background()
	x.svc.dir.AddGroup(ctx, "cli", "famille", "", "")
	lea, _ := x.svc.dir.AddUser(ctx, "cli", "lea", "Léa", "", []string{"famille"})
	maman, _ := x.svc.dir.AddUser(ctx, "cli", "maman", "Maman", "", []string{"famille"})
	L, M := x.browser("lea"), x.browser("maman")
	leaPhone, _ := x.svc.auth.NewDevice(ctx, lea.ID, "davx5", "Pixel", "readwrite")
	mamanPhone, _ := x.svc.auth.NewDevice(ctx, maman.ID, "davx5", "Pixel", "readwrite")
	LD, MD := davClient{x, "lea", leaPhone.Secret}, davClient{x, "maman", mamanPhone.Secret}

	// The pages are there, each with a way to sign out.
	for _, p := range []string{"/", "/calendars", "/devices", "/me"} {
		if a := L.call("GET", p, nil); a.status != 200 || !strings.Contains(a.body, "app-content") || !strings.Contains(a.body, `action="/signout"`) {
			t.Errorf("page %s: %d", p, a.status)
		}
	}

	// A calendar made in the web app.
	c, _ := L.json("POST", "/api/calendars", map[string]any{"name": "École de Léa"}, 201)
	ref := c["ref"].(string)
	if ref != "lea~ecole-de-lea" || c["access"] != "owner" {
		t.Fatalf("created %v", c)
	}
	base := "/api/calendars/" + ref

	// A weekly event, then every scope of an edit.
	ev, a := L.json("POST", base+"/events", map[string]any{"summary": "Judo", "start": "2026-11-04T14:00", "end": "2026-11-04T15:30",
		"rrule": "FREQ=WEEKLY;UNTIL=20270630T215959Z"}, 201)
	name, etag := ev["name"].(string), a.header.Get("ETag")
	occ, _ := L.json("GET", "/api/occurrences?from=2026-11-01&to=2026-11-30", nil, 200)
	if got := summaries(occ); got != "11-04T14:00 Judo | 11-11T14:00 Judo | 11-18T14:00 Judo | 11-25T14:00 Judo" {
		t.Fatalf("occurrences: %s", got)
	}
	edit := map[string]any{"summary": "Judo (gymnase B)", "start": "2026-11-18T16:00", "end": "2026-11-18T17:00"}
	if a := L.call("PUT", base+"/events/"+name+"?scope=this&occurrence=2026-11-18T14:00:00", edit); a.status != http.StatusPreconditionRequired {
		t.Errorf("an edit without If-Match: %d", a.status)
	}
	_, a = L.json("PUT", base+"/events/"+name+"?scope=this&occurrence=2026-11-18T14:00:00", edit, 200, "If-Match", etag)
	stale := etag
	etag = a.header.Get("ETag")
	if a := L.call("PUT", base+"/events/"+name+"?scope=all", map[string]any{"summary": "x", "start": "2026-11-04T14:00"}, "If-Match", stale); a.status != http.StatusPreconditionFailed || !strings.Contains(a.body, `"by":"lea"`) {
		t.Errorf("a stale edit: %d %s", a.status, a.body)
	}
	occ, _ = L.json("GET", "/api/occurrences?from=2026-11-01&to=2026-11-30", nil, 200)
	if got := summaries(occ); !strings.Contains(got, "11-18T16:00 Judo (gymnase B)") || strings.Contains(got, "11-18T14:00") {
		t.Errorf("after editing one: %s", got)
	}
	// Delete one Wednesday.
	if a := L.call("DELETE", base+"/events/"+name+"?scope=this&occurrence=2026-11-25T14:00:00", nil, "If-Match", etag); a.status != http.StatusNoContent {
		t.Fatalf("deleting one: %d %s", a.status, a.body)
	}
	occ, _ = L.json("GET", "/api/occurrences?from=2026-11-01&to=2026-11-30", nil, 200)
	if got := summaries(occ); strings.Contains(got, "11-25") {
		t.Errorf("after deleting one: %s", got)
	}
	ev, a = L.json("GET", base+"/events/"+name, nil, 200)
	etag = a.header.Get("ETag")
	// Following: a split, two objects.
	r, _ := L.json("PUT", base+"/events/"+name+"?scope=following&occurrence=2026-12-02T14:00:00",
		map[string]any{"summary": "Judo, nouvel horaire", "start": "2026-12-02T17:00", "end": "2026-12-02T18:00"}, 200, "If-Match", etag)
	if r["created"] == nil {
		t.Errorf("a split did not create the new series: %v", r)
	}
	occ, _ = L.json("GET", "/api/occurrences?from=2026-11-28&to=2026-12-12", nil, 200)
	if got := summaries(occ); got != "12-02T17:00 Judo, nouvel horaire | 12-09T17:00 Judo, nouvel horaire" {
		t.Errorf("after the split: %s", got)
	}

	// An event a phone made keeps its alarm and its X- property through a web edit.
	phone := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//DAVx5//EN\r\nBEGIN:VEVENT\r\nUID:phone-uid\r\nSUMMARY:Dentiste\r\n" +
		"DTSTART;TZID=Europe/Paris:20261126T173000\r\nDTEND;TZID=Europe/Paris:20261126T180000\r\nX-MOZ-SNOOZE:1\r\n" +
		"BEGIN:VALARM\r\nACTION:DISPLAY\r\nTRIGGER:-PT15M\r\nDESCRIPTION:Dentiste\r\nEND:VALARM\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"
	dav := "/dav/calendars/lea/" + ref + "/"
	if a := LD.req("PUT", dav+"phone.ics", phone); a.status != http.StatusCreated {
		t.Fatalf("PUT from the phone: %d", a.status)
	}
	ev, a = L.json("GET", base+"/events/phone.ics", nil, 200)
	if ev["has_alarms"] != true {
		t.Errorf("the alarm is not seen: %v", ev)
	}
	L.json("PUT", base+"/events/phone.ics?scope=all", map[string]any{"summary": "Dentiste (Dr Martin)", "start": "2026-11-26T17:30", "end": "2026-11-26T18:15"},
		200, "If-Match", a.header.Get("ETag"))
	back := LD.req("GET", dav+"phone.ics", "").body
	for _, want := range []string{"BEGIN:VALARM", "TRIGGER:-PT15M", "X-MOZ-SNOOZE:1", "Dentiste (Dr Martin)", "SEQUENCE:1"} {
		if !strings.Contains(back, want) {
			t.Errorf("after a web edit the phone's object lacks %s:\n%s", want, back)
		}
	}

	// Shared in the web app, read-only: it appears on maman's phone, read-only.
	L.json("PUT", base+"/shares/u:maman", map[string]any{"access": "read"}, 200)
	ms := parseMS(t, MD.req("PROPFIND", "/dav/calendars/maman/", propfindHome, "Depth", "1"))
	if r := ms.find("/dav/calendars/maman/" + ref + "/"); r == nil || strings.Contains(r.ok(), "<d:write") {
		t.Errorf("the shared calendar on maman's phone: %+v", r)
	}
	mc, _ := M.json("GET", "/api/calendars", nil, 200)
	if !strings.Contains(asJSON(mc), `"access":"read"`) {
		t.Errorf("maman's calendars: %v", mc)
	}
	if a := M.call("POST", base+"/events", map[string]any{"summary": "x", "start": "2026-11-26T10:00"}); a.status != http.StatusForbidden {
		t.Errorf("maman writing to a read share: %d", a.status)
	}
	if a := M.call("PUT", base+"/shares/u:lea", map[string]any{"access": "read"}); a.status != http.StatusForbidden {
		t.Errorf("a sharee sharing: %d", a.status)
	}
	// Hidden by maman: it leaves her phone.
	M.json("PATCH", base, map[string]any{"hidden": true}, 200)
	if ms := parseMS(t, MD.req("PROPFIND", "/dav/calendars/maman/", propfindHome, "Depth", "1")); ms.find("/dav/calendars/maman/"+ref+"/") != nil {
		t.Error("a hidden calendar is still on maman's phone")
	}
	M.json("PATCH", base, map[string]any{"hidden": false}, 200)

	// Live: maman's open page hears lea's change on the shared calendar.
	got := make(chan string, 1)
	go func() {
		req, _ := http.NewRequest("GET", x.public+"/api/live", nil)
		req.AddCookie(&http.Cookie{Name: auth.AppCookie, Value: M.session})
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			got <- "error"
			return
		}
		defer resp.Body.Close()
		sc := bufio.NewScanner(resp.Body)
		for sc.Scan() {
			if strings.HasPrefix(sc.Text(), "event: changed") {
				got <- "changed"
				return
			}
		}
	}()
	time.Sleep(200 * time.Millisecond)
	L.json("POST", base+"/events", map[string]any{"summary": "Réunion", "start": "2026-11-27T18:00"}, 201)
	select {
	case v := <-got:
		if v != "changed" {
			t.Errorf("live: %s", v)
		}
	case <-time.After(5 * time.Second):
		t.Error("maman's live feed did not hear the change")
	}

	// Moving an event to another calendar.
	other, _ := L.json("POST", "/api/calendars", map[string]any{"name": "Perso"}, 201)
	ev, a = L.json("GET", base+"/events/phone.ics", nil, 200)
	mv, _ := L.json("POST", base+"/events/phone.ics/move", map[string]any{"to": other["ref"]}, 200, "If-Match", a.header.Get("ETag"))
	if mv["calendar"] != other["ref"] {
		t.Errorf("moved to %v", mv["calendar"])
	}
	if a := L.call("GET", base+"/events/phone.ics", nil); a.status != http.StatusNotFound {
		t.Errorf("the moved event is still in the first calendar: %d", a.status)
	}

	// Devices: DAVx5 gets its link, an iPhone its profile, once.
	d, _ := L.json("POST", "/api/devices", map[string]any{"kind": "davx5", "label": "Pixel 8"}, 201)
	if !strings.HasPrefix(d["davx5_uri"].(string), "davx5://lea:agd_") {
		t.Errorf("davx5: %v", d["davx5_uri"])
	}
	d, _ = L.json("POST", "/api/devices", map[string]any{"kind": "ios", "label": "iPhone"}, 201)
	if a := M.call("GET", d["mobileconfig_url"].(string), nil); a.status != http.StatusGone {
		t.Errorf("maman fetching lea's profile: %d", a.status)
	}
	prof := L.call("GET", d["mobileconfig_url"].(string), nil)
	if prof.status != 200 || prof.header.Get("Content-Type") != "application/x-apple-aspen-config" || !strings.Contains(prof.body, "com.apple.caldav.account") {
		t.Errorf("the profile: %d %s", prof.status, prof.header.Get("Content-Type"))
	}
	if a := L.call("GET", d["mobileconfig_url"].(string), nil); a.status != http.StatusGone {
		t.Errorf("the profile twice: %d", a.status)
	}

	// A subscription link: the TV reads, without a password; revoked, it stops.
	link, _ := L.json("POST", "/api/links", map[string]any{"label": "Télé", "calendars": []string{ref}}, 201)
	url := strings.TrimPrefix(link["url"].(string), x.public)
	if a := x.do("GET", x.public+url, nil, nil); a.status != 200 || !strings.Contains(a.body, "SUMMARY:Judo") || !strings.Contains(a.body, "REFRESH-INTERVAL") {
		t.Errorf("the link: %d %s", a.status, a.body)
	}
	L.json("DELETE", "/api/links/"+fmtID(link["id"]), nil, 204)
	if a := x.do("GET", x.public+url, nil, nil); a.status != http.StatusNotFound {
		t.Errorf("a revoked link: %d", a.status)
	}

	if c, _ := L.json("GET", base, nil, 200); c["name"] != "École de Léa" {
		t.Errorf("GET one calendar: %v", c)
	}

	// Me, people, sessions.
	me, _ := L.json("PATCH", "/api/me", map[string]any{"display_name": "Léa B."}, 200)
	if me["display_name"] != "Léa B." {
		t.Errorf("me: %v", me)
	}
	people, _ := L.json("GET", "/api/people", nil, 200)
	if !strings.Contains(asJSON(people), `"maman"`) {
		t.Errorf("people: %v", people)
	}
	ss, _ := L.json("GET", "/api/sessions", nil, 200)
	if !strings.Contains(asJSON(ss), `"current":true`) {
		t.Errorf("sessions: %v", ss)
	}
	// The app's sign-in codes come from the admin only: a browser cannot
	// mint them (it was approved by the app, and would get more than it has).
	if a := L.call("POST", "/api/me/enrollment", nil); a.status != http.StatusNotFound && a.status != http.StatusMethodNotAllowed {
		t.Errorf("a browser made an enrollment link: %d %s", a.status, a.body)
	}

	// Signing out ends this browser's session, and only it.
	out := L.call("POST", "/signout", nil)
	if out.status != http.StatusSeeOther || out.header.Get("Location") != "/" {
		t.Errorf("sign out: %d %s", out.status, out.header.Get("Location"))
	}
	if a := L.call("GET", "/api/me", nil); a.status != http.StatusUnauthorized {
		t.Errorf("after signing out: %d", a.status)
	}
	if a := M.call("GET", "/api/me", nil); a.status != 200 {
		t.Errorf("another browser was signed out too: %d", a.status)
	}

}

func fmtID(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}
