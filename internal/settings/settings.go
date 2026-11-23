// Package settings holds the settings (PLAN.md 11.2, DECISIONS: "settings
// in the configuration file"): the public listener and URL, backups,
// limits, defaults. They live in /etc/agendouille.yaml beside the
// bootstrap keys, are read with it (internal/config embeds Values), are
// validated together, and are applied live: serve re-reads the file when
// it changes and Replace tells the subscribers (the public listener
// rebinds, backups reschedule). The admin UI shows them; it does not edit
// them. The database's old settings table is read once, to move its rows
// into the file (agendouille config migrate).
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
)

// Values is every setting. The json tag is the key.
type Values struct {
	// Network
	PublicListen        string   `json:"public_listen" yaml:"public_listen" group:"network"`
	PublicURL           string   `json:"public_url" yaml:"public_url" group:"network"`
	PublicAllowInsecure bool     `json:"public_allow_insecure" yaml:"public_allow_insecure" group:"network"`
	TrustedProxies      []string `json:"trusted_proxies" yaml:"trusted_proxies" group:"network"`

	// Storage and backups
	BackupSchedule         string `json:"backup_schedule" yaml:"backup_schedule" group:"storage"`
	BackupKeep             int    `json:"backup_keep" yaml:"backup_keep" group:"storage"`
	SyncTokenRetentionDays int    `json:"sync_token_retention_days" yaml:"sync_token_retention_days" group:"storage"`
	MaxObjectBytes         int    `json:"max_object_bytes" yaml:"max_object_bytes" group:"storage"`

	// Limits
	MaxCalendarsPerUser  int `json:"max_calendars_per_user" yaml:"max_calendars_per_user" group:"limits"`
	MaxEventsPerCalendar int `json:"max_events_per_calendar" yaml:"max_events_per_calendar" group:"limits"`
	MaxDevicesPerUser    int `json:"max_devices_per_user" yaml:"max_devices_per_user" group:"limits"`

	// Defaults
	FamilyTimezone         string   `json:"family_timezone" yaml:"family_timezone" group:"defaults"`
	Palette                []string `json:"palette" yaml:"palette" group:"defaults"`
	EnrolmentLinkTTLHours  int      `json:"enrolment_link_ttl_hours" yaml:"enrolment_link_ttl_hours" group:"defaults"`
	SessionTTLDays         int      `json:"session_ttl_days" yaml:"session_ttl_days" group:"defaults"`
	AppSyncIntervalMinutes int      `json:"app_sync_interval_minutes" yaml:"app_sync_interval_minutes" group:"defaults"`
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
// bare text, and for a list setting also comma-separated text.
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

// Change is one applied change.
type Change struct {
	Old, New Values
	Keys     []string
}

// Service is the live settings.
type Service struct {
	mu   sync.RWMutex
	cur  Values
	subs []func(Change)
}

// New is a service holding v, which the caller has validated (the config
// loader has).
func New(v Values) *Service { return &Service{cur: clone(v)} }

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

// Replace puts next in place, when it differs, and tells the subscribers:
// what serve does when the file changed. It returns the keys that changed.
func (s *Service) Replace(next Values) ([]string, error) {
	if err := next.Validate(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	old := clone(s.cur)
	var keys []string
	for _, k := range Keys() {
		a, _ := json.Marshal(reflect.ValueOf(old).Field(fields[k.Name].index).Interface())
		b, _ := json.Marshal(reflect.ValueOf(next).Field(fields[k.Name].index).Interface())
		if string(a) != string(b) {
			keys = append(keys, k.Name)
		}
	}
	if len(keys) == 0 {
		s.mu.Unlock()
		return nil, nil
	}
	s.cur = clone(next)
	subs := append([]func(Change){}, s.subs...)
	s.mu.Unlock()
	for _, f := range subs {
		f(Change{Old: old, New: clone(next), Keys: keys})
	}
	return keys, nil
}

// Apply changes some settings over the current ones, all or none, as the
// file would: for tests, and for the migration's preview.
func (s *Service) Apply(changes map[string]string) (Values, error) {
	next := s.Get()
	for k, v := range changes {
		if err := apply(&next, k, v); err != nil {
			return s.Get(), err
		}
	}
	if _, err := s.Replace(next); err != nil {
		return s.Get(), err
	}
	return s.Get(), nil
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

// Stored reads the settings an older version kept in the database, as
// key and JSON value, for agendouille config migrate. Unknown keys are left
// out.
func Stored(ctx context.Context, db *sql.DB) (map[string]string, error) {
	rows, err := db.QueryContext(ctx, `SELECT key, value FROM settings`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			return nil, err
		}
		if _, ok := fields[key]; ok {
			out[key] = value
		}
	}
	return out, rows.Err()
}

// ForgetStored empties the database's settings once they are in the file.
func ForgetStored(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, `DELETE FROM settings`)
	return err
}

// Parse reads one setting's text as the file or the environment would give
// it: JSON, or a bare string, or a comma-separated list.
func Parse(key, raw string) (any, error) {
	v := Default()
	if err := apply(&v, key, raw); err != nil {
		return nil, err
	}
	return reflect.ValueOf(v).Field(fields[key].index).Interface(), nil
}
