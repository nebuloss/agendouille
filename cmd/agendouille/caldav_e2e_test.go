package main

import (
	"context"
	"encoding/xml"
	"net/http"
	"strings"
	"testing"

	"github.com/nebuloss/agendouille/internal/calendar"
	"github.com/nebuloss/agendouille/internal/sharing"
	"github.com/nebuloss/agendouille/internal/store"
)

// The M2 "done when" (PLAN.md 15), with the requests a CalDAV client sends,
// written out by hand: no CalDAV library on either side.

type msResponse struct {
	Href     string `xml:"href"`
	Status   string `xml:"status"`
	Propstat []struct {
		Status string `xml:"status"`
		Prop   struct {
			Inner string `xml:",innerxml"`
		} `xml:"prop"`
	} `xml:"propstat"`
}

type multistatus struct {
	Responses []msResponse `xml:"response"`
	SyncToken string       `xml:"sync-token"`
}

func parseMS(t *testing.T, a answer) multistatus {
	t.Helper()
	if a.status != http.StatusMultiStatus {
		t.Fatalf("want 207, got %d: %s", a.status, a.body)
	}
	var ms multistatus
	if err := xml.Unmarshal([]byte(a.body), &ms); err != nil {
		t.Fatalf("multistatus: %v\n%s", err, a.body)
	}
	return ms
}

// ok is the 200 propstat of a response, as XML text.
func (r msResponse) ok() string {
	for _, p := range r.Propstat {
		if strings.Contains(p.Status, "200") {
			return p.Prop.Inner
		}
	}
	return ""
}

func (ms multistatus) find(href string) *msResponse {
	for i := range ms.Responses {
		if ms.Responses[i].Href == href {
			return &ms.Responses[i]
		}
	}
	return nil
}

type davClient struct {
	x          *e2e
	user, pass string
}

func (c davClient) req(method, path, body string, headers ...string) answer {
	c.x.t.Helper()
	var r *strings.Reader
	if body != "" {
		r = strings.NewReader(body)
	} else {
		r = strings.NewReader("")
	}
	return c.x.do(method, c.x.public+path, r, func(req *http.Request) {
		req.SetBasicAuth(c.user, c.pass)
		if body != "" {
			if strings.HasPrefix(body, "BEGIN:") {
				req.Header.Set("Content-Type", "text/calendar; charset=utf-8")
			} else {
				req.Header.Set("Content-Type", "application/xml; charset=utf-8")
			}
		}
		for i := 0; i+1 < len(headers); i += 2 {
			req.Header.Set(headers[i], headers[i+1])
		}
	})
}

const propfindHome = `<?xml version="1.0"?><d:propfind xmlns:d="DAV:" xmlns:c="urn:ietf:params:xml:ns:caldav" xmlns:cs="http://calendarserver.org/ns/" xmlns:a="http://apple.com/ns/ical/">
<d:prop><d:resourcetype/><d:displayname/><a:calendar-color/><d:current-user-privilege-set/><cs:getctag/><d:sync-token/>
<c:supported-calendar-component-set/><d:supported-report-set/><x:unknown xmlns:x="urn:example"/></d:prop></d:propfind>`

const judo = "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//DAVx5//EN\r\nBEGIN:VEVENT\r\nUID:judo-uid\r\nSUMMARY:Judo\r\n" +
	"DTSTART;TZID=Europe/Paris:20261104T140000\r\nDTEND;TZID=Europe/Paris:20261104T153000\r\n" +
	"RRULE:FREQ=WEEKLY;BYDAY=WE;UNTIL=20270630T215959Z\r\nEXDATE;TZID=Europe/Paris:20261118T140000\r\n" +
	"BEGIN:VALARM\r\nACTION:DISPLAY\r\nTRIGGER:-PT15M\r\nEND:VALARM\r\nX-APPLE-TRAVEL-ADVISORY-BEHAVIOR:AUTOMATIC\r\n" +
	"END:VEVENT\r\nEND:VCALENDAR\r\n"

func once(uid, summary, day string) string {
	return "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//test//EN\r\nBEGIN:VEVENT\r\nUID:" + uid + "\r\nSUMMARY:" + summary +
		"\r\nDTSTART;TZID=Europe/Paris:" + day + "T170000\r\nDTEND;TZID=Europe/Paris:" + day + "T173000\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"
}

func timeRange(comp, start, end string) string {
	return `<?xml version="1.0"?><c:calendar-query xmlns:d="DAV:" xmlns:c="urn:ietf:params:xml:ns:caldav">` +
		`<d:prop><d:getetag/></d:prop><c:filter><c:comp-filter name="VCALENDAR"><c:comp-filter name="` + comp + `">` +
		`<c:time-range start="` + start + `" end="` + end + `"/></c:comp-filter></c:comp-filter></c:filter></c:calendar-query>`
}

func syncReport(token, limit string) string {
	l := ""
	if limit != "" {
		l = "<d:limit><d:nresults>" + limit + "</d:nresults></d:limit>"
	}
	return `<?xml version="1.0"?><d:sync-collection xmlns:d="DAV:"><d:sync-token>` + token + `</d:sync-token>` +
		`<d:sync-level>1</d:sync-level>` + l + `<d:prop><d:getetag/></d:prop></d:sync-collection>`
}

func TestM2CalDAV(t *testing.T) {
	x := startServer(t)
	x.publicOn()
	ctx := context.Background()
	x.svc.dir.AddGroup(ctx, "cli", "famille", "", "")
	lea, _ := x.svc.dir.AddUser(ctx, "cli", "lea", "Léa", "", []string{"famille"})
	maman, _ := x.svc.dir.AddUser(ctx, "cli", "maman", "Maman", "", []string{"famille"})
	leaPhone, _ := x.svc.auth.NewDevice(ctx, lea.ID, "davx5", "Pixel", "readwrite")
	leaTV, _ := x.svc.auth.NewDevice(ctx, lea.ID, "other", "Télé", "read")
	mamanPhone, _ := x.svc.auth.NewDevice(ctx, maman.ID, "ios", "iPhone", "readwrite")
	L := davClient{x, "lea", leaPhone.Secret}
	M := davClient{x, "maman", mamanPhone.Secret}
	TV := davClient{x, "lea", leaTV.Secret}

	// --- discovery
	if a := x.do("GET", x.public+"/.well-known/caldav", nil, nil); a.status != http.StatusMovedPermanently || a.header.Get("Location") != "/dav/" {
		t.Errorf("well-known: %d %s", a.status, a.header.Get("Location"))
	}
	if a := L.req("OPTIONS", "/dav/", ""); !strings.Contains(a.header.Get("DAV"), "calendar-access") {
		t.Errorf("OPTIONS DAV header %q", a.header.Get("DAV"))
	}
	ms := parseMS(t, L.req("PROPFIND", "/dav/", `<d:propfind xmlns:d="DAV:"><d:prop><d:current-user-principal/></d:prop></d:propfind>`, "Depth", "0"))
	if !strings.Contains(ms.Responses[0].ok(), "/dav/principals/lea/") {
		t.Fatalf("current-user-principal: %s", ms.Responses[0].ok())
	}
	ms = parseMS(t, L.req("PROPFIND", "/dav/principals/lea/", `<d:propfind xmlns:d="DAV:" xmlns:c="urn:ietf:params:xml:ns:caldav"><d:prop><c:calendar-home-set/></d:prop></d:propfind>`, "Depth", "0"))
	if !strings.Contains(ms.Responses[0].ok(), "/dav/calendars/lea/") {
		t.Fatalf("calendar-home-set: %s", ms.Responses[0].ok())
	}
	if a := L.req("PROPFIND", "/dav/principals/maman/", "", "Depth", "0"); a.status != http.StatusNotFound {
		t.Errorf("someone else's principal: %d", a.status)
	}
	if a := L.req("PROPFIND", "/dav/calendars/lea/", "", "Depth", "infinity"); a.status != http.StatusForbidden {
		t.Errorf("Depth: infinity: %d", a.status)
	}

	// --- MKCALENDAR, as DAVx5 and iOS send it (an upper-case UUID from iOS)
	mk := `<?xml version="1.0"?><c:mkcalendar xmlns:d="DAV:" xmlns:c="urn:ietf:params:xml:ns:caldav" xmlns:a="http://apple.com/ns/ical/">` +
		`<d:set><d:prop><d:displayname>Perso</d:displayname><a:calendar-color>#FF0000FF</a:calendar-color>` +
		`<c:supported-calendar-component-set><c:comp name="VEVENT"/></c:supported-calendar-component-set></d:prop></d:set></c:mkcalendar>`
	a := L.req("MKCALENDAR", "/dav/calendars/lea/2A3B-C4D5/", mk)
	if a.status != http.StatusCreated || a.header.Get("Location") != "/dav/calendars/lea/lea~2a3b-c4d5/" {
		t.Fatalf("MKCALENDAR: %d %s %s", a.status, a.header.Get("Location"), a.body)
	}
	if a := L.req("MKCALENDAR", "/dav/calendars/lea/2A3B-C4D5/", mk); a.status != http.StatusMethodNotAllowed {
		t.Errorf("MKCALENDAR again: %d", a.status)
	}
	todo := strings.Replace(mk, `name="VEVENT"`, `name="VTODO"`, 1)
	if a := L.req("MKCALENDAR", "/dav/calendars/lea/tasks/", todo); a.status != http.StatusForbidden || !strings.Contains(a.body, "supported-calendar-component") {
		t.Errorf("a task list: %d %s", a.status, a.body)
	}
	if a := TV.req("MKCALENDAR", "/dav/calendars/lea/tv/", mk); a.status != http.StatusForbidden {
		t.Errorf("a read-only device making a calendar: %d", a.status)
	}
	cal := "/dav/calendars/lea/lea~2a3b-c4d5/"
	// The URL the client asked for keeps working for its owner.
	if a := L.req("PROPFIND", "/dav/calendars/lea/2a3b-c4d5/", "", "Depth", "0"); a.status != http.StatusMultiStatus {
		t.Errorf("the bare slug: %d", a.status)
	}

	ms = parseMS(t, L.req("PROPFIND", "/dav/calendars/lea/", propfindHome, "Depth", "1"))
	r := ms.find(cal)
	if r == nil {
		t.Fatalf("the home does not list %s:\n%s", cal, ms.Responses)
	}
	for _, want := range []string{"<c:calendar/>", "Perso", "#ff0000", "<d:write-content/>", "<c:comp name=\"VEVENT\"/>", "sync-collection", "/ns/sync/"} {
		if !strings.Contains(r.ok(), want) {
			t.Errorf("calendar props lack %s:\n%s", want, r.ok())
		}
	}
	if len(r.Propstat) < 2 || !strings.Contains(r.Propstat[1].Status, "404") || !strings.Contains(r.Propstat[1].Prop.Inner, "unknown") {
		t.Errorf("an unknown property is not a 404 propstat: %+v", r.Propstat)
	}

	// --- PUT with preconditions
	a = L.req("PUT", cal+"judo.ics", judo, "If-None-Match", "*")
	etag := a.header.Get("ETag")
	if a.status != http.StatusCreated || etag == "" {
		t.Fatalf("PUT: %d %s", a.status, a.body)
	}
	if a := L.req("PUT", cal+"judo.ics", judo, "If-None-Match", "*"); a.status != http.StatusPreconditionFailed {
		t.Errorf("PUT over an existing object with If-None-Match: %d", a.status)
	}
	if a := L.req("PUT", cal+"judo.ics", judo, "If-Match", `"stale"`); a.status != http.StatusPreconditionFailed {
		t.Errorf("PUT with a stale If-Match: %d", a.status)
	}
	judo2 := strings.Replace(judo, "SUMMARY:Judo", "SUMMARY:Judo (gymnase B)", 1)
	a = L.req("PUT", cal+"judo.ics", judo2, "If-Match", etag)
	if a.status != http.StatusNoContent || a.header.Get("ETag") == etag {
		t.Fatalf("PUT with the right If-Match: %d", a.status)
	}
	etag = a.header.Get("ETag")
	if a := L.req("GET", cal+"judo.ics", ""); a.body != judo2 || a.header.Get("ETag") != etag {
		t.Errorf("GET returns other bytes than were put (alarms and X- properties must survive)")
	}
	if a := L.req("PUT", cal+"other.ics", strings.Replace(once("judo-uid", "dup", "20261125"), "", "", 0)); a.status != http.StatusForbidden || !strings.Contains(a.body, "no-uid-conflict") {
		t.Errorf("the same UID twice: %d %s", a.status, a.body)
	}
	if a := L.req("PUT", cal+"bad.ics", "BEGIN:VCALENDAR\r\nnot a calendar\r\n"); a.status != http.StatusForbidden || !strings.Contains(a.body, "valid-calendar-data") {
		t.Errorf("garbage: %d %s", a.status, a.body)
	}
	if a := L.req("PUT", cal+"dentiste.ics", once("dentiste-uid", "Dentiste", "20261126"), "If-None-Match", "*"); a.status != http.StatusCreated {
		t.Fatalf("a second event: %d", a.status)
	}

	// --- time-range queries expand the weekly rule and honour the EXDATE
	count := func(comp, start, end string) int {
		return len(parseMS(t, L.req("REPORT", cal, timeRange(comp, start, end), "Depth", "1")).Responses)
	}
	if n := count("VEVENT", "20261111T000000Z", "20261112T000000Z"); n != 1 {
		t.Errorf("Wednesday 11 Nov: %d objects, want judo", n)
	}
	if n := count("VEVENT", "20261118T000000Z", "20261119T000000Z"); n != 0 {
		t.Errorf("Wednesday 18 Nov (EXDATE): %d", n)
	}
	if n := count("VEVENT", "20261125T000000Z", "20261127T000000Z"); n != 2 {
		t.Errorf("25 to 26 Nov: %d, want judo and the dentist", n)
	}
	if n := count("VTODO", "20260101T000000Z", "20280101T000000Z"); n != 0 {
		t.Errorf("tasks: %d", n)
	}

	// --- multiget, one href missing
	mg := `<?xml version="1.0"?><c:calendar-multiget xmlns:d="DAV:" xmlns:c="urn:ietf:params:xml:ns:caldav"><d:prop><d:getetag/><c:calendar-data/></d:prop>` +
		`<d:href>` + cal + `judo.ics</d:href><d:href>` + cal + `nope.ics</d:href></c:calendar-multiget>`
	ms = parseMS(t, L.req("REPORT", cal, mg, "Depth", "1"))
	if r := ms.find(cal + "judo.ics"); r == nil || !strings.Contains(r.ok(), "RRULE:FREQ=WEEKLY") {
		t.Errorf("multiget judo: %+v", ms)
	}
	if r := ms.find(cal + "nope.ics"); r == nil || !strings.Contains(r.Status, "404") {
		t.Errorf("multiget of a missing object: %+v", r)
	}

	// --- sync-collection through creates, changes and deletes
	ms = parseMS(t, L.req("REPORT", cal, syncReport("", ""), "Depth", "0"))
	if len(ms.Responses) != 2 || ms.SyncToken == "" {
		t.Fatalf("an initial sync: %+v", ms)
	}
	t1 := ms.SyncToken
	ctag := func() string {
		r := parseMS(t, L.req("PROPFIND", cal, propfindHome, "Depth", "0")).Responses[0].ok()
		i := strings.Index(r, "getctag>")
		return r[i : i+20]
	}
	before := ctag()
	L.req("PUT", cal+"judo.ics", judo, "If-Match", etag)
	L.req("DELETE", cal+"dentiste.ics", "")
	L.req("PUT", cal+"piscine.ics", once("piscine-uid", "Piscine", "20261128"), "If-None-Match", "*")
	if ctag() == before {
		t.Error("the ctag did not change")
	}
	ms = parseMS(t, L.req("REPORT", cal, syncReport(t1, ""), "Depth", "0"))
	if len(ms.Responses) != 3 || ms.SyncToken == t1 {
		t.Fatalf("since t1: %+v", ms)
	}
	if r := ms.find(cal + "dentiste.ics"); r == nil || !strings.Contains(r.Status, "404") {
		t.Errorf("the deleted object: %+v", r)
	}
	if r := ms.find(cal + "piscine.ics"); r == nil || !strings.Contains(r.ok(), "getetag") {
		t.Errorf("the new object: %+v", r)
	}
	ms = parseMS(t, L.req("REPORT", cal, syncReport(t1, "1"), "Depth", "0"))
	if r := ms.find(cal); r == nil || !strings.Contains(r.Status, "507") {
		t.Errorf("a limited sync does not say so: %+v", ms)
	}
	if a := L.req("REPORT", cal, syncReport("https://elsewhere/ns/sync/1/1", ""), "Depth", "0"); a.status != http.StatusForbidden || !strings.Contains(a.body, "valid-sync-token") {
		t.Errorf("a foreign token: %d %s", a.status, a.body)
	}

	// --- sharing: read-only with maman's group, she sees it and cannot write
	info, err := x.svc.calendar.Info(ctx, calendar.Operator("test"), calendar.Ref{Owner: "lea", Slug: "2a3b-c4d5"})
	if err != nil {
		t.Fatal(err)
	}
	if err := x.svc.calendar.Sharing().ShareWith(ctx, "lea", lea.ID, true, info.ID, sharing.Target{User: "maman"}, sharing.Read, store.Time(svcNow())); err != nil {
		t.Fatal(err)
	}
	mcal := "/dav/calendars/maman/lea~2a3b-c4d5/"
	ms = parseMS(t, M.req("PROPFIND", "/dav/calendars/maman/", propfindHome, "Depth", "1"))
	r = ms.find(mcal)
	if r == nil {
		t.Fatalf("maman's home does not list the shared calendar: %+v", ms)
	}
	if strings.Contains(r.ok(), "write") || !strings.Contains(r.ok(), "<d:read/>") {
		t.Errorf("maman's privileges are not read-only: %s", r.ok())
	}
	if a := M.req("GET", mcal+"judo.ics", ""); a.status != 200 {
		t.Errorf("maman reading: %d", a.status)
	}
	if a := M.req("PUT", mcal+"m.ics", once("m-uid", "x", "20261201")); a.status != http.StatusForbidden || !strings.Contains(a.body, "need-privileges") {
		t.Errorf("maman writing to a read share: %d %s", a.status, a.body)
	}
	if a := M.req("PROPFIND", cal, "", "Depth", "0"); a.status != http.StatusNotFound {
		t.Errorf("maman inside lea's home: %d", a.status)
	}
	if a := TV.req("PUT", cal+"tv.ics", once("tv-uid", "x", "20261201")); a.status != http.StatusForbidden {
		t.Errorf("lea's read-only device writing: %d", a.status)
	}
	if a := TV.req("GET", cal+"judo.ics", ""); a.status != 200 {
		t.Errorf("lea's read-only device reading: %d", a.status)
	}

	// --- PROPPATCH: maman renames it for herself only
	pp := `<?xml version="1.0"?><d:propertyupdate xmlns:d="DAV:"><d:set><d:prop><d:displayname>École de Léa</d:displayname></d:prop></d:set></d:propertyupdate>`
	if r := parseMS(t, M.req("PROPPATCH", mcal, pp)).Responses[0]; !strings.Contains(r.ok(), "displayname") {
		t.Errorf("maman's PROPPATCH: %+v", r)
	}
	if got := parseMS(t, M.req("PROPFIND", mcal, propfindHome, "Depth", "0")).Responses[0].ok(); !strings.Contains(got, "École de Léa") {
		t.Errorf("maman's view: %s", got)
	}
	if got := parseMS(t, L.req("PROPFIND", cal, propfindHome, "Depth", "0")).Responses[0].ok(); !strings.Contains(got, "Perso") {
		t.Errorf("lea's name changed: %s", got)
	}
	bad := `<?xml version="1.0"?><d:propertyupdate xmlns:d="DAV:" xmlns:x="urn:x"><d:set><d:prop><d:displayname>Y</d:displayname><x:foo>1</x:foo></d:prop></d:set></d:propertyupdate>`
	if body := L.req("PROPPATCH", cal, bad).body; !strings.Contains(body, "403") || !strings.Contains(body, "424") {
		t.Errorf("a PROPPATCH with an unknown property is not all or nothing: %s", body)
	}

	// --- deleting the calendar: only its owner
	if a := M.req("DELETE", mcal, ""); a.status != http.StatusForbidden {
		t.Errorf("maman deleting lea's calendar: %d", a.status)
	}
	if a := L.req("DELETE", cal, ""); a.status != http.StatusNoContent {
		t.Fatalf("lea deleting her calendar: %d", a.status)
	}
	if a := L.req("PROPFIND", cal, "", "Depth", "0"); a.status != http.StatusNotFound {
		t.Errorf("a deleted calendar: %d", a.status)
	}
	if ms := parseMS(t, M.req("PROPFIND", "/dav/calendars/maman/", propfindHome, "Depth", "1")); ms.find(mcal) != nil {
		t.Error("the deleted calendar is still in maman's home")
	}
}
