// Package surface runs the listeners (PLAN.md 9.1). The admin listener
// serves the admin surface and comes from the bootstrap file, so nothing
// done in the UI can take it away. The public listener serves the dav and
// app surfaces and comes from the public_listen setting: it starts, moves
// and stops while the server runs, without a restart.
//
// Each listener has its own mux holding only its surfaces' routes, so a
// path of a surface a listener does not serve is a 404 there, and the
// proxy can forward a whole port with no path rules.
package surface

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"syscall"
	"time"
)

// Surface names a group of routes.
type Surface string

const (
	DAV   Surface = "dav"   // CalDAV, /ics/
	App   Surface = "app"   // the family web app and the additional API
	Admin Surface = "admin" // the admin UI
)

// DrainTimeout is how long a listener being replaced or stopped keeps
// serving the requests it already has.
const DrainTimeout = 10 * time.Second

// Server holds the two listeners.
type Server struct {
	log      *slog.Logger
	baseCtx  context.Context
	public   http.Handler
	newHTTP  func(http.Handler) *http.Server
	mu       sync.Mutex
	pub      *http.Server
	pubLn    net.Listener
	pubAddr  string // the setting it was started from
	adminSrv *http.Server
	adminLn  net.Listener
	wg       sync.WaitGroup
	errs     chan error
}

// New prepares the listeners. public serves dav and app, admin serves admin.
func New(ctx context.Context, log *slog.Logger, public http.Handler) *Server {
	s := &Server{log: log, baseCtx: ctx, public: public, errs: make(chan error, 2)}
	s.newHTTP = func(h http.Handler) *http.Server {
		return &http.Server{
			Handler: h,
			// No ReadTimeout or WriteTimeout: SSE streams and the
			// approval long-poll stay open. Handlers that need a deadline
			// set one through http.ResponseController.
			ReadHeaderTimeout: 10 * time.Second,
			IdleTimeout:       2 * time.Minute,
			ErrorLog:          slog.NewLogLogger(log.Handler(), slog.LevelWarn),
			BaseContext:       func(net.Listener) context.Context { return ctx },
		}
	}
	return s
}

// StartAdmin binds the admin listener. A failure stops the start.
func (s *Server) StartAdmin(address string, h http.Handler) (net.Addr, error) {
	ln, err := net.Listen("tcp", address)
	if err != nil {
		return nil, fmt.Errorf("admin listener: %w", err)
	}
	s.mu.Lock()
	s.adminSrv, s.adminLn = s.newHTTP(h), ln
	srv := s.adminSrv
	s.mu.Unlock()
	s.run(srv, ln, true)
	s.log.Info("listening", "surfaces", "admin", "addr", ln.Addr().String())
	return ln.Addr(), nil
}

// run serves srv on ln. An admin listener that dies stops the server; a
// public one that dies is logged and left down until the setting changes.
func (s *Server) run(srv *http.Server, ln net.Listener, fatal bool) {
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		err := srv.Serve(ln)
		if err == nil || errors.Is(err, http.ErrServerClosed) || errors.Is(err, net.ErrClosed) {
			return
		}
		if fatal {
			s.errs <- err
		} else {
			s.log.Error("public listener stopped", "err", err)
		}
	}()
}

// Errors carries a failure of the admin listener.
func (s *Server) Errors() <-chan error { return s.errs }

// ApplyPublic makes the public listener match the public_listen setting:
// started, moved, or stopped (""). The new address is bound before the old
// listener is let go, so a bad address leaves the old one serving; when
// the new address needs the old one's port, the old one goes first and
// comes back if the new one still cannot bind.
func (s *Server) ApplyPublic(address string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if address == s.pubAddr && (address == "" || s.pub != nil) {
		return nil
	}
	if address == "" {
		s.stopPublicLocked("public_listen cleared")
		s.pubAddr = ""
		return nil
	}
	ln, err := net.Listen("tcp", address)
	if err != nil && errors.Is(err, syscall.EADDRINUSE) && s.pub != nil {
		old := s.pubAddr
		s.stopPublicLocked("moving to " + address)
		if ln, err = net.Listen("tcp", address); err != nil {
			if back, berr := net.Listen("tcp", old); berr == nil {
				s.startPublicLocked(back, old)
			}
			return fmt.Errorf("public listener %s: %w", address, err)
		}
	} else if err != nil {
		return fmt.Errorf("public listener %s: %w", address, err)
	}
	s.stopPublicLocked("moving to " + address)
	s.startPublicLocked(ln, address)
	return nil
}

func (s *Server) startPublicLocked(ln net.Listener, address string) {
	s.pub, s.pubLn, s.pubAddr = s.newHTTP(s.public), ln, address
	s.run(s.pub, ln, false)
	s.log.Info("listening", "surfaces", "dav,app", "addr", ln.Addr().String())
}

// stopPublicLocked closes the public listener's socket now, so its port is
// free for the next one, and lets its open requests drain in the
// background.
func (s *Server) stopPublicLocked(why string) {
	if s.pub == nil {
		return
	}
	old := s.pub
	s.log.Info("public listener stopping", "addr", s.pubLn.Addr().String(), "why", why)
	_ = s.pubLn.Close() // Serve then returns net.ErrClosed, which run ignores
	s.pub, s.pubLn = nil, nil
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		ctx, cancel := context.WithTimeout(context.Background(), DrainTimeout)
		defer cancel()
		_ = old.Shutdown(ctx)
	}()
}

// AdminAddr is the bound admin address, or nil before StartAdmin.
func (s *Server) AdminAddr() net.Addr {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.adminLn == nil {
		return nil
	}
	return s.adminLn.Addr()
}

// PublicAddr is the bound public address, or nil while it is off.
func (s *Server) PublicAddr() net.Addr {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.pubLn == nil {
		return nil
	}
	return s.pubLn.Addr()
}

// Shutdown stops both listeners, draining for at most DrainTimeout, and
// waits for every serving goroutine.
func (s *Server) Shutdown() error {
	ctx, cancel := context.WithTimeout(context.Background(), DrainTimeout)
	defer cancel()
	s.mu.Lock()
	var first error
	for _, srv := range []*http.Server{s.pub, s.adminSrv} {
		if srv != nil {
			if err := srv.Shutdown(ctx); err != nil && first == nil {
				first = err
			}
		}
	}
	s.pub, s.pubLn = nil, nil
	s.mu.Unlock()
	s.wg.Wait()
	return first
}
