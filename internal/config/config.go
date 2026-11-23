// Package config loads agendouille's configuration from
// /etc/agendouille.yaml, the environment and flags, in that order of
// increasing precedence: where the data lives, where the admin UI listens,
// and every setting (the public listener, limits, backups, defaults;
// internal/settings), as cafard and claudication keep theirs. The admin UI
// shows them and where each came from; it does not edit them. serve re-reads
// the file when it changes and applies the settings live; the keys before
// the settings need a restart.
//
// Every scalar key gets a flag and an environment variable for free: the
// key admin_listen is -admin-listen and AGENDOUILLE_ADMIN_LISTEN.
package config

import (
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"reflect"
	"strconv"
	"strings"

	"github.com/nebuloss/agendouille/internal/settings"
)

// DefaultPath is where the service looks when neither -config nor
// AGENDOUILLE_CONFIG names a file. A missing file there is not an error.
const DefaultPath = "/etc/agendouille.yaml"

// EnvPrefix starts every environment variable the configuration reads.
const EnvPrefix = "AGENDOUILLE_"

// Config is everything the bootstrap file can say.
type Config struct {
	DataDir string `yaml:"data_dir"`
	// AdminListen is the address of the admin UI. Meant for the LAN only:
	// the reverse proxy in the DMZ never forwards to it.
	AdminListen string `yaml:"admin_listen"`
	// AdminPublicURL is the address the admin's browser uses. Empty means
	// http://<admin_listen>, which is only allowed on a loopback address.
	AdminPublicURL string `yaml:"admin_public_url"`
	// AdminAllowInsecure allows a plain http:// admin URL on an address
	// other than loopback, for a LAN where the admin UI has no certificate.
	AdminAllowInsecure bool   `yaml:"admin_allow_insecure"`
	LogLevel           string `yaml:"log_level"`

	// Settings are the rest of the file, at its top level: applied live.
	Settings settings.Values `yaml:",inline"`

	// Path is the file read, or "" when there was none.
	Path string `yaml:"-"`
	// Stamp is the file's modification time and size when it was read: a
	// watcher starts from it, so an edit made right after the read is seen.
	Stamp string `yaml:"-"`
	// Origins says where each key's value came from: default, file, env or
	// flag (claudication's Origin), for the admin UI.
	Origins map[string]string `yaml:"-"`

	reload func() (*Config, error)
}

// Reload reads the configuration again, from the same file, environment and
// flags. Nil when the configuration was not loaded from a Loader (tests).
func (c *Config) Reload() (*Config, error) {
	if c.reload == nil {
		return nil, fmt.Errorf("this configuration was not loaded from a file")
	}
	return c.reload()
}

// Restart lists the keys that changed between c and next and that only a
// restart applies: everything before the settings.
func (c *Config) Restart(next *Config) []string {
	var out []string
	if c.DataDir != next.DataDir {
		out = append(out, "data_dir")
	}
	if c.AdminListen != next.AdminListen {
		out = append(out, "admin_listen")
	}
	if c.AdminPublicURL != next.AdminPublicURL {
		out = append(out, "admin_public_url")
	}
	if c.AdminAllowInsecure != next.AdminAllowInsecure {
		out = append(out, "admin_allow_insecure")
	}
	if c.LogLevel != next.LogLevel {
		out = append(out, "log_level")
	}
	return out
}

// Validator is implemented by sub-structs that check themselves.
type Validator interface {
	Validate() error
}

// Defaulter is implemented by sub-structs that have defaults.
type Defaulter interface {
	SetDefaults()
}

// Default is the configuration with nothing set.
func Default() Config {
	c := Config{
		DataDir:     "/var/lib/agendouille",
		AdminListen: "127.0.0.1:8792",
		LogLevel:    "info",
		Settings:    settings.Default(),
	}
	applyDefaults(&c)
	return c
}

// Validate checks every key, then the exposure rules for the admin UI.
func (c *Config) Validate() error {
	if c.DataDir == "" {
		return fmt.Errorf("data_dir: must be set")
	}
	host, err := checkListen(c.AdminListen)
	if err != nil {
		return fmt.Errorf("admin_listen: %w", err)
	}
	switch c.LogLevel {
	case "debug", "info", "warn", "error":
	default:
		return fmt.Errorf("log_level: %q is not one of debug, info, warn, error", c.LogLevel)
	}
	if c.AdminPublicURL != "" {
		if err := checkPublicURL(c.AdminPublicURL); err != nil {
			return fmt.Errorf("admin_public_url: %w", err)
		}
	}
	// The admin UI holds the only password and every setting. Off loopback
	// it must say where it is reached, and say so over https unless the
	// operator accepts plain http on the LAN in so many words.
	if !isLoopback(host) {
		if c.AdminPublicURL == "" {
			return fmt.Errorf("admin_public_url: must be set when admin_listen (%s) is not a loopback address", c.AdminListen)
		}
		if strings.HasPrefix(c.AdminPublicURL, "http://") && !c.AdminAllowInsecure {
			return fmt.Errorf("admin_public_url: %q is plain http off loopback; use https, or set admin_allow_insecure: true for a LAN-only admin UI", c.AdminPublicURL)
		}
	}
	return validateStructs(reflect.ValueOf(c).Elem())
}

// AdminURL is where the admin UI is reached, without a trailing slash.
func (c *Config) AdminURL() string {
	if c.AdminPublicURL != "" {
		return strings.TrimRight(c.AdminPublicURL, "/")
	}
	return "http://" + c.AdminListen
}

func checkListen(address string) (host string, err error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return "", fmt.Errorf("%q is not host:port: %w", address, err)
	}
	n, err := strconv.Atoi(port)
	if err != nil || n < 0 || n > 65535 {
		return "", fmt.Errorf("%q has no valid port", address)
	}
	return host, nil
}

// isLoopback reports whether host names only this machine. An empty host
// (":8792") listens everywhere, so it is not.
func isLoopback(host string) bool {
	if host == "localhost" {
		return true
	}
	addr, err := netip.ParseAddr(host)
	return err == nil && addr.IsLoopback()
}

func checkPublicURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return err
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("%q must start with http:// or https://", raw)
	}
	if u.Host == "" {
		return fmt.Errorf("%q has no host", raw)
	}
	if u.RawQuery != "" || u.Fragment != "" || u.User != nil {
		return fmt.Errorf("%q must be a plain scheme://host[:port][/path]", raw)
	}
	return nil
}
