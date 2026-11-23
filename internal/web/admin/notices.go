package admin

import (
	"context"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"time"

	"github.com/nebuloss/agendouille/internal/store"
	"github.com/nebuloss/agendouille/internal/web/webx"
)

// The notification centre, as claudication's and cafard's: what needs the
// admin now, worked out from the state on every read, each a link to the
// page where it is dealt with. A notice marked read leaves the bell until
// its problem is gone; the mark is forgotten then.

type notice struct {
	ID       string     `json:"id"`
	Kind     string     `json:"kind"`     // server, device, user
	Severity string     `json:"severity"` // error, warn, info
	Title    string     `json:"title"`
	Detail   string     `json:"detail,omitempty"`
	Link     string     `json:"link,omitempty"`
	Since    *time.Time `json:"since,omitempty"`
	Read     bool       `json:"read"`
}

var warningNotice = map[string]struct{ severity, link string }{
	"no_public_listener":   {"error", "/admin/settings"},
	"public_listener_down": {"error", "/admin/settings"},
	"admin_insecure":       {"warn", "/admin/settings"},
	"backup_old":           {"warn", "/admin/backups"},
}

// currentNotices is every current notice, newest problem first within a severity.
func (h *handlers) currentNotices(r *http.Request) ([]notice, error) {
	ctx := r.Context()
	out := []notice{}
	for _, w := range h.warnings(r) {
		n := warningNotice[w.Code]
		if n.severity == "" {
			n.severity = "warn"
		}
		out = append(out, notice{ID: "server:" + w.Code, Kind: "server", Severity: n.severity, Title: w.Message, Link: n.link})
	}
	stale, err := h.d.Auth.StaleDevices(ctx)
	if err != nil {
		return nil, err
	}
	for _, d := range stale {
		title := "« " + d.Label + " » ne s'est pas synchronisé depuis une semaine"
		if d.LastUsedAt == nil {
			title = "« " + d.Label + " » ne s'est jamais synchronisé"
		}
		out = append(out, notice{ID: "device:" + strconv.FormatInt(d.ID, 10), Kind: "device", Severity: "warn", Title: title,
			Detail: "Appareil de " + d.User + " : vérifiez qu'il est allumé et que son mot de passe est le bon, ou retirez-le.",
			Link:   "/admin/users/" + url.PathEscape(d.User), Since: d.LastUsedAt})
	}
	users, err := h.d.Directory.Users(ctx)
	if err != nil {
		return nil, err
	}
	for _, u := range users {
		if u.Disabled || u.LastSeenAt != nil {
			continue
		}
		created := u.CreatedAt
		out = append(out, notice{ID: "user:never:" + u.Name, Kind: "user", Severity: "info",
			Title:  u.DisplayName + " ne s'est jamais connecté",
			Detail: "Donnez-lui un QR de connexion depuis sa page.",
			Link:   "/admin/users/" + url.PathEscape(u.Name), Since: &created})
	}
	rank := map[string]int{"error": 0, "warn": 1, "info": 2}
	sort.SliceStable(out, func(a, b int) bool { return rank[out[a].Severity] < rank[out[b].Severity] })
	return out, nil
}

// marks reads the read marks, forgetting those whose notice is gone.
func (h *handlers) marks(ctx context.Context, current []notice) (map[string]bool, error) {
	db := h.d.Store.DB()
	rows, err := db.QueryContext(ctx, `SELECT id FROM notice_reads`)
	if err != nil {
		return nil, err
	}
	read := map[string]bool{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		read[id] = true
	}
	rows.Close()
	live := map[string]bool{}
	for _, n := range current {
		live[n.ID] = true
	}
	for id := range read {
		if !live[id] {
			if _, err := db.ExecContext(ctx, `DELETE FROM notice_reads WHERE id = ?`, id); err != nil {
				return nil, err
			}
			delete(read, id)
		}
	}
	return read, nil
}

func (h *handlers) notices(w http.ResponseWriter, r *http.Request) {
	ns, err := h.currentNotices(r)
	if err != nil {
		h.fail(w, err)
		return
	}
	read, err := h.marks(r.Context(), ns)
	if err != nil {
		h.fail(w, err)
		return
	}
	for i := range ns {
		ns[i].Read = read[ns[i].ID]
	}
	webx.JSON(w, http.StatusOK, map[string]any{"notices": ns})
}

func (h *handlers) readNotices(w http.ResponseWriter, r *http.Request) {
	var body struct {
		IDs []string `json:"ids"`
	}
	if !decode(w, r, &body) {
		return
	}
	ns, err := h.currentNotices(r)
	if err != nil {
		h.fail(w, err)
		return
	}
	live := map[string]bool{}
	for _, n := range ns {
		live[n.ID] = true
	}
	now := store.Time(h.d.Clock())
	for _, id := range body.IDs {
		if !live[id] {
			continue // gone already, or never was: nothing to remember
		}
		if _, err := h.d.Store.DB().ExecContext(r.Context(), `INSERT INTO notice_reads (id, read_at) VALUES (?, ?)
			ON CONFLICT (id) DO UPDATE SET read_at = excluded.read_at`, id, now); err != nil {
			h.fail(w, err)
			return
		}
	}
	h.notices(w, r)
}
