package auth

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"math/big"
	"time"

	"github.com/nebuloss/agendouille/internal/store"
)

// ApprovalTTL is how long a browser waits for someone to approve it.
const ApprovalTTL = 2 * time.Minute

// maxPending caps the sign-ins waiting at once. /signin is open to anyone
// on the internet and each visit stores a row: this keeps the table small
// whatever happens. A family never has more than a handful waiting.
const maxPending = 200

// Approval is a browser's request to sign in (PLAN.md 8.2), as the waiting
// page shows it. Code goes in the QR code, in the URL fragment; ShortCode
// is typed instead when no camera can read the screen; Number is what the
// approver must pick.
type Approval struct {
	ID        string    `json:"id"`
	Code      string    `json:"-"`
	ShortCode string    `json:"short_code"`
	Number    int       `json:"number"`
	ExpiresAt time.Time `json:"expires_at"`
}

// ApprovalView is what the approving device shows before saying yes.
type ApprovalView struct {
	UserAgent string    `json:"user_agent"`
	IP        string    `json:"ip"`
	Numbers   []int     `json:"numbers"`
	ExpiresAt time.Time `json:"expires_at"`
}

// StartApproval records a browser that wants to sign in.
func (s *Service) StartApproval(ctx context.Context, userAgent, ip string) (Approval, error) {
	now := s.now()
	if _, err := s.db.ExecContext(ctx, `DELETE FROM approvals WHERE expires_at < ?`, store.Time(now.Add(-time.Hour))); err != nil {
		return Approval{}, err
	}
	var pending int
	if err := s.db.QueryRowContext(ctx, `SELECT count(*) FROM approvals WHERE used_at IS NULL AND failed = 0 AND expires_at > ?`,
		store.Time(now)).Scan(&pending); err != nil {
		return Approval{}, err
	}
	if pending >= maxPending {
		return Approval{}, ErrBusy
	}
	idb := make([]byte, 16)
	if _, err := rand.Read(idb); err != nil {
		panic("crypto/rand: " + err.Error())
	}
	a := Approval{ID: hex.EncodeToString(idb), Code: newToken(32), ShortCode: newShortCode(), Number: randomInt(10, 99),
		ExpiresAt: now.Add(ApprovalTTL)}
	_, err := s.db.ExecContext(ctx, `INSERT INTO approvals(id, code_hash, short_code_hash, number, user_agent, ip, created_at, expires_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, a.ID, hash(a.Code), hash(a.ShortCode), a.Number, truncate(userAgent, 200), ip,
		store.Time(now), store.Time(a.ExpiresAt))
	if err != nil {
		// A short-code collision among the few pending ones: try once more.
		a.ShortCode = newShortCode()
		_, err = s.db.ExecContext(ctx, `INSERT INTO approvals(id, code_hash, short_code_hash, number, user_agent, ip, created_at, expires_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, a.ID, hash(a.Code), hash(a.ShortCode), a.Number, truncate(userAgent, 200), ip,
			store.Time(now), store.Time(a.ExpiresAt))
	}
	return a, err
}

type pendingApproval struct {
	id      string
	number  int
	ua, ip  string
	expires time.Time
}

// pending finds a waiting approval by its code or its short code.
func (s *Service) pending(ctx context.Context, code string) (pendingApproval, error) {
	var p pendingApproval
	var expires string
	err := s.db.QueryRowContext(ctx, `SELECT id, number, user_agent, ip, expires_at FROM approvals
		WHERE (code_hash = ? OR short_code_hash = ?) AND used_at IS NULL AND approved_by IS NULL AND failed = 0`,
		hash(code), hash(normalizeShortCode(code))).Scan(&p.id, &p.number, &p.ua, &p.ip, &expires)
	if errors.Is(err, sql.ErrNoRows) {
		return p, ErrInvalid
	} else if err != nil {
		return p, err
	}
	p.expires, _ = store.ParseTime(expires)
	if !s.now().Before(p.expires) {
		return p, ErrExpired
	}
	return p, nil
}

// LookupApproval is what a signed-in user's device shows for a code it
// scanned or was given: the waiting browser, and three numbers in random
// order, one of them right.
func (s *Service) LookupApproval(ctx context.Context, p *Principal, code string) (ApprovalView, error) {
	if p == nil || p.User.ID == 0 {
		return ApprovalView{}, ErrInvalid
	}
	a, err := s.pending(ctx, code)
	if err != nil {
		return ApprovalView{}, err
	}
	numbers := []int{a.number}
	for len(numbers) < 3 {
		n := randomInt(10, 99)
		if n != numbers[0] && (len(numbers) < 2 || n != numbers[1]) {
			numbers = append(numbers, n)
		}
	}
	for i := len(numbers) - 1; i > 0; i-- {
		j, _ := rand.Int(rand.Reader, big.NewInt(int64(i+1)))
		numbers[i], numbers[j.Int64()] = numbers[j.Int64()], numbers[i]
	}
	return ApprovalView{UserAgent: a.ua, IP: a.ip, Numbers: numbers, ExpiresAt: a.expires}, nil
}

// Approve signs the waiting browser in as p's user when number is the one
// it shows. A wrong number cancels the request: the browser must start
// over, so a code cannot be tried three ways.
func (s *Service) Approve(ctx context.Context, p *Principal, code string, number int) error {
	if p == nil || p.User.ID == 0 || p.Surface != SurfaceApp {
		return ErrInvalid
	}
	a, err := s.pending(ctx, code)
	if err != nil {
		return err
	}
	if number != a.number {
		_, err := s.db.ExecContext(ctx, `UPDATE approvals SET failed = 1 WHERE id = ?`, a.id)
		s.wake(a.id)
		if err != nil {
			return err
		}
		return ErrWrongNumber
	}
	res, err := s.db.ExecContext(ctx, `UPDATE approvals SET approved_by = ? WHERE id = ? AND approved_by IS NULL AND failed = 0`, p.User.ID, a.id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrUsed
	}
	_, _ = s.db.ExecContext(ctx, `INSERT INTO audit(at, actor, kind, action, detail) VALUES (?, ?, 'approval', 'approved', ?)`,
		store.Time(s.now()), p.User.Name, a.ua)
	s.wake(a.id)
	return nil
}

func (s *Service) waiter(id string) chan struct{} {
	s.mu.Lock()
	defer s.mu.Unlock()
	ch, ok := s.waiters[id]
	if !ok {
		ch = make(chan struct{})
		s.waiters[id] = ch
	}
	return ch
}

func (s *Service) wake(id string) {
	s.mu.Lock()
	if ch, ok := s.waiters[id]; ok {
		close(ch)
		delete(s.waiters, id)
	}
	s.mu.Unlock()
}

// WaitApproval blocks until the approval id is decided, expires, or ctx
// ends (the long-poll's own timeout). Once approved it opens the session,
// once: the secret is returned to the first caller only.
func (s *Service) WaitApproval(ctx context.Context, id, userAgent string) (string, error) {
	for {
		ch := s.waiter(id)
		secret, done, err := s.collect(ctx, id, userAgent)
		if done || err != nil {
			return secret, err
		}
		select {
		case <-ch:
		case <-time.After(time.Second): // another process decided it, or the clock passed expiry
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}
}

func (s *Service) collect(ctx context.Context, id, userAgent string) (secret string, done bool, err error) {
	var approvedBy sql.NullInt64
	var failed int
	var used sql.NullString
	var expires string
	err = s.db.QueryRowContext(ctx, `SELECT approved_by, failed, used_at, expires_at FROM approvals WHERE id = ?`, id).
		Scan(&approvedBy, &failed, &used, &expires)
	if errors.Is(err, sql.ErrNoRows) {
		return "", true, ErrInvalid
	} else if err != nil {
		return "", true, err
	}
	switch {
	case used.Valid:
		return "", true, ErrUsed
	case failed != 0:
		return "", true, ErrWrongNumber
	case approvedBy.Valid:
		res, err := s.db.ExecContext(ctx, `UPDATE approvals SET used_at = ? WHERE id = ? AND used_at IS NULL`, store.Time(s.now()), id)
		if err != nil {
			return "", true, err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return "", true, ErrUsed
		}
		secret, err := s.newSession(ctx, SurfaceApp, approvedBy.Int64, 0, userAgent, s.settings.Get().SessionTTL())
		return secret, true, err
	}
	if exp, _ := store.ParseTime(expires); !s.now().Before(exp) {
		return "", true, ErrExpired
	}
	return "", false, nil
}
