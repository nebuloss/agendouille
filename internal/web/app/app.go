// Package app serves the app surface: the family web app's pages and the
// additional API (CONTRACTS.md 2.2), the Android app's own calls included.
// In M1 that is signing in: enrolment links, approvals of a browser from a
// signed-in device, and the app's enrolment and WebView session.
package app

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/nebuloss/agendouille/internal/auth"
	"github.com/nebuloss/agendouille/internal/deps"
	"github.com/nebuloss/agendouille/internal/web/assets"
	"github.com/nebuloss/agendouille/internal/web/webx"
)

// WaitTimeout is how long GET /signin/wait/{id} holds a request open
// before answering "pending".
var WaitTimeout = 25 * time.Second

type handlers struct{ d *deps.Deps }

// Register adds the app surface to mux, which must serve nothing else but
// the dav surface.
func Register(mux *http.ServeMux, d *deps.Deps) error {
	if err := assets.Register(mux, assets.App); err != nil {
		return err
	}
	h := &handlers{d: d}
	guard := func(next http.Handler) http.Handler { return webx.SameOrigin(d.PublicURL, next) }

	for path, page := range map[string][2]string{
		"GET /{$}":       {"calendar", "Calendrier"},
		"GET /calendars": {"calendars", "Calendriers"},
		"GET /devices":   {"devices", "Appareils"},
		"GET /me":        {"me", "Moi"},
	} {
		mux.Handle(path, h.page(h.shell(page[0], page[1])))
	}
	h.registerFamily(mux, guard)
	mux.HandleFunc("GET /signin", h.signin)
	mux.HandleFunc("GET /signin/wait/{id}", h.wait)
	mux.HandleFunc("GET /enrol/{token}", h.enrolPage)
	mux.Handle("POST /enrol/{token}", guard(http.HandlerFunc(h.enrol)))
	mux.Handle("POST /signout", guard(http.HandlerFunc(h.signout)))
	mux.Handle("GET /approve", h.page(h.approvePage))
	mux.Handle("POST /approve", guard(h.page(h.approvePost)))

	mux.Handle("GET /api/me", h.api(h.me))
	mux.Handle("GET /api/approvals/{code}", h.api(h.approvalView))
	mux.Handle("POST /api/approvals/{code}", guard(h.api(h.approvalPost)))

	mux.HandleFunc("POST /api/app/enrol", h.appEnrol)
	mux.Handle("POST /api/app/session", h.basic(h.appSession))
	mux.Handle("GET /api/app/config", h.basic(h.appConfig))
	return nil
}

func (h *handlers) clientIP(r *http.Request) string {
	return auth.ClientIP(r, h.d.Trusted()).String()
}

func (h *handlers) secure() bool { return webx.Secure(h.d.PublicURL()) }

func (h *handlers) render(w http.ResponseWriter, r *http.Request, status int, name, title string, data any) {
	webx.Render(w, r, h.d.Log, status, name, webx.Page{Title: title, Page: strings.TrimSuffix(strings.TrimPrefix(name, "app/"), ".html"),
		Bundle: assets.App, Data: data})
}

type withPrincipal func(w http.ResponseWriter, r *http.Request, p *auth.Principal)

// page needs an agd_app session and sends to /signin without one.
func (h *handlers) page(next withPrincipal) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, err := h.d.Auth.Session(r, auth.SurfaceApp)
		if err != nil {
			http.Redirect(w, r, "/signin", http.StatusSeeOther)
			return
		}
		next(w, r.WithContext(auth.WithPrincipal(r.Context(), p)), p)
	})
}

// api needs an agd_app session and answers 401 without one.
func (h *handlers) api(next withPrincipal) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, err := h.d.Auth.Session(r, auth.SurfaceApp)
		if err != nil {
			webx.Fail(w, http.StatusUnauthorized, "unauthenticated", "sign in first")
			return
		}
		next(w, r.WithContext(auth.WithPrincipal(r.Context(), p)), p)
	})
}

// basic needs a device credential (the Android app's own calls).
func (h *handlers) basic(next withPrincipal) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, err := h.d.Auth.Basic(r, h.clientIP(r))
		if err != nil {
			w.Header().Set("WWW-Authenticate", `Basic realm="agendouille", charset="UTF-8"`)
			webx.Fail(w, http.StatusUnauthorized, "unauthenticated", "a device credential is needed")
			return
		}
		next(w, r, p)
	})
}

// shell is a page of the family web app: the navigation and a mount point
// the app bundle fills from the API (PLAN.md 9.3).
func (h *handlers) shell(page, title string) withPrincipal {
	return func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
		webx.Render(w, r, h.d.Log, http.StatusOK, "app/shell.html", webx.Page{Title: title, Page: page, Bundle: assets.App,
			Data: map[string]any{"Page": page, "Title": title, "User": p.User, "Initial": initial(p.User.DisplayName, p.User.Name),
				"Zone": h.d.Settings.Get().FamilyTimezone}})
	}
}

// signin starts an approval and shows its QR code, short code and number.
func (h *handlers) signin(w http.ResponseWriter, r *http.Request) {
	a, err := h.d.Auth.StartApproval(r.Context(), r.UserAgent(), h.clientIP(r))
	if err != nil {
		status, msg := http.StatusInternalServerError, "Une erreur est survenue."
		if errors.Is(err, auth.ErrBusy) {
			status, msg = http.StatusServiceUnavailable, "Trop de connexions en attente. Réessayez dans une minute."
		} else {
			h.d.Log.Error("starting an approval", "err", err)
		}
		h.render(w, r, status, "app/signin.html", "Se connecter", map[string]any{"Error": msg})
		return
	}
	// The code rides in the fragment, which browsers never send: it stays
	// out of every server and proxy log (CONTRACTS.md 3.4).
	qr, err := webx.QRDataURL(h.d.PublicURL() + "/approve#c=" + a.Code)
	if err != nil {
		h.d.Log.Error("drawing a QR code", "err", err)
	}
	// What is left, not when: the page counts it down on its own clock, so
	// a phone whose clock is wrong still refreshes the code on time.
	h.render(w, r, http.StatusOK, "app/signin.html", "Se connecter", map[string]any{"Approval": a, "QR": qr,
		"RemainingMs": a.ExpiresAt.Sub(h.d.Clock()).Milliseconds(), "TTLMs": auth.ApprovalTTL.Milliseconds()})
}

// wait long-polls an approval. Approved, it sets the session cookie; the
// bundle polls it with fetch, and a browser without the bundle follows the
// "continue" form with then=redirect.
func (h *handlers) wait(w http.ResponseWriter, r *http.Request) {
	redirect := r.URL.Query().Get("then") == "redirect"
	ctx, cancel := context.WithTimeout(r.Context(), WaitTimeout)
	defer cancel()
	if redirect {
		// The person says it is done: look once, without waiting long.
		ctx, cancel = context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
	}
	secret, err := h.d.Auth.WaitApproval(ctx, r.PathValue("id"), r.UserAgent())
	if err == nil {
		auth.SetCookie(w, auth.SurfaceApp, secret, h.secure(), h.d.Settings.Get().SessionTTL())
		if redirect {
			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		}
		webx.JSON(w, http.StatusOK, map[string]string{"status": "approved"})
		return
	}
	status, state := http.StatusGone, "expired"
	switch {
	case errors.Is(err, context.DeadlineExceeded), errors.Is(err, context.Canceled):
		status, state = http.StatusOK, "pending"
	case errors.Is(err, auth.ErrWrongNumber):
		state = "cancelled"
	case errors.Is(err, auth.ErrUsed):
		state = "used"
	case errors.Is(err, auth.ErrInvalid):
		status, state = http.StatusNotFound, "unknown"
	case !errors.Is(err, auth.ErrExpired):
		h.d.Log.Error("waiting on an approval", "err", err)
		status, state = http.StatusInternalServerError, "error"
	}
	if redirect {
		http.Redirect(w, r, "/signin", http.StatusSeeOther)
		return
	}
	webx.JSON(w, status, map[string]string{"status": state})
}

// enrolPage only shows a button: a GET never burns the link, since chat
// apps and mail scanners open links to preview them.
func (h *handlers) enrolPage(w http.ResponseWriter, r *http.Request) {
	h.render(w, r, http.StatusOK, "app/enrol.html", "Bienvenue", map[string]any{"Token": r.PathValue("token")})
}

func enrolError(err error) (int, string) {
	switch {
	case errors.Is(err, auth.ErrUsed):
		return http.StatusGone, "Ce lien a déjà servi."
	case errors.Is(err, auth.ErrExpired):
		return http.StatusGone, "Ce lien a expiré."
	case errors.Is(err, auth.ErrInvalid):
		return http.StatusNotFound, "Ce lien n'est pas valable."
	}
	return http.StatusInternalServerError, "Une erreur est survenue."
}

func (h *handlers) enrol(w http.ResponseWriter, r *http.Request) {
	secret, err := h.d.Auth.RedeemForBrowser(r.Context(), r.PathValue("token"), r.UserAgent())
	if err != nil {
		status, msg := enrolError(err)
		if status == http.StatusInternalServerError {
			h.d.Log.Error("redeeming an enrolment", "err", err)
		}
		h.render(w, r, status, "app/enrol.html", "Bienvenue", map[string]any{"Error": msg})
		return
	}
	auth.SetCookie(w, auth.SurfaceApp, secret, h.secure(), h.d.Settings.Get().SessionTTL())
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (h *handlers) signout(w http.ResponseWriter, r *http.Request) {
	h.d.Auth.SignOut(r, auth.SurfaceApp)
	auth.ClearCookie(w, auth.SurfaceApp, h.secure())
	http.Redirect(w, r, "/signin", http.StatusSeeOther)
}

func approvalError(err error) (int, string, string) {
	switch {
	case errors.Is(err, auth.ErrWrongNumber):
		return http.StatusGone, "cancelled", "Mauvais nombre : la demande est annulée. L'autre appareil doit recommencer."
	case errors.Is(err, auth.ErrExpired):
		return http.StatusGone, "expired", "Ce code a expiré. L'autre appareil doit recommencer."
	case errors.Is(err, auth.ErrUsed):
		return http.StatusGone, "used", "Cette demande a déjà été approuvée."
	case errors.Is(err, auth.ErrInvalid):
		return http.StatusNotFound, "not_found", "Ce code ne correspond à aucune demande en attente."
	}
	return http.StatusInternalServerError, "error", "Une erreur est survenue."
}

func (h *handlers) approvePage(w http.ResponseWriter, r *http.Request, _ *auth.Principal) {
	h.render(w, r, http.StatusOK, "app/approve.html", "Approuver", map[string]any{})
}

// approvePost is the form flow: a code alone looks the request up and
// shows the three numbers; a code and a number approves.
func (h *handlers) approvePost(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	code := strings.TrimSpace(r.PostFormValue("code"))
	data := map[string]any{"Code": code, "User": p.User.DisplayName}
	if raw := r.PostFormValue("number"); raw != "" {
		n, _ := strconv.Atoi(raw)
		if err := h.d.Auth.Approve(r.Context(), p, code, n); err != nil {
			status, _, msg := approvalError(err)
			data["Error"] = msg
			h.render(w, r, status, "app/approve.html", "Approuver", data)
			return
		}
		data["Done"] = true
		h.render(w, r, http.StatusOK, "app/approve.html", "Approuver", data)
		return
	}
	view, err := h.d.Auth.LookupApproval(r.Context(), p, code)
	if err != nil {
		status, _, msg := approvalError(err)
		data["Error"] = msg
		h.render(w, r, status, "app/approve.html", "Approuver", data)
		return
	}
	data["View"] = view
	h.render(w, r, http.StatusOK, "app/approve.html", "Approuver", data)
}

func (h *handlers) me(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	groups, err := h.d.Directory.GroupsOf(r.Context(), p.User.ID)
	if err != nil {
		webx.Fail(w, http.StatusInternalServerError, "error", "")
		return
	}
	type group struct {
		Name        string `json:"name"`
		DisplayName string `json:"display_name"`
	}
	gs := make([]group, 0, len(groups))
	for _, g := range groups {
		gs = append(gs, group{g.Name, g.DisplayName})
	}
	webx.JSON(w, http.StatusOK, map[string]any{"name": p.User.Name, "display_name": p.User.DisplayName,
		"color": p.User.Color, "groups": gs})
}

func (h *handlers) approvalView(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	view, err := h.d.Auth.LookupApproval(r.Context(), p, r.PathValue("code"))
	if err != nil {
		status, code, msg := approvalError(err)
		webx.Fail(w, status, code, msg)
		return
	}
	webx.JSON(w, http.StatusOK, view)
}

func (h *handlers) approvalPost(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	var body struct {
		Number int `json:"number"`
	}
	if !webx.DecodeJSON(w, r, &body) {
		return
	}
	if err := h.d.Auth.Approve(r.Context(), p, r.PathValue("code"), body.Number); err != nil {
		status, code, msg := approvalError(err)
		webx.Fail(w, status, code, msg)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// appEnrol is the Android app redeeming the admin's enrolment link it was
// opened with (PLAN.md 7.2). The answer carries the device credential, once.
func (h *handlers) appEnrol(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Token      string `json:"token"`
		Model      string `json:"model"`
		Android    int    `json:"android"`
		AppVersion string `json:"app_version"`
	}
	if !webx.DecodeJSON(w, r, &body) {
		return
	}
	d, err := h.d.Auth.RedeemForApp(r.Context(), body.Token, body.Model)
	if err != nil {
		status, msg := enrolError(err)
		code := map[int]string{http.StatusGone: "used", http.StatusNotFound: "not_found"}[status]
		if errors.Is(err, auth.ErrExpired) {
			code = "expired"
		}
		if errors.Is(err, auth.ErrTooMany) {
			status, code, msg = http.StatusInsufficientStorage, "quota", err.Error()
		} else if status == http.StatusInternalServerError {
			h.d.Log.Error("enrolling the app", "err", err)
			code = "error"
		}
		webx.Fail(w, status, code, msg)
		return
	}
	base := h.d.PublicURL()
	webx.JSON(w, http.StatusOK, map[string]any{"user": d.Username, "device_id": d.ID, "username": d.Username,
		"secret": d.Secret, "dav_url": base + "/dav/", "app_url": base + "/"})
}

// appSession opens the WebView's session from the app's credential.
func (h *handlers) appSession(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	secret, err := h.d.Auth.AppSession(r.Context(), p, r.UserAgent())
	if err != nil {
		webx.Fail(w, http.StatusForbidden, "forbidden", "only the agendouille app's credential opens a web session")
		return
	}
	auth.SetCookie(w, auth.SurfaceApp, secret, h.secure(), h.d.Settings.Get().SessionTTL())
	w.WriteHeader(http.StatusNoContent)
}

func (h *handlers) appConfig(w http.ResponseWriter, _ *http.Request, _ *auth.Principal) {
	webx.JSON(w, http.StatusOK, map[string]any{"server_version": h.d.Version, "min_app_version": "0.0.0",
		"apk_sha256": "", "bridge": 1, "sync_interval_s": h.d.Settings.Get().AppSyncIntervalMinutes * 60})
}

// initial is the letter on a user's avatar: the first of the display name,
// a whole character ("É", not half of it).
func initial(display, name string) string {
	for _, s := range []string{display, name} {
		for _, r := range s {
			return strings.ToUpper(string(r))
		}
	}
	return "?"
}
