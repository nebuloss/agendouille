// Package backup snapshots the database (PLAN.md 13.6): on the
// backup_schedule setting every day, keeping backup_keep of them, and on
// demand. A snapshot is a whole SQLite file made with VACUUM INTO, so it
// opens as it is.
//
// Restoring swaps files while nothing has the database open: the admin UI
// stages the backup as restore-pending.db and asks the server to restart;
// the next start (OpenDir, before the store opens) keeps the current file
// as before-restore-<time>.db and puts the staged one in its place. The
// CLI's restore, with the server stopped, does the same at once.
package backup

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/nebuloss/agendouille/internal/settings"
	"github.com/nebuloss/agendouille/internal/store"
)

// Dir is where backups live inside the data directory.
const Dir = "backups"

// pending is the staged restore, beside the database.
const pending = "restore-pending.db"

var fileRule = regexp.MustCompile(`^(agendouille-[0-9]{8}T[0-9]{6}Z|before-restore-[0-9]{8}T[0-9]{6}Z)\.db$`)

// ErrNotFound is a backup name that is not one of ours.
var ErrNotFound = errors.New("no such backup")

// File is one backup.
type File struct {
	Name    string    `json:"name"`
	Size    int64     `json:"size"`
	Made    time.Time `json:"made"`
	Restore bool      `json:"before_restore"` // kept from before a restore, never pruned
}

// Service makes and lists backups.
type Service struct {
	db       *sql.DB
	dataDir  string
	settings *settings.Service
	now      func() time.Time
	log      *slog.Logger
}

// New wraps the open database of dataDir.
func New(db *sql.DB, dataDir string, set *settings.Service, now func() time.Time, log *slog.Logger) *Service {
	return &Service{db: db, dataDir: dataDir, settings: set, now: now, log: log}
}

func (s *Service) dir() string { return filepath.Join(s.dataDir, Dir) }

// Make writes a backup now and prunes the old ones.
func (s *Service) Make(ctx context.Context) (File, error) {
	if err := os.MkdirAll(s.dir(), 0o700); err != nil {
		return File{}, err
	}
	name := "agendouille-" + s.now().UTC().Format("20060102T150405Z") + ".db"
	path := filepath.Join(s.dir(), name)
	if _, err := s.db.ExecContext(ctx, `VACUUM INTO ?`, path); err != nil {
		return File{}, fmt.Errorf("backup: %w", err)
	}
	_ = os.Chmod(path, 0o600)
	info, err := os.Stat(path)
	if err != nil {
		return File{}, err
	}
	s.prune()
	return File{Name: name, Size: info.Size(), Made: info.ModTime().UTC()}, nil
}

// List is every backup, newest first.
func (s *Service) List() ([]File, error) {
	entries, err := os.ReadDir(s.dir())
	if errors.Is(err, os.ErrNotExist) {
		return []File{}, nil
	} else if err != nil {
		return nil, err
	}
	out := []File{}
	for _, e := range entries {
		if e.IsDir() || !fileRule.MatchString(e.Name()) {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		out = append(out, File{Name: e.Name(), Size: info.Size(), Made: madeAt(e.Name(), info.ModTime()),
			Restore: strings.HasPrefix(e.Name(), "before-restore-")})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Made.After(out[j].Made) })
	return out, nil
}

func madeAt(name string, fallback time.Time) time.Time {
	stamp := strings.TrimSuffix(name[strings.LastIndex(name, "-")+1:], ".db")
	if t, err := time.Parse("20060102T150405Z", stamp); err == nil {
		return t
	}
	return fallback.UTC()
}

// prune keeps the newest backup_keep scheduled backups; the copies kept
// from before a restore stay until removed by hand.
func (s *Service) prune() {
	files, err := s.List()
	if err != nil {
		return
	}
	keep := s.settings.Get().BackupKeep
	n := 0
	for _, f := range files {
		if f.Restore {
			continue
		}
		n++
		if n > keep {
			if err := os.Remove(filepath.Join(s.dir(), f.Name)); err == nil {
				s.log.Info("backup pruned", "file", f.Name)
			}
		}
	}
}

// Path is the file of a backup, after checking its name.
func (s *Service) Path(name string) (string, error) {
	if !fileRule.MatchString(name) {
		return "", ErrNotFound
	}
	p := filepath.Join(s.dir(), name)
	if _, err := os.Stat(p); err != nil {
		return "", ErrNotFound
	}
	return p, nil
}

// Last is the newest backup's time, or zero.
func (s *Service) Last() time.Time {
	files, _ := s.List()
	for _, f := range files {
		if !f.Restore {
			return f.Made
		}
	}
	return time.Time{}
}

// Stage copies a backup as the pending restore. The caller then restarts
// the server, which applies it while nothing holds the database.
func (s *Service) Stage(name string) error {
	src, err := s.Path(name)
	if err != nil {
		return err
	}
	if err := check(src); err != nil {
		return err
	}
	return copyFile(src, filepath.Join(s.dataDir, pending))
}

// check opens a backup read-only and runs SQLite's own integrity check, so
// a damaged file is refused before it replaces anything.
func check(path string) error {
	st, err := store.OpenReadOnly(context.Background(), path)
	if err != nil {
		return fmt.Errorf("the backup does not open: %w", err)
	}
	defer st.Close()
	var ok string
	if err := st.DB().QueryRow(`PRAGMA integrity_check`).Scan(&ok); err != nil || ok != "ok" {
		return fmt.Errorf("the backup is damaged: %v %s", err, ok)
	}
	return nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	tmp := dst + ".tmp"
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		os.Remove(tmp)
		return err
	}
	if err := out.Sync(); err != nil {
		out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, dst)
}

// ApplyPending puts a staged restore in place of the database. It runs
// before the store opens; it returns the name the old database was kept
// under, or "" when nothing was staged.
func ApplyPending(dataDir string, now time.Time) (string, error) {
	staged := filepath.Join(dataDir, pending)
	if _, err := os.Stat(staged); errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	return Restore(dataDir, staged, now)
}

// Restore puts the backup file src in place of the database of dataDir,
// keeping the current one in backups/ as before-restore-<time>.db. The
// server must not be running.
func Restore(dataDir, src string, now time.Time) (string, error) {
	if err := check(src); err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Join(dataDir, Dir), 0o700); err != nil {
		return "", err
	}
	db := filepath.Join(dataDir, store.FileName)
	kept := ""
	if _, err := os.Stat(db); err == nil {
		kept = "before-restore-" + now.UTC().Format("20060102T150405Z") + ".db"
		if err := os.Rename(db, filepath.Join(dataDir, Dir, kept)); err != nil {
			return "", err
		}
		for _, suffix := range []string{"-wal", "-shm"} {
			os.Remove(db + suffix) // the WAL belonged to the file just kept
		}
	}
	if filepath.Base(src) == pending && filepath.Dir(src) == dataDir {
		if err := os.Rename(src, db); err != nil {
			return kept, err
		}
	} else if err := copyFile(src, db); err != nil {
		return kept, err
	}
	return kept, os.Chmod(db, 0o600)
}

// Run makes a backup every day at backup_schedule (local time) until ctx
// ends.
func (s *Service) Run(ctx context.Context) {
	for {
		next := nextRun(s.now(), s.settings.Get().BackupSchedule)
		t := time.NewTimer(time.Until(next))
		select {
		case <-ctx.Done():
			t.Stop()
			return
		case <-t.C:
		}
		if f, err := s.Make(ctx); err != nil {
			s.log.Error("scheduled backup", "err", err)
		} else {
			s.log.Info("backup made", "file", f.Name, "bytes", f.Size)
		}
	}
}

// nextRun is the next HH:MM after now, in now's zone.
func nextRun(now time.Time, hhmm string) time.Time {
	var h, m int
	fmt.Sscanf(hhmm, "%d:%d", &h, &m)
	next := time.Date(now.Year(), now.Month(), now.Day(), h, m, 0, 0, now.Location())
	if !next.After(now) {
		next = next.AddDate(0, 0, 1)
	}
	return next
}
