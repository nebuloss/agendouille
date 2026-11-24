package admin

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/nebuloss/agendouille/internal/auth"
	"github.com/nebuloss/agendouille/internal/web/webx"
)

// The admin sets a device up for someone (DECISIONS: devices from the
// admin): a new device with its manual settings (address, username,
// password, shown once), in the audit.

// addDevice is a new device for a user, made by the admin.
func (h *handlers) addDevice(w http.ResponseWriter, r *http.Request) {
	u, ok := h.lookupUser(w, r)
	if !ok {
		return
	}
	var body struct {
		Kind  string `json:"kind"`
		Label string `json:"label"`
		Cap   string `json:"cap"`
	}
	if !decode(w, r, &body) {
		return
	}
	if body.Cap == "" {
		body.Cap = "readwrite"
	}
	ctx := r.Context()
	d, err := h.d.Auth.NewDevice(ctx, u.ID, body.Kind, body.Label, body.Cap)
	if err != nil {
		if errors.Is(err, auth.ErrTooMany) || errors.Is(err, auth.ErrInvalid) {
			h.failIn(w, r, err)
		} else if strings.TrimSpace(body.Label) == "" || len(body.Label) > 100 {
			h.failIn(w, r, validation{key: "error.device.label"})
		} else {
			h.failIn(w, r, validation{key: "error.device.invalid"})
		}
		return
	}
	_ = h.d.Auth.Audit(ctx, "admin", "device", "added", fmt.Sprintf("%s: %s (%s)", u.Name, d.Label, d.Kind))
	base := h.d.PublicURL()
	webx.JSON(w, http.StatusCreated, map[string]any{"id": d.ID, "kind": d.Kind, "label": d.Label, "cap": d.Cap,
		"server": base + "/dav/", "username": d.Username, "secret": d.Secret})
}
