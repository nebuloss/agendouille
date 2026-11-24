package auth

import (
	"context"

	"github.com/nebuloss/agendouille/internal/store"
)

// Audit records an action in the audit log (the admin's, done here
// rather than through a service of its own).
func (s *Service) Audit(ctx context.Context, actor, kind, action, detail string) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO audit(at, actor, kind, action, detail) VALUES (?, ?, ?, ?, ?)`,
		store.Time(s.now()), actor, kind, action, detail)
	return err
}

// RandomToken is a random string for a one-time link.
func RandomToken() string { return newToken(24) }
