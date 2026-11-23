package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"text/tabwriter"

	"github.com/nebuloss/agendouille/internal/config"
	"github.com/nebuloss/agendouille/internal/settings"
	"github.com/nebuloss/agendouille/internal/store"
)

// cmdConfig loads and validates the bootstrap configuration exactly as
// serve would and prints it, then the settings kept in the database when
// there is one. `config set k v [k v]...` changes settings; a running
// server picks them up within seconds.
//
// Printing never creates the database: the installer runs this as root
// before the service user ever started the server.
func cmdConfig(ctx context.Context, e env, args []string) error {
	set := flag.NewFlagSet("agendouille config", flag.ContinueOnError)
	loader := config.NewLoader(set)
	help, pos, err := parseArgs(set, e, args)
	if help || err != nil {
		return err
	}
	cfg, path, err := loader.Load(e.getenv)
	if err != nil {
		return err
	}
	if len(pos) > 0 {
		if pos[0] != "set" || len(pos) < 3 || len(pos)%2 != 1 {
			return errors.New("usage: agendouille config [set <key> <value> [<key> <value>]...]")
		}
		changes := map[string]string{}
		for i := 1; i < len(pos); i += 2 {
			changes[pos[i]] = pos[i+1]
		}
		svc, err := openServices(ctx, cfg)
		if err != nil {
			return err
		}
		defer svc.store.Close()
		if _, err := svc.settings.Set(ctx, "cli", changes); err != nil {
			return err
		}
		for k := range changes {
			v, _ := svc.settings.Value(k)
			fmt.Fprintf(e.stdout, "%s = %s\n", k, v)
		}
		return nil
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

	db := filepath.Join(cfg.DataDir, store.FileName)
	if _, err := os.Stat(db); errors.Is(err, fs.ErrNotExist) {
		fmt.Fprintf(e.stdout, "\n# settings: no database at %s yet, so every setting has its default\n", db)
		return nil
	}
	st, err := store.Open(ctx, db)
	if err != nil {
		return err
	}
	defer st.Close()
	s, err := settings.Open(ctx, st.DB(), nil)
	if err != nil {
		return err
	}
	fmt.Fprintf(e.stdout, "\n# settings, from %s (change with: agendouille config set <key> <value>)\n", db)
	tw := tabwriter.NewWriter(e.stdout, 0, 4, 2, ' ', 0)
	for _, k := range settings.Keys() {
		v, _ := s.Value(k.Name)
		fmt.Fprintf(tw, "%s\t%s\t# %s\n", k.Name, v, k.Group)
	}
	return tw.Flush()
}
