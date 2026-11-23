package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/nebuloss/agendouille/internal/config"
	"github.com/nebuloss/agendouille/internal/web/assets"
)

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

// adminMux is everything the admin listener serves. In M0 that is the
// health check and the admin bundle; the admin UI arrives in M1 and M3.
func adminMux() (*http.ServeMux, error) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", healthz)
	if err := assets.Register(mux, assets.Admin); err != nil {
		return nil, err
	}
	return mux, nil
}

// serve runs the server until ctx is cancelled, then shuts it down within
// ten seconds. ready, when set, gets the admin listener's bound address.
//
// M0 binds the admin listener only. The public listener (CalDAV and the
// family web app) is a setting kept in the database and is started from it
// in M1 (PLAN.md 11.2).
func serve(ctx context.Context, cfg *config.Config, log *slog.Logger, ready func(admin net.Addr)) error {
	mux, err := adminMux()
	if err != nil {
		return err
	}
	ln, err := net.Listen("tcp", cfg.AdminListen)
	if err != nil {
		return fmt.Errorf("admin listener: %w", err)
	}
	srv := &http.Server{
		Handler: mux,
		// No ReadTimeout or WriteTimeout: SSE streams stay open for
		// minutes. Handlers that need a deadline set one through
		// http.ResponseController.
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       2 * time.Minute,
		ErrorLog:          slog.NewLogLogger(log.Handler(), slog.LevelWarn),
		BaseContext:       func(net.Listener) context.Context { return ctx },
	}

	served := make(chan error, 1)
	go func() { served <- srv.Serve(ln) }()
	log.Info("listening", "surface", "admin", "addr", ln.Addr().String(), "url", cfg.AdminURL(),
		"version", version, "app_bundle", assets.Built(assets.App), "admin_bundle", assets.Built(assets.Admin),
		"data_dir", cfg.DataDir)
	if ready != nil {
		ready(ln.Addr())
	}

	select {
	case err := <-served:
		// Serve returned on its own: the listener failed.
		return err
	case <-ctx.Done():
	}
	log.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	err = srv.Shutdown(shutdownCtx)
	if serr := <-served; serr != nil && !errors.Is(serr, http.ErrServerClosed) && err == nil {
		err = serr
	}
	if err == nil {
		log.Info("stopped")
	}
	return err
}
