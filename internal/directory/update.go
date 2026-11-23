package directory

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/nebuloss/agendouille/internal/store"
)

// UpdateUser changes a user's display name and colour; empty strings are
// left as they are.
func (s *Service) UpdateUser(ctx context.Context, actor, name, displayName, color string) error {
	sets, args := []string{}, []any{}
	if displayName != "" {
		d, err := checkDisplay(displayName)
		if err != nil {
			return err
		}
		sets, args = append(sets, "display_name = ?"), append(args, d)
	}
	if color != "" {
		if !colorRule.MatchString(color) {
			return badColor(color)
		}
		sets, args = append(sets, "color = ?"), append(args, color)
	}
	if len(sets) == 0 {
		return nil
	}
	return store.InTx(ctx, s.db, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, `UPDATE users SET `+strings.Join(sets, ", ")+` WHERE name = ? AND removed_at IS NULL`,
			append(args, name)...)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return ErrNotFound
		}
		return audit(ctx, tx, s.now(), actor, "user", "changed", name)
	})
}

// SetGroups makes a user's groups exactly these.
func (s *Service) SetGroups(ctx context.Context, actor, user string, groups []string) error {
	return store.InTx(ctx, s.db, func(tx *sql.Tx) error {
		var uid int64
		err := tx.QueryRowContext(ctx, `SELECT id FROM users WHERE name = ? AND removed_at IS NULL`, user).Scan(&uid)
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("user %q: %w", user, ErrNotFound)
		} else if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM group_members WHERE user_id = ?`, uid); err != nil {
			return err
		}
		for _, g := range groups {
			if err := addMember(ctx, tx, g, uid); err != nil {
				return err
			}
		}
		sorted := append([]string(nil), groups...)
		sort.Strings(sorted)
		return audit(ctx, tx, s.now(), actor, "user", "groups", user+" "+strings.Join(sorted, ","))
	})
}

// UpdateGroup changes a group's display name and colour.
func (s *Service) UpdateGroup(ctx context.Context, actor, name, displayName, color string) error {
	sets, args := []string{}, []any{}
	if displayName != "" {
		d, err := checkDisplay(displayName)
		if err != nil {
			return err
		}
		sets, args = append(sets, "display_name = ?"), append(args, d)
	}
	if color != "" {
		if !colorRule.MatchString(color) {
			return badColor(color)
		}
		sets, args = append(sets, "color = ?"), append(args, color)
	}
	if len(sets) == 0 {
		return nil
	}
	return store.InTx(ctx, s.db, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, `UPDATE groups SET `+strings.Join(sets, ", ")+` WHERE name = ?`, append(args, name)...)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return ErrNotFound
		}
		return audit(ctx, tx, s.now(), actor, "group", "changed", name)
	})
}

// GroupShares counts the calendar shares that name a group: deleting it
// removes them, so the admin UI asks first.
func (s *Service) GroupShares(ctx context.Context, name string) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT count(*) FROM shares WHERE group_id = (SELECT id FROM groups WHERE name = ?)`, name).Scan(&n)
	return n, err
}
