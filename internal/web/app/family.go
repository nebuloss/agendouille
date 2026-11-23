package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/nebuloss/agendouille/internal/auth"
	"github.com/nebuloss/agendouille/internal/calendar"
	"github.com/nebuloss/agendouille/internal/directory"
	"github.com/nebuloss/agendouille/internal/hub"
	"github.com/nebuloss/agendouille/internal/ical"
	"github.com/nebuloss/agendouille/internal/sharing"
	"github.com/nebuloss/agendouille/internal/store"
	"github.com/nebuloss/agendouille/internal/web/webx"
)

// The additional API of the family web app (CONTRACTS.md 2.2, 3.2 to 3.4):
// what CalDAV has no standard for. Sync never depends on it.

func (h *handlers) registerFamily(mux *http.ServeMux, guard func(http.Handler) http.Handler) {
	api := func(pattern string, f withPrincipal) {
		var handler = h.api(f)
		if !strings.HasPrefix(pattern, "GET ") {
			handler = guard(handler)
		}
		mux.Handle(pattern, handler)
	}
	api("PATCH /api/me", h.patchMe)
	api("POST /api/me/enrolment", h.selfEnrolment)
	api("GET /api/people", h.people)

	api("GET /api/calendars", h.calendars)
	api("POST /api/calendars", h.createCalendar)
	api("GET /api/calendars/{ref}", h.getCalendar)
	api("PATCH /api/calendars/{ref}", h.patchCalendar)
	api("DELETE /api/calendars/{ref}", h.deleteCalendar)
	api("GET /api/calendars/{ref}/shares", h.shares)
	api("PUT /api/calendars/{ref}/shares/{target}", h.share)
	api("DELETE /api/calendars/{ref}/shares/{target}", h.unshare)
	api("POST /api/calendars/{ref}/transfer", h.transfer)
	api("POST /api/calendars/{ref}/import", h.importICS)
	api("GET /api/calendars/{ref}/export", h.exportICS)

	api("GET /api/occurrences", h.occurrences)
	api("GET /api/calendars/{ref}/events/{name}", h.event)
	api("POST /api/calendars/{ref}/events", h.createEvent)
	api("PUT /api/calendars/{ref}/events/{name}", h.editEvent)
	api("DELETE /api/calendars/{ref}/events/{name}", h.deleteEvent)
	api("POST /api/calendars/{ref}/events/{name}/move", h.moveEvent)

	api("GET /api/devices", h.devices)
	api("POST /api/devices", h.addDevice)
	api("DELETE /api/devices/{id}", h.revokeDevice)
	api("GET /api/devices/{id}/mobileconfig", h.mobileconfig)
	api("POST /api/devices/{id}/certificate", h.newCertificate)
	api("GET /api/devices/{id}/certificate.p12", h.certificateFile)
	api("GET /api/links", h.links)
	api("POST /api/links", h.addLink)
	api("DELETE /api/links/{id}", h.revokeLink)
	api("GET /api/sessions", h.sessions)
	api("DELETE /api/sessions/{id}", h.revokeSession)
	api("GET /api/live", h.live)
}

func (h *handlers) actor(p *auth.Principal) calendar.Actor {
	via := "web"
	if p.DeviceID != 0 {
		via = "app"
	}
	return calendar.Actor{UserID: p.User.ID, Name: p.User.Name, DeviceID: p.DeviceID, Cap: p.DeviceCap, Via: via}
}

func (h *handlers) family() *time.Location {
	if loc, err := time.LoadLocation(h.d.Settings.Get().FamilyTimezone); err == nil {
		return loc
	}
	return time.UTC
}

// fail maps the services' errors onto CONTRACTS.md 3.1.
func (h *handlers) fail(w http.ResponseWriter, err error) {
	var conflict *calendar.UIDConflict
	switch {
	case errors.As(err, &conflict):
		webx.Fail(w, http.StatusConflict, "uid_conflict", "cet événement existe déjà dans ce calendrier")
	case errors.Is(err, calendar.ErrNotFound), errors.Is(err, directory.ErrNotFound), errors.Is(err, sharing.ErrNotFound), errors.Is(err, auth.ErrInvalid):
		webx.Fail(w, http.StatusNotFound, "not_found", "introuvable")
	case errors.Is(err, calendar.ErrForbidden), errors.Is(err, calendar.ErrNotOwner):
		webx.Fail(w, http.StatusForbidden, "forbidden", "vous ne pouvez pas modifier ce calendrier")
	case errors.Is(err, sharing.ErrNotVisible):
		webx.Fail(w, http.StatusForbidden, "not_visible", "vous ne pouvez partager qu'avec les gens de vos groupes")
	case errors.Is(err, calendar.ErrPrecondition):
		webx.Fail(w, http.StatusPreconditionFailed, "precondition_failed", "modifié entre-temps")
	case errors.Is(err, ical.ErrInvalid), errors.Is(err, ical.ErrUnsupported), errors.Is(err, calendar.ErrBadSlug), errors.Is(err, directory.ErrBadName):
		webx.Fail(w, http.StatusUnprocessableEntity, "invalid_event", err.Error())
	case errors.Is(err, ical.ErrTooLarge):
		webx.Fail(w, http.StatusRequestEntityTooLarge, "too_large", "trop gros")
	case errors.Is(err, calendar.ErrQuota), errors.Is(err, auth.ErrTooMany):
		webx.Fail(w, http.StatusInsufficientStorage, "quota", err.Error())
	case errors.Is(err, calendar.ErrExists):
		webx.Fail(w, http.StatusConflict, "slug_taken", err.Error())
	default:
		var bad badRequest
		if errors.As(err, &bad) {
			webx.Fail(w, http.StatusBadRequest, "bad_request", bad.msg)
			return
		}
		h.d.Log.Error("family api", "err", err)
		webx.Fail(w, http.StatusInternalServerError, "error", "")
	}
}

type badRequest struct{ msg string }

func (b badRequest) Error() string { return b.msg }

func (h *handlers) ref(r *http.Request) (calendar.Ref, error) {
	return calendar.ParseRef(r.PathValue("ref"))
}

// access tells the live feeds that who sees what may have changed.
func (h *handlers) access(i calendar.Info) {
	if h.d.Hub != nil {
		h.d.Hub.Publish(hub.Event{CalendarID: i.ID, Calendar: i.Ref.String(), Action: "access"})
	}
}

// --- me, people

func (h *handlers) patchMe(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	var body struct {
		DisplayName string `json:"display_name"`
		Color       string `json:"color"`
	}
	if !webx.DecodeJSON(w, r, &body) {
		return
	}
	if err := h.d.Directory.UpdateUser(r.Context(), p.User.Name, p.User.Name, body.DisplayName, body.Color); err != nil {
		h.fail(w, err)
		return
	}
	p.User, _ = h.d.Directory.UserByID(r.Context(), p.User.ID)
	h.me(w, r, p)
}

// selfEnrolment is "enrol another phone" (PLAN.md 8.5): a link that lives
// ten minutes, shown as a QR on this signed-in device.
func (h *handlers) selfEnrolment(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	e, err := h.d.Auth.NewEnrolment(r.Context(), p.User.Name, p.User.ID, 10*time.Minute)
	if err != nil {
		h.fail(w, err)
		return
	}
	link := h.d.PublicURL() + "/enrol/" + e.Token
	qr, err := webx.QRSVG(link)
	if err != nil {
		h.fail(w, err)
		return
	}
	webx.JSON(w, http.StatusCreated, map[string]any{"url": link, "qr_svg": qr, "expires_at": e.ExpiresAt,
		"expires_in_ms": e.ExpiresAt.Sub(h.d.Clock()).Milliseconds(), "ttl_ms": (10 * time.Minute).Milliseconds()})
}

type personView struct {
	Name        string `json:"name"`
	DisplayName string `json:"display_name"`
	Color       string `json:"color"`
}

func (h *handlers) people(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	ctx := r.Context()
	users, err := h.d.Directory.Visible(ctx, p.User.ID)
	if err != nil {
		h.fail(w, err)
		return
	}
	groups, err := h.d.Directory.GroupsOf(ctx, p.User.ID)
	if err != nil {
		h.fail(w, err)
		return
	}
	us := []personView{}
	for _, u := range users {
		us = append(us, personView{u.Name, u.DisplayName, u.Color})
	}
	gs := []personView{}
	for _, g := range groups {
		gs = append(gs, personView{g.Name, g.DisplayName, g.Color})
	}
	webx.JSON(w, http.StatusOK, map[string]any{"users": us, "groups": gs})
}

// --- calendars

type calendarView struct {
	Ref         string          `json:"ref"`
	Owner       string          `json:"owner"`
	Name        string          `json:"name"`
	Color       string          `json:"color"`
	Timezone    string          `json:"timezone"`
	Description string          `json:"description"`
	Access      string          `json:"access"`
	Shown       bool            `json:"shown"`
	Hidden      bool            `json:"hidden"`
	Seq         int64           `json:"seq"`
	Shares      []sharing.Share `json:"shares"`
}

func (h *handlers) calendarView(r *http.Request, i calendar.Info) (calendarView, error) {
	v := calendarView{Ref: i.Ref.String(), Owner: i.Ref.Owner, Name: i.DisplayName(), Color: i.DisplayColor(), Timezone: i.Timezone,
		Description: i.Description, Access: i.Access.String(), Shown: i.View.Shown, Hidden: i.View.Hidden, Seq: i.Seq}
	if i.Access == sharing.Owner {
		var err error
		if v.Shares, err = h.d.Calendar.Sharing().Shares(r.Context(), i.ID); err != nil {
			return v, err
		}
	}
	return v, nil
}

func (h *handlers) calendars(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	infos, err := h.d.Calendar.Home(r.Context(), h.actor(p), true)
	if err != nil {
		h.fail(w, err)
		return
	}
	out := []calendarView{}
	for _, i := range infos {
		v, err := h.calendarView(r, i)
		if err != nil {
			h.fail(w, err)
			return
		}
		out = append(out, v)
	}
	sort.SliceStable(out, func(a, b int) bool {
		if (out[a].Owner == p.User.Name) != (out[b].Owner == p.User.Name) {
			return out[a].Owner == p.User.Name
		}
		return out[a].Name < out[b].Name
	})
	webx.JSON(w, http.StatusOK, map[string]any{"family_timezone": h.d.Settings.Get().FamilyTimezone, "calendars": out})
}

func (h *handlers) getCalendar(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	ref, err := h.ref(r)
	if err != nil {
		h.fail(w, err)
		return
	}
	i, err := h.d.Calendar.Info(r.Context(), h.actor(p), ref)
	if err != nil {
		h.fail(w, err)
		return
	}
	v, err := h.calendarView(r, i)
	if err != nil {
		h.fail(w, err)
		return
	}
	webx.JSON(w, http.StatusOK, v)
}

func (h *handlers) createCalendar(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	var body struct {
		Name        string `json:"name"`
		Color       string `json:"color"`
		Timezone    string `json:"timezone"`
		Description string `json:"description"`
	}
	if !webx.DecodeJSON(w, r, &body) {
		return
	}
	body.Name = strings.TrimSpace(body.Name)
	if body.Name == "" {
		h.fail(w, badRequest{"un calendrier a un nom"})
		return
	}
	base, err := calendar.NormalizeSlug(slugOf(body.Name))
	if err != nil {
		base = "calendrier"
	}
	props := calendar.Props{Name: &body.Name, Description: &body.Description}
	if body.Color != "" {
		props.Color = &body.Color
	}
	if body.Timezone != "" {
		props.Timezone = &body.Timezone
	}
	for n := 1; n < 100; n++ {
		slug := base
		if n > 1 {
			slug = fmt.Sprintf("%s-%d", base, n)
		}
		i, err := h.d.Calendar.Create(r.Context(), h.actor(p), p.User.Name, slug, props)
		if errors.Is(err, calendar.ErrExists) {
			continue
		}
		if err != nil {
			h.fail(w, err)
			return
		}
		v, _ := h.calendarView(r, i)
		h.access(i)
		webx.JSON(w, http.StatusCreated, v)
		return
	}
	h.fail(w, calendar.ErrExists)
}

// slugOf turns a name into ASCII for a slug: "École de Léa" -> "ecole-de-lea".
func slugOf(name string) string {
	r := strings.NewReplacer("à", "a", "â", "a", "ä", "a", "é", "e", "è", "e", "ê", "e", "ë", "e", "î", "i", "ï", "i",
		"ô", "o", "ö", "o", "ù", "u", "û", "u", "ü", "u", "ç", "c", "œ", "oe", "æ", "ae", "ÿ", "y")
	return r.Replace(strings.ToLower(name))
}

func (h *handlers) patchCalendar(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	ref, err := h.ref(r)
	if err != nil {
		h.fail(w, err)
		return
	}
	var body struct {
		Name        *string `json:"name"`
		Color       *string `json:"color"`
		Timezone    *string `json:"timezone"`
		Description *string `json:"description"`
		Shown       *bool   `json:"shown"`
		Hidden      *bool   `json:"hidden"`
	}
	if !webx.DecodeJSON(w, r, &body) {
		return
	}
	ctx := r.Context()
	a := h.actor(p)
	i, err := h.d.Calendar.Info(ctx, a, ref)
	if err != nil {
		h.fail(w, err)
		return
	}
	if body.Shown != nil || body.Hidden != nil {
		if err := h.d.Calendar.Sharing().SetView(ctx, p.User.ID, i.ID, body.Shown, body.Hidden, nil, nil, nil); err != nil {
			h.fail(w, err)
			return
		}
		if body.Hidden != nil {
			h.access(i) // it leaves (or comes back to) the user's devices
		}
	}
	if body.Name != nil || body.Color != nil || body.Timezone != nil || body.Description != nil {
		if i.Access != sharing.Owner && (body.Timezone != nil || body.Description != nil) {
			h.fail(w, calendar.ErrNotOwner)
			return
		}
		if _, err := h.d.Calendar.Update(ctx, a, ref, calendar.Props{Name: body.Name, Color: body.Color, Timezone: body.Timezone,
			Description: body.Description}); err != nil {
			h.fail(w, err)
			return
		}
	}
	i, err = h.d.Calendar.Info(ctx, a, ref)
	if err != nil {
		h.fail(w, err)
		return
	}
	v, err := h.calendarView(r, i)
	if err != nil {
		h.fail(w, err)
		return
	}
	webx.JSON(w, http.StatusOK, v)
}

func (h *handlers) deleteCalendar(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	ref, err := h.ref(r)
	if err != nil {
		h.fail(w, err)
		return
	}
	i, err := h.d.Calendar.Info(r.Context(), h.actor(p), ref)
	if err != nil {
		h.fail(w, err)
		return
	}
	if err := h.d.Calendar.DeleteCalendar(r.Context(), h.actor(p), ref); err != nil {
		h.fail(w, err)
		return
	}
	h.access(i)
	w.WriteHeader(http.StatusNoContent)
}

// owned looks a calendar up and checks the caller owns it.
func (h *handlers) owned(w http.ResponseWriter, r *http.Request, p *auth.Principal) (calendar.Info, bool) {
	ref, err := h.ref(r)
	if err != nil {
		h.fail(w, err)
		return calendar.Info{}, false
	}
	i, err := h.d.Calendar.Info(r.Context(), h.actor(p), ref)
	if err != nil {
		h.fail(w, err)
		return calendar.Info{}, false
	}
	if i.Access != sharing.Owner {
		h.fail(w, calendar.ErrNotOwner)
		return calendar.Info{}, false
	}
	return i, true
}

func (h *handlers) shares(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	i, ok := h.owned(w, r, p)
	if !ok {
		return
	}
	s, err := h.d.Calendar.Sharing().Shares(r.Context(), i.ID)
	if err != nil {
		h.fail(w, err)
		return
	}
	webx.JSON(w, http.StatusOK, map[string]any{"shares": s})
}

func (h *handlers) share(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	i, ok := h.owned(w, r, p)
	if !ok {
		return
	}
	t, err := sharing.ParseTarget(r.PathValue("target"))
	if err != nil {
		h.fail(w, badRequest{err.Error()})
		return
	}
	var body struct {
		Access string `json:"access"`
	}
	if !webx.DecodeJSON(w, r, &body) {
		return
	}
	a, err := sharing.ParseAccess(body.Access)
	if err != nil {
		h.fail(w, badRequest{err.Error()})
		return
	}
	if err := h.d.Calendar.Sharing().ShareWith(r.Context(), p.User.Name, p.User.ID, true, i.ID, t, a, store.Time(h.d.Clock())); err != nil {
		h.fail(w, err)
		return
	}
	h.access(i)
	h.shares(w, r, p)
}

func (h *handlers) unshare(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	i, ok := h.owned(w, r, p)
	if !ok {
		return
	}
	t, err := sharing.ParseTarget(r.PathValue("target"))
	if err != nil {
		h.fail(w, badRequest{err.Error()})
		return
	}
	if err := h.d.Calendar.Sharing().Unshare(r.Context(), p.User.Name, i.ID, t, store.Time(h.d.Clock())); err != nil {
		h.fail(w, err)
		return
	}
	h.access(i)
	h.shares(w, r, p)
}

func (h *handlers) transfer(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	i, ok := h.owned(w, r, p)
	if !ok {
		return
	}
	var body struct {
		To string `json:"to"`
	}
	if !webx.DecodeJSON(w, r, &body) {
		return
	}
	n, err := h.d.Calendar.Transfer(r.Context(), h.actor(p), i.Ref, body.To, true)
	if err != nil {
		h.fail(w, err)
		return
	}
	h.access(i)
	webx.JSON(w, http.StatusOK, map[string]string{"ref": n.Ref.String()})
}

func (h *handlers) importICS(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	ref, err := h.ref(r)
	if err != nil {
		h.fail(w, err)
		return
	}
	data, err := io.ReadAll(io.LimitReader(r.Body, 20<<20))
	if err != nil {
		h.fail(w, badRequest{"le fichier n'a pas pu être lu"})
		return
	}
	res, err := h.d.Calendar.Import(r.Context(), h.actor(p), ref, data, r.URL.Query().Get("replace") == "1")
	if err != nil {
		h.fail(w, err)
		return
	}
	webx.JSON(w, http.StatusOK, res)
}

func (h *handlers) exportICS(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	ref, err := h.ref(r)
	if err != nil {
		h.fail(w, err)
		return
	}
	data, err := h.d.Calendar.Export(r.Context(), h.actor(p), ref)
	if err != nil {
		h.fail(w, err)
		return
	}
	w.Header().Set("Content-Type", "text/calendar; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="`+ref.Slug+`.ics"`)
	_, _ = w.Write(data)
}

// --- events

type occurrenceView struct {
	Calendar     string  `json:"calendar"`
	Name         string  `json:"name"`
	ETag         string  `json:"etag"`
	UID          string  `json:"uid"`
	RecurrenceID *string `json:"recurrence_id"`
	Summary      string  `json:"summary"`
	Location     string  `json:"location"`
	AllDay       bool    `json:"all_day"`
	Start        string  `json:"start"`
	End          string  `json:"end"`
	Recurring    bool    `json:"recurring"`
	Overridden   bool    `json:"overridden"`
	CreatedBy    string  `json:"created_by"`
	ModifiedBy   string  `json:"modified_by"`
}

// names maps user ids to names, for "added by" (removed users included).
type names struct {
	h    *handlers
	r    *http.Request
	seen map[int64]string
}

func (n *names) of(id int64) string {
	if v, ok := n.seen[id]; ok {
		return v
	}
	u, err := n.h.d.Directory.UserByID(n.r.Context(), id)
	v := ""
	if err == nil {
		v = u.Name
	}
	n.seen[id] = v
	return v
}

// maxRange bounds one occurrences request (CONTRACTS.md 3.2).
const maxRange = 62 * 24 * time.Hour

func (h *handlers) occurrences(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	ctx := r.Context()
	fam := h.family()
	q := r.URL.Query()
	from, err1 := time.ParseInLocation("2006-01-02", q.Get("from"), fam)
	to, err2 := time.ParseInLocation("2006-01-02", q.Get("to"), fam)
	if err1 != nil || err2 != nil || !to.After(from) || to.Sub(from) > maxRange+2*time.Hour {
		h.fail(w, badRequest{"from et to sont des dates, au plus 62 jours d'écart"})
		return
	}
	a := h.actor(p)
	home, err := h.d.Calendar.Home(ctx, a, false)
	if err != nil {
		h.fail(w, err)
		return
	}
	want := map[string]bool{}
	if c := q.Get("calendars"); c != "" {
		for _, ref := range strings.Split(c, ",") {
			want[ref] = true
		}
	}
	nm := &names{h: h, r: r, seen: map[int64]string{}}
	out := []occurrenceView{}
	truncated := false
	for _, i := range home {
		if (len(want) > 0 && !want[i.Ref.String()]) || (len(want) == 0 && !i.View.Shown) {
			continue
		}
		objs, err := h.d.Calendar.Range(ctx, i, from, to)
		if err != nil {
			h.fail(w, err)
			return
		}
		loc := i.Location()
		for _, o := range objs {
			obj, _, err := ical.Read(o.Data, 0, loc)
			if err != nil {
				continue
			}
			occ, cut, err := obj.Expand(loc, from, to, 2000)
			if err != nil {
				continue
			}
			truncated = truncated || cut
			for _, oc := range occ {
				v := occurrenceView{Calendar: i.Ref.String(), Name: o.Name, ETag: o.ETag, UID: o.UID,
					Summary: oc.Event.Text("SUMMARY"), Location: oc.Event.Text("LOCATION"), AllDay: oc.AllDay,
					Start: ical.LocalString(oc.Start, oc.AllDay, fam), End: ical.LocalString(oc.End, oc.AllDay, fam),
					Recurring: oc.RecurrenceID != nil, Overridden: oc.Overridden,
					CreatedBy: nm.of(o.CreatedBy), ModifiedBy: nm.of(o.ModifiedBy)}
				if oc.RecurrenceID != nil {
					s := ical.LocalString(*oc.RecurrenceID, oc.RecurrenceAllDay, fam)
					v.RecurrenceID = &s
				}
				out = append(out, v)
			}
		}
	}
	sort.SliceStable(out, func(a, b int) bool { return out[a].Start < out[b].Start })
	webx.JSON(w, http.StatusOK, map[string]any{"occurrences": out, "truncated": truncated})
}

type eventView struct {
	ical.EventView
	Calendar   string    `json:"calendar"`
	Name       string    `json:"name"`
	ETag       string    `json:"etag"`
	CreatedBy  string    `json:"created_by"`
	CreatedAt  time.Time `json:"created_at"`
	ModifiedBy string    `json:"modified_by"`
	ModifiedAt time.Time `json:"modified_at"`
	Writable   bool      `json:"writable"`
}

func (h *handlers) eventView(r *http.Request, i calendar.Info, o calendar.Object) (eventView, error) {
	v, err := ical.View(o.Data, i.Location(), h.family())
	if err != nil {
		return eventView{}, err
	}
	nm := &names{h: h, r: r, seen: map[int64]string{}}
	return eventView{EventView: v, Calendar: i.Ref.String(), Name: o.Name, ETag: o.ETag, CreatedBy: nm.of(o.CreatedBy),
		CreatedAt: o.CreatedAt, ModifiedBy: nm.of(o.ModifiedBy), ModifiedAt: o.ModifiedAt, Writable: i.Access.CanWrite()}, nil
}

func (h *handlers) writeEvent(w http.ResponseWriter, r *http.Request, status int, i calendar.Info, o calendar.Object) {
	v, err := h.eventView(r, i, o)
	if err != nil {
		h.fail(w, err)
		return
	}
	w.Header().Set("ETag", o.ETag)
	webx.JSON(w, status, v)
}

func (h *handlers) event(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	ref, err := h.ref(r)
	if err != nil {
		h.fail(w, err)
		return
	}
	i, o, err := h.d.Calendar.Get(r.Context(), h.actor(p), ref, r.PathValue("name"))
	if err != nil {
		h.fail(w, err)
		return
	}
	h.writeEvent(w, r, http.StatusOK, i, o)
}

// nameFor is the object name of a new event: the UID's random part.
func nameFor(uid string) string {
	return strings.SplitN(uid, "@", 2)[0] + ".ics"
}

func (h *handlers) createEvent(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	ref, err := h.ref(r)
	if err != nil {
		h.fail(w, err)
		return
	}
	var in ical.EventInput
	if !webx.DecodeJSON(w, r, &in) {
		return
	}
	data, uid, err := ical.New(in, h.family(), h.d.Clock())
	if err != nil {
		h.fail(w, err)
		return
	}
	o, _, err := h.d.Calendar.Put(r.Context(), h.actor(p), ref, nameFor(uid), data, calendar.Precondition{IfNoneMatch: true})
	if err != nil {
		h.fail(w, err)
		return
	}
	i, _ := h.d.Calendar.Info(r.Context(), h.actor(p), ref)
	h.writeEvent(w, r, http.StatusCreated, i, o)
}

// stale answers 412 with who changed the event since the form opened.
func (h *handlers) stale(w http.ResponseWriter, r *http.Request, o calendar.Object) {
	nm := &names{h: h, r: r, seen: map[int64]string{}}
	webx.JSON(w, http.StatusPreconditionFailed, map[string]string{"error": "precondition_failed",
		"message": "modifié entre-temps", "by": nm.of(o.ModifiedBy)})
}

// editTarget reads the object an edit is about and checks If-Match.
func (h *handlers) editTarget(w http.ResponseWriter, r *http.Request, p *auth.Principal) (calendar.Info, calendar.Object, string, bool) {
	ref, err := h.ref(r)
	if err != nil {
		h.fail(w, err)
		return calendar.Info{}, calendar.Object{}, "", false
	}
	ifMatch := r.Header.Get("If-Match")
	if ifMatch == "" {
		webx.Fail(w, http.StatusPreconditionRequired, "precondition_required", "If-Match is required")
		return calendar.Info{}, calendar.Object{}, "", false
	}
	i, o, err := h.d.Calendar.Get(r.Context(), h.actor(p), ref, r.PathValue("name"))
	if err != nil {
		h.fail(w, err)
		return calendar.Info{}, calendar.Object{}, "", false
	}
	if ifMatch != o.ETag && ifMatch != "*" {
		h.stale(w, r, o)
		return calendar.Info{}, calendar.Object{}, "", false
	}
	return i, o, ifMatch, true
}

func (h *handlers) editEvent(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	i, o, etag, ok := h.editTarget(w, r, p)
	if !ok {
		return
	}
	var in ical.EventInput
	if !webx.DecodeJSON(w, r, &in) {
		return
	}
	q := r.URL.Query()
	res, err := ical.Edit(o.Data, in, ical.Scope(q.Get("scope")), q.Get("occurrence"), i.Location(), h.family(), h.d.Clock())
	if err != nil {
		h.fail(w, err)
		return
	}
	ctx := r.Context()
	a := h.actor(p)
	saved, _, err := h.d.Calendar.Put(ctx, a, i.Ref, o.Name, res.Data, calendar.Precondition{IfMatch: []string{etag}})
	if errors.Is(err, calendar.ErrPrecondition) {
		h.stale(w, r, o)
		return
	}
	if err != nil {
		h.fail(w, err)
		return
	}
	extra := map[string]any{}
	if res.NewData != nil {
		v, err := ical.View(res.NewData, i.Location(), h.family())
		if err != nil {
			h.fail(w, err)
			return
		}
		n, _, err := h.d.Calendar.Put(ctx, a, i.Ref, nameFor(v.UID), res.NewData, calendar.Precondition{IfNoneMatch: true})
		if err != nil {
			h.fail(w, err)
			return
		}
		extra["created"] = n.Name
	}
	v, err := h.eventView(r, i, saved)
	if err != nil {
		h.fail(w, err)
		return
	}
	w.Header().Set("ETag", saved.ETag)
	if len(extra) == 0 {
		webx.JSON(w, http.StatusOK, v)
		return
	}
	b, _ := json.Marshal(v)
	merged := map[string]any{}
	_ = json.Unmarshal(b, &merged)
	merged["created"] = extra["created"]
	webx.JSON(w, http.StatusOK, merged)
}

func (h *handlers) deleteEvent(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	i, o, etag, ok := h.editTarget(w, r, p)
	if !ok {
		return
	}
	q := r.URL.Query()
	res, err := ical.Remove(o.Data, ical.Scope(q.Get("scope")), q.Get("occurrence"), i.Location(), h.family(), h.d.Clock())
	if err != nil {
		h.fail(w, err)
		return
	}
	pre := calendar.Precondition{IfMatch: []string{etag}}
	if res.Delete {
		err = h.d.Calendar.Delete(r.Context(), h.actor(p), i.Ref, o.Name, pre)
	} else {
		_, _, err = h.d.Calendar.Put(r.Context(), h.actor(p), i.Ref, o.Name, res.Data, pre)
	}
	if errors.Is(err, calendar.ErrPrecondition) {
		h.stale(w, r, o)
		return
	}
	if err != nil {
		h.fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *handlers) moveEvent(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	i, o, etag, ok := h.editTarget(w, r, p)
	if !ok {
		return
	}
	var body struct {
		To string `json:"to"`
	}
	if !webx.DecodeJSON(w, r, &body) {
		return
	}
	to, err := calendar.ParseRef(body.To)
	if err != nil {
		h.fail(w, err)
		return
	}
	moved, err := h.d.Calendar.Move(r.Context(), h.actor(p), i.Ref, o.Name, to, calendar.Precondition{IfMatch: []string{etag}})
	if err != nil {
		h.fail(w, err)
		return
	}
	dst, _ := h.d.Calendar.Info(r.Context(), h.actor(p), to)
	h.writeEvent(w, r, http.StatusOK, dst, moved)
}

// --- devices, links, sessions

func (h *handlers) devices(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	ds, err := h.d.Auth.Devices(r.Context(), p.User.ID)
	if err != nil {
		h.fail(w, err)
		return
	}
	webx.JSON(w, http.StatusOK, map[string]any{"devices": ds, "dav_auth": h.d.Auth.DavAuthMode(r.Context())})
}

// profiles keeps a new iPhone device's profile for ten minutes, behind a
// one-time token and its owner's session: the device password is in it and
// is never stored.
type profiles struct {
	mu   sync.Mutex
	byID map[string]profile
}

type profile struct {
	userID   int64
	deviceID int64
	kind     string // mobileconfig, p12
	body     []byte
	expires  time.Time
}

var pending = &profiles{byID: map[string]profile{}}

func (ps *profiles) put(token string, p profile, now time.Time) {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	for k, v := range ps.byID {
		if now.After(v.expires) {
			delete(ps.byID, k)
		}
	}
	p.expires = now.Add(10 * time.Minute)
	ps.byID[token] = p
}

func (ps *profiles) take(token, kind string, userID, deviceID int64, now time.Time) ([]byte, bool) {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	p, ok := ps.byID[token]
	if !ok || p.userID != userID || p.deviceID != deviceID || p.kind != kind {
		return nil, false // someone else's token stays usable by its owner
	}
	delete(ps.byID, token)
	if now.After(p.expires) {
		return nil, false
	}
	return p.body, true
}

func (h *handlers) addDevice(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	var body struct {
		Kind  string `json:"kind"`
		Label string `json:"label"`
		Cap   string `json:"cap"`
	}
	if !webx.DecodeJSON(w, r, &body) {
		return
	}
	if body.Cap == "" {
		body.Cap = "readwrite"
	}
	base := h.d.PublicURL()
	d, err := h.d.Auth.NewDevice(r.Context(), p.User.ID, body.Kind, body.Label, body.Cap)
	if err != nil {
		if errors.Is(err, auth.ErrTooMany) || errors.Is(err, auth.ErrInvalid) {
			h.fail(w, err)
			return
		}
		h.fail(w, badRequest{err.Error()})
		return
	}
	pub, _ := url.Parse(base)
	out := map[string]any{"id": d.ID, "kind": d.Kind, "label": d.Label, "cap": d.Cap, "username": d.Username, "secret": d.Secret,
		"manual": map[string]string{"server": base + "/dav/", "username": d.Username}}
	if d.Kind == "davx5" || d.Kind == "app" {
		link := "davx5://" + url.QueryEscape(d.Username) + ":" + url.QueryEscape(d.Secret) + "@" + pub.Host + "/dav/"
		out["davx5_uri"] = link
		if qr, err := webx.QRSVG(link); err == nil {
			out["qr_svg"] = qr
		}
	}
	if d.Kind == "ios" {
		token := newProfileToken()
		pending.put(token, profile{userID: p.User.ID, deviceID: d.ID, kind: "mobileconfig", body: mobileconfig(pub.Host, d.Username, d.Secret, d.Label, d.ID)}, h.d.Clock())
		out["mobileconfig_url"] = fmt.Sprintf("/api/devices/%d/mobileconfig?t=%s", d.ID, token)
	}
	// When CalDAV wants a certificate (DECISIONS: device keys), the device
	// gets one now: a .p12 to download once, and its password.
	mode := h.d.Auth.DavAuthMode(r.Context())
	out["dav_auth"] = mode
	if mode != auth.DavPassword {
		cert, err := h.certificateFor(r, p, d.ID)
		if err != nil {
			h.fail(w, err)
			return
		}
		for k, v := range cert {
			out[k] = v
		}
	}
	webx.JSON(w, http.StatusCreated, out)
}

// certificateFor issues a device's certificate and keeps its .p12 ten
// minutes behind a one-time link: the private key is never stored.
func (h *handlers) certificateFor(r *http.Request, p *auth.Principal, deviceID int64) (map[string]any, error) {
	c, err := h.d.Auth.IssueCertificate(r.Context(), p.User.ID, deviceID)
	if err != nil {
		return nil, err
	}
	token := newProfileToken()
	pending.put(token, profile{userID: p.User.ID, deviceID: deviceID, kind: "p12", body: c.P12}, h.d.Clock())
	return map[string]any{
		"certificate_url":        fmt.Sprintf("/api/devices/%d/certificate.p12?t=%s", deviceID, token),
		"certificate_password":   c.Password,
		"certificate_expires_at": c.ExpiresAt,
	}, nil
}

// newCertificate gives an existing device a new certificate (the mode
// changed, or the file was lost); the one it had stops working.
func (h *handlers) newCertificate(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		h.fail(w, auth.ErrInvalid)
		return
	}
	out, err := h.certificateFor(r, p, id)
	if err != nil {
		h.fail(w, err)
		return
	}
	out["dav_auth"] = h.d.Auth.DavAuthMode(r.Context())
	webx.JSON(w, http.StatusCreated, out)
}

func (h *handlers) certificateFile(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	body, ok := pending.take(r.URL.Query().Get("t"), "p12", p.User.ID, id, h.d.Clock())
	if !ok {
		http.Error(w, "ce lien a servi ou a expiré : demandez un nouveau certificat", http.StatusGone)
		return
	}
	w.Header().Set("Content-Type", "application/x-pkcs12")
	w.Header().Set("Content-Disposition", `attachment; filename="agendouille-appareil.p12"`)
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(body)
}

func (h *handlers) mobileconfig(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	body, ok := pending.take(r.URL.Query().Get("t"), "mobileconfig", p.User.ID, id, h.d.Clock())
	if !ok {
		http.Error(w, "this profile link has been used or has expired; add the device again", http.StatusGone)
		return
	}
	w.Header().Set("Content-Type", "application/x-apple-aspen-config")
	w.Header().Set("Content-Disposition", `attachment; filename="agendouille.mobileconfig"`)
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(body)
}

func (h *handlers) revokeDevice(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		h.fail(w, auth.ErrInvalid)
		return
	}
	if err := h.d.Auth.RevokeDevice(r.Context(), p.User.Name, p.User.ID, id); err != nil {
		h.fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type linkView struct {
	ID         int64      `json:"id"`
	Label      string     `json:"label"`
	Calendars  []string   `json:"calendars"`
	CreatedAt  time.Time  `json:"created_at"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
	URL        string     `json:"url,omitempty"`
}

func (h *handlers) links(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	ls, err := h.d.Auth.Links(r.Context(), p.User.ID)
	if err != nil {
		h.fail(w, err)
		return
	}
	out := []linkView{}
	a := h.actor(p)
	for _, l := range ls {
		v := linkView{ID: l.ID, Label: l.Label, Calendars: []string{}, CreatedAt: l.CreatedAt, LastUsedAt: l.LastUsedAt}
		for _, id := range l.CalendarIDs {
			if i, err := h.d.Calendar.InfoByID(r.Context(), a, id); err == nil {
				v.Calendars = append(v.Calendars, i.Ref.String())
			}
		}
		out = append(out, v)
	}
	webx.JSON(w, http.StatusOK, map[string]any{"links": out})
}

func (h *handlers) addLink(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	var body struct {
		Label     string   `json:"label"`
		Calendars []string `json:"calendars"`
	}
	if !webx.DecodeJSON(w, r, &body) {
		return
	}
	var ids []int64
	var refs []string
	for _, c := range body.Calendars {
		ref, err := calendar.ParseRef(c)
		if err != nil {
			h.fail(w, err)
			return
		}
		i, err := h.d.Calendar.Info(r.Context(), h.actor(p), ref)
		if err != nil {
			h.fail(w, err)
			return
		}
		ids = append(ids, i.ID)
		refs = append(refs, i.Ref.String())
	}
	l, secret, err := h.d.Auth.NewLink(r.Context(), p.User.ID, body.Label, ids)
	if err != nil {
		h.fail(w, badRequest{err.Error()})
		return
	}
	webx.JSON(w, http.StatusCreated, linkView{ID: l.ID, Label: l.Label, Calendars: refs, CreatedAt: l.CreatedAt,
		URL: h.d.PublicURL() + "/ics/" + secret + ".ics"})
}

func (h *handlers) revokeLink(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		h.fail(w, auth.ErrInvalid)
		return
	}
	if err := h.d.Auth.RevokeLink(r.Context(), p.User.ID, id); err != nil {
		h.fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *handlers) sessions(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	ss, err := h.d.Auth.Sessions(r.Context(), p.User.ID)
	if err != nil {
		h.fail(w, err)
		return
	}
	type view struct {
		auth.Session
		Current bool `json:"current"`
	}
	out := []view{}
	for _, s := range ss {
		out = append(out, view{Session: s, Current: s.ID == p.SessionID})
	}
	webx.JSON(w, http.StatusOK, map[string]any{"sessions": out})
}

func (h *handlers) revokeSession(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		h.fail(w, auth.ErrInvalid)
		return
	}
	if err := h.d.Auth.RevokeSession(r.Context(), p.User.ID, id); err != nil {
		h.fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
