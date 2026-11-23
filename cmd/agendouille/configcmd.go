package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/nebuloss/agendouille/internal/config"
	"github.com/nebuloss/agendouille/internal/settings"
	"github.com/nebuloss/agendouille/internal/store"
)

// cmdConfig loads and validates the configuration exactly as serve would
// and prints it, with where each value came from. Every setting lives in
// the file (DECISIONS: settings in the configuration file): there is no
// `config set`; edit the file, and a running server applies it within
// seconds.
//
// `config migrate` moves the settings an older version kept in the
// database into the file, once; install.sh runs it, as root, on every
// install. Printing never creates the database.
func cmdConfig(ctx context.Context, e env, args []string) error {
	set := flag.NewFlagSet("agendouille config", flag.ContinueOnError)
	loader := config.NewLoader(set)
	help, pos, err := parseArgs(set, e, args)
	if help || err != nil {
		return err
	}
	if len(pos) > 0 && pos[0] == "set" {
		path, _ := loader.Path(e.getenv)
		return fmt.Errorf("settings live in the configuration file now: edit %s (a running server applies it within seconds), then check it with: agendouille config", path)
	}
	if len(pos) > 0 && pos[0] == "migrate" {
		return migrateSettings(ctx, e, loader)
	}
	if len(pos) > 0 {
		return errors.New("usage: agendouille config [migrate]")
	}
	cfg, path, err := loader.Load(e.getenv)
	if err != nil {
		return err
	}
	out, err := cfg.Marshal()
	if err != nil {
		return err
	}
	if path == "" {
		fmt.Fprintf(e.stdout, "# no file at %s: defaults, environment and flags\n", config.DefaultPath)
	} else {
		fmt.Fprintf(e.stdout, "# loaded from %s\n", path)
	}
	if _, err := e.stdout.Write(out); err != nil {
		return err
	}
	var notes []string
	for k, o := range cfg.Origins {
		if o != "file" {
			notes = append(notes, fmt.Sprintf("#   %s from %s", k, o))
		}
	}
	sort.Strings(notes)
	if len(notes) > 0 {
		fmt.Fprintln(e.stdout, "\n# not from the file:")
		fmt.Fprintln(e.stdout, strings.Join(notes, "\n"))
	}
	return nil
}

// migrateSettings writes the settings kept in the database by an older
// version into the configuration file, after what is there, checks the
// result loads, and only then empties the database's table. A key the
// file already has keeps the file's value.
func migrateSettings(ctx context.Context, e env, loader *config.Loader) error {
	cfg, path, err := loader.Load(e.getenv)
	if err != nil {
		return err
	}
	db := filepath.Join(cfg.DataDir, store.FileName)
	if _, err := os.Stat(db); errors.Is(err, fs.ErrNotExist) {
		fmt.Fprintln(e.stdout, "No database yet: nothing to move.")
		return nil
	}
	st, err := store.Open(ctx, db)
	if err != nil {
		return err
	}
	defer st.Close()
	stored, err := settings.Stored(ctx, st.DB())
	if err != nil {
		return err
	}
	if len(stored) == 0 {
		fmt.Fprintln(e.stdout, "No settings in the database: nothing to move.")
		return nil
	}
	if path == "" {
		path = config.DefaultPath
	}
	old, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	keys := make([]string, 0, len(stored))
	for k := range stored {
		if cfg.Origins[k] == "file" {
			continue // the file says it already
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	b.Write(old)
	if len(old) > 0 && !strings.HasSuffix(string(old), "\n") {
		b.WriteString("\n")
	}
	if len(keys) > 0 {
		fmt.Fprintf(&b, "\n# Moved from the database on %s (agendouille config migrate).\n", time.Now().Format("2006-01-02"))
		for _, k := range keys {
			fmt.Fprintf(&b, "%s: %s\n", k, stored[k]) // JSON is YAML
		}
	}
	mode, uid, gid := os.FileMode(0o640), -1, -1
	if info, err := os.Stat(path); err == nil {
		mode = info.Mode().Perm()
		if st, ok := info.Sys().(*syscall.Stat_t); ok {
			uid, gid = int(st.Uid), int(st.Gid)
		}
	}
	tmp := path + ".new"
	if err := os.WriteFile(tmp, []byte(b.String()), mode); err != nil {
		return err
	}
	if uid >= 0 {
		_ = os.Chown(tmp, uid, gid)
	}
	// The file with the moved keys must load before it replaces the old one.
	check := config.NewLoader(flag.NewFlagSet("check", flag.ContinueOnError))
	if _, _, err := check.Load(func(k string) string {
		if k == config.EnvPrefix+"CONFIG" {
			return tmp
		}
		return e.getenv(k)
	}); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("the settings from the database do not load in the file, nothing was changed: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return err
	}
	if err := settings.ForgetStored(ctx, st.DB()); err != nil {
		return err
	}
	fmt.Fprintf(e.stdout, "Moved %d setting(s) from the database into %s: %s\n", len(keys), path, strings.Join(keys, ", "))
	return nil
}
