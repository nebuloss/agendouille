package admin

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/nebuloss/agendouille/internal/auth"
	"github.com/nebuloss/agendouille/internal/backup"
	"github.com/nebuloss/agendouille/internal/calendar"
	"github.com/nebuloss/agendouille/internal/directory"
	"github.com/nebuloss/agendouille/internal/settings"
	"github.com/nebuloss/agendouille/internal/sharing"
	"github.com/nebuloss/agendouille/internal/store"
	"github.com/nebuloss/agendouille/internal/web/webx"
)

// The admin API (CONTRACTS.md 3.5). Every answer is metadata: a test walks
// each route and checks no event's text, title or place is in it.

func (h *handlers) registerAPI(mux *http.ServeMux, guard func(http.Handler) http.Handler) {
	api := func(pattern string, f http.HandlerFunc) {
		var handler http.Handler = h.apiAuth(f)
		if !strings.HasPrefix(pattern, "GET ") {
			handler = guard(handler)
		}
		mux.Handle(pattern, handler)
	}
	api("GET /api/admin/overview", h.overview)
	api("GET /api/admin/users", h.users)
	api("POST /api/admin/users", h.addUser)
	api("GET /api/admin/users/{name}", h.user)
	api("PATCH /api/admin/users/{name}", h.patchUser)
	api("POST /api/admin/users/{name}/remove", h.removeUser)
	api("POST /api/admin/users/{name}/enrolments", h.addEnrolment)
	api("DELETE /api/admin/users/{name}/enrolments/{id}", h.revokeEnrolment)
	api("DELETE /api/admin/users/{name}/devices/{id}", h.revokeDevice)
	api("DELETE /api/admin/users/{name}/sessions/{id}", h.revokeSession)
	api("GET /api/admin/groups", h.groups)
	api("POST /api/admin/groups", h.addGroup)
	api("PATCH /api/admin/groups/{name}", h.patchGroup)
	api("DELETE /api/admin/groups/{name}", h.deleteGroup)
	api("GET /api/admin/calendars", h.calendars)
	api("DELETE /api/admin/calendars/{ref}/shares/{target}", h.unshare)
	api("POST /api/admin/calendars/{ref}/transfer", h.transfer)
	api("DELETE /api/admin/calendars/{ref}", h.deleteCalendar)
	api("GET /api/admin/activity", h.activity)
	api("GET /api/admin/settings", h.settings)
	api("PATCH /api/admin/settings", h.patchSettings)
	api("GET /api/admin/backups", h.backups)
	api("POST /api/admin/backups", h.makeBackup)
	api("GET /api/admin/backups/{file}", h.downloadBackup)
	api("POST /api/admin/backups/{file}/restore", h.restore)
	api("GET /api/admin/system", h.system)
}

func (h *handlers) apiAuth(next http.HandlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, err := h.d.Auth.Session(r, auth.SurfaceAdmin)
		if err != nil || !p.Admin {
			webx.Fail(w, http.StatusUnauthorized, "unauthenticated", "connectez-vous à l'administration")
			return
		}
		next(w, r.WithContext(auth.WithPrincipal(r.Context(), p)))
	})
}

// fail maps the services' errors onto CONTRACTS.md 3.1.
func (h *handlers) fail(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, directory.ErrNotFound), errors.Is(err, calendar.ErrNotFound), errors.Is(err, sharing.ErrNotFound),
		errors.Is(err, backup.ErrNotFound), errors.Is(err, auth.ErrInvalid):
		webx.Fail(w, http.StatusNotFound, "not_found", err.Error())
	case errors.Is(err, directory.ErrNameTaken), errors.Is(err, calendar.ErrExists):
		webx.Fail(w, http.StatusConflict, "name_taken", err.Error())
	case errors.Is(err, directory.ErrBadName), errors.Is(err, calendar.ErrBadSlug), errors.Is(err, settings.ErrUnknownKey):
		webx.Fail(w, http.StatusBadRequest, "bad_request", err.Error())
	case errors.Is(err, calendar.ErrQuota), errors.Is(err, auth.ErrTooMany):
		webx.Fail(w, http.StatusInsufficientStorage, "quota", err.Error())
	default:
		var verr validation
		if errors.As(err, &verr) {
			webx.Fail(w, http.StatusBadRequest, "bad_request", err.Error())
			return
		}
		h.d.Log.Error("admin api", "err", err)
		webx.Fail(w, http.StatusInternalServerError, "error", "")
	}
}

type validation struct{ msg string }

func (v validation) Error() string { return v.msg }

func decode(w http.ResponseWriter, r *http.Request, v any) bool { return webx.DecodeJSON(w, r, v) }

func pathID(r *http.Request, name string) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue(name), 10, 64)
	return id, err == nil
}

// --- overview, system

type warning struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (h *handlers) warnings(r *http.Request) []warning {
	out := []warning{}
	set := h.d.Settings.Get()
	if set.PublicListen == "" {
		out = append(out, warning{"no_public_listener", "CalDAV et l'application familiale sont arrêtés : renseignez public_listen et public_url dans les réglages."})
	} else if h.d.PublicStatus != nil {
		if addr, err := h.d.PublicStatus(); err != "" {
			out = append(out, warning{"public_listener_down", "L'accès public n'a pas pu démarrer sur " + set.PublicListen + " : " + err})
		} else if addr == "" {
			out = append(out, warning{"public_listener_down", "L'accès public ne tourne pas."})
		}
	}
	if strings.HasPrefix(h.d.Config.AdminURL(), "http://") && !strings.HasPrefix(h.d.Config.AdminListen, "127.") &&
		!strings.HasPrefix(h.d.Config.AdminListen, "localhost") {
		out = append(out, warning{"admin_insecure", "L'administration est servie en http hors de cette machine : gardez-la sur le réseau local."})
	}
	if h.d.Backup != nil {
		if last := h.d.Backup.Last(); last.IsZero() || time.Since(last) > 48*time.Hour {
			out = append(out, warning{"backup_old", "Aucune sauvegarde depuis deux jours."})
		}
	}
	return out
}

func dirSize(path string) int64 {
	var n int64
	entries, _ := os.ReadDir(path)
	for _, e := range entries {
		if info, err := e.Info(); err == nil && !e.IsDir() {
			n += info.Size()
		}
	}
	return n
}

func (h *handlers) overview(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	users, err := h.d.Directory.Users(ctx)
	if err != nil {
		h.fail(w, err)
		return
	}
	groups, err := h.d.Directory.Groups(ctx)
	if err != nil {
		h.fail(w, err)
		return
	}
	counts, err := h.d.Calendar.AdminCounts(ctx)
	if err != nil {
		h.fail(w, err)
		return
	}
	devices, err := h.d.Auth.CountDevices(ctx)
	if err != nil {
		h.fail(w, err)
		return
	}
	stale, err := h.d.Auth.StaleDevices(ctx)
	if err != nil {
		h.fail(w, err)
		return
	}
	var last *time.Time
	if h.d.Backup != nil {
		if t := h.d.Backup.Last(); !t.IsZero() {
			last = &t
		}
	}
	dbSize := int64(0)
	if info, err := os.Stat(h.d.Store.Path()); err == nil {
		dbSize = info.Size()
	}
	webx.JSON(w, http.StatusOK, map[string]any{
		"users": len(users), "groups": len(groups), "calendars": counts.Calendars, "events": counts.Events,
		"devices": devices.Devices, "stale_devices": stale,
		"storage":     map[string]int64{"database": dbSize, "backups": dirSize(h.d.Config.DataDir + "/" + backup.Dir)},
		"last_backup": last, "version": h.d.Version, "public_url": h.d.PublicURL(), "warnings": h.warnings(r),
	})
}

func (h *handlers) system(w http.ResponseWriter, _ *http.Request) {
	addr, perr := "", ""
	if h.d.PublicStatus != nil {
		addr, perr = h.d.PublicStatus()
	}
	set := h.d.Settings.Get()
	webx.JSON(w, http.StatusOK, map[string]any{
		"version": h.d.Version, "data_dir": h.d.Config.DataDir,
		"listeners": []map[string]any{
			{"surfaces": []string{"admin"}, "listen": h.d.Config.AdminListen, "public_url": h.d.Config.AdminURL(), "from": "bootstrap file"},
			{"surfaces": []string{"dav", "app"}, "listen": set.PublicListen, "public_url": set.PublicURL, "bound": addr, "error": perr, "from": "settings"},
		},
	})
}

// --- users

type userView struct {
	directory.User
	Groups  []string `json:"groups"`
	Devices int      `json:"devices"`
	Owned   int      `json:"calendars_owned"`
}

func (h *handlers) users(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	users, err := h.d.Directory.Users(ctx)
	if err != nil {
		h.fail(w, err)
		return
	}
	out := make([]userView, 0, len(users))
	for _, u := range users {
		v := userView{User: u, Groups: []string{}}
		gs, err := h.d.Directory.GroupsOf(ctx, u.ID)
		if err != nil {
			h.fail(w, err)
			return
		}
		for _, g := range gs {
			v.Groups = append(v.Groups, g.Name)
		}
		ds, err := h.d.Auth.Devices(ctx, u.ID)
		if err != nil {
			h.fail(w, err)
			return
		}
		v.Devices = len(ds)
		owned, err := h.d.Calendar.Owned(ctx, u.ID)
		if err != nil {
			h.fail(w, err)
			return
		}
		v.Owned = len(owned)
		out = append(out, v)
	}
	webx.JSON(w, http.StatusOK, map[string]any{"users": out})
}

// enrolmentView is an enrolment with its link and QR code, as made.
func (h *handlers) enrolmentView(e auth.Enrolment) (map[string]any, error) {
	base := h.d.PublicURL()
	if base == "" {
		return nil, validation{"public_url n'est pas renseignée : il n'y a pas d'adresse pour le lien de connexion. Renseignez-la dans les réglages."}
	}
	link := base + "/enrol/" + e.Token
	qr, err := webx.QRSVG(link)
	if err != nil {
		return nil, err
	}
	return map[string]any{"id": e.ID, "url": link, "qr_svg": qr, "created_at": e.CreatedAt, "expires_at": e.ExpiresAt}, nil
}

func (h *handlers) addUser(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name        string   `json:"name"`
		DisplayName string   `json:"display_name"`
		Color       string   `json:"color"`
		Groups      []string `json:"groups"`
		Enrol       bool     `json:"enrol"`
	}
	if !decode(w, r, &body) {
		return
	}
	u, err := h.d.Directory.AddUser(r.Context(), "admin", body.Name, body.DisplayName, body.Color, body.Groups)
	if err != nil {
		h.fail(w, err)
		return
	}
	out := map[string]any{"user": u}
	if body.Enrol && h.d.PublicURL() != "" {
		e, err := h.d.Auth.NewEnrolment(r.Context(), "admin", u.ID, 0)
		if err != nil {
			h.fail(w, err)
			return
		}
		if out["enrolment"], err = h.enrolmentView(e); err != nil {
			h.fail(w, err)
			return
		}
	}
	webx.JSON(w, http.StatusCreated, out)
}

func (h *handlers) lookupUser(w http.ResponseWriter, r *http.Request) (directory.User, bool) {
	u, err := h.d.Directory.User(r.Context(), r.PathValue("name"))
	if err != nil || u.RemovedAt != nil {
		webx.Fail(w, http.StatusNotFound, "not_found", "no such user")
		return directory.User{}, false
	}
	return u, true
}

func (h *handlers) user(w http.ResponseWriter, r *http.Request) {
	u, ok := h.lookupUser(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	gs, err := h.d.Directory.GroupsOf(ctx, u.ID)
	if err != nil {
		h.fail(w, err)
		return
	}
	groups := []string{}
	for _, g := range gs {
		groups = append(groups, g.Name)
	}
	enrolments, err := h.d.Auth.Enrolments(ctx, u.ID)
	if err != nil {
		h.fail(w, err)
		return
	}
	devices, err := h.d.Auth.Devices(ctx, u.ID)
	if err != nil {
		h.fail(w, err)
		return
	}
	sessions, err := h.d.Auth.Sessions(ctx, u.ID)
	if err != nil {
		h.fail(w, err)
		return
	}
	owned, err := h.d.Calendar.AdminCalendars(ctx, u.Name)
	if err != nil {
		h.fail(w, err)
		return
	}
	shared, err := h.d.Calendar.AdminSharedWith(ctx, u.ID)
	if err != nil {
		h.fail(w, err)
		return
	}
	webx.JSON(w, http.StatusOK, map[string]any{"user": u, "groups": groups, "enrolments": enrolments, "devices": devices,
		"sessions": sessions, "calendars_owned": owned, "calendars_shared": shared})
}

func (h *handlers) patchUser(w http.ResponseWriter, r *http.Request) {
	u, ok := h.lookupUser(w, r)
	if !ok {
		return
	}
	var body struct {
		DisplayName *string   `json:"display_name"`
		Color       *string   `json:"color"`
		Groups      *[]string `json:"groups"`
		Disabled    *bool     `json:"disabled"`
	}
	if !decode(w, r, &body) {
		return
	}
	ctx := r.Context()
	d, c := "", ""
	if body.DisplayName != nil {
		d = *body.DisplayName
	}
	if body.Color != nil {
		c = *body.Color
	}
	if err := h.d.Directory.UpdateUser(ctx, "admin", u.Name, d, c); err != nil {
		h.fail(w, err)
		return
	}
	if body.Groups != nil {
		if err := h.d.Directory.SetGroups(ctx, "admin", u.Name, *body.Groups); err != nil {
			h.fail(w, err)
			return
		}
	}
	if body.Disabled != nil {
		if err := h.d.Directory.SetDisabled(ctx, "admin", u.Name, *body.Disabled); err != nil {
			h.fail(w, err)
			return
		}
	}
	h.user(w, r)
}

// removeUser takes, for each calendar the user owns, either a user to
// transfer it to or a deletion (PLAN.md 6): there is no orphan calendar.
func (h *handlers) removeUser(w http.ResponseWriter, r *http.Request) {
	u, ok := h.lookupUser(w, r)
	if !ok {
		return
	}
	var body struct {
		Calendars map[string]struct {
			TransferTo string `json:"transfer_to"`
			Delete     bool   `json:"delete"`
		} `json:"calendars"`
	}
	if !decode(w, r, &body) {
		return
	}
	ctx := r.Context()
	owned, err := h.d.Calendar.Owned(ctx, u.ID)
	if err != nil {
		h.fail(w, err)
		return
	}
	for _, i := range owned {
		choice, ok := body.Calendars[i.Ref.String()]
		if !ok || (choice.TransferTo == "") == !choice.Delete {
			webx.JSON(w, http.StatusConflict, map[string]any{"error": "owns_calendars",
				"message": "dites, pour chaque calendrier, à qui le donner ou qu'il est supprimé", "calendars": refsOf(owned)})
			return
		}
	}
	op := calendar.Operator("admin")
	for _, i := range owned {
		choice := body.Calendars[i.Ref.String()]
		if choice.Delete {
			err = h.d.Calendar.DeleteCalendar(ctx, op, i.Ref)
		} else {
			_, err = h.d.Calendar.Transfer(ctx, op, i.Ref, choice.TransferTo, false)
		}
		if err != nil {
			h.fail(w, err)
			return
		}
	}
	if err := h.d.Directory.Remove(ctx, "admin", u.Name); err != nil {
		h.fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func refsOf(is []calendar.Info) []string {
	out := []string{}
	for _, i := range is {
		out = append(out, i.Ref.String())
	}
	return out
}

func (h *handlers) addEnrolment(w http.ResponseWriter, r *http.Request) {
	u, ok := h.lookupUser(w, r)
	if !ok {
		return
	}
	if h.d.PublicURL() == "" {
		h.fail(w, validation{"public_url n'est pas renseignée : il n'y a pas d'adresse pour le lien de connexion. Renseignez-la dans les réglages."})
		return
	}
	e, err := h.d.Auth.NewEnrolment(r.Context(), "admin", u.ID, 0)
	if err != nil {
		h.fail(w, err)
		return
	}
	view, err := h.enrolmentView(e)
	if err != nil {
		h.fail(w, err)
		return
	}
	webx.JSON(w, http.StatusCreated, view)
}

func (h *handlers) revokeEnrolment(w http.ResponseWriter, r *http.Request) {
	u, ok := h.lookupUser(w, r)
	id, idOK := pathID(r, "id")
	if !ok || !idOK {
		return
	}
	if err := h.d.Auth.RevokeEnrolment(r.Context(), u.ID, id); err != nil {
		h.fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *handlers) revokeDevice(w http.ResponseWriter, r *http.Request) {
	u, ok := h.lookupUser(w, r)
	id, idOK := pathID(r, "id")
	if !ok || !idOK {
		return
	}
	if err := h.d.Auth.RevokeDevice(r.Context(), "admin", u.ID, id); err != nil {
		h.fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *handlers) revokeSession(w http.ResponseWriter, r *http.Request) {
	u, ok := h.lookupUser(w, r)
	id, idOK := pathID(r, "id")
	if !ok || !idOK {
		return
	}
	if err := h.d.Auth.RevokeSession(r.Context(), u.ID, id); err != nil {
		h.fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- groups

func (h *handlers) groups(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	gs, err := h.d.Directory.Groups(ctx)
	if err != nil {
		h.fail(w, err)
		return
	}
	type view struct {
		directory.Group
		Members []string `json:"members"`
		Shares  int      `json:"shares"`
	}
	out := make([]view, 0, len(gs))
	for _, g := range gs {
		ms, err := h.d.Directory.Members(ctx, g.Name)
		if err != nil {
			h.fail(w, err)
			return
		}
		v := view{Group: g, Members: []string{}}
		for _, m := range ms {
			v.Members = append(v.Members, m.Name)
		}
		if v.Shares, err = h.d.Directory.GroupShares(ctx, g.Name); err != nil {
			h.fail(w, err)
			return
		}
		out = append(out, v)
	}
	webx.JSON(w, http.StatusOK, map[string]any{"groups": out})
}

func (h *handlers) addGroup(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name        string `json:"name"`
		DisplayName string `json:"display_name"`
		Color       string `json:"color"`
	}
	if !decode(w, r, &body) {
		return
	}
	g, err := h.d.Directory.AddGroup(r.Context(), "admin", body.Name, body.DisplayName, body.Color)
	if err != nil {
		h.fail(w, err)
		return
	}
	webx.JSON(w, http.StatusCreated, g)
}

func (h *handlers) patchGroup(w http.ResponseWriter, r *http.Request) {
	var body struct {
		DisplayName string    `json:"display_name"`
		Color       string    `json:"color"`
		Members     *[]string `json:"members"`
	}
	if !decode(w, r, &body) {
		return
	}
	ctx := r.Context()
	name := r.PathValue("name")
	if err := h.d.Directory.UpdateGroup(ctx, "admin", name, body.DisplayName, body.Color); err != nil {
		h.fail(w, err)
		return
	}
	if body.Members != nil {
		current, err := h.d.Directory.Members(ctx, name)
		if err != nil {
			h.fail(w, err)
			return
		}
		want := map[string]bool{}
		for _, m := range *body.Members {
			want[m] = true
		}
		for _, m := range current {
			if !want[m.Name] {
				if err := h.d.Directory.RemoveMember(ctx, "admin", name, m.Name); err != nil {
					h.fail(w, err)
					return
				}
			}
			delete(want, m.Name)
		}
		for m := range want {
			if err := h.d.Directory.AddMember(ctx, "admin", name, m); err != nil {
				h.fail(w, err)
				return
			}
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *handlers) deleteGroup(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	name := r.PathValue("name")
	n, err := h.d.Directory.GroupShares(ctx, name)
	if err != nil {
		h.fail(w, err)
		return
	}
	if n > 0 && r.URL.Query().Get("confirm") != "1" {
		webx.JSON(w, http.StatusConflict, map[string]any{"error": "has_shares", "shares": n,
			"message": "des calendriers sont partagés avec ce groupe ; le supprimer retire ces partages (confirm=1)"})
		return
	}
	if err := h.d.Directory.RemoveGroup(ctx, "admin", name); err != nil {
		h.fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- calendars (metadata only)

func (h *handlers) calendars(w http.ResponseWriter, r *http.Request) {
	cs, err := h.d.Calendar.AdminCalendars(r.Context(), r.URL.Query().Get("owner"))
	if err != nil {
		h.fail(w, err)
		return
	}
	webx.JSON(w, http.StatusOK, map[string]any{"calendars": cs})
}

func (h *handlers) unshare(w http.ResponseWriter, r *http.Request) {
	ref, err := calendar.ParseRef(r.PathValue("ref"))
	if err != nil {
		h.fail(w, err)
		return
	}
	t, err := sharing.ParseTarget(r.PathValue("target"))
	if err != nil {
		h.fail(w, validation{err.Error()})
		return
	}
	i, err := h.d.Calendar.Info(r.Context(), calendar.Operator("admin"), ref)
	if err != nil {
		h.fail(w, err)
		return
	}
	if err := h.d.Calendar.Sharing().Unshare(r.Context(), "admin", i.ID, t, store.Time(h.d.Clock())); err != nil {
		h.fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *handlers) transfer(w http.ResponseWriter, r *http.Request) {
	ref, err := calendar.ParseRef(r.PathValue("ref"))
	if err != nil {
		h.fail(w, err)
		return
	}
	var body struct {
		To   string `json:"to"`
		Keep bool   `json:"keep"`
	}
	if !decode(w, r, &body) {
		return
	}
	i, err := h.d.Calendar.Transfer(r.Context(), calendar.Operator("admin"), ref, body.To, body.Keep)
	if err != nil {
		h.fail(w, err)
		return
	}
	webx.JSON(w, http.StatusOK, map[string]string{"ref": i.Ref.String()})
}

func (h *handlers) deleteCalendar(w http.ResponseWriter, r *http.Request) {
	ref, err := calendar.ParseRef(r.PathValue("ref"))
	if err != nil {
		h.fail(w, err)
		return
	}
	op := calendar.Operator("admin")
	i, err := h.d.Calendar.Info(r.Context(), op, ref)
	if err != nil {
		h.fail(w, err)
		return
	}
	// The admin types the calendar's name: deleting is for everyone, and
	// there is no undo but a backup.
	if r.URL.Query().Get("confirm") != i.Name {
		h.fail(w, validation{"tapez le nom du calendrier pour confirmer"})
		return
	}
	if err := h.d.Calendar.DeleteCalendar(r.Context(), op, ref); err != nil {
		h.fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- activity

func (h *handlers) activity(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	f := calendar.ActivityFilter{User: q.Get("user"), Calendar: q.Get("calendar"), Kind: q.Get("kind")}
	if b := q.Get("before"); b != "" {
		t, err := time.Parse(time.RFC3339Nano, b)
		if err != nil {
			h.fail(w, validation{"before is an RFC 3339 time"})
			return
		}
		f.Before = t
	}
	items, err := h.d.Calendar.AdminActivity(r.Context(), f)
	if err != nil {
		h.fail(w, err)
		return
	}
	webx.JSON(w, http.StatusOK, map[string]any{"activity": items})
}

// --- settings

func (h *handlers) settings(w http.ResponseWriter, _ *http.Request) {
	type item struct {
		settings.Key
		Value json.RawMessage `json:"value"`
	}
	out := []item{}
	for _, k := range settings.Keys() {
		v, err := h.d.Settings.Value(k.Name)
		if err != nil {
			h.fail(w, err)
			return
		}
		out = append(out, item{Key: k, Value: json.RawMessage(v)})
	}
	webx.JSON(w, http.StatusOK, map[string]any{"settings": out})
}

// patchSettings takes {"key": value, ...} with JSON values, applied
// together (rules between keys hold) and live.
func (h *handlers) patchSettings(w http.ResponseWriter, r *http.Request) {
	var body map[string]json.RawMessage
	if !decode(w, r, &body) {
		return
	}
	changes := map[string]string{}
	for k, v := range body {
		changes[k] = string(v)
	}
	if _, err := h.d.Settings.Set(r.Context(), "admin", changes); err != nil {
		if errors.Is(err, settings.ErrUnknownKey) {
			h.fail(w, err)
			return
		}
		h.fail(w, validation{err.Error()})
		return
	}
	h.settings(w, r)
}

// --- backups

func (h *handlers) backups(w http.ResponseWriter, _ *http.Request) {
	files, err := h.d.Backup.List()
	if err != nil {
		h.fail(w, err)
		return
	}
	webx.JSON(w, http.StatusOK, map[string]any{"backups": files})
}

func (h *handlers) makeBackup(w http.ResponseWriter, r *http.Request) {
	f, err := h.d.Backup.Make(r.Context())
	if err != nil {
		h.fail(w, err)
		return
	}
	webx.JSON(w, http.StatusCreated, f)
}

func (h *handlers) downloadBackup(w http.ResponseWriter, r *http.Request) {
	path, err := h.d.Backup.Path(r.PathValue("file"))
	if err != nil {
		h.fail(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/vnd.sqlite3")
	w.Header().Set("Content-Disposition", `attachment; filename="`+r.PathValue("file")+`"`)
	w.Header().Set("Cache-Control", "no-store")
	http.ServeFile(w, r, path)
}

// restore stages a backup and restarts the server, which swaps the files
// before it opens the database (package backup).
func (h *handlers) restore(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Confirm string `json:"confirm"`
	}
	if !decode(w, r, &body) {
		return
	}
	name := r.PathValue("file")
	if body.Confirm != name {
		h.fail(w, validation{"confirmez avec le nom du fichier de la sauvegarde"})
		return
	}
	if err := h.d.Backup.Stage(name); err != nil {
		if errors.Is(err, backup.ErrNotFound) {
			h.fail(w, err)
			return
		}
		h.fail(w, validation{err.Error()})
		return
	}
	webx.JSON(w, http.StatusAccepted, map[string]string{"status": "restarting",
		"message": "Le serveur redémarre avec cette sauvegarde ; reconnectez-vous dans quelques secondes."})
	if h.d.Restart != nil {
		go h.d.Restart("restore " + name)
	}
}
