package httpapi

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"sync"
	"syscall"
	"time"

	"github.com/exustash/trainsty/scheduler"
)

// Port is hardcoded with no fallback and no flag (ADR-011).
//
// A fallback would be worse than the error it avoids: the Runner's URL is a
// constant too, so a daemon on another port is a daemon nothing can reach while
// looking healthy.
const Port = 45678

// Addr is loopback only. Never 0.0.0.0 — that would expose an unauthenticated
// process-kill endpoint to the network (SDR-001).
const Addr = "127.0.0.1:45678"

// BaseURL is what the CLI and the dashboard talk to.
const BaseURL = "http://127.0.0.1:45678"

// ErrAlreadyRunning means another trainsty daemon holds the port.
var ErrAlreadyRunning = errors.New("another trainsty daemon is already running")

// ErrPortTaken means something that is not trainsty holds the port.
//
// Distinguished from ErrAlreadyRunning because the remedies differ: one is
// `trainsty stop`, the other is finding out what else wants 45678 (ADR-011).
var ErrPortTaken = errors.New("port is in use by another program")

// Server is the daemon's HTTP surface.
type Server struct {
	sched *scheduler.Scheduler
	log   *log.Logger
	mux   *http.ServeMux
	http  *http.Server

	// shutdown is closed by POST /shutdown to end Serve.
	shutdown     chan struct{}
	shutdownOnce sync.Once
}

// New wires the routes. The scheduler and the logger are injected so tests can
// drive a Server with an in-memory logger and no file.
func New(sched *scheduler.Scheduler, logger *log.Logger) *Server {
	s := &Server{
		sched:    sched,
		log:      logger,
		mux:      http.NewServeMux(),
		shutdown: make(chan struct{}),
	}
	// Routes are registered by the file that implements them, listed here so the
	// whole surface is visible in one place. /status is wired first because
	// Listen's bind classification probes it: without it, "already running" and
	// "something else holds the port" cannot be told apart.
	s.mux.HandleFunc("/status", guardReadOnly(s.handleStatus))
	s.mux.HandleFunc("/register", guardReadOnly(s.handleRegister))
	s.mux.HandleFunc("/release", guardMutating(s.handleRelease))
	s.mux.HandleFunc("/stop", guardMutating(s.handleStop))
	s.mux.HandleFunc("/shutdown", guardMutating(s.handleShutdown))
	s.mux.HandleFunc("/", s.handleRoot)

	// The liveness probe is started by the scheduler on every Grant, so it is
	// owned by the Job rather than by the server.
	sched.SetOnGrant(s.startProbe)

	s.http = &http.Server{
		Handler: s.mux,
		// ReadHeaderTimeout bounds the headers, not a wait, so it stays set.
		ReadHeaderTimeout: 5 * time.Second,
		// WriteTimeout STAYS SET. /register removes its own deadline per request
		// via http.ResponseController, so a 30-minute queue wait survives without
		// every other route losing its write timeout (research.md → R1).
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
	}
	return s
}

// Handler exposes the mux for tests using httptest.
func (s *Server) Handler() http.Handler { return s.mux }

// Listen binds the port, classifying the failure a caller actually needs to act on.
//
// The successful bind is also the single-instance mechanism: there is no PID file
// and no lock file, so there is no stale-lock failure mode — which is precisely
// the failure mode this product exists to prevent in other people's tooling
// (ADR-011).
func Listen() (net.Listener, error) {
	ln, err := net.Listen("tcp", Addr)
	if err == nil {
		return ln, nil
	}
	if !errors.Is(err, syscall.EADDRINUSE) {
		return nil, fmt.Errorf("listen on %s: %w", Addr, err)
	}
	// Whoever holds it: ask. A trainsty daemon answers /status with JSON; anything
	// else does not, and the two need different advice.
	if Reachable() {
		return nil, ErrAlreadyRunning
	}
	return nil, ErrPortTaken
}

// Reachable reports whether a trainsty daemon answers on the port.
func Reachable() bool {
	client := &http.Client{Timeout: 750 * time.Millisecond}
	resp, err := client.Get(BaseURL + "/status")
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

// Serve runs until POST /shutdown, then stops accepting and returns.
func (s *Server) Serve(ln net.Listener) error {
	errc := make(chan error, 1)
	go func() {
		err := s.http.Serve(ln)
		if errors.Is(err, http.ErrServerClosed) {
			err = nil
		}
		errc <- err
	}()

	select {
	case err := <-errc:
		return err
	case <-s.shutdown:
		// Release the Lock before going, without killing the Job: the suite keeps
		// running, unsupervised (ADR-009).
		if job := s.sched.Current(); job != nil {
			s.sched.Release(job)
			s.logf("shutdown: released the lock held by pid %d (%s); the suite keeps running", job.PID, job.Repo)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		// Closing rather than draining: every open /register is a Waiter that must
		// be told to go away, and Shutdown would wait for them for ever.
		_ = s.http.Shutdown(ctx)
		_ = s.http.Close()
		return nil
	}
}

func (s *Server) logf(format string, args ...any) {
	if s.log != nil {
		s.log.Printf(format, args...)
	}
}
