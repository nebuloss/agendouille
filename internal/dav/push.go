package dav

import (
	"errors"
	"net/http"
	"strings"

	"github.com/nebuloss/agendouille/internal/push"
)

// WebDAV-Push (draft-bitfire-webdav-push-00, PLAN.md M9): a device POSTs a
// push-register to a calendar, gets its registration URL in Location, and
// DELETEs it when done. internal/push keeps them and sends the messages.

// pushRegister answers a POST to a calendar: a push-register body.
func (c *call) pushRegister() {
	ps := c.s.d.Push
	if c.t.kind != kCalendar || ps == nil {
		c.w.Header().Set("Allow", allow)
		http.Error(c.w, "not supported", http.StatusMethodNotAllowed)
		return
	}
	i, err := c.info()
	if err != nil {
		c.fail(err)
		return
	}
	root, err := parse(http.MaxBytesReader(c.w, c.r.Body, maxBody))
	if err != nil || root == nil || !root.is(nsPush, "push-register") {
		http.Error(c.w, "a push-register body is expected", http.StatusBadRequest)
		return
	}
	sub := push.Subscription{UserID: c.p.User.ID, DeviceID: c.p.DeviceID, CalendarID: i.ID}
	if wp := root.child(nsPush, "subscription").child(nsPush, "web-push-subscription"); wp != nil {
		sub.Resource = text(wp.child(nsPush, "push-resource"))
		sub.PublicKey = text(wp.child(nsPush, "subscription-public-key"))
		sub.AuthSecret = text(wp.child(nsPush, "auth-secret"))
		if enc := text(wp.child(nsPush, "content-encoding")); enc != "" && enc != "aes128gcm" {
			c.xmlError(http.StatusForbidden, nsPush, "invalid-subscription", "")
			return
		}
		if k := wp.child(nsPush, "subscription-public-key"); k != nil && k.attr("type") != "" && k.attr("type") != "p256dh" {
			c.xmlError(http.StatusForbidden, nsPush, "invalid-subscription", "")
			return
		}
	} else {
		c.xmlError(http.StatusForbidden, nsPush, "invalid-subscription", "")
		return
	}
	// Any depth asked is served at the one supported (members, the
	// calendar's own properties): the draft's downgrading.
	if tr := root.child(nsPush, "trigger"); tr != nil {
		sub.Content = tr.child(nsPush, "content-update") != nil
		sub.Property = tr.child(nsPush, "property-update") != nil
	}
	if e := text(root.child(nsPush, "expires")); e != "" {
		if t, err := http.ParseTime(e); err == nil {
			sub.Expires = t
		}
	}
	id, expires, err := ps.Register(c.r.Context(), sub)
	switch {
	case errors.Is(err, push.ErrNoTrigger):
		c.xmlError(http.StatusForbidden, nsPush, "no-supported-trigger", "")
		return
	case errors.Is(err, push.ErrInvalid):
		c.xmlError(http.StatusForbidden, nsPush, "invalid-subscription", "")
		return
	case errors.Is(err, push.ErrTooMany):
		c.xmlError(http.StatusForbidden, nsPush, "push-not-available", "")
		return
	case err != nil:
		c.fail(err)
		return
	}
	c.w.Header().Set("Location", ps.URL(id))
	c.w.Header().Set("Expires", expires.UTC().Format(http.TimeFormat))
	c.w.WriteHeader(http.StatusNoContent)
}

// pushUnregister answers a DELETE on a registration URL.
func (c *call) pushUnregister() {
	ps := c.s.d.Push
	if ps == nil {
		http.NotFound(c.w, c.r)
		return
	}
	ok, err := ps.Unregister(c.r.Context(), c.p.User.ID, c.t.name)
	if err != nil {
		c.fail(err)
		return
	}
	if !ok {
		http.NotFound(c.w, c.r)
		return
	}
	c.w.WriteHeader(http.StatusNoContent)
}

// dontNotify reads Push-Dont-Notify: quoted registration URLs, or "*".
func dontNotify(values []string) []string {
	var out []string
	for _, v := range values {
		for _, part := range strings.Split(v, ",") {
			part = strings.Trim(strings.TrimSpace(part), `"`)
			if part != "" {
				out = append(out, part)
			}
		}
	}
	return out
}

func text(n *node) string {
	if n == nil {
		return ""
	}
	return strings.TrimSpace(n.Text)
}
