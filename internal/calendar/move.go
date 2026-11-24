package calendar

import (
	"context"
	"database/sql"
	"errors"

	"github.com/nebuloss/agendouille/internal/store"
)

// Move puts an object in another calendar, in one transaction: the web
// app's calendar picker on an existing event. CalDAV clients do it
// themselves with a DELETE and a PUT.
func (s *Service) Move(ctx context.Context, a Actor, from Ref, name string, to Ref, p Precondition) (Object, error) {
	src, err := s.lookup(ctx, a, from)
	if err != nil {
		return Object{}, err
	}
	dst, err := s.lookup(ctx, a, to)
	if err != nil {
		return Object{}, err
	}
	if !src.Access.CanWrite() || !dst.Access.CanWrite() {
		return Object{}, ErrForbidden
	}
	if src.ID == dst.ID {
		_, o, err := s.Get(ctx, a, from, name)
		return o, err
	}
	var o Object
	var seqFrom, seqTo int64
	err = store.InTx(ctx, s.db, func(tx *sql.Tx) error {
		if err := s.recheck(ctx, tx, a, src, false); err != nil {
			return err
		}
		if err := s.recheck(ctx, tx, a, dst, false); err != nil {
			return err
		}
		var err error
		o, err = scanObject(tx.QueryRowContext(ctx, `SELECT `+objCols+` FROM objects WHERE calendar_id = ? AND name = ?`, src.ID, name))
		if err != nil {
			return err
		}
		if err := p.check(true, o.ETag); err != nil {
			return err
		}
		var holder string
		err = tx.QueryRowContext(ctx, `SELECT name FROM objects WHERE calendar_id = ? AND (uid = ? OR name = ?)`, dst.ID, o.UID, name).Scan(&holder)
		if err == nil {
			return &UIDConflict{Name: holder}
		} else if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		var n int
		if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM objects WHERE calendar_id = ?`, dst.ID).Scan(&n); err != nil {
			return err
		}
		if limit := s.settings.Get().MaxEventsPerCalendar; n >= limit {
			return &QuotaError{Setting: "max_events_per_calendar", Limit: limit}
		}
		var dev any
		if a.DeviceID != 0 {
			dev = a.DeviceID
		}
		by := a.UserID
		if by == 0 {
			by = dst.OwnerID
		}
		if _, err := tx.ExecContext(ctx, `UPDATE objects SET calendar_id = ?, modified_by = ?, modified_via = ?, device_id = ?, modified_at = ?
			WHERE calendar_id = ? AND name = ?`, dst.ID, by, a.Via, dev, store.Time(s.now()), src.ID, name); err != nil {
			return err
		}
		if seqFrom, err = s.change(ctx, tx, a, src.ID, name, o.Summary, "deleted"); err != nil {
			return err
		}
		seqTo, err = s.change(ctx, tx, a, dst.ID, name, o.Summary, "created")
		return err
	})
	if err != nil {
		return Object{}, err
	}
	s.publish(src, seqFrom, name, "deleted", a)
	s.publish(dst, seqTo, name, "created", a)
	return o, nil
}
