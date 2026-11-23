// Package webx is the plumbing the family web app and the admin UI share:
// page rendering in the layout, JSON answers and errors, the security
// headers, the same-origin guard, and QR codes.
package webx

import (
	"bytes"
	"encoding/json"
	"html/template"
	"log/slog"
	"net/http"
	"strings"
	"sync"

	"github.com/nebuloss/agendouille/internal/auth"
	"github.com/nebuloss/agendouille/internal/web/assets"
)

// Page is the data every page template gets.
type Page struct {
	Title  string
	Page   string // <body data-page>, picks the TypeScript page
	Bundle assets.Bundle
	Data   any
	// Theme is the reader's choice (ThemeCookie): "light", "dark", or ""
	// to follow the system. Render sets it.
	Theme string
}

// ThemeCookie holds the reader's choice of theme, set by the page
// (src/shared/core/Theme.ts) and read here so the first paint is right.
const ThemeCookie = "agd_theme"

func themeOf(r *http.Request) string {
	if c, err := r.Cookie(ThemeCookie); err == nil && (c.Value == "light" || c.Value == "dark") {
		return c.Value
	}
	return ""
}

// Stylesheet is the bundle's stylesheet, or "" without the bundle.
func (p Page) Stylesheet() string {
	if assets.Built(p.Bundle) {
		return strings.TrimSuffix(p.Bundle.URL(), ".js") + ".css"
	}
	return ""
}

// Script is the bundle's URL, or "" when this binary was built without it.
func (p Page) Script() string {
	if assets.Built(p.Bundle) {
		return p.Bundle.URL()
	}
	return ""
}

var (
	tmplOnce sync.Once
	tmpl     *template.Template
	tmplErr  error
)

// funcs are what the templates may call.
var funcs = template.FuncMap{
	// railItem is one entry of the admin UI's rail: where, what, its icon's
	// path, and whether it is the page.
	"railItem": func(href, label, icon string, current bool) map[string]any {
		return map[string]any{"Href": href, "Label": label, "Icon": icon, "Current": current}
	},
}

// Templates parses the embedded templates once.
func Templates() (*template.Template, error) {
	tmplOnce.Do(func() { tmpl, tmplErr = assets.Templates(funcs) })
	return tmpl, tmplErr
}

// Render executes the template name (which defines "content") inside
// layout.html. It renders to a buffer first, so a template error is a 500,
// not half a page.
func Render(w http.ResponseWriter, r *http.Request, log *slog.Logger, status int, name string, p Page) {
	p.Theme = themeOf(r)
	t, err := Templates()
	if err == nil {
		t, err = t.Clone()
	}
	if err == nil {
		// The page template gets Data as its dot, so it never needs a
		// {{with .Data}}: an empty map is false there, and the page vanished.
		_, err = t.New("content-holder").Parse(`{{define "content"}}{{template "` + name + `" .Data}}{{end}}`)
	}
	var buf bytes.Buffer
	if err == nil {
		err = t.ExecuteTemplate(&buf, "layout.html", p)
	}
	if err != nil {
		log.Error("rendering a page", "template", name, "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_, _ = w.Write(buf.Bytes())
}

// JSON writes v with status.
func JSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// Error is the API error body (CONTRACTS.md 3.1).
type Error struct {
	Code    string `json:"error"`
	Message string `json:"message,omitempty"`
	Field   string `json:"field,omitempty"`
}

// Fail writes an API error.
func Fail(w http.ResponseWriter, status int, code, message string) {
	JSON(w, status, Error{Code: code, Message: message})
}

// DecodeJSON reads a small JSON body into v, refusing unknown fields.
func DecodeJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		Fail(w, http.StatusBadRequest, "bad_request", "the body is not the JSON this route takes: "+err.Error())
		return false
	}
	return true
}

// Headers sets the security headers on every answer (PLAN.md 10). Images
// may be data: URLs, for the QR codes; nothing else comes from anywhere
// but this origin, and no script runs inline.
func Headers(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Security-Policy", "default-src 'self'; img-src 'self' data:; object-src 'none'; base-uri 'none'; form-action 'self'; frame-ancestors 'none'")
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "same-origin")
		h.Set("X-Frame-Options", "DENY")
		next.ServeHTTP(w, r)
	})
}

// SameOrigin refuses state-changing requests that did not come from this
// surface's own pages (PLAN.md 10). url gives the surface's public URL at
// the time of the request, since it is a live setting.
func SameOrigin(url func() string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
		default:
			if !auth.SameOrigin(r, url()) {
				Fail(w, http.StatusForbidden, "cross_origin", "this request did not come from this site's own pages")
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// Secure reports whether cookies for a surface reached at url must be
// Secure.
func Secure(url string) bool { return strings.HasPrefix(url, "https://") }
