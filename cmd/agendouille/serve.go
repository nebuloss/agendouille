package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/nebuloss/agendouille/internal/auth"
	"github.com/nebuloss/agendouille/internal/backup"
	"github.com/nebuloss/agendouille/internal/calendar"
	"github.com/nebuloss/agendouille/internal/config"
	"github.com/nebuloss/agendouille/internal/dav"
	"github.com/nebuloss/agendouille/internal/deps"
	"github.com/nebuloss/agendouille/internal/directory"
	"github.com/nebuloss/agendouille/internal/hub"
	"github.com/nebuloss/agendouille/internal/live"
	"github.com/nebuloss/agendouille/internal/push"
	"github.com/nebuloss/agendouille/internal/settings"
	"github.com/nebuloss/agendouille/internal/sharing"
	"github.com/nebuloss/agendouille/internal/store"
	"github.com/nebuloss/agendouille/internal/surface"
	"github.com/nebuloss/agendouille/internal/web/admin"
	"github.com/nebuloss/agendouille/internal/web/app"
	"github.com/nebuloss/agendouille/internal/web/assets"
	"github.com/nebuloss/agendouille/internal/web/webx"
)

// settingsPoll is how often serve looks at the configuration file for an
// edit to apply.
var settingsPoll = 5 * time.Second

func cmdServe(ctx context.Context, e env, args []string) error {
	set := flag.NewFlagSet("agendouille serve", flag.ContinueOnError)
	loader := config.NewLoader(set)
	if help, err := parseFlags(set, e, args); help || err != nil {
		return err
	}
	cfg, path, err := loader.Load(e.getenv)
	if err != nil {
		return err
	}
	log := newLogger(e, cfg.LogLevel)
	if path == "" {
		log.Info("no configuration file, using defaults, environment and flags", "looked_at", config.DefaultPath)
	} else {
		log.Info("configuration loaded", "path", path)
	}
	return serve(ctx, cfg, log, nil)
}

func newLogger(e env, level string) *slog.Logger {
	var l slog.Level
	_ = l.UnmarshalText([]byte(level))
	return slog.New(slog.NewTextHandler(e.stderr, &slog.HandlerOptions{Level: l}))
}

// healthz answers every listener: "ok <version>".
func healthz(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	fmt.Fprintln(w, "ok "+version)
}

// favicon answers /favicon.ico with the andouille in SVG, which every
// current browser takes whatever the path says.
func favicon(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "image/svg+xml")
	w.Header().Set("Cache-Control", "max-age=86400")
	_, _ = w.Write(assets.Icon())
}

// services is everything that reads and writes the database.
type services struct {
	store    *store.Store
	settings *settings.Service
	dir      *directory.Service
	auth     *auth.Service
	calendar *calendar.Service
	hub      *hub.Hub
	push     *push.Service
}

func openServices(ctx context.Context, cfg *config.Config) (*services, error) {
	// A restore staged by the admin UI is put in place before anything
	// opens the database.
	if kept, err := backup.ApplyPending(cfg.DataDir, time.Now()); err != nil {
		return nil, fmt.Errorf("applying the staged restore: %w", err)
	} else if kept != "" {
		slog.Info("restored a backup; the database before it is kept", "as", kept)
	}
	st, err := store.OpenDir(ctx, cfg.DataDir)
	if err != nil {
		return nil, err
	}
	set := settings.New(cfg.Settings)
	dir := directory.New(st.DB(), time.Now, func() []string { return set.Get().Palette })
	h := hub.New()
	cal := calendar.New(st.DB(), time.Now, set, dir, sharing.New(st.DB(), dir), h)
	a := auth.New(st.DB(), time.Now, set, dir)
	ps := push.New(st.DB(), cal.Sharing(), time.Now, func() string { return strings.TrimRight(set.Get().PublicURL, "/") }, slog.Default())
	return &services{store: st, settings: set, dir: dir, auth: a, calendar: cal, hub: h, push: ps}, nil
}

// muxes builds each listener's routes once: the public one holds the dav
// and app surfaces, the admin one the admin surface, and nothing is in
// both but /healthz.
func muxes(d *deps.Deps) (public, adminMux http.Handler, err error) {
	pub := http.NewServeMux()
	pub.HandleFunc("GET /healthz", healthz)
	pub.HandleFunc("GET /favicon.ico", favicon)
	dav.Register(pub, d)
	if err := app.Register(pub, d); err != nil {
		return nil, nil, err
	}
	adm := http.NewServeMux()
	adm.HandleFunc("GET /healthz", healthz)
	adm.HandleFunc("GET /favicon.ico", favicon)
	if err := admin.Register(adm, d); err != nil {
		return nil, nil, err
	}
	return webx.Headers(pub), webx.Headers(adm), nil
}

// prune drops old change rows once an hour (sync_token_retention_days).
func prune(ctx context.Context, cal *calendar.Service, log *slog.Logger) {
	t := time.NewTicker(time.Hour)
	defer t.Stop()
	for {
		if n, err := cal.Prune(ctx); err != nil && ctx.Err() == nil {
			log.Error("pruning the change log", "err", err)
		} else if n > 0 {
			log.Info("pruned the change log", "rows", n)
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// ErrRestart is what serve returns when the admin UI asked for a restart
// (a restore): main exits with a code the service manager restarts on.
var ErrRestart = errors.New("restart requested")

// serve runs the server until ctx is cancelled, then shuts it down within
// surface.DrainTimeout. ready, when set, gets the running listeners.
func serve(ctx context.Context, cfg *config.Config, log *slog.Logger, ready func(*surface.Server)) error {
	svc, err := openServices(ctx, cfg)
	if err != nil {
		return err
	}
	defer svc.store.Close()
	ok, err := svc.auth.HasAdminPassword(ctx)
	if err != nil {
		return err
	}
	if !ok {
		return errors.New("no admin password is set: run `agendouille admin passwd` first (install.sh does it with --random)")
	}

	restart := make(chan string, 1)
	var pubMu sync.Mutex
	pubErr := ""
	bk := backup.New(svc.store.DB(), cfg.DataDir, svc.settings, time.Now, log)
	// What changed reaches the admin's open tabs (internal/live): every
	// committed write by its tables, the settings when the file is read
	// again, the backup files.
	lv := live.New()
	svc.store.OnChange(func(tables []string) { lv.Changed(live.ForTables(tables)...) })
	svc.settings.Subscribe(func(settings.Change) { lv.Changed(live.Settings) })
	bk.OnChange(func() { lv.Changed(live.Backups) })
	d := &deps.Deps{Config: cfg, Settings: svc.settings, Log: log, Store: svc.store, Directory: svc.dir, Auth: svc.auth,
		Calendar: svc.calendar, Backup: bk, Hub: svc.hub, Push: svc.push, Live: lv, Clock: time.Now, Version: version,
		Restart: func(reason string) {
			select {
			case restart <- reason:
			default:
			}
		}}
	public, adminHandler, err := muxes(d)
	if err != nil {
		return err
	}
	srv := surface.New(ctx, log, public)
	d.PublicStatus = func() (string, string) {
		pubMu.Lock()
		defer pubMu.Unlock()
		addr := ""
		if a := srv.PublicAddr(); a != nil {
			addr = a.String()
		}
		return addr, pubErr
	}
	if _, err := srv.StartAdmin(cfg.AdminListen, adminHandler); err != nil {
		return err
	}
	log.Info("admin UI", "url", cfg.AdminURL()+"/", "version", version,
		"app_bundle", assets.Built(assets.App), "admin_bundle", assets.Built(assets.Admin), "data_dir", cfg.DataDir)

	// The public listener follows the public_listen setting from now on.
	// A failure to bind is logged and leaves the admin UI up to fix it.
	apply := func(address string) {
		err := srv.ApplyPublic(address)
		pubMu.Lock()
		pubErr = ""
		if err != nil {
			pubErr = err.Error()
		}
		pubMu.Unlock()
		if err != nil {
			log.Error("public listener", "err", err)
		}
	}
	svc.settings.Subscribe(func(c settings.Change) {
		if c.Old.PublicListen != c.New.PublicListen {
			apply(c.New.PublicListen)
		}
	})
	if address := svc.settings.Get().PublicListen; address != "" {
		apply(address)
	} else {
		log.Warn("the public listener (CalDAV and the family web app) is off: set public_listen and public_url in the configuration file", "path", cfg.Path)
	}
	watchCtx, stopWatch := context.WithCancel(ctx)
	defer stopWatch()
	go watchConfig(watchCtx, cfg, svc.settings, log)
	go prune(watchCtx, svc.calendar, log)
	go bk.Run(watchCtx)
	go svc.push.Run(watchCtx, svc.hub)
	go lv.Run(watchCtx.Done())

	if ready != nil {
		ready(srv)
	}
	var runErr error
	select {
	case runErr = <-srv.Errors():
	case <-ctx.Done():
	case reason := <-restart:
		log.Info("restarting", "why", reason)
		runErr = ErrRestart
	}
	log.Info("shutting down")
	stopWatch()
	if err := srv.Shutdown(); err != nil && runErr == nil {
		runErr = err
	}
	if runErr == nil {
		log.Info("stopped")
	}
	return runErr
}

// watchConfig applies an edit of the configuration file while serve runs:
// every settingsPoll it looks at the file, and when it changed reads the
// configuration again and puts the new settings in place (the subscribers
// rebind the public listener, reschedule the backups). A file that does
// not read or validate is logged and the settings in force are kept; a key
// only a restart applies is logged as such.
func watchConfig(ctx context.Context, cfg *config.Config, set *settings.Service, log *slog.Logger) {
	if cfg.Path == "" {
		return
	}
	stamp := func() string { return config.FileStamp(cfg.Path) }
	// From the read the settings in force came from, not from now: an edit
	// made between the two is not missed.
	last := cfg.Stamp
	t := time.NewTicker(settingsPoll)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		now := stamp()
		if now == last {
			continue
		}
		last = now
		next, err := cfg.Reload()
		if err != nil {
			log.Error("the configuration file does not read; the settings in force are kept", "path", cfg.Path, "err", err)
			continue
		}
		keys, err := set.Replace(next.Settings)
		if err != nil {
			log.Error("the configuration file's settings are not valid; the settings in force are kept", "err", err)
			continue
		}
		if len(keys) > 0 {
			log.Info("settings applied from the configuration file", "keys", keys)
		}
		if r := cfg.Restart(next); len(r) > 0 {
			log.Warn("these keys apply at the next restart", "keys", r)
		}
	}
}
