// Package settings holds the settings kept in the database (PLAN.md 11.2):
// everything the admin edits in the admin UI, as opposed to the bootstrap
// file. Values are validated together, stored as JSON one row per key,
// and applied live: subscribers hear of every change.
package settings

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	_ "time/tzdata" // the family zone must load on a host without zoneinfo

	"github.com/nebuloss/agendouille/internal/store"
)

// Values is every setting. The json tag is the key.
type Values struct {
	// Network
	PublicListen        string   `json:"public_listen" group:"network"`
	PublicURL           string   `json:"public_url" group:"network"`
	PublicAllowInsecure bool     `json:"public_allow_insecure" group:"network"`
	TrustedProxies      []string `json:"trusted_proxies" group:"network"`

	// Storage and backups
	BackupSchedule         string `json:"backup_schedule" group:"storage"`
	BackupKeep             int    `json:"backup_keep" group:"storage"`
	SyncTokenRetentionDays int    `json:"sync_token_retention_days" group:"storage"`
	MaxObjectBytes         int    `json:"max_object_bytes" group:"storage"`

	// Limits
	MaxCalendarsPerUser  int `json:"max_calendars_per_user" group:"limits"`
	MaxEventsPerCalendar int `json:"max_events_per_calendar" group:"limits"`
	MaxDevicesPerUser    int `json:"max_devices_per_user" group:"limits"`

	// Defaults
	FamilyTimezone         string   `json:"family_timezone" group:"defaults"`
	Palette                []string `json:"palette" group:"defaults"`
	EnrolmentLinkTTLHours  int      `json:"enrolment_link_ttl_hours" group:"defaults"`
	SessionTTLDays         int      `json:"session_ttl_days" group:"defaults"`
	AppSyncIntervalMinutes int      `json:"app_sync_interval_minutes" group:"defaults"`
}

// DefaultPalette is twelve colours that read on light and dark surfaces.
var DefaultPalette = []string{
	"#3b6ea5", "#c2185b", "#2e7d32", "#6a1b9a", "#ef6c00", "#00838f",
	"#ad1457", "#558b2f", "#4527a0", "#d84315", "#00695c", "#5d4037",
}

// Default is every setting before the admin changes any.
func Default() Values {
	return Values{
		TrustedProxies:         []string{},
		BackupSchedule:         "03:30",
		BackupKeep:             14,
		SyncTokenRetentionDays: 180,
		MaxObjectBytes:         256 << 10,
		MaxCalendarsPerUser:    50,
		MaxEventsPerCalendar:   50000,
		MaxDevicesPerUser:      20,
		FamilyTimezone:         "Europe/Paris",
		Palette:                append([]string(nil), DefaultPalette...),
		EnrolmentLinkTTLHours:  72,
		SessionTTLDays:         90,
		AppSyncIntervalMinutes: 30,
	}
}

// EnrolmentLinkTTL, SessionTTL: the durations the auth package needs.
func (v Values) EnrolmentLinkTTL() time.Duration {
	return time.Duration(v.EnrolmentLinkTTLHours) * time.Hour
}
func (v Values) SessionTTL() time.Duration { return time.Duration(v.SessionTTLDays) * 24 * time.Hour }

// TrustedPrefixes parses TrustedProxies; Validate has checked them.
func (v Values) TrustedPrefixes() []netip.Prefix {
	out := make([]netip.Prefix, 0, len(v.TrustedProxies))
	for _, s := range v.TrustedProxies {
		if p, err := parsePrefix(s); err == nil {
			out = append(out, p)
		}
	}
	return out
}

var (
	hexColor = regexp.MustCompile(`^#[0-9a-f]{6}$`)
	hhmm     = regexp.MustCompile(`^([01][0-9]|2[0-3]):[0-5][0-9]$`)
)

// Validate checks every value, and the rules between them.
func (v Values) Validate() error {
	if v.PublicListen != "" {
		if _, _, err := splitListen(v.PublicListen); err != nil {
			return fmt.Errorf("public_listen: %w", err)
		}
		if v.PublicURL == "" {
			return fmt.Errorf("public_url: must be set with public_listen: phones, QR codes and CalDAV answers need the address the proxy serves")
		}
	}
	if v.PublicURL != "" {
		u, err := url.Parse(v.PublicURL)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" ||
			u.RawQuery != "" || u.Fragment != "" || u.User != nil || (u.Path != "" && u.Path != "/") {
			return fmt.Errorf("public_url: %q must be a plain https://host[:port]", v.PublicURL)
		}
		if u.Scheme == "http" && !v.PublicAllowInsecure && !isLoopbackHost(u.Hostname()) {
			return fmt.Errorf("public_url: %q is plain http; use https (the proxy's address), or set public_allow_insecure for a test on the LAN", v.PublicURL)
		}
	}
	for _, p := range v.TrustedProxies {
		if _, err := parsePrefix(p); err != nil {
			return fmt.Errorf("trusted_proxies: %q is not an address or a CIDR", p)
		}
	}
	if !hhmm.MatchString(v.BackupSchedule) {
		return fmt.Errorf("backup_schedule: %q is not HH:MM", v.BackupSchedule)
	}
	for key, n := range map[string]int{
		"backup_keep": v.BackupKeep, "sync_token_retention_days": v.SyncTokenRetentionDays,
		"max_object_bytes": v.MaxObjectBytes, "max_calendars_per_user": v.MaxCalendarsPerUser,
		"max_events_per_calendar": v.MaxEventsPerCalendar, "max_devices_per_user": v.MaxDevicesPerUser,
		"enrolment_link_ttl_hours": v.EnrolmentLinkTTLHours, "session_ttl_days": v.SessionTTLDays,
		"app_sync_interval_minutes": v.AppSyncIntervalMinutes,
	} {
		if n < 1 {
			return fmt.Errorf("%s: must be at least 1, not %d", key, n)
		}
	}
	if v.MaxObjectBytes < 4096 {
		return fmt.Errorf("max_object_bytes: %d is too small for a real event", v.MaxObjectBytes)
	}
	if v.AppSyncIntervalMinutes < 15 {
		return fmt.Errorf("app_sync_interval_minutes: Android runs periodic syncs at most every 15 minutes")
	}
	if _, err := time.LoadLocation(v.FamilyTimezone); err != nil || v.FamilyTimezone == "" || v.FamilyTimezone == "Local" {
		return fmt.Errorf("family_timezone: %q is not a time zone name like Europe/Paris", v.FamilyTimezone)
	}
	if len(v.Palette) == 0 {
		return fmt.Errorf("palette: needs at least one colour")
	}
	for _, c := range v.Palette {
		if !hexColor.MatchString(c) {
			return fmt.Errorf("palette: %q is not a colour like #3b6ea5", c)
		}
	}
	return nil
}

func splitListen(address string) (host string, port int, err error) {
	host, p, err := net.SplitHostPort(address)
	if err != nil {
		return "", 0, fmt.Errorf("%q is not host:port: %w", address, err)
	}
	port, err = strconv.Atoi(p)
	if err != nil || port < 0 || port > 65535 {
		return "", 0, fmt.Errorf("%q has no valid port", address)
	}
	return host, port, nil
}

func isLoopbackHost(host string) bool {
	if host == "localhost" {
		return true
	}
	a, err := netip.ParseAddr(host)
	return err == nil && a.IsLoopback()
}

func parsePrefix(s string) (netip.Prefix, error) {
	if strings.Contains(s, "/") {
		p, err := netip.ParsePrefix(s)
		return p.Masked(), err
	}
	a, err := netip.ParseAddr(s)
	if err != nil {
		return netip.Prefix{}, err
	}
	return netip.PrefixFrom(a, a.BitLen()), nil
}

// Key describes one setting for the admin UI and the CLI.
type Key struct {
	Name    string `json:"key"`
	Group   string `json:"group"`
	Type    string `json:"type"` // string, bool, int, list
	Default any    `json:"default"`
}

type field struct {
	index int
	key   Key
}

var fields = func() map[string]field {
	out := map[string]field{}
	t := reflect.TypeFor[Values]()
	def := reflect.ValueOf(Default())
	for i := range t.NumField() {
		f := t.Field(i)
		name := strings.Split(f.Tag.Get("json"), ",")[0]
		typ := map[reflect.Kind]string{reflect.String: "string", reflect.Bool: "bool", reflect.Int: "int", reflect.Slice: "list"}[f.Type.Kind()]
		out[name] = field{index: i, key: Key{Name: name, Group: f.Tag.Get("group"), Type: typ, Default: def.Field(i).Interface()}}
	}
	return out
}()

// Keys lists every setting, by group then name.
func Keys() []Key {
	out := make([]Key, 0, len(fields))
	for _, f := range fields {
		out = append(out, f.key)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Group != out[j].Group {
			return out[i].Group < out[j].Group
		}
		return out[i].Name < out[j].Name
	})
	return out
}

// ErrUnknownKey is a key no setting has.
var ErrUnknownKey = errors.New("unknown setting")

// apply sets key to raw on v. raw is JSON, or for a string setting also the
// bare text, and for a list setting also comma-separated text, so the CLI
// reads naturally: config set public_url https://agenda.example.org.
func apply(v *Values, key, raw string) error {
	f, ok := fields[key]
	if !ok {
		return fmt.Errorf("%w %q", ErrUnknownKey, key)
	}
	target := reflect.ValueOf(v).Elem().Field(f.index)
	ptr := reflect.New(target.Type())
	if err := json.Unmarshal([]byte(raw), ptr.Interface()); err != nil {
		switch f.key.Type {
		case "string":
			ptr.Elem().SetString(raw)
		case "list":
			items := []string{}
			for _, s := range strings.Split(raw, ",") {
				if s = strings.TrimSpace(s); s != "" {
					items = append(items, s)
				}
			}
			ptr.Elem().Set(reflect.ValueOf(items))
		default:
			return fmt.Errorf("%s: %q is not a %s", key, raw, f.key.Type)
		}
	}
	target.Set(ptr.Elem())
	return nil
}

// Change is one applied Set.
type Change struct {
	Old, New Values
	Keys     []string
}

// Service is the live settings.
type Service struct {
	db  *sql.DB
	now func() time.Time

	mu   sync.RWMutex
	cur  Values
	subs []func(Change)
}

// Open loads the settings from the database over the defaults.
func Open(ctx context.Context, db *sql.DB, now func() time.Time) (*Service, error) {
	s := &Service{db: db, now: now, cur: Default()}
	rows, err := db.QueryContext(ctx, `SELECT key, value FROM settings`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	v := Default()
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			return nil, err
		}
		if err := apply(&v, key, value); err != nil {
			// A key from a newer version, or one removed: kept in the
			// table, ignored here.
			if errors.Is(err, ErrUnknownKey) {
				continue
			}
			return nil, fmt.Errorf("setting %s: %w", key, err)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := v.Validate(); err != nil {
		return nil, fmt.Errorf("stored settings: %w (fix with agendouille config set)", err)
	}
	s.cur = v
	return s, nil
}

// Get is the current settings.
func (s *Service) Get() Values {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return clone(s.cur)
}

func clone(v Values) Values {
	v.TrustedProxies = append([]string{}, v.TrustedProxies...)
	v.Palette = append([]string{}, v.Palette...)
	return v
}

// Subscribe calls f after every change, in the order of subscription, with
// the lock released.
func (s *Service) Subscribe(f func(Change)) {
	s.mu.Lock()
	s.subs = append(s.subs, f)
	s.mu.Unlock()
}

// Set changes several settings at once, so rules between them hold
// (public_listen needs public_url). actor is who did it, for the audit log.
func (s *Service) Set(ctx context.Context, actor string, changes map[string]string) (Values, error) {
	s.mu.Lock()
	old := clone(s.cur)
	next := clone(s.cur)
	keys := make([]string, 0, len(changes))
	for k := range changes {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if err := apply(&next, k, changes[k]); err != nil {
			s.mu.Unlock()
			return old, err
		}
	}
	if err := next.Validate(); err != nil {
		s.mu.Unlock()
		return old, err
	}
	now := store.Time(s.now())
	err := store.InTx(ctx, s.db, func(tx *sql.Tx) error {
		for _, k := range keys {
			value, err := json.Marshal(reflect.ValueOf(next).Field(fields[k].index).Interface())
			if err != nil {
				return err
			}
			before, _ := json.Marshal(reflect.ValueOf(old).Field(fields[k].index).Interface())
			if _, err := tx.ExecContext(ctx, `INSERT INTO settings(key, value, updated_at) VALUES (?, ?, ?)
				ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`, k, string(value), now); err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, `INSERT INTO audit(at, actor, kind, action, detail) VALUES (?, ?, 'setting', 'changed', ?)`,
				now, actor, k+" "+string(before)+" -> "+string(value)); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		s.mu.Unlock()
		return old, err
	}
	s.cur = next
	subs := append([]func(Change){}, s.subs...)
	s.mu.Unlock()
	for _, f := range subs {
		f(Change{Old: old, New: clone(next), Keys: keys})
	}
	return clone(next), nil
}

// Watch reloads the settings every interval when another process (the CLI's
// config set) changed them, and tells the subscribers. It returns when ctx
// ends.
func (s *Service) Watch(ctx context.Context, interval time.Duration, onError func(error)) {
	last := s.stamp(ctx)
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		stamp := s.stamp(ctx)
		if stamp == last {
			continue
		}
		last = stamp
		fresh, err := Open(ctx, s.db, s.now)
		if err != nil {
			onError(err)
			continue
		}
		next := fresh.Get()
		s.mu.Lock()
		old := clone(s.cur)
		if reflect.DeepEqual(old, next) {
			s.mu.Unlock()
			continue
		}
		s.cur = next
		subs := append([]func(Change){}, s.subs...)
		s.mu.Unlock()
		var keys []string
		for _, k := range Keys() {
			a, _ := json.Marshal(reflect.ValueOf(old).Field(fields[k.Name].index).Interface())
			b, _ := json.Marshal(reflect.ValueOf(next).Field(fields[k.Name].index).Interface())
			if string(a) != string(b) {
				keys = append(keys, k.Name)
			}
		}
		for _, f := range subs {
			f(Change{Old: old, New: clone(next), Keys: keys})
		}
	}
}

// stamp changes whenever a setting is written, by this process or another.
func (s *Service) stamp(ctx context.Context) string {
	var n int
	var latest sql.NullString
	_ = s.db.QueryRowContext(ctx, `SELECT count(*), max(updated_at) FROM settings`).Scan(&n, &latest)
	return fmt.Sprint(n, latest.String)
}

// Value is one setting's current value as JSON, for printing.
func (s *Service) Value(key string) (string, error) {
	f, ok := fields[key]
	if !ok {
		return "", fmt.Errorf("%w %q", ErrUnknownKey, key)
	}
	b, err := json.Marshal(reflect.ValueOf(s.Get()).Field(f.index).Interface())
	return string(b), err
}
