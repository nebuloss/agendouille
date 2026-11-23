// Package admin serves the admin surface, on the admin listener only. In
// M1 that is the password sign-in and a placeholder home; the admin UI
// itself is M3.
package admin

import (
	"errors"
	"net/http"

	"github.com/nebuloss/agendouille/internal/auth"
	"github.com/nebuloss/agendouille/internal/deps"
	"github.com/nebuloss/agendouille/internal/web/assets"
	"github.com/nebuloss/agendouille/internal/web/webx"
)

type handlers struct{ d *deps.Deps }

// Register adds the admin surface to mux, which must serve nothing else.
func Register(mux *http.ServeMux, d *deps.Deps) error {
	if err := assets.Register(mux, assets.Admin); err != nil {
		return err
	}
	h := &handlers{d: d}
	guard := func(next http.Handler) http.Handler { return webx.SameOrigin(d.Config.AdminURL, next) }
	// The admin listener serves the admin surface alone, so its pages sit
	// at the root, with no /admin prefix (the user's call).
	mux.HandleFunc("GET /signin", h.signinPage)
	mux.Handle("POST /signin", guard(http.HandlerFunc(h.signin)))
	mux.Handle("POST /signout", guard(http.HandlerFunc(h.signout)))
	mux.Handle("GET /{$}", h.page(h.shell("overview", "Vue d'ensemble")))
	for path, page := range map[string][2]string{
		"GET /users":        {"users", "Utilisateurs"},
		"GET /users/{name}": {"user", "Utilisateur"},
		"GET /groups":       {"groups", "Groupes"},
		"GET /activity":     {"activity", "Activité"},
		"GET /settings":     {"settings", "Réglages"},
		"GET /backups":      {"backups", "Sauvegardes"},
	} {
		mux.Handle(path, h.page(h.shell(page[0], page[1])))
	}
	// Calendars are under their owner now, as on claudication's pages: an
	// old link lands on the users.
	mux.Handle("GET /calendars", h.page(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/users", http.StatusSeeOther)
	}))
	// Links from before (/admin, /admin/users/lea, a bookmark) land on the
	// same page without the prefix.
	legacy := func(w http.ResponseWriter, r *http.Request) {
		to := "/" + r.PathValue("rest")
		if to == "/calendars" {
			to = "/users"
		}
		http.Redirect(w, r, to, http.StatusMovedPermanently)
	}
	mux.HandleFunc("GET /admin", legacy)
	mux.HandleFunc("GET /admin/{rest...}", legacy)
	h.registerAPI(mux, guard)
	return nil
}

// shell is a page of the admin UI: the layout, the navigation, and a
// mount point the admin bundle fills from /api/admin (PLAN.md 9.4).
func (h *handlers) shell(page, title string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		webx.Render(w, h.d.Log, http.StatusOK, "admin/shell.html", webx.Page{Title: title, Page: page, Bundle: assets.Admin,
			Data: map[string]any{"Page": page, "Title": title, "Name": r.PathValue("name"), "Version": h.d.Version}})
	}
}

func (h *handlers) secure() bool { return webx.Secure(h.d.Config.AdminURL()) }

func (h *handlers) render(w http.ResponseWriter, status int, name, page string, data any) {
	webx.Render(w, h.d.Log, status, name, webx.Page{Title: "Administration", Page: page, Bundle: assets.Admin, Data: data})
}

func (h *handlers) page(next http.HandlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, err := h.d.Auth.Session(r, auth.SurfaceAdmin)
		if err != nil || !p.Admin {
			http.Redirect(w, r, "/signin", http.StatusSeeOther)
			return
		}
		next(w, r.WithContext(auth.WithPrincipal(r.Context(), p)))
	})
}

func (h *handlers) signinPage(w http.ResponseWriter, _ *http.Request) {
	h.render(w, http.StatusOK, "admin/signin.html", "signin", map[string]any{})
}

func (h *handlers) signin(w http.ResponseWriter, r *http.Request) {
	secret, err := h.d.Auth.AdminSignIn(r.Context(), r.PostFormValue("password"), r.UserAgent())
	if err != nil {
		status, msg := http.StatusInternalServerError, "Une erreur est survenue."
		if errors.Is(err, auth.ErrWrongPassword) {
			status, msg = http.StatusUnauthorized, "Mot de passe incorrect."
			h.d.Log.Warn("admin sign-in refused", "ip", auth.ClientIP(r, nil).String())
		} else {
			h.d.Log.Error("admin sign-in", "err", err)
		}
		h.render(w, status, "admin/signin.html", "signin", map[string]any{"Error": msg})
		return
	}
	auth.SetCookie(w, auth.SurfaceAdmin, secret, h.secure(), 0)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (h *handlers) signout(w http.ResponseWriter, r *http.Request) {
	h.d.Auth.SignOut(r, auth.SurfaceAdmin)
	auth.ClearCookie(w, auth.SurfaceAdmin, h.secure())
	http.Redirect(w, r, "/signin", http.StatusSeeOther)
}
