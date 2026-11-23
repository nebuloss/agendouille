// Package deps is what every HTTP package is handed (CONTRACTS.md 1.2). It
// exists so packages that register routes need not import each other.
package deps

import (
	"log/slog"
	"net/netip"
	"time"

	"github.com/nebuloss/agendouille/internal/auth"
	"github.com/nebuloss/agendouille/internal/calendar"
	"github.com/nebuloss/agendouille/internal/config"
	"github.com/nebuloss/agendouille/internal/directory"
	"github.com/nebuloss/agendouille/internal/settings"
	"github.com/nebuloss/agendouille/internal/store"
)

// Deps is filled once by serve.
type Deps struct {
	Config    *config.Config
	Settings  *settings.Service
	Log       *slog.Logger
	Store     *store.Store
	Directory *directory.Service
	Auth      *auth.Service
	Calendar  *calendar.Service
	Clock     func() time.Time
	Version   string
}

// Trusted is the trusted_proxies setting, now.
func (d *Deps) Trusted() []netip.Prefix { return d.Settings.Get().TrustedPrefixes() }

// PublicURL is the public_url setting, now, without a trailing slash.
func (d *Deps) PublicURL() string {
	u := d.Settings.Get().PublicURL
	for len(u) > 0 && u[len(u)-1] == '/' {
		u = u[:len(u)-1]
	}
	return u
}
