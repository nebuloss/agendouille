package push

import (
	"context"
	"crypto/rand"
	"database/sql"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/nebuloss/agendouille/internal/store"
)

// The push service itself (RFC 8030's push service, DECISIONS: push without
// ntfy): agendouille receives Web Push messages on /push/<id> and keeps them
// for the device until its stream (Server-Sent Events, held open by the
// agendouille app) takes them. The app hands them on as a UnifiedPush
// distributor: to itself, or to DAVx5.

const (
	// MaxMessage is RFC 8030's floor for a push service, and UnifiedPush's limit.
	MaxMessage = 4096
	// MaxTTL caps how long a message waits for its device.
	MaxTTL = 7 * 24 * time.Hour
	// maxQueued bounds one device's waiting messages; the oldest go first.
	maxQueued = 500
	// maxEndpoints bounds one device's endpoints (its apps).
	maxEndpoints = 50
)

var (
	// ErrNoEndpoint is an unknown endpoint (404).
	ErrNoEndpoint = errors.New("push: no such endpoint")
	// ErrTooLarge is a message over MaxMessage (413).
	ErrTooLarge = errors.New("push: message too large")
)

// QueuedMessage is one waiting message, as the stream sends it.
type QueuedMessage struct {
	Seq      int64
	Endpoint string
	Body     []byte
	Urgency  string
}

// Relay keeps the endpoints and their messages, and wakes the streams.
type Relay struct {
	db   *sql.DB
	now  func() time.Time
	base func() string

	mu      sync.Mutex
	waiters map[int64]map[chan struct{}]struct{} // device -> its open streams
}

// NewRelay makes the push service. base is the public URL.
func NewRelay(db *sql.DB, now func() time.Time, base func() string) *Relay {
	return &Relay{db: db, now: now, base: base, waiters: map[int64]map[chan struct{}]struct{}{}}
}

// EndpointURL is an endpoint's public address.
func (r *Relay) EndpointURL(id string) string { return r.base() + "/push/" + id }

// Local is the endpoint id of url when it is one of this server's, for a
// message from this server to skip the network.
func (r *Relay) Local(url string) (string, bool) {
	id, ok := strings.CutPrefix(url, r.base()+"/push/")
	if !ok || id == "" || strings.ContainsAny(id, "/?#") {
		return "", false
	}
	return id, true
}

// Endpoint gives the device's endpoint for instance, made on first ask.
func (r *Relay) Endpoint(ctx context.Context, deviceID int64, instance string) (string, error) {
	if instance == "" || len(instance) > 300 {
		return "", errors.New("push: an instance names the app, at most 300 bytes")
	}
	var id string
	err := r.db.QueryRowContext(ctx, `SELECT id FROM push_endpoints WHERE device_id = ? AND instance = ?`, deviceID, instance).Scan(&id)
	if err == nil {
		return id, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return "", err
	}
	var n int
	if err := r.db.QueryRowContext(ctx, `SELECT count(*) FROM push_endpoints WHERE device_id = ?`, deviceID).Scan(&n); err != nil {
		return "", err
	}
	if n >= maxEndpoints {
		return "", ErrTooMany
	}
	raw := make([]byte, 24) // 192 bits: the URL is the only secret a sender needs
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	id = b64.EncodeToString(raw)
	_, err = r.db.ExecContext(ctx, `INSERT INTO push_endpoints (id, device_id, instance, created_at) VALUES (?, ?, ?, ?)`,
		id, deviceID, instance, store.Time(r.now()))
	return id, err
}

// DropEndpoint removes one of the device's endpoints and its messages.
func (r *Relay) DropEndpoint(ctx context.Context, deviceID int64, id string) (bool, error) {
	res, err := r.db.ExecContext(ctx, `DELETE FROM push_endpoints WHERE id = ? AND device_id = ?`, id, deviceID)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// Accept queues a message for endpoint id (RFC 8030 section 5): ttl 0 is
// only for a device connected now; a topic replaces the message waiting
// with the same one.
func (r *Relay) Accept(ctx context.Context, id string, body []byte, ttl time.Duration, topic, urgency string) error {
	if len(body) > MaxMessage {
		return ErrTooLarge
	}
	var device int64
	if err := r.db.QueryRowContext(ctx, `SELECT device_id FROM push_endpoints WHERE id = ?`, id).Scan(&device); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNoEndpoint
		}
		return err
	}
	if ttl <= 0 {
		if !r.connected(device) {
			return nil // accepted, and dropped: nobody to give it to now
		}
		ttl = time.Minute
	}
	if ttl > MaxTTL {
		ttl = MaxTTL
	}
	switch urgency {
	case "very-low", "low", "normal", "high":
	default:
		urgency = "normal"
	}
	now := r.now()
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if topic != "" {
		if _, err := tx.ExecContext(ctx, `DELETE FROM push_messages WHERE endpoint_id = ? AND topic = ?`, id, topic); err != nil {
			return err
		}
	}
	var t any
	if topic != "" {
		t = topic
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO push_messages (endpoint_id, device_id, topic, urgency, body, expires_at, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`, id, device, t, urgency, body, store.Time(now.Add(ttl)), store.Time(now)); err != nil {
		return err
	}
	// The oldest go when a device lets too many wait.
	if _, err := tx.ExecContext(ctx, `DELETE FROM push_messages WHERE device_id = ? AND seq NOT IN
		(SELECT seq FROM push_messages WHERE device_id = ? ORDER BY seq DESC LIMIT ?)`, device, device, maxQueued); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	r.wake(device)
	return nil
}

// Ack drops what the device says it received: every message up to seq
// (the stream's Last-Event-ID, or the app's acknowledgement).
func (r *Relay) Ack(ctx context.Context, deviceID, seq int64) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM push_messages WHERE device_id = ? AND seq <= ?`, deviceID, seq)
	return err
}

// Pending is what waits for the device after seq, oldest first. Nothing
// goes until acknowledged: a stream cut mid-way loses nothing.
func (r *Relay) Pending(ctx context.Context, deviceID, after int64) ([]QueuedMessage, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT seq, endpoint_id, body, urgency FROM push_messages
		WHERE device_id = ? AND seq > ? AND expires_at > ? ORDER BY seq`, deviceID, after, store.Time(r.now()))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []QueuedMessage
	for rows.Next() {
		var m QueuedMessage
		if err := rows.Scan(&m.Seq, &m.Endpoint, &m.Body, &m.Urgency); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// Wait is a channel that hears when a message arrives for the device; stop
// when the stream closes.
func (r *Relay) Wait(deviceID int64) (c chan struct{}, stop func()) {
	c = make(chan struct{}, 1)
	r.mu.Lock()
	if r.waiters[deviceID] == nil {
		r.waiters[deviceID] = map[chan struct{}]struct{}{}
	}
	r.waiters[deviceID][c] = struct{}{}
	r.mu.Unlock()
	return c, func() {
		r.mu.Lock()
		delete(r.waiters[deviceID], c)
		if len(r.waiters[deviceID]) == 0 {
			delete(r.waiters, deviceID)
		}
		r.mu.Unlock()
	}
}

func (r *Relay) connected(deviceID int64) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.waiters[deviceID]) > 0
}

func (r *Relay) wake(deviceID int64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for c := range r.waiters[deviceID] {
		select {
		case c <- struct{}{}:
		default:
		}
	}
}

// Clean drops expired messages.
func (r *Relay) Clean(ctx context.Context) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM push_messages WHERE expires_at <= ?`, store.Time(r.now()))
	return err
}
