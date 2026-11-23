package main

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/nebuloss/agendouille/internal/calendar"
	"github.com/nebuloss/agendouille/internal/sharing"
	"github.com/nebuloss/agendouille/internal/store"
)

// The CalDAV findings of the review of 2026-11-23, each as a request.
func TestCalDAVReviewFindings(t *testing.T) {
	x := startServer(t)
	x.publicOn()
	ctx := context.Background()
	x.svc.dir.AddGroup(ctx, "cli", "famille", "", "")
	lea, _ := x.svc.dir.AddUser(ctx, "cli", "lea", "", "", []string{"famille"})
	maman, _ := x.svc.dir.AddUser(ctx, "cli", "maman", "", "", []string{"famille"})
	rw, _ := x.svc.auth.NewDevice(ctx, lea.ID, "davx5", "Pixel", "readwrite")
	ro, _ := x.svc.auth.NewDevice(ctx, lea.ID, "other", "Télé", "read")
	mp, _ := x.svc.auth.NewDevice(ctx, maman.ID, "ios", "iPhone", "readwrite")
	L, TV, M := davClient{x, "lea", rw.Secret}, davClient{x, "lea", ro.Secret}, davClient{x, "maman", mp.Secret}

	mk := `<?xml version="1.0"?><c:mkcalendar xmlns:d="DAV:" xmlns:c="urn:ietf:params:xml:ns:caldav"><d:set><d:prop><d:displayname>Perso</d:displayname></d:prop></d:set></c:mkcalendar>`
	if a := L.req("MKCALENDAR", "/dav/calendars/lea/ABC-123/", mk); a.status != http.StatusCreated {
		t.Fatalf("MKCALENDAR: %d", a.status)
	}
	bare := "/dav/calendars/lea/ABC-123/"
	L.req("PUT", bare+"x.ics", once("x-uid", "x", "20261201"), "If-None-Match", "*")

	// A request at the bare-slug URL is answered with hrefs under it.
	ms := parseMS(t, L.req("PROPFIND", bare, propfindHome, "Depth", "1"))
	if ms.find(bare) == nil || ms.find(bare+"x.ics") == nil {
		t.Errorf("hrefs at the bare slug: %+v", ms.Responses)
	}

	// A read-only device cannot PROPPATCH, even its owner's calendar.
	pp := `<?xml version="1.0"?><d:propertyupdate xmlns:d="DAV:"><d:set><d:prop><d:displayname>Télé</d:displayname></d:prop></d:set></d:propertyupdate>`
	if a := TV.req("PROPPATCH", bare, pp); a.status != http.StatusForbidden {
		t.Errorf("PROPPATCH from a read-only device: %d", a.status)
	}

	// A sharee's description is refused, not silently dropped; the others 424.
	info, _ := x.svc.calendar.Info(ctx, calendar.Operator("t"), calendar.Ref{Owner: "lea", Slug: "abc-123"})
	x.svc.calendar.Sharing().ShareWith(ctx, "lea", lea.ID, true, info.ID, sharing.Target{User: "maman"}, sharing.ReadWrite, store.Time(svcNow()))
	desc := `<?xml version="1.0"?><d:propertyupdate xmlns:d="DAV:" xmlns:c="urn:ietf:params:xml:ns:caldav"><d:set><d:prop>` +
		`<d:displayname>Chez Léa</d:displayname><c:calendar-description>mine</c:calendar-description></d:prop></d:set></d:propertyupdate>`
	body := M.req("PROPPATCH", "/dav/calendars/maman/lea~abc-123/", desc).body
	if !strings.Contains(body, "403") || !strings.Contains(body, "424") {
		t.Errorf("a sharee's description: %s", body)
	}
	if got := parseMS(t, M.req("PROPFIND", "/dav/calendars/maman/lea~abc-123/", propfindHome, "Depth", "0")).Responses[0].ok(); strings.Contains(got, "Chez Léa") {
		t.Error("a refused PROPPATCH changed the name anyway")
	}
	// The owner sets the zone through calendar-timezone.
	tz := `<?xml version="1.0"?><d:propertyupdate xmlns:d="DAV:" xmlns:c="urn:ietf:params:xml:ns:caldav"><d:set><d:prop><c:calendar-timezone>` +
		"BEGIN:VCALENDAR\r\nBEGIN:VTIMEZONE\r\nTZID:America/Montreal\r\nEND:VTIMEZONE\r\nEND:VCALENDAR\r\n" +
		`</c:calendar-timezone></d:prop></d:set></d:propertyupdate>`
	if a := L.req("PROPPATCH", bare, tz); !strings.Contains(a.body, "200 OK") {
		t.Errorf("the owner setting the zone: %s", a.body)
	}
	if i, _ := x.svc.calendar.Info(ctx, calendar.Operator("t"), info.Ref); i.Timezone != "America/Montreal" {
		t.Errorf("zone %q", i.Timezone)
	}

	// A hostile RRULE is refused at PUT, quickly.
	hostile := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nBEGIN:VEVENT\r\nUID:h\r\nDTSTART:20261123T090000Z\r\n" +
		"RRULE:FREQ=SECONDLY;INTERVAL=36028797018963968;COUNT=2\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"
	if a := L.req("PUT", bare+"h.ics", hostile); a.status != http.StatusForbidden || !strings.Contains(a.body, "valid-calendar-data") {
		t.Errorf("a hostile RRULE: %d %s", a.status, a.body)
	}
}
