package admin

import (
	"archive/zip"
	"encoding/csv"
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
	"github.com/nebuloss/agendouille/internal/config"
	"github.com/nebuloss/agendouille/internal/directory"
	"github.com/nebuloss/agendouille/internal/hub"
	"github.com/nebuloss/agendouille/internal/i18n"
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
	api("POST /api/admin/users/{name}/enrollments", h.addEnrollment)
	api("GET /api/admin/users/{name}/enrollments/{id}", h.showEnrollment)
	api("DELETE /api/admin/users/{name}/enrollments/{id}", h.revokeEnrollment)
	api("POST /api/admin/users/{name}/devices", h.addDevice)
	api("PUT /api/admin/users/{name}/dav-auth", h.userDavAuth)
	api("DELETE /api/admin/users/{name}/devices/{id}", h.revokeDevice)
	api("POST /api/admin/users/{name}/devices/{id}/certificate", h.newCertificate)
	api("GET /api/admin/users/{name}/devices/{id}/certificate.p12", h.certificateFile)
	api("DELETE /api/admin/users/{name}/sessions/{id}", h.revokeSession)
	api("GET /api/admin/groups", h.groups)
	api("POST /api/admin/groups", h.addGroup)
	api("PATCH /api/admin/groups/{name}", h.patchGroup)
	api("DELETE /api/admin/groups/{name}", h.deleteGroup)
	api("GET /api/admin/calendars", h.calendars)
	api("GET /api/admin/calendars/{ref}/stats", h.calendarStats)
	api("GET /api/admin/calendars/{ref}/export", h.exportCalendar)
	api("GET /api/admin/calendars/export.zip", h.exportCalendars)
	api("PUT /api/admin/calendars/{ref}/shares/{target}", h.share)
	api("DELETE /api/admin/calendars/{ref}/shares/{target}", h.unshare)
	api("POST /api/admin/calendars/{ref}/transfer", h.transfer)
	api("DELETE /api/admin/calendars/{ref}", h.deleteCalendar)
	api("GET /api/admin/activity", h.activity)
	api("GET /api/admin/activity/export", h.exportActivity)
	api("GET /api/admin/settings", h.settings)
	api("GET /api/admin/backups", h.backups)
	api("POST /api/admin/backups", h.makeBackup)
	api("GET /api/admin/backups/{file}", h.downloadBackup)
	api("POST /api/admin/backups/{file}/restore", h.restore)
	api("GET /api/admin/system", h.system)
	api("GET /api/admin/dav-auth", h.davAuth)
	api("PUT /api/admin/dav-auth", h.setDavAuth)
	api("GET /api/admin/notices", h.notices)
	api("POST /api/admin/notices/read", h.readNotices)
}

func (h *handlers) apiAuth(next http.HandlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, err := h.d.Auth.Session(r, auth.SurfaceAdmin)
		if err != nil || !p.Admin {
			webx.FailT(w, r, http.StatusUnauthorized, "unauthenticated", "error.admin.signinRequired")
			return
		}
		next(w, r.WithContext(auth.WithPrincipal(r.Context(), p)))
	})
}

// fail is failIn in the default language, for a caller without the
// request.
func (h *handlers) fail(w http.ResponseWriter, err error) { h.failIn(w, nil, err) }

// failIn maps the services' errors onto CONTRACTS.md 3.1. The code is for
// programs and never changes; the message is for the administrator, in the
// request's language (internal/i18n, the error. keys). The services' own
// texts stay in the log and the CLI.
func (h *handlers) failIn(w http.ResponseWriter, r *http.Request, err error) {
	status, code, key, args := h.explain(err)
	if status == http.StatusInternalServerError {
		h.d.Log.Error("admin api", "err", err)
	}
	lang := i18n.Default
	if r != nil {
		lang = i18n.FromRequest(r)
	}
	webx.Fail(w, status, code, i18n.T(lang, key, args...))
}

func (h *handlers) explain(err error) (status int, code, key string, args []string) {
	var verr validation
	var bad directory.Invalid
	var quota *calendar.QuotaError
	switch {
	case errors.As(err, &verr):
		return http.StatusBadRequest, "bad_request", verr.key, verr.args
	case errors.Is(err, backup.ErrNotFound):
		return http.StatusNotFound, "not_found", "error.backup.notFound", nil
	case errors.Is(err, directory.ErrNotFound), errors.Is(err, sharing.ErrNotFound):
		return http.StatusNotFound, "not_found", "error.admin.nobody", nil
	case errors.Is(err, calendar.ErrNotFound), errors.Is(err, auth.ErrInvalid):
		return http.StatusNotFound, "not_found", "error.notFound", nil
	case errors.Is(err, directory.ErrNameTaken):
		return http.StatusConflict, "name_taken", "error.name.taken", nil
	case errors.Is(err, calendar.ErrExists):
		return http.StatusConflict, "name_taken", "error.calendar.exists", nil
	case errors.As(err, &bad):
		return http.StatusBadRequest, "bad_request", invalidKey(bad), nil
	case errors.Is(err, calendar.ErrBadSlug):
		return http.StatusBadRequest, "bad_request", "error.calendar.badName", nil
	case errors.Is(err, settings.ErrUnknownKey):
		return http.StatusBadRequest, "bad_request", "error.setting.unknown", nil
	case errors.Is(err, directory.ErrBadName):
		return http.StatusBadRequest, "bad_request", "error.name.invalid", nil
	case errors.As(err, &quota):
		key := "error.quota.events"
		if quota.Setting == "max_calendars_per_user" {
			key = "error.quota.calendars"
		}
		return http.StatusInsufficientStorage, "quota", key, []string{"max", strconv.Itoa(quota.Limit)}
	case errors.Is(err, auth.ErrTooMany):
		return http.StatusInsufficientStorage, "quota", "error.quota.devices", []string{"max", strconv.Itoa(h.d.Settings.Get().MaxDevicesPerUser)}
	case errors.Is(err, calendar.ErrQuota):
		return http.StatusInsufficientStorage, "quota", "error.quota", nil
	}
	return http.StatusInternalServerError, "error", "error.internal", nil
}

// invalidKey is the message of a bad name, display name or colour.
func invalidKey(e directory.Invalid) string {
	switch e.Field {
	case "display_name":
		return "error.displayName.invalid"
	case "color":
		return "error.color.invalid"
	}
	return "error.name.invalid"
}

// validation is a 400 whose message is the catalog's key.
type validation struct {
	key  string
	args []string
}

func (v validation) Error() string { return "invalid: " + v.key }

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
	lang := i18n.FromRequest(r)
	add := func(code, key string, args ...string) {
		out = append(out, warning{code, i18n.T(lang, key, args...)})
	}
	set := h.d.Settings.Get()
	if set.PublicListen == "" {
		add("no_public_listener", "error.warning.noPublicListener")
	} else if h.d.PublicStatus != nil {
		if addr, err := h.d.PublicStatus(); err != "" {
			add("public_listener_down", "error.warning.publicDown", "address", set.PublicListen, "error", err)
		} else if addr == "" {
			add("public_listener_down", "error.warning.publicStopped")
		}
	}
	if strings.HasPrefix(h.d.Config.AdminURL(), "http://") && !strings.HasPrefix(h.d.Config.AdminListen, "127.") &&
		!strings.HasPrefix(h.d.Config.AdminListen, "localhost") {
		add("admin_insecure", "error.warning.adminInsecure")
	}
	if h.d.Backup != nil {
		if last := h.d.Backup.Last(); last.IsZero() || time.Since(last) > 48*time.Hour {
			add("backup_old", "error.warning.backupOld")
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
		h.failIn(w, r, err)
		return
	}
	groups, err := h.d.Directory.Groups(ctx)
	if err != nil {
		h.failIn(w, r, err)
		return
	}
	counts, err := h.d.Calendar.AdminCounts(ctx)
	if err != nil {
		h.failIn(w, r, err)
		return
	}
	devices, err := h.d.Auth.CountDevices(ctx)
	if err != nil {
		h.failIn(w, r, err)
		return
	}
	stale, err := h.d.Auth.StaleDevices(ctx)
	if err != nil {
		h.failIn(w, r, err)
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
		h.failIn(w, r, err)
		return
	}
	out := make([]userView, 0, len(users))
	for _, u := range users {
		v := userView{User: u, Groups: []string{}}
		gs, err := h.d.Directory.GroupsOf(ctx, u.ID)
		if err != nil {
			h.failIn(w, r, err)
			return
		}
		for _, g := range gs {
			v.Groups = append(v.Groups, g.Name)
		}
		ds, err := h.d.Auth.Devices(ctx, u.ID)
		if err != nil {
			h.failIn(w, r, err)
			return
		}
		v.Devices = len(ds)
		owned, err := h.d.Calendar.Owned(ctx, u.ID)
		if err != nil {
			h.failIn(w, r, err)
			return
		}
		v.Owned = len(owned)
		out = append(out, v)
	}
	webx.JSON(w, http.StatusOK, map[string]any{"users": out})
}

// enrollmentView is an enrollment with its link and QR code, as made.
func (h *handlers) enrollmentView(e auth.Enrollment) (map[string]any, error) {
	base := h.d.PublicURL()
	if base == "" {
		return nil, validation{key: "error.admin.noPublicURL"}
	}
	link := base + "/enroll/" + e.Token
	qr, err := webx.QRSVG(link)
	if err != nil {
		return nil, err
	}
	return map[string]any{"id": e.ID, "kind": e.Kind, "url": link, "qr_svg": qr, "created_at": e.CreatedAt, "expires_at": e.ExpiresAt}, nil
}

func (h *handlers) addUser(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name        string   `json:"name"`
		DisplayName string   `json:"display_name"`
		Color       string   `json:"color"`
		Groups      []string `json:"groups"`
		Enroll      bool     `json:"enroll"`
	}
	if !decode(w, r, &body) {
		return
	}
	u, err := h.d.Directory.AddUser(r.Context(), "admin", body.Name, body.DisplayName, body.Color, body.Groups)
	if err != nil {
		h.failIn(w, r, err)
		return
	}
	out := map[string]any{"user": u}
	if body.Enroll && h.d.PublicURL() != "" {
		e, err := h.d.Auth.NewEnrollment(r.Context(), "admin", u.ID, 0)
		if err != nil {
			h.failIn(w, r, err)
			return
		}
		if out["enrollment"], err = h.enrollmentView(e); err != nil {
			h.failIn(w, r, err)
			return
		}
	}
	webx.JSON(w, http.StatusCreated, out)
}

func (h *handlers) lookupUser(w http.ResponseWriter, r *http.Request) (directory.User, bool) {
	u, err := h.d.Directory.User(r.Context(), r.PathValue("name"))
	if err != nil || u.RemovedAt != nil {
		webx.FailT(w, r, http.StatusNotFound, "not_found", "error.admin.noUser")
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
		h.failIn(w, r, err)
		return
	}
	groups := []string{}
	for _, g := range gs {
		groups = append(groups, g.Name)
	}
	enrollments, err := h.d.Auth.Enrollments(ctx, u.ID)
	if err != nil {
		h.failIn(w, r, err)
		return
	}
	devices, err := h.d.Auth.Devices(ctx, u.ID)
	if err != nil {
		h.failIn(w, r, err)
		return
	}
	sessions, err := h.d.Auth.Sessions(ctx, u.ID)
	if err != nil {
		h.failIn(w, r, err)
		return
	}
	owned, err := h.d.Calendar.AdminCalendars(ctx, u.Name)
	if err != nil {
		h.failIn(w, r, err)
		return
	}
	shared, err := h.d.Calendar.AdminSharedWith(ctx, u.ID)
	if err != nil {
		h.failIn(w, r, err)
		return
	}
	webx.JSON(w, http.StatusOK, map[string]any{"user": u, "groups": groups, "enrollments": enrollments, "devices": devices,
		"dav_auth": h.d.Auth.UserDavAuthOwn(ctx, u.ID), "dav_auth_effective": h.d.Auth.UserDavAuth(ctx, u.ID), "dav_auth_server": h.d.Auth.DavAuthMode(ctx),
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
		h.failIn(w, r, err)
		return
	}
	if body.Groups != nil {
		if err := h.d.Directory.SetGroups(ctx, "admin", u.Name, *body.Groups); err != nil {
			h.failIn(w, r, err)
			return
		}
	}
	if body.Disabled != nil {
		if err := h.d.Directory.SetDisabled(ctx, "admin", u.Name, *body.Disabled); err != nil {
			h.failIn(w, r, err)
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
		h.failIn(w, r, err)
		return
	}
	for _, i := range owned {
		choice, ok := body.Calendars[i.Ref.String()]
		if !ok || (choice.TransferTo == "") == !choice.Delete {
			webx.JSON(w, http.StatusConflict, map[string]any{"error": "owns_calendars",
				"message": i18n.T(i18n.FromRequest(r), "error.admin.ownsCalendars"), "calendars": refsOf(owned)})
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
			h.failIn(w, r, err)
			return
		}
	}
	if err := h.d.Directory.Remove(ctx, "admin", u.Name); err != nil {
		h.failIn(w, r, err)
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

func (h *handlers) addEnrollment(w http.ResponseWriter, r *http.Request) {
	u, ok := h.lookupUser(w, r)
	if !ok {
		return
	}
	if h.d.PublicURL() == "" {
		h.failIn(w, r, validation{key: "error.admin.noPublicURL"})
		return
	}
	// {"kind": "phone"} (the default: a QR for the phone's camera, the
	// enrollment_link_ttl_hours setting) or "browser" (an hour, to sign a
	// browser in for testing). An empty body is a phone's.
	var body struct {
		Kind string `json:"kind"`
	}
	if r.ContentLength != 0 && !decode(w, r, &body) {
		return
	}
	if body.Kind == "" {
		body.Kind = "phone"
	}
	e, err := h.d.Auth.NewEnrollmentKind(r.Context(), "admin", u.ID, 0, body.Kind)
	if err != nil {
		h.failIn(w, r, err)
		return
	}
	view, err := h.enrollmentView(e)
	if err != nil {
		h.failIn(w, r, err)
		return
	}
	webx.JSON(w, http.StatusCreated, view)
}

// showEnrollment shows a pending link again: its QR and its address. Gone
// (410) once used, cancelled or expired.
func (h *handlers) showEnrollment(w http.ResponseWriter, r *http.Request) {
	u, ok := h.lookupUser(w, r)
	if !ok {
		return
	}
	id, ok := pathID(r, "id")
	if !ok {
		h.failIn(w, r, auth.ErrInvalid)
		return
	}
	e, err := h.d.Auth.PendingEnrollment(r.Context(), u.ID, id)
	switch {
	case errors.Is(err, auth.ErrUsed):
		webx.FailT(w, r, http.StatusGone, "used", "error.admin.enrollmentUsed")
		return
	case errors.Is(err, auth.ErrExpired):
		webx.FailT(w, r, http.StatusGone, "expired", "error.admin.enrollmentExpired")
		return
	case err != nil:
		h.failIn(w, r, err)
		return
	}
	view, err := h.enrollmentView(e)
	if err != nil {
		h.failIn(w, r, err)
		return
	}
	webx.JSON(w, http.StatusOK, view)
}

func (h *handlers) revokeEnrollment(w http.ResponseWriter, r *http.Request) {
	u, ok := h.lookupUser(w, r)
	id, idOK := pathID(r, "id")
	if !ok || !idOK {
		return
	}
	if err := h.d.Auth.RevokeEnrollment(r.Context(), u.ID, id); err != nil {
		h.failIn(w, r, err)
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
		h.failIn(w, r, err)
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
		h.failIn(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- groups

func (h *handlers) groups(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	gs, err := h.d.Directory.Groups(ctx)
	if err != nil {
		h.failIn(w, r, err)
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
			h.failIn(w, r, err)
			return
		}
		v := view{Group: g, Members: []string{}}
		for _, m := range ms {
			v.Members = append(v.Members, m.Name)
		}
		if v.Shares, err = h.d.Directory.GroupShares(ctx, g.Name); err != nil {
			h.failIn(w, r, err)
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
		h.failIn(w, r, err)
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
		h.failIn(w, r, err)
		return
	}
	if body.Members != nil {
		current, err := h.d.Directory.Members(ctx, name)
		if err != nil {
			h.failIn(w, r, err)
			return
		}
		want := map[string]bool{}
		for _, m := range *body.Members {
			want[m] = true
		}
		for _, m := range current {
			if !want[m.Name] {
				if err := h.d.Directory.RemoveMember(ctx, "admin", name, m.Name); err != nil {
					h.failIn(w, r, err)
					return
				}
			}
			delete(want, m.Name)
		}
		for m := range want {
			if err := h.d.Directory.AddMember(ctx, "admin", name, m); err != nil {
				h.failIn(w, r, err)
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
		h.failIn(w, r, err)
		return
	}
	if n > 0 && r.URL.Query().Get("confirm") != "1" {
		webx.JSON(w, http.StatusConflict, map[string]any{"error": "has_shares", "shares": n,
			"message": i18n.T(i18n.FromRequest(r), "error.admin.groupHasShares")})
		return
	}
	if err := h.d.Directory.RemoveGroup(ctx, "admin", name); err != nil {
		h.failIn(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- calendars (metadata only)

func (h *handlers) calendars(w http.ResponseWriter, r *http.Request) {
	cs, err := h.d.Calendar.AdminCalendars(r.Context(), r.URL.Query().Get("owner"))
	if err != nil {
		h.failIn(w, r, err)
		return
	}
	webx.JSON(w, http.StatusOK, map[string]any{"calendars": cs})
}

// exportCalendar is one calendar as .ics, a backup of it (recorded).
func (h *handlers) exportCalendar(w http.ResponseWriter, r *http.Request) {
	ref, err := h.d.Calendar.RefOf(r.Context(), r.PathValue("ref"))
	if err != nil {
		h.failIn(w, r, err)
		return
	}
	data, _, err := h.d.Calendar.AdminExport(r.Context(), "admin", ref)
	if err != nil {
		h.failIn(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "text/calendar; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="`+ref.String()+`.ics"`)
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(data)
}

// exportCalendars is every calendar as .ics, one file each, in a zip
// named for the day: a backup that any calendar app reads back.
func (h *handlers) exportCalendars(w http.ResponseWriter, r *http.Request) {
	cs, err := h.d.Calendar.AdminCalendars(r.Context(), "")
	if err != nil {
		h.failIn(w, r, err)
		return
	}
	day := h.d.Clock().UTC().Format("2006-01-02")
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", `attachment; filename="agendouille-calendriers-`+day+`.zip"`)
	w.Header().Set("Cache-Control", "no-store")
	z := zip.NewWriter(w)
	for _, c := range cs {
		ref, err := calendar.ParseRef(c.Ref)
		if err != nil {
			continue
		}
		data, _, err := h.d.Calendar.AdminExport(r.Context(), "admin", ref)
		if err != nil {
			h.d.Log.Error("calendar export", "ref", c.Ref, "err", err)
			continue
		}
		f, err := z.Create(c.Ref + ".ics")
		if err != nil {
			break
		}
		_, _ = f.Write(data)
	}
	_ = z.Close()
}

// calendarStats is one calendar's stats page: counts, times, who and from
// what, never an event (calendar.AdminStats).
func (h *handlers) calendarStats(w http.ResponseWriter, r *http.Request) {
	st, err := h.d.Calendar.AdminStats(r.Context(), r.PathValue("ref"))
	if err != nil {
		h.failIn(w, r, err)
		return
	}
	webx.JSON(w, http.StatusOK, st)
}

// share adds a member to a calendar, or changes their access: a user or a
// group, read or readwrite. The admin manages who is in a calendar (the
// user's call), so the owner's group visibility does not bound it; the
// owner always has it, and is not a member.
func (h *handlers) share(w http.ResponseWriter, r *http.Request) {
	ref, err := h.d.Calendar.RefOf(r.Context(), r.PathValue("ref"))
	if err != nil {
		h.failIn(w, r, err)
		return
	}
	t, err := sharing.ParseTarget(r.PathValue("target"))
	if err != nil {
		h.failIn(w, r, validation{key: "error.share.invalid"})
		return
	}
	var body struct {
		Access string `json:"access"`
	}
	if !decode(w, r, &body) {
		return
	}
	acc, err := sharing.ParseAccess(body.Access)
	if err != nil {
		h.failIn(w, r, validation{key: "error.share.invalid"})
		return
	}
	i, err := h.d.Calendar.Info(r.Context(), calendar.Operator("admin"), ref)
	if err != nil {
		h.failIn(w, r, err)
		return
	}
	if err := h.d.Calendar.Sharing().ShareWith(r.Context(), "admin", i.OwnerID, false, i.ID, t, acc, store.Time(h.d.Clock())); err != nil {
		if errors.Is(err, sharing.ErrNotFound) {
			h.failIn(w, r, err)
			return
		}
		key := "error.share.invalid"
		if errors.Is(err, sharing.ErrIsOwner) {
			key = "error.share.owner"
		} else {
			h.d.Log.Warn("admin share", "err", err)
		}
		h.failIn(w, r, validation{key: key})
		return
	}
	h.accessChanged(i)
	w.WriteHeader(http.StatusNoContent)
}

// accessChanged tells the open family pages that who sees what changed.
func (h *handlers) accessChanged(i calendar.Info) {
	if h.d.Hub != nil {
		h.d.Hub.Publish(hub.Event{CalendarID: i.ID, Calendar: i.Ref.String(), Action: "access"})
	}
}

func (h *handlers) unshare(w http.ResponseWriter, r *http.Request) {
	ref, err := h.d.Calendar.RefOf(r.Context(), r.PathValue("ref"))
	if err != nil {
		h.failIn(w, r, err)
		return
	}
	t, err := sharing.ParseTarget(r.PathValue("target"))
	if err != nil {
		h.failIn(w, r, validation{key: "error.share.invalid"})
		return
	}
	i, err := h.d.Calendar.Info(r.Context(), calendar.Operator("admin"), ref)
	if err != nil {
		h.failIn(w, r, err)
		return
	}
	if err := h.d.Calendar.Sharing().Unshare(r.Context(), "admin", i.ID, t, store.Time(h.d.Clock())); err != nil {
		h.failIn(w, r, err)
		return
	}
	h.accessChanged(i)
	w.WriteHeader(http.StatusNoContent)
}

func (h *handlers) transfer(w http.ResponseWriter, r *http.Request) {
	ref, err := h.d.Calendar.RefOf(r.Context(), r.PathValue("ref"))
	if err != nil {
		h.failIn(w, r, err)
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
		h.failIn(w, r, err)
		return
	}
	id, _ := h.d.Calendar.IDOf(r.Context(), i.Ref)
	webx.JSON(w, http.StatusOK, map[string]string{"ref": i.Ref.String(), "id": id})
}

func (h *handlers) deleteCalendar(w http.ResponseWriter, r *http.Request) {
	ref, err := h.d.Calendar.RefOf(r.Context(), r.PathValue("ref"))
	if err != nil {
		h.failIn(w, r, err)
		return
	}
	op := calendar.Operator("admin")
	i, err := h.d.Calendar.Info(r.Context(), op, ref)
	if err != nil {
		h.failIn(w, r, err)
		return
	}
	// The admin types the calendar's name: deleting is for everyone, and
	// there is no undo but a backup.
	if r.URL.Query().Get("confirm") != i.Name {
		h.failIn(w, r, validation{key: "error.admin.confirmCalendar"})
		return
	}
	if err := h.d.Calendar.DeleteCalendar(r.Context(), op, ref); err != nil {
		h.failIn(w, r, err)
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
			h.failIn(w, r, validation{key: "error.admin.badBefore"})
			return
		}
		f.Before = t
	}
	items, err := h.d.Calendar.AdminActivity(r.Context(), f)
	if err != nil {
		h.failIn(w, r, err)
		return
	}
	webx.JSON(w, http.StatusOK, map[string]any{"activity": items})
}

// exportActivity is the activity as CSV, with the table's filter: what
// lands in the file is what was on the screen, and the rows not loaded yet.
// At most 50 000 rows, newest first.
func (h *handlers) exportActivity(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	f := calendar.ActivityFilter{User: q.Get("user"), Calendar: q.Get("calendar"), Kind: q.Get("kind"), Limit: 500}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="agendouille-activite.csv"`)
	w.Header().Set("Cache-Control", "no-store")
	out := csv.NewWriter(w)
	l := i18n.FromRequest(r)
	head := []string{}
	for _, c := range []string{"at", "by", "kind", "action", "calendar", "via", "device", "detail"} {
		head = append(head, i18n.T(l, "activity.csv."+c))
	}
	_ = out.Write(head)
	for n := 0; n < 50000; {
		items, err := h.d.Calendar.AdminActivity(r.Context(), f)
		if err != nil {
			h.d.Log.Error("activity export", "err", err)
			break
		}
		for _, a := range items {
			_ = out.Write([]string{a.At.UTC().Format(time.RFC3339), a.By, a.Kind, a.Action, a.Calendar, a.Via, a.Device, a.Detail})
		}
		n += len(items)
		if len(items) < f.Limit {
			break
		}
		f.Before = items[len(items)-1].At
	}
	out.Flush()
}

// --- settings

// settings is every setting as the server holds it, where its value came
// from (the file, the environment, a flag, or the default), and the file
// to edit: the settings live in the configuration file (DECISIONS), as
// cafard's and claudication's; this page shows them, it does not edit them.
// Each comes with its help text in the reader's language.
func (h *handlers) settings(w http.ResponseWriter, r *http.Request) {
	type item struct {
		settings.Key
		Value  json.RawMessage `json:"value"`
		Origin string          `json:"origin"`
		Env    string          `json:"env"`
		Help   string          `json:"help,omitempty"`
	}
	l := i18n.FromRequest(r)
	origins := h.d.Config.Origins
	if fresh, err := h.d.Config.Reload(); err == nil {
		origins = fresh.Origins // the file as it is now
	}
	out := []item{}
	for _, k := range settings.Keys() {
		v, err := h.d.Settings.Value(k.Name)
		if err != nil {
			h.failIn(w, r, err)
			return
		}
		origin := origins[k.Name]
		if origin == "" {
			origin = "default"
		}
		help := ""
		if key := "setting." + k.Name + ".help"; i18n.Has(key) {
			help = i18n.T(l, key)
		}
		out = append(out, item{Key: k, Value: json.RawMessage(v), Origin: origin, Env: config.EnvName(k.Name), Help: help})
	}
	path := h.d.Config.Path
	if path == "" {
		path = config.DefaultPath
	}
	webx.JSON(w, http.StatusOK, map[string]any{"settings": out, "path": path, "exists": h.d.Config.Path != ""})
}

// davAuth is how CalDAV signs a device in, chosen in the admin UI (the one
// switch kept out of the configuration file, the user's call), and how
// many devices have no certificate yet.
func (h *handlers) davAuth(w http.ResponseWriter, r *http.Request) {
	n, err := h.d.Auth.DevicesWithoutCert(r.Context())
	if err != nil {
		h.failIn(w, r, err)
		return
	}
	webx.JSON(w, http.StatusOK, map[string]any{"mode": h.d.Auth.DavAuthMode(r.Context()), "devices_without_certificate": n})
}

func (h *handlers) setDavAuth(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Mode string `json:"mode"`
	}
	if !decode(w, r, &body) {
		return
	}
	if err := h.d.Auth.SetDavAuthMode(r.Context(), "admin", body.Mode); err != nil {
		h.failIn(w, r, validation{key: "error.admin.davAuthMode"})
		return
	}
	h.davAuth(w, r)
}

// --- backups

func (h *handlers) backups(w http.ResponseWriter, r *http.Request) {
	files, err := h.d.Backup.List()
	if err != nil {
		h.failIn(w, r, err)
		return
	}
	webx.JSON(w, http.StatusOK, map[string]any{"backups": files})
}

func (h *handlers) makeBackup(w http.ResponseWriter, r *http.Request) {
	f, err := h.d.Backup.Make(r.Context())
	if err != nil {
		h.failIn(w, r, err)
		return
	}
	webx.JSON(w, http.StatusCreated, f)
}

func (h *handlers) downloadBackup(w http.ResponseWriter, r *http.Request) {
	path, err := h.d.Backup.Path(r.PathValue("file"))
	if err != nil {
		h.failIn(w, r, err)
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
		h.failIn(w, r, validation{key: "error.admin.confirmRestore"})
		return
	}
	if err := h.d.Backup.Stage(name); err != nil {
		if errors.Is(err, backup.ErrNotFound) {
			h.failIn(w, r, err)
			return
		}
		h.d.Log.Warn("restore refused", "file", name, "err", err)
		h.failIn(w, r, validation{key: "error.admin.backupDamaged"})
		return
	}
	webx.JSON(w, http.StatusAccepted, map[string]string{"status": "restarting",
		"message": i18n.T(i18n.FromRequest(r), "error.admin.restarting")})
	if h.d.Restart != nil {
		go h.d.Restart("restore " + name)
	}
}
