// Package app serves the app surface: the family web app's pages and the
// additional API (CONTRACTS.md 2.2), the Android app's own calls included.
// In M1 that is signing in: enrollment links, approvals of a browser from a
// signed-in device, and the app's enrollment and WebView session.
package app

import (
	"bytes"
	"context"
	"errors"
	"html/template"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/nebuloss/agendouille/internal/auth"
	"github.com/nebuloss/agendouille/internal/deps"
	"github.com/nebuloss/agendouille/internal/i18n"
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
		"GET /{$}":       {"calendar", "app.title.calendar"},
		"GET /calendars": {"calendars", "app.title.calendars"},
		"GET /devices":   {"devices", "app.title.devices"},
		"GET /me":        {"me", "app.title.me"},
	} {
		mux.Handle(path, h.page(h.shell(page[0], page[1])))
	}
	h.registerFamily(mux, guard)
	// The sign-in page is the root's, without a session; the old address leads there.
	mux.HandleFunc("GET /signin", func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, "/", http.StatusMovedPermanently) })
	mux.HandleFunc("GET /signin/wait/{id}", h.wait)
	mux.HandleFunc("GET /enroll/{token}", h.enrollPage)
	// The links made before the spelling changed (DECISIONS: enroll): a QR
	// code already handed out still opens the page.
	mux.HandleFunc("GET /enrol/{token}", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/enroll/"+url.PathEscape(r.PathValue("token")), http.StatusMovedPermanently)
	})
	mux.Handle("POST /enroll/{token}", guard(http.HandlerFunc(h.enroll)))
	mux.Handle("POST /signout", guard(http.HandlerFunc(h.signout)))
	mux.Handle("GET /approve", h.page(h.approvePage))
	mux.Handle("POST /approve", guard(h.page(h.approvePost)))

	mux.Handle("GET /api/me", h.api(h.me))
	mux.Handle("GET /api/approvals/{code}", h.api(h.approvalView))
	mux.Handle("POST /api/approvals/{code}", guard(h.api(h.approvalPost)))
	mux.Handle("DELETE /api/approvals/{code}", guard(h.api(h.approvalRefuse)))

	mux.HandleFunc("POST /api/app/enroll", h.appEnroll)
	mux.HandleFunc("POST /api/app/crash", h.appCrash)
	mux.HandleFunc("POST /api/app/davx5", h.appDavx5)
	mux.HandleFunc("GET /app/agendouille.apk", h.apkFile)
	mux.HandleFunc("GET /davx5/{token}", h.davx5Page)
	mux.HandleFunc("GET /.well-known/assetlinks.json", h.assetLinks)
	mux.Handle("POST /api/app/session", h.basic(h.appSession))
	mux.Handle("GET /api/app/config", h.basic(h.appConfig))
	return nil
}

func (h *handlers) clientIP(r *http.Request) string {
	return auth.ClientIP(r, h.d.Trusted()).String()
}

func (h *handlers) secure() bool { return webx.Secure(h.d.PublicURL()) }

// render shows a page outside the shell; titleKey is its title's key in
// internal/i18n.
func (h *handlers) render(w http.ResponseWriter, r *http.Request, status int, name, titleKey string, data any) {
	webx.Render(w, r, h.d.Log, status, name, webx.Page{Title: i18n.T(i18n.FromRequest(r), titleKey), Page: strings.TrimSuffix(strings.TrimPrefix(name, "app/"), ".html"),
		Bundle: assets.App, Data: data})
}

type withPrincipal func(w http.ResponseWriter, r *http.Request, p *auth.Principal)

// page needs an agd_app session. Without one, the root is the sign-in
// page itself and every other page leads there.
func (h *handlers) page(next withPrincipal) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, err := h.d.Auth.Session(r, auth.SurfaceApp)
		if err != nil {
			if r.URL.Path == "/" {
				w.Header().Set("Cache-Control", "no-store")
				h.signin(w, r)
				return
			}
			http.Redirect(w, r, "/", http.StatusSeeOther)
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
			webx.FailT(w, r, http.StatusUnauthorized, "unauthenticated", "error.signin.required")
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
			webx.FailT(w, r, http.StatusUnauthorized, "unauthenticated", "error.device.unknown")
			return
		}
		next(w, r, p)
	})
}

// shell is a page of the family web app: the navigation and a mount point
// the app bundle fills from the API (PLAN.md 9.3).
func (h *handlers) shell(page, titleKey string) withPrincipal {
	return func(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
		title := i18n.T(i18n.FromRequest(r), titleKey)
		webx.Render(w, r, h.d.Log, http.StatusOK, "app/shell.html", webx.Page{Title: title, Page: page, Bundle: assets.App,
			Data: map[string]any{"Page": page, "Title": title, "User": p.User, "Initial": initial(p.User.DisplayName, p.User.Name),
				"Zone": h.d.Settings.Get().FamilyTimezone}})
	}
}

// signin starts an approval and shows its QR code, short code and number.
func (h *handlers) signin(w http.ResponseWriter, r *http.Request) {
	a, err := h.d.Auth.StartApproval(r.Context(), r.UserAgent(), h.clientIP(r))
	if err != nil {
		status, key := http.StatusInternalServerError, "error.internal"
		if errors.Is(err, auth.ErrBusy) {
			status, key = http.StatusServiceUnavailable, "error.signin.busy"
		} else {
			h.d.Log.Error("starting an approval", "err", err)
		}
		h.render(w, r, status, "app/signin.html", "app.title.signin", map[string]any{"Error": i18n.T(i18n.FromRequest(r), key)})
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
	data := map[string]any{"Approval": a, "QR": qr,
		"RemainingMs": a.ExpiresAt.Sub(h.d.Clock()).Milliseconds(), "TTLMs": auth.ApprovalTTL.Milliseconds()}
	// The Android app, when this server has it, and only on an Android
	// phone: nowhere else can it be installed.
	if apk := assets.AndroidApp(); apk != nil && platformOf(r.UserAgent()) == "android" {
		data["APKSHA256"] = apk.SHA256
	}
	h.render(w, r, http.StatusOK, "app/signin.html", "app.title.signin", data)
}

// platformOf tells, from a User-Agent, "android", "ios" (iPhone, iPad; an
// iPad's Safari says Macintosh) or "desktop": the sign-in page offers the
// Android app on Android only.
func platformOf(ua string) string {
	switch {
	case strings.Contains(ua, "Android"):
		return "android"
	case strings.Contains(ua, "iPhone"), strings.Contains(ua, "iPad"), strings.Contains(ua, "iPod"):
		return "ios"
	}
	return "desktop"
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
	secret, seen, err := h.d.Auth.WaitApproval(ctx, r.PathValue("id"), r.UserAgent(), r.URL.Query().Get("seen") == "1")
	if err == nil && secret == "" && seen != "" {
		// Someone's device has the code: the page says whose to look at.
		if redirect {
			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		}
		webx.JSON(w, http.StatusOK, map[string]string{"status": "scanned", "name": seen})
		return
	}
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
	case errors.Is(err, auth.ErrRefused):
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
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	webx.JSON(w, status, map[string]string{"status": state})
}

// enrollPage only shows a button: a GET never burns the link, since chat
// apps and mail scanners open links to preview them.
func (h *handlers) enrollPage(w http.ResponseWriter, r *http.Request) {
	data := map[string]any{"Token": r.PathValue("token")}
	// On a phone, the app: open this link in it (any server, through its
	// own scheme), or get it first (PLAN.md 7.2).
	if a := assets.AndroidApp(); a != nil {
		link := h.d.PublicURL() + "/enroll/" + url.PathEscape(r.PathValue("token"))
		data["AppLink"] = template.URL("agendouille://enroll?u=" + url.QueryEscape(link))
		data["APKSHA256"] = a.SHA256
		data["Android"] = strings.Contains(r.UserAgent(), "Android")
	}
	h.render(w, r, http.StatusOK, "app/enroll.html", "app.title.welcome", data)
}

// davx5Page is where the QR code of a new DAVx5 device leads, on the
// phone: a button that opens DAVx5 with the account filled in, and the
// same by hand. The token is the secret (ten minutes, then gone).
func (h *handlers) davx5Page(w http.ResponseWriter, r *http.Request) {
	body, ok := pending.peek(r.PathValue("token"), "davx5", h.d.Clock())
	if !ok {
		h.render(w, r, http.StatusGone, "app/davx5.html", "app.title.davx5", map[string]any{"Gone": true})
		return
	}
	link := string(body)
	u, err := url.Parse(link)
	if err != nil || u.User == nil {
		http.NotFound(w, r)
		return
	}
	pass, _ := u.User.Password()
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	h.render(w, r, http.StatusOK, "app/davx5.html", "app.title.davx5", map[string]any{
		"Link": template.URL(link), "Server": h.d.PublicURL() + "/dav/", "Username": u.User.Username(), "Password": pass,
	})
}

// appDavx5 gives the agendouille app what the page of a DAVx5 device's QR
// code holds (davx5Page): the davx5:// link, so that code signs the app in
// too. The token is the secret, as for the page: ten minutes, then gone.
func (h *handlers) appDavx5(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Token string `json:"token"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4<<10)
	if !webx.DecodeJSON(w, r, &body) {
		return
	}
	link, ok := pending.peek(body.Token, "davx5", h.d.Clock())
	if !ok {
		webx.JSON(w, http.StatusGone, map[string]any{"error": "expired"})
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	webx.JSON(w, http.StatusOK, map[string]any{"link": string(link)})
}

// apkFile is the Android app of this server's version (PLAN.md 7.6).
func (h *handlers) apkFile(w http.ResponseWriter, r *http.Request) {
	a := assets.AndroidApp()
	if a == nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "application/vnd.android.package-archive")
	w.Header().Set("Content-Disposition", `attachment; filename="agendouille.apk"`)
	w.Header().Set("X-Content-SHA256", a.SHA256)
	http.ServeContent(w, r, "agendouille.apk", time.Time{}, bytes.NewReader(a.Body))
}

// assetLinks lets Android open this server's enrollment links in the app
// (App Links): the app's package and its signing certificate.
func (h *handlers) assetLinks(w http.ResponseWriter, r *http.Request) {
	a := assets.AndroidApp()
	if a == nil || a.Cert == "" {
		http.NotFound(w, r)
		return
	}
	webx.JSON(w, http.StatusOK, []map[string]any{{
		"relation": []string{"delegate_permission/common.handle_all_urls"},
		"target": map[string]any{"namespace": "android_app", "package_name": AndroidPackage,
			"sha256_cert_fingerprints": []string{a.Cert}},
	}})
}

// AndroidPackage is the app's package name (android/AndroidManifest.xml).
const AndroidPackage = "io.github.nebuloss.agendouille"

func enrollError(err error) (int, string) {
	switch {
	case errors.Is(err, auth.ErrUsed):
		return http.StatusGone, "error.enroll.used"
	case errors.Is(err, auth.ErrExpired):
		return http.StatusGone, "error.enroll.expired"
	case errors.Is(err, auth.ErrInvalid):
		return http.StatusNotFound, "error.enroll.invalid"
	}
	return http.StatusInternalServerError, "error.internal"
}

func (h *handlers) enroll(w http.ResponseWriter, r *http.Request) {
	secret, err := h.d.Auth.RedeemForBrowser(r.Context(), r.PathValue("token"), r.UserAgent())
	if err != nil {
		status, key := enrollError(err)
		if status == http.StatusInternalServerError {
			h.d.Log.Error("redeeming an enrollment", "err", err)
		}
		h.render(w, r, status, "app/enroll.html", "app.title.welcome", map[string]any{"Error": i18n.T(i18n.FromRequest(r), key)})
		return
	}
	auth.SetCookie(w, auth.SurfaceApp, secret, h.secure(), h.d.Settings.Get().SessionTTL())
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (h *handlers) signout(w http.ResponseWriter, r *http.Request) {
	h.d.Auth.SignOut(r, auth.SurfaceApp)
	auth.ClearCookie(w, auth.SurfaceApp, h.secure())
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func approvalError(err error) (int, string, string) {
	switch {
	case errors.Is(err, auth.ErrRefused):
		return http.StatusGone, "cancelled", "error.approval.refused"
	case errors.Is(err, auth.ErrExpired):
		return http.StatusGone, "expired", "error.approval.expired"
	case errors.Is(err, auth.ErrUsed):
		return http.StatusGone, "used", "error.approval.used"
	case errors.Is(err, auth.ErrInvalid):
		return http.StatusNotFound, "not_found", "error.approval.notFound"
	}
	return http.StatusInternalServerError, "error", "error.internal"
}

func (h *handlers) approvePage(w http.ResponseWriter, r *http.Request, _ *auth.Principal) {
	h.render(w, r, http.StatusOK, "app/approve.html", "app.title.approve", map[string]any{})
}

// approvePost is the form flow: a code alone looks the request up and asks
// to confirm; then « Se connecter » approves and « Ce n'est pas moi »
// refuses.
func (h *handlers) approvePost(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	code := strings.TrimSpace(r.PostFormValue("code"))
	data := map[string]any{"Code": code, "User": p.User.DisplayName}
	switch r.PostFormValue("decision") {
	case "refuse":
		if err := h.d.Auth.Refuse(r.Context(), p, code); err != nil {
			status, _, key := approvalError(err)
			data["Error"] = i18n.T(i18n.FromRequest(r), key)
			h.render(w, r, status, "app/approve.html", "app.title.approve", data)
			return
		}
		data["Refused"] = true
		h.render(w, r, http.StatusOK, "app/approve.html", "app.title.approve", data)
		return
	case "approve":
		if err := h.d.Auth.Approve(r.Context(), p, code); err != nil {
			status, _, key := approvalError(err)
			data["Error"] = i18n.T(i18n.FromRequest(r), key)
			h.render(w, r, status, "app/approve.html", "app.title.approve", data)
			return
		}
		data["Done"] = true
		h.render(w, r, http.StatusOK, "app/approve.html", "app.title.approve", data)
		return
	}
	view, err := h.d.Auth.LookupApproval(r.Context(), p, code)
	if err != nil {
		status, _, key := approvalError(err)
		data["Error"] = i18n.T(i18n.FromRequest(r), key)
		h.render(w, r, status, "app/approve.html", "app.title.approve", data)
		return
	}
	data["View"] = view
	h.render(w, r, http.StatusOK, "app/approve.html", "app.title.approve", data)
}

func (h *handlers) me(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	groups, err := h.d.Directory.GroupsOf(r.Context(), p.User.ID)
	if err != nil {
		webx.FailT(w, r, http.StatusInternalServerError, "error", "error.internal")
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
		status, code, key := approvalError(err)
		webx.FailT(w, r, status, code, key)
		return
	}
	webx.JSON(w, http.StatusOK, view)
}

// approvalPost confirms (the app's « Se connecter »); the body is empty.
func (h *handlers) approvalPost(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	if err := h.d.Auth.Approve(r.Context(), p, r.PathValue("code")); err != nil {
		status, code, key := approvalError(err)
		webx.FailT(w, r, status, code, key)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// approvalRefuse is « Ce n'est pas moi »: the waiting browser starts over.
func (h *handlers) approvalRefuse(w http.ResponseWriter, r *http.Request, p *auth.Principal) {
	if err := h.d.Auth.Refuse(r.Context(), p, r.PathValue("code")); err != nil {
		status, code, key := approvalError(err)
		webx.FailT(w, r, status, code, key)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// appEnroll is the Android app redeeming the admin's enrollment link it was
// opened with (PLAN.md 7.2). The answer carries the device credential, once.
func (h *handlers) appEnroll(w http.ResponseWriter, r *http.Request) {
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
		status, key := enrollError(err)
		code := map[int]string{http.StatusGone: "used", http.StatusNotFound: "not_found"}[status]
		if errors.Is(err, auth.ErrExpired) {
			code = "expired"
		}
		var args []string
		if errors.Is(err, auth.ErrTooMany) {
			e := h.explain(err)
			status, code, key, args = e.status, e.code, e.key, e.args
		} else if status == http.StatusInternalServerError {
			h.d.Log.Error("enrolling the app", "err", err)
			code = "error"
		}
		webx.FailT(w, r, status, code, key, args...)
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
		webx.FailT(w, r, http.StatusForbidden, "forbidden", "error.app.sessionOnly")
		return
	}
	auth.SetCookie(w, auth.SurfaceApp, secret, h.secure(), h.d.Settings.Get().SessionTTL())
	w.WriteHeader(http.StatusNoContent)
}

func (h *handlers) appConfig(w http.ResponseWriter, _ *http.Request, _ *auth.Principal) {
	apkSHA, apkVersion := "", ""
	if a := assets.AndroidApp(); a != nil {
		apkSHA, apkVersion = a.SHA256, a.Version
	}
	webx.JSON(w, http.StatusOK, map[string]any{"server_version": h.d.Version, "min_app_version": "0.0.0",
		"apk_sha256": apkSHA, "apk_version": apkVersion, "bridge": 1, "sync_interval_s": h.d.Settings.Get().AppSyncIntervalMinutes * 60})
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

// appCrash is the Android app's report of what made it stop (CrashLog),
// written to the server's log for the administrator: a phone's own log is
// out of their reach. No credential (the app may not be signed in yet), and
// small: 16 KB at most, one log line.
func (h *handlers) appCrash(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Report     string `json:"report"`
		AppVersion string `json:"app_version"`
		Model      string `json:"model"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
	if !webx.DecodeJSON(w, r, &body) {
		return
	}
	h.d.Log.Warn("android app crash", "ip", h.clientIP(r), "app_version", truncateText(body.AppVersion, 40),
		"model", truncateText(body.Model, 80), "report", truncateText(body.Report, 12000))
	w.WriteHeader(http.StatusNoContent)
}

func truncateText(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}
