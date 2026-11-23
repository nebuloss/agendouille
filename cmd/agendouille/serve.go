package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
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
	"github.com/nebuloss/agendouille/internal/settings"
	"github.com/nebuloss/agendouille/internal/sharing"
	"github.com/nebuloss/agendouille/internal/store"
	"github.com/nebuloss/agendouille/internal/surface"
	"github.com/nebuloss/agendouille/internal/web/admin"
	"github.com/nebuloss/agendouille/internal/web/app"
	"github.com/nebuloss/agendouille/internal/web/assets"
	"github.com/nebuloss/agendouille/internal/web/webx"
)

// settingsPoll is how often serve looks for settings another process
// (agendouille config set) wrote.
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

// noFavicon answers the browsers' favicon request with nothing, rather than
// a 404 in every console (an icon comes with the web app's manifest, M4).
func noFavicon(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Cache-Control", "max-age=86400")
	w.WriteHeader(http.StatusNoContent)
}

// services is everything that reads and writes the database.
type services struct {
	store    *store.Store
	settings *settings.Service
	dir      *directory.Service
	auth     *auth.Service
	calendar *calendar.Service
	hub      *hub.Hub
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
	set, err := settings.Open(ctx, st.DB(), time.Now)
	if err != nil {
		st.Close()
		return nil, err
	}
	dir := directory.New(st.DB(), time.Now, func() []string { return set.Get().Palette })
	h := hub.New()
	cal := calendar.New(st.DB(), time.Now, set, dir, sharing.New(st.DB(), dir), h)
	return &services{store: st, settings: set, dir: dir, auth: auth.New(st.DB(), time.Now, set, dir), calendar: cal, hub: h}, nil
}

// muxes builds each listener's routes once: the public one holds the dav
// and app surfaces, the admin one the admin surface, and nothing is in
// both but /healthz.
func muxes(d *deps.Deps) (public, adminMux http.Handler, err error) {
	pub := http.NewServeMux()
	pub.HandleFunc("GET /healthz", healthz)
	pub.HandleFunc("GET /favicon.ico", noFavicon)
	dav.Register(pub, d)
	if err := app.Register(pub, d); err != nil {
		return nil, nil, err
	}
	adm := http.NewServeMux()
	adm.HandleFunc("GET /healthz", healthz)
	adm.HandleFunc("GET /favicon.ico", noFavicon)
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
	d := &deps.Deps{Config: cfg, Settings: svc.settings, Log: log, Store: svc.store, Directory: svc.dir, Auth: svc.auth,
		Calendar: svc.calendar, Backup: bk, Hub: svc.hub, Clock: time.Now, Version: version,
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
	log.Info("admin UI", "url", cfg.AdminURL()+"/admin", "version", version,
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
		log.Warn("the public listener (CalDAV and the family web app) is off: set public_listen and public_url in the admin UI or with agendouille config set")
	}
	watchCtx, stopWatch := context.WithCancel(ctx)
	defer stopWatch()
	go svc.settings.Watch(watchCtx, settingsPoll, func(err error) { log.Error("reloading settings", "err", err) })
	go prune(watchCtx, svc.calendar, log)
	go bk.Run(watchCtx)

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
