// Package push sends WebDAV-Push messages (PLAN.md M9, draft-bitfire-
// webdav-push-00): a device subscribes to a calendar over CalDAV with a Web
// Push resource (its UnifiedPush endpoint, through the push server the
// family runs, ntfy), and every change to the calendar sends that resource
// an encrypted message which makes the device sync at once. DAVx5 and our
// app subscribe the same way; nothing here goes through Google.
//
// A message says only which calendar changed and its new sync token: no
// event ever leaves the server this way.
package push

import (
	"bytes"
	"context"
	"crypto/ecdh"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/nebuloss/agendouille/internal/calendar"
	"github.com/nebuloss/agendouille/internal/hub"
	"github.com/nebuloss/agendouille/internal/sharing"
	"github.com/nebuloss/agendouille/internal/store"
)

// Namespace is WebDAV-Push's XML namespace.
const Namespace = "https://bitfire.at/webdav-push"

var (
	// ErrInvalid is a subscription the server cannot use (403 invalid-subscription).
	ErrInvalid = errors.New("push: invalid subscription")
	// ErrNoTrigger is a registration with no trigger the server supports.
	ErrNoTrigger = errors.New("push: no supported trigger")
	// ErrTooMany is a user over MaxPerUser subscriptions (403 push-not-available).
	ErrTooMany = errors.New("push: too many subscriptions")
)

const (
	// DefaultLifetime is a subscription's life when the client asks none.
	DefaultLifetime = 7 * 24 * time.Hour
	// MaxLifetime caps what a client asks: it registers again before.
	MaxLifetime = 30 * 24 * time.Hour
	// MaxPerUser bounds one user's subscriptions (each device, each calendar).
	MaxPerUser = 200
	// coalesce is how long changes to one calendar wait to go as one message.
	coalesce = 300 * time.Millisecond
)

// Transport, when set, carries the requests to push services (tests: an
// httptest TLS server's).
var Transport http.RoundTripper

// Service keeps the subscriptions and sends the messages.
type Service struct {
	db      *sql.DB
	sharing *sharing.Service
	now     func() time.Time
	base    func() string // the public URL, for registration URLs and tokens
	log     *slog.Logger
	client  *http.Client

	relay *Relay

	mu  sync.Mutex
	key *ecdsa.PrivateKey
}

// Relay is the push service the server is itself (relay.go).
func (s *Service) Relay() *Relay { return s.relay }

// New makes the service. base is the dav surface's public URL, now.
func New(db *sql.DB, sh *sharing.Service, now func() time.Time, base func() string, log *slog.Logger) *Service {
	client := &http.Client{
		Timeout:   15 * time.Second,
		Transport: Transport,
		// A push service answers where it stands: never follow it elsewhere.
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	return &Service{db: db, sharing: sh, now: now, base: base, log: log, client: client, relay: NewRelay(db, now, base)}
}

// vapidKey is the server's VAPID key, made and kept on first use.
func (s *Service) vapidKey(ctx context.Context) (*ecdsa.PrivateKey, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.key != nil {
		return s.key, nil
	}
	var enc string
	err := s.db.QueryRowContext(ctx, `SELECT private FROM push_keys WHERE id = 1`).Scan(&enc)
	if errors.Is(err, sql.ErrNoRows) {
		k, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			return nil, err
		}
		raw, err := k.Bytes()
		if err != nil {
			return nil, err
		}
		enc = b64.EncodeToString(raw)
		if _, err := s.db.ExecContext(ctx, `INSERT OR IGNORE INTO push_keys (id, private, created_at) VALUES (1, ?, ?)`, enc, store.Time(s.now())); err != nil {
			return nil, err
		}
		// Another request may have made one meanwhile: the row wins.
		if err := s.db.QueryRowContext(ctx, `SELECT private FROM push_keys WHERE id = 1`).Scan(&enc); err != nil {
			return nil, err
		}
	} else if err != nil {
		return nil, err
	}
	raw, err := b64.DecodeString(enc)
	if err != nil {
		return nil, err
	}
	k, err := ecdsa.ParseRawPrivateKey(elliptic.P256(), raw)
	if err != nil {
		return nil, err
	}
	s.key = k
	return k, nil
}

// PublicKey is the server's VAPID public key, base64url: what a client gives
// its UnifiedPush distributor, and the transports property.
func (s *Service) PublicKey(ctx context.Context) (string, error) {
	k, err := s.vapidKey(ctx)
	if err != nil {
		return "", err
	}
	pub, err := publicBytes(k)
	if err != nil {
		return "", err
	}
	return b64.EncodeToString(pub), nil
}

// Topic is a calendar's push topic: its random uid, the same for every user
// (it says nothing of the calendar; the Topic header shows it to the push
// service).
func (s *Service) Topic(ctx context.Context, calendarID int64) (string, error) {
	var uid string
	if err := s.db.QueryRowContext(ctx, `SELECT uid FROM calendars WHERE id = ?`, calendarID).Scan(&uid); err != nil {
		return "", err
	}
	return strings.ReplaceAll(uid, "-", ""), nil
}

// URL is a registration's address.
func (s *Service) URL(id string) string { return s.base() + "/dav/push/" + id }

// Subscription is what a device registers for one calendar.
type Subscription struct {
	UserID, DeviceID, CalendarID int64
	Resource                     string // the push resource, an https URL
	PublicKey, AuthSecret        string // base64url
	Content, Property            bool   // the triggers
	Expires                      time.Time
}

// Register keeps sub, or renews the one with the same push resource: its
// id and when it expires.
func (s *Service) Register(ctx context.Context, sub Subscription) (string, time.Time, error) {
	if !sub.Content && !sub.Property {
		return "", time.Time{}, ErrNoTrigger
	}
	u, err := url.Parse(sub.Resource)
	_, local := s.relay.Local(sub.Resource)
	if err != nil || (u.Scheme != "https" && !local) || u.Host == "" || u.User != nil || len(sub.Resource) > 1000 {
		return "", time.Time{}, fmt.Errorf("%w: the push resource is an https URL", ErrInvalid)
	}
	pub, err := decode64(sub.PublicKey)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("%w: the public key", ErrInvalid)
	}
	if _, err := ecdh.P256().NewPublicKey(pub); err != nil {
		return "", time.Time{}, fmt.Errorf("%w: the public key is not a P-256 point", ErrInvalid)
	}
	auth, err := decode64(sub.AuthSecret)
	if err != nil || len(auth) != 16 {
		return "", time.Time{}, fmt.Errorf("%w: the auth secret is 16 bytes", ErrInvalid)
	}
	now := s.now()
	expires := now.Add(DefaultLifetime)
	if !sub.Expires.IsZero() && sub.Expires.After(now) {
		expires = sub.Expires
		if expires.After(now.Add(MaxLifetime)) {
			expires = now.Add(MaxLifetime)
		}
	}
	var dev any
	if sub.DeviceID != 0 {
		dev = sub.DeviceID
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return "", time.Time{}, err
	}
	defer func() { _ = tx.Rollback() }()
	var id string
	var owner int64
	err = tx.QueryRowContext(ctx, `SELECT id, user_id FROM push_subscriptions WHERE calendar_id = ? AND push_resource = ?`,
		sub.CalendarID, sub.Resource).Scan(&id, &owner)
	switch {
	case err == nil && owner != sub.UserID:
		return "", time.Time{}, fmt.Errorf("%w: the push resource is someone else's", ErrInvalid)
	case err == nil:
		// The same resource again: the registration is renewed, not doubled.
		_, err = tx.ExecContext(ctx, `UPDATE push_subscriptions SET device_id = ?, public_key = ?, auth_secret = ?,
			content_update = ?, property_update = ?, expires_at = ? WHERE id = ?`,
			dev, b64.EncodeToString(pub), b64.EncodeToString(auth), sub.Content, sub.Property, store.Time(expires), id)
	case errors.Is(err, sql.ErrNoRows):
		var n int
		if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM push_subscriptions WHERE user_id = ?`, sub.UserID).Scan(&n); err != nil {
			return "", time.Time{}, err
		}
		if n >= MaxPerUser {
			return "", time.Time{}, ErrTooMany
		}
		raw := make([]byte, 18)
		if _, err := rand.Read(raw); err != nil {
			return "", time.Time{}, err
		}
		id = b64.EncodeToString(raw)
		_, err = tx.ExecContext(ctx, `INSERT INTO push_subscriptions (id, user_id, device_id, calendar_id, push_resource,
			public_key, auth_secret, content_update, property_update, expires_at, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, id, sub.UserID, dev, sub.CalendarID, sub.Resource,
			b64.EncodeToString(pub), b64.EncodeToString(auth), sub.Content, sub.Property, store.Time(expires), store.Time(now))
	}
	if err != nil {
		return "", time.Time{}, err
	}
	return id, expires, tx.Commit()
}

// Unregister removes one of the user's registrations; false when there is
// none by that id (unknown, expired, or someone else's).
func (s *Service) Unregister(ctx context.Context, userID int64, id string) (bool, error) {
	res, err := s.db.ExecContext(ctx, `DELETE FROM push_subscriptions WHERE id = ? AND user_id = ? AND expires_at > ?`,
		id, userID, store.Time(s.now()))
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// batch is what waits to go for one calendar: the changes of a few hundred
// milliseconds, as one message.
type batch struct {
	seq               int64
	content, property bool
	// The device that made every change of the batch (0: several, or none):
	// it is not told of its own changes.
	device int64
	// Push-Dont-Notify: the registrations not to tell; all for "*".
	dontAll bool
	dont    map[string]bool
}

func (b *batch) merge(e hub.Event, first bool) {
	if e.Seq > b.seq {
		b.seq = e.Seq
	}
	if e.Name == "" && e.Action == "calendar_changed" {
		b.property = true
	} else {
		b.content = true
	}
	all, set := false, map[string]bool{}
	for _, v := range e.DontNotify {
		if v == "*" {
			all = true
		}
		set[v] = true
	}
	if first {
		b.device, b.dontAll, b.dont = e.DeviceID, all, set
		return
	}
	if b.device != e.DeviceID {
		b.device = 0
	}
	// A registration is spared only if every change of the batch spared it.
	switch {
	case all && b.dontAll:
	case all:
	case b.dontAll:
		b.dontAll, b.dont = false, set
	default:
		for k := range b.dont {
			if !set[k] {
				delete(b.dont, k)
			}
		}
	}
}

func (b *batch) spares(regURL string, device int64) bool {
	return b.dontAll || b.dont[regURL] || (b.device != 0 && b.device == device)
}

// Run sends a message for every change the hub publishes, until ctx ends,
// and drops expired subscriptions every hour.
func (s *Service) Run(ctx context.Context, h *hub.Hub) {
	l, stop := h.Listen(1024)
	defer stop()
	pending := map[int64]*batch{}
	flush := time.NewTicker(coalesce)
	defer flush.Stop()
	clean := time.NewTicker(time.Hour)
	defer clean.Stop()
	sem := make(chan struct{}, 8)
	for {
		select {
		case <-ctx.Done():
			return
		case e := <-l.C:
			if e.Action == "calendar_created" || e.Action == "calendar_deleted" {
				continue // nobody subscribed yet; or the subscriptions went with it
			}
			b, ok := pending[e.CalendarID]
			if !ok {
				b = &batch{}
				pending[e.CalendarID] = b
			}
			b.merge(e, !ok)
		case <-flush.C:
			for id, b := range pending {
				delete(pending, id)
				sem <- struct{}{}
				go func() {
					defer func() { <-sem }()
					if err := s.send(ctx, id, b); err != nil && ctx.Err() == nil {
						s.log.Warn("push", "calendar", id, "err", err)
					}
				}()
			}
		case <-clean.C:
			if _, err := s.db.ExecContext(ctx, `DELETE FROM push_subscriptions WHERE expires_at <= ?`, store.Time(s.now())); err != nil && ctx.Err() == nil {
				s.log.Warn("push: dropping expired subscriptions", "err", err)
			}
			if err := s.relay.Clean(ctx); err != nil && ctx.Err() == nil {
				s.log.Warn("push: dropping expired messages", "err", err)
			}
		}
	}
}

type target struct {
	id, resource, pub, auth string
	user, device            int64
	content, property       bool
}

// send tells the calendar's subscriptions of b.
func (s *Service) send(ctx context.Context, calendarID int64, b *batch) error {
	rows, err := s.db.QueryContext(ctx, `SELECT id, user_id, COALESCE(device_id, 0), push_resource, public_key, auth_secret,
		content_update, property_update FROM push_subscriptions WHERE calendar_id = ? AND expires_at > ?`,
		calendarID, store.Time(s.now()))
	if err != nil {
		return err
	}
	var targets []target
	for rows.Next() {
		var t target
		if err := rows.Scan(&t.id, &t.user, &t.device, &t.resource, &t.pub, &t.auth, &t.content, &t.property); err != nil {
			_ = rows.Close()
			return err
		}
		targets = append(targets, t)
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if len(targets) == 0 {
		return nil
	}
	topic, err := s.Topic(ctx, calendarID)
	if err != nil {
		return err
	}
	key, err := s.vapidKey(ctx)
	if err != nil {
		return err
	}
	token := calendar.Token(s.base(), calendar.Info{ID: calendarID, Seq: b.seq})
	for _, t := range targets {
		if b.spares(s.URL(t.id), t.device) || !((b.content && t.content) || (b.property && t.property)) {
			continue
		}
		// Only while the calendar is still theirs to read.
		if a, err := s.sharing.Access(ctx, t.user, calendarID); err != nil || !a.CanRead() {
			continue
		}
		msg := Message(topic, token, b.content && t.content, b.property && t.property)
		if err := s.deliver(ctx, key, t, topic, msg); err != nil {
			s.log.Info("push: not delivered", "subscription", t.id, "err", err)
		}
	}
	return nil
}

// Message is a push message's XML: the topic, and what changed.
func Message(topic, syncToken string, content, property bool) []byte {
	var m strings.Builder
	m.WriteString(`<?xml version="1.0" encoding="utf-8"?>` + "\n")
	m.WriteString(`<P:push-message xmlns:D="DAV:" xmlns:P="` + Namespace + `"><P:topic>` + topic + `</P:topic>`)
	if content {
		m.WriteString(`<P:content-update><D:sync-token>` + escape(syncToken) + `</D:sync-token></P:content-update>`)
	}
	if property {
		m.WriteString(`<P:property-update/>`)
	}
	m.WriteString(`</P:push-message>`)
	return []byte(m.String())
}

func escape(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;").Replace(s)
}

// deliver posts one encrypted message; a push service that says the
// resource is gone (404, 410) takes the subscription with it.
func (s *Service) deliver(ctx context.Context, key *ecdsa.PrivateKey, t target, topic string, msg []byte) error {
	pub, err := decode64(t.pub)
	if err != nil {
		return err
	}
	auth, err := decode64(t.auth)
	if err != nil {
		return err
	}
	body, err := seal(msg, pub, auth)
	if err != nil {
		return err
	}
	// One of this server's own endpoints: straight into its queue.
	if id, ok := s.relay.Local(t.resource); ok {
		err := s.relay.Accept(ctx, id, body, 24*time.Hour, topic, "high")
		if errors.Is(err, ErrNoEndpoint) {
			_, _ = s.db.ExecContext(ctx, `DELETE FROM push_subscriptions WHERE id = ?`, t.id)
		}
		return err
	}
	subject := s.base()
	if !strings.HasPrefix(subject, "https://") {
		subject = "mailto:agendouille@localhost"
	}
	authz, err := vapid(key, t.resource, subject, s.now())
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.resource, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("Content-Encoding", "aes128gcm")
	req.Header.Set("TTL", "86400")
	req.Header.Set("Urgency", "high")
	req.Header.Set("Topic", topic)
	req.Header.Set("Authorization", authz)
	res, err := s.client.Do(req)
	if err != nil {
		return err
	}
	_ = res.Body.Close()
	switch {
	case res.StatusCode >= 200 && res.StatusCode < 300:
		return nil
	case res.StatusCode == http.StatusNotFound || res.StatusCode == http.StatusGone:
		_, err := s.db.ExecContext(ctx, `DELETE FROM push_subscriptions WHERE id = ?`, t.id)
		if err == nil {
			err = fmt.Errorf("the push service says it is gone (%d): unsubscribed", res.StatusCode)
		}
		return err
	default:
		return fmt.Errorf("the push service answered %d", res.StatusCode)
	}
}
