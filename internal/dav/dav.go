// Package dav serves the dav surface (PLAN.md 5, CONTRACTS.md 4): CalDAV
// under /dav/ for every client, the agendouille app included, and its
// discovery at /.well-known/caldav. It is written here rather than taken
// from a library (DECISIONS 2026-11-23): one calendar model, VEVENT only,
// three reports, and the exact access rules of PLAN.md 5.3.
//
// Every write goes through calendar.Service; this package only speaks the
// protocol.
package dav

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/nebuloss/agendouille/internal/auth"
	"github.com/nebuloss/agendouille/internal/calendar"
	"github.com/nebuloss/agendouille/internal/deps"
	"github.com/nebuloss/agendouille/internal/ical"
	"github.com/nebuloss/agendouille/internal/sharing"
)

const (
	davHeader = "1, 3, calendar-access, extended-mkcol, webdav-push"
	allow     = "OPTIONS, GET, HEAD, POST, PUT, DELETE, PROPFIND, PROPPATCH, REPORT, MKCALENDAR, MKCOL"
)

type server struct{ d *deps.Deps }

// Register adds the dav surface to mux.
func Register(mux *http.ServeMux, d *deps.Deps) {
	s := &server{d: d}
	mux.HandleFunc("/.well-known/caldav", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/dav/", http.StatusMovedPermanently)
	})
	mux.HandleFunc("/dav/", s.serve)
	mux.HandleFunc("/dav", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/dav/", http.StatusMovedPermanently)
	})
	mux.HandleFunc("GET /ics/{file}", s.subscription)
}

// subscription serves a read-only link (PLAN.md 7): its calendars as one
// .ics, each still filtered by its user's access at this moment, so a
// share removed since leaves the link too.
func (s *server) subscription(w http.ResponseWriter, r *http.Request) {
	secret, ok := strings.CutSuffix(r.PathValue("file"), ".ics")
	if !ok {
		http.NotFound(w, r)
		return
	}
	ctx := r.Context()
	userID, ids, err := s.d.Auth.OpenLink(ctx, secret)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	u, err := s.d.Directory.UserByID(ctx, userID)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	a := calendar.Actor{UserID: u.ID, Name: u.Name, Cap: "read", Via: "ics"}
	var infos []calendar.Info
	var names []string
	for _, id := range ids {
		i, err := s.d.Calendar.InfoByID(ctx, a, id)
		if err != nil {
			continue // deleted, or no longer shared with them
		}
		infos = append(infos, i)
		names = append(names, i.DisplayName())
	}
	zone := s.d.Settings.Get().FamilyTimezone
	data, err := s.d.Calendar.ExportMany(ctx, infos, strings.Join(names, ", "), zone, "PT15M")
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/calendar; charset=utf-8")
	w.Header().Set("Cache-Control", "private, max-age=300")
	_, _ = w.Write(data)
}

// kind of resource a path names.
type kind int

const (
	kRoot kind = iota
	kPrincipal
	kHome
	kCalendar
	kObject
	kPush // a WebDAV-Push registration, /dav/push/<id>
	kNone
)

// target is a parsed request path.
type target struct {
	kind   kind
	user   string // the home or principal in the path
	calSeg string
	name   string
}

// route reads /dav/..., decoded.
func route(p string) target {
	rest := strings.TrimPrefix(p, "/dav/")
	trailing := strings.HasSuffix(rest, "/")
	parts := strings.Split(strings.Trim(rest, "/"), "/")
	if rest == "" {
		return target{kind: kRoot}
	}
	switch {
	case len(parts) == 2 && parts[0] == "push" && !trailing:
		return target{kind: kPush, name: parts[1]}
	case len(parts) == 2 && parts[0] == "principals":
		return target{kind: kPrincipal, user: parts[1]}
	case len(parts) == 2 && parts[0] == "calendars":
		return target{kind: kHome, user: parts[1]}
	case len(parts) == 3 && parts[0] == "calendars":
		return target{kind: kCalendar, user: parts[1], calSeg: parts[2]}
	case len(parts) == 4 && parts[0] == "calendars" && !trailing:
		return target{kind: kObject, user: parts[1], calSeg: parts[2], name: parts[3]}
	}
	return target{kind: kNone}
}

// ref is the calendar a path names from user's home: "<owner>~<slug>", or a
// bare slug for one of the user's own (the URL a client MKCALENDARed at).
func (t target) ref() (calendar.Ref, error) {
	if strings.Contains(t.calSeg, "~") {
		return calendar.ParseRef(t.calSeg)
	}
	slug, err := calendar.NormalizeSlug(t.calSeg)
	if err != nil {
		return calendar.Ref{}, calendar.ErrNotFound
	}
	return calendar.Ref{Owner: t.user, Slug: slug}, nil
}

func principalPath(user string) string { return "/dav/principals/" + url.PathEscape(user) + "/" }
func homePath(user string) string      { return "/dav/calendars/" + url.PathEscape(user) + "/" }
func calendarPath(user string, r calendar.Ref) string {
	return homePath(user) + url.PathEscape(r.String()) + "/"
}
func objectPath(user string, r calendar.Ref, name string) string {
	return calendarPath(user, r) + url.PathEscape(name)
}

func (s *server) serve(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("DAV", davHeader)
	if r.Method == http.MethodOptions {
		// Unauthenticated: clients probe what the server speaks first.
		w.Header().Set("Allow", allow)
		w.WriteHeader(http.StatusOK)
		return
	}
	ip := auth.ClientIP(r, s.d.Trusted()).String()
	p, err := s.d.Auth.Basic(r, ip)
	if err != nil {
		// What failed, never the credential: a client that cannot sign in
		// is found here.
		user, _, basic := r.BasicAuth()
		s.d.Log.Info("caldav sign-in refused", "ip", ip, "basic_user", user, "basic", basic, "agent", r.UserAgent())
		w.Header().Set("WWW-Authenticate", `Basic realm="agendouille", charset="UTF-8"`)
		http.Error(w, "a device credential is needed", http.StatusUnauthorized)
		return
	}
	t := route(r.URL.Path)
	if t.kind == kNone || (t.kind != kRoot && t.kind != kPush && t.user != p.User.Name) {
		http.NotFound(w, r)
		return
	}
	a := calendar.Actor{UserID: p.User.ID, Name: p.User.Name, DeviceID: p.DeviceID, Cap: p.DeviceCap, Via: "caldav",
		DontNotify: dontNotify(r.Header.Values("Push-Dont-Notify"))}
	c := &call{s: s, w: w, r: r, p: p, a: a, t: t}
	switch r.Method {
	case "PROPFIND":
		c.propfind()
	case "REPORT":
		c.report()
	case http.MethodGet, http.MethodHead:
		c.get()
	case http.MethodPut:
		c.put()
	case http.MethodPost:
		c.pushRegister()
	case http.MethodDelete:
		if t.kind == kPush {
			c.pushUnregister()
			return
		}
		c.delete()
	case "MKCALENDAR", "MKCOL":
		c.mkcalendar()
	case "PROPPATCH":
		c.proppatch()
	case "MOVE", "COPY", "LOCK", "UNLOCK", "ACL":
		w.Header().Set("Allow", allow)
		http.Error(w, "not supported", http.StatusMethodNotAllowed)
	default:
		w.Header().Set("Allow", allow)
		http.Error(w, "not supported", http.StatusMethodNotAllowed)
	}
}

// call is one request.
type call struct {
	s *server
	w http.ResponseWriter
	r *http.Request
	p *auth.Principal
	a calendar.Actor
	t target
}

func (c *call) multistatus(body string) {
	c.w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	c.w.WriteHeader(http.StatusMultiStatus)
	_, _ = io.WriteString(c.w, body)
}

func (c *call) xmlError(status int, space, local, inner string) {
	c.w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	c.w.WriteHeader(status)
	_, _ = io.WriteString(c.w, davError(space, local, inner))
}

// fail maps the calendar's errors onto CalDAV's (CONTRACTS.md 4.3).
func (c *call) fail(err error) {
	var conflict *calendar.UIDConflict
	switch {
	case errors.As(err, &conflict):
		c.xmlError(http.StatusForbidden, nsCalDAV, "no-uid-conflict", href(homePath(c.p.User.Name)+url.PathEscape(c.t.calSeg)+"/"+url.PathEscape(conflict.Name)))
	case errors.Is(err, calendar.ErrNotFound), errors.Is(err, sharing.ErrNotFound):
		http.Error(c.w, "not found", http.StatusNotFound)
	case errors.Is(err, calendar.ErrForbidden), errors.Is(err, calendar.ErrNotOwner):
		c.xmlError(http.StatusForbidden, nsDAV, "need-privileges", "")
	case errors.Is(err, calendar.ErrPrecondition):
		http.Error(c.w, "precondition failed", http.StatusPreconditionFailed)
	case errors.Is(err, ical.ErrUnsupported):
		c.xmlError(http.StatusForbidden, nsCalDAV, "supported-calendar-component", "")
	case errors.Is(err, ical.ErrTooLarge):
		c.xmlError(http.StatusForbidden, nsCalDAV, "max-resource-size", "")
	case errors.Is(err, ical.ErrInvalid):
		c.xmlError(http.StatusForbidden, nsCalDAV, "valid-calendar-data", "")
	case errors.Is(err, calendar.ErrQuota):
		c.xmlError(http.StatusInsufficientStorage, nsDAV, "quota-not-exceeded", "")
	case errors.Is(err, calendar.ErrExists):
		c.w.Header().Set("Allow", allow)
		http.Error(c.w, "exists", http.StatusMethodNotAllowed)
	case errors.Is(err, calendar.ErrBadName), errors.Is(err, calendar.ErrBadSlug):
		http.Error(c.w, err.Error(), http.StatusForbidden)
	case errors.Is(err, calendar.ErrTokenExpired):
		c.xmlError(http.StatusForbidden, nsDAV, "valid-sync-token", "")
	default:
		c.s.d.Log.Error("caldav", "method", c.r.Method, "path", c.r.URL.Path, "err", err)
		http.Error(c.w, "internal error", http.StatusInternalServerError)
	}
}

// info looks up the calendar the path names, with the caller's access.
func (c *call) info() (calendar.Info, error) {
	ref, err := c.t.ref()
	if err != nil {
		return calendar.Info{}, err
	}
	return c.s.d.Calendar.Info(c.r.Context(), c.a, ref)
}

func (c *call) get() {
	switch c.t.kind {
	case kObject:
		ref, err := c.t.ref()
		if err != nil {
			c.fail(err)
			return
		}
		_, o, err := c.s.d.Calendar.Get(c.r.Context(), c.a, ref, c.t.name)
		if err != nil {
			c.fail(err)
			return
		}
		h := c.w.Header()
		h.Set("ETag", o.ETag)
		h.Set("Content-Type", "text/calendar; charset=utf-8")
		h.Set("Last-Modified", o.ModifiedAt.UTC().Format(http.TimeFormat))
		h.Set("Cache-Control", "no-cache")
		if inm := c.r.Header.Get("If-None-Match"); inm != "" && containsETag(splitETags(inm), o.ETag) {
			c.w.WriteHeader(http.StatusNotModified)
			return
		}
		h.Set("Content-Length", fmt.Sprint(len(o.Data)))
		if c.r.Method == http.MethodHead {
			return
		}
		_, _ = c.w.Write(o.Data)
	case kCalendar:
		i, err := c.info()
		if err != nil {
			c.fail(err)
			return
		}
		data, err := c.s.d.Calendar.ExportInfo(c.r.Context(), i)
		if err != nil {
			c.fail(err)
			return
		}
		c.w.Header().Set("Content-Type", "text/calendar; charset=utf-8")
		c.w.Header().Set("Content-Disposition", `attachment; filename="`+i.Ref.Slug+`.ics"`)
		c.w.Header().Set("Cache-Control", "no-store")
		if c.r.Method != http.MethodHead {
			_, _ = c.w.Write(data)
		}
	default:
		c.w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = io.WriteString(c.w, "agendouille CalDAV: point your calendar app at this address.\n")
	}
}

func splitETags(h string) []string {
	var out []string
	for _, part := range strings.Split(h, ",") {
		if p := strings.TrimSpace(part); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func containsETag(list []string, etag string) bool {
	for _, e := range list {
		if e == "*" || e == etag || strings.TrimPrefix(e, "W/") == etag {
			return true
		}
	}
	return false
}

func (c *call) put() {
	if c.t.kind != kObject {
		c.w.Header().Set("Allow", allow)
		http.Error(c.w, "PUT only goes to an object", http.StatusMethodNotAllowed)
		return
	}
	if ct := c.r.Header.Get("Content-Type"); ct != "" && !strings.HasPrefix(strings.ToLower(ct), "text/calendar") {
		http.Error(c.w, "a calendar object is text/calendar", http.StatusUnsupportedMediaType)
		return
	}
	ref, err := c.t.ref()
	if err != nil {
		c.fail(err)
		return
	}
	limit := int64(c.s.d.Settings.Get().MaxObjectBytes)
	data, err := io.ReadAll(io.LimitReader(c.r.Body, limit+1))
	if err != nil {
		http.Error(c.w, "reading the body", http.StatusBadRequest)
		return
	}
	if int64(len(data)) > limit {
		c.fail(ical.ErrTooLarge)
		return
	}
	pre := calendar.Precondition{IfNoneMatch: strings.TrimSpace(c.r.Header.Get("If-None-Match")) == "*"}
	if im := c.r.Header.Get("If-Match"); im != "" {
		pre.IfMatch = splitETags(im)
	}
	o, created, err := c.s.d.Calendar.Put(c.r.Context(), c.a, ref, c.t.name, data, pre)
	if err != nil {
		c.fail(err)
		return
	}
	// The bytes are stored as sent, so the ETag is the client's to keep.
	c.w.Header().Set("ETag", o.ETag)
	if created {
		c.w.WriteHeader(http.StatusCreated)
	} else {
		c.w.WriteHeader(http.StatusNoContent)
	}
}

func (c *call) delete() {
	switch c.t.kind {
	case kObject:
		ref, err := c.t.ref()
		if err != nil {
			c.fail(err)
			return
		}
		pre := calendar.Precondition{}
		if im := c.r.Header.Get("If-Match"); im != "" {
			pre.IfMatch = splitETags(im)
		}
		if err := c.s.d.Calendar.Delete(c.r.Context(), c.a, ref, c.t.name, pre); err != nil {
			c.fail(err)
			return
		}
		c.w.WriteHeader(http.StatusNoContent)
	case kCalendar:
		i, err := c.info()
		if err != nil {
			c.fail(err)
			return
		}
		if i.Access != sharing.Owner {
			// A sharee hides a calendar in the web app; it never deletes it.
			c.xmlError(http.StatusForbidden, nsDAV, "need-privileges", "")
			return
		}
		if err := c.s.d.Calendar.DeleteCalendar(c.r.Context(), c.a, i.Ref); err != nil {
			c.fail(err)
			return
		}
		c.w.WriteHeader(http.StatusNoContent)
	default:
		c.xmlError(http.StatusForbidden, nsDAV, "need-privileges", "")
	}
}
