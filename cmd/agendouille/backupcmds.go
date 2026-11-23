package main

import (
	"context"
	"flag"
	"fmt"
	"path/filepath"
	"time"

	"github.com/nebuloss/agendouille/internal/backup"
	"github.com/nebuloss/agendouille/internal/config"
)

// cmdBackup makes a backup now, into the backups directory (or prints
// where it went).
func cmdBackup(ctx context.Context, e env, args []string) error {
	set := flag.NewFlagSet("agendouille backup", flag.ContinueOnError)
	loader := config.NewLoader(set)
	if help, err := parseFlags(set, e, args); help || err != nil {
		return err
	}
	cfg, svc, err := openFor(ctx, e, loader)
	if err != nil {
		return err
	}
	defer svc.store.Close()
	f, err := backup.New(svc.store.DB(), cfg.DataDir, svc.settings, time.Now, newLogger(e, "warn")).Make(ctx)
	if err != nil {
		return err
	}
	fmt.Fprintf(e.stdout, "%s (%d bytes)\n", filepath.Join(cfg.DataDir, backup.Dir, f.Name), f.Size)
	return nil
}

// cmdRestore puts a backup in place of the database. The server must be
// stopped: the admin UI's restore is the way while it runs.
func cmdRestore(ctx context.Context, e env, args []string) error {
	set := flag.NewFlagSet("agendouille restore", flag.ContinueOnError)
	loader := config.NewLoader(set)
	help, pos, err := parseArgs(set, e, args)
	if help || err != nil {
		return err
	}
	if err := want(pos, 1, "restore <backup.db> (with the server stopped)"); err != nil {
		return err
	}
	cfg, _, err := loader.Load(e.getenv)
	if err != nil {
		return err
	}
	kept, err := backup.Restore(cfg.DataDir, pos[0], time.Now())
	if err != nil {
		return err
	}
	fmt.Fprintf(e.stdout, "Restored %s. The database before it is kept as %s.\n", pos[0], filepath.Join(cfg.DataDir, backup.Dir, kept))
	return nil
}
