// Package store owns the SQLite handle and the migrations. Other packages
// own their tables and query them through DB(); the calendar tables
// (calendars, objects, changes) are written only by internal/calendar.
package store

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var migrations embed.FS

// FileName is the database file inside the data directory.
const FileName = "agendouille.db"

// TimeFormat is how every timestamp is stored: RFC 3339 with nanoseconds,
// in UTC, so text order is time order.
const TimeFormat = "2006-01-02T15:04:05.000000000Z"

// Time formats t for a column.
func Time(t time.Time) string { return t.UTC().Format(TimeFormat) }

// ParseTime reads a column written by Time.
func ParseTime(s string) (time.Time, error) { return time.Parse(TimeFormat, s) }

// Store is the database handle.
type Store struct {
	db   *sql.DB
	path string
}

// EnsureDataDir creates the data directory with mode 0700, and tightens an
// existing one: it holds the family's calendars.
func EnsureDataDir(dir string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	return os.Chmod(dir, 0o700)
}

// OpenDir opens the database in the data directory, creating both.
func OpenDir(ctx context.Context, dir string) (*Store, error) {
	if err := EnsureDataDir(dir); err != nil {
		return nil, fmt.Errorf("data_dir: %w", err)
	}
	return Open(ctx, filepath.Join(dir, FileName))
}

// Open opens (creating if needed) the database at path, applies the
// migrations, and makes the file mode 0600.
func Open(ctx context.Context, path string) (*Store, error) {
	if strings.ContainsAny(path, "?#") {
		return nil, fmt.Errorf("database path %q must not contain ? or #", path)
	}
	// busy_timeout covers concurrent writers; _txlock=immediate makes a
	// transaction take the write lock up front instead of failing on
	// upgrade.
	dsn := "file:" + path +
		"?_pragma=foreign_keys(1)" +
		"&_pragma=journal_mode(WAL)" +
		"&_pragma=synchronous(NORMAL)" +
		"&_pragma=busy_timeout(10000)" +
		"&_txlock=immediate"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, err
	}
	s := &Store{db: db, path: path}
	if err := s.migrate(ctx); err != nil {
		db.Close()
		return nil, err
	}
	if err := s.tightenModes(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

// OpenReadOnly opens a database file without writing to it or migrating
// it: for checking a backup before it is restored.
func OpenReadOnly(ctx context.Context, path string) (*Store, error) {
	if strings.ContainsAny(path, "?#") {
		return nil, fmt.Errorf("database path %q must not contain ? or #", path)
	}
	db, err := sql.Open("sqlite", "file:"+path+"?mode=ro&_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, err
	}
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, err
	}
	var n int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM schema_migrations`).Scan(&n); err != nil {
		db.Close()
		return nil, fmt.Errorf("not an agendouille database: %w", err)
	}
	return &Store{db: db, path: path}, nil
}

// tightenModes sets 0600 on the database and its WAL files. SQLite creates
// them with the process umask.
func (s *Store) tightenModes() error {
	for _, suffix := range []string{"", "-wal", "-shm"} {
		err := os.Chmod(s.path+suffix, 0o600)
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
	}
	return nil
}

// DB is the handle for the packages that own tables.
func (s *Store) DB() *sql.DB { return s.db }

// Path is the database file.
func (s *Store) Path() string { return s.path }

func (s *Store) Close() error { return s.db.Close() }

// Tx runs f in a transaction, committing when it returns nil.
func (s *Store) Tx(ctx context.Context, f func(tx *sql.Tx) error) error {
	return InTx(ctx, s.db, f)
}

// InTx runs f in a transaction on db, committing when it returns nil.
func InTx(ctx context.Context, db *sql.DB, f func(tx *sql.Tx) error) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if err := f(tx); err != nil {
		tx.Rollback()
		return err
	}
	return tx.Commit()
}

// migrate applies every migration not recorded yet, in name order, each in
// its own transaction.
func (s *Store) migrate(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		name TEXT PRIMARY KEY, applied_at TEXT NOT NULL)`); err != nil {
		return err
	}
	files, err := fs.Glob(migrations, "migrations/*.sql")
	if err != nil {
		return err
	}
	sort.Strings(files)
	for _, f := range files {
		base := filepath.Base(f)
		var done int
		if err := s.db.QueryRowContext(ctx, `SELECT count(*) FROM schema_migrations WHERE name = ?`, base).Scan(&done); err != nil {
			return err
		}
		if done > 0 {
			continue
		}
		body, err := migrations.ReadFile(f)
		if err != nil {
			return err
		}
		err = s.Tx(ctx, func(tx *sql.Tx) error {
			if _, err := tx.ExecContext(ctx, string(body)); err != nil {
				return err
			}
			_, err := tx.ExecContext(ctx, `INSERT INTO schema_migrations(name, applied_at) VALUES (?, ?)`, base, Time(time.Now()))
			return err
		})
		if err != nil {
			return fmt.Errorf("migration %s: %w", base, err)
		}
	}
	return nil
}
