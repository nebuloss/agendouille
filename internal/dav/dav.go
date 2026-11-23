// Package dav serves the dav surface: CalDAV under /dav/, its discovery at
// /.well-known/caldav, and the read-only subscriptions at /ics/. In M1 it
// only checks device credentials; CalDAV itself is M2 (PLAN.md 5).
package dav

import (
	"net/http"

	"github.com/nebuloss/agendouille/internal/auth"
	"github.com/nebuloss/agendouille/internal/deps"
)

// Register adds the dav surface to mux.
func Register(mux *http.ServeMux, d *deps.Deps) {
	mux.HandleFunc("/.well-known/caldav", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/dav/", http.StatusMovedPermanently)
	})
	mux.HandleFunc("/dav/", func(w http.ResponseWriter, r *http.Request) {
		if _, err := d.Auth.Basic(r, auth.ClientIP(r, d.Trusted()).String()); err != nil {
			w.Header().Set("WWW-Authenticate", `Basic realm="agendouille", charset="UTF-8"`)
			http.Error(w, "a device credential is needed", http.StatusUnauthorized)
			return
		}
		// Signed in: CalDAV is not built yet.
		http.Error(w, "CalDAV arrives with M2", http.StatusNotImplemented)
	})
}
