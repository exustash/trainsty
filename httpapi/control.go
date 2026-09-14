package httpapi

import (
	"net/http"
	"strconv"
	"strings"
)

// handleRelease frees the Lock and advances the Queue.
//
// The optional `pid` parameter is what makes this identity-checked, and it closes
// a real race: a Runner's release and its Registration dropping can arrive in
// either order, and releasing "whatever holds the Lock" lets the loser revoke the
// winner's SUCCESSOR's Lock. `trainsty wrap` always sends it; a hand-written
// Runner that sends nothing gets the older, racier behaviour.
//
// A no-op is a success. A Runner's trap fires on paths where the Lock is already
// free, and a trap that prints errors on the normal path is a trap developers
// delete.
func (s *Server) handleRelease(w http.ResponseWriter, r *http.Request) {
	pid := optionalPID(r)
	released := s.sched.ReleaseByPID(pid)
	if released {
		s.logf("release: pid %d — requested by the runner", pid)
	}
	writeJSON(w, map[string]bool{"released": released})
}

// handleShutdown releases the Lock and exits — WITHOUT terminating the Job.
//
// The suite keeps running, unsupervised, and the next daemon knows nothing about
// it (ADR-009). jobWasActive is what lets `trainsty stop` warn before it confirms.
func (s *Server) handleShutdown(w http.ResponseWriter, r *http.Request) {
	active := s.sched.Current() != nil
	writeJSON(w, map[string]any{"shuttingDown": true, "jobWasActive": active})
	if f, ok := w.(http.Flusher); ok {
		f.Flush() // answer before the server stops accepting
	}
	s.requestShutdown()
}

func (s *Server) requestShutdown() {
	s.shutdownOnce.Do(func() {
		s.logf("shutdown requested")
		close(s.shutdown)
	})
}

// optionalPID reads ?pid=N. Absent or unparseable means "whatever holds the Lock",
// which is the only thing a Runner that sends no pid can ask for.
func optionalPID(r *http.Request) int {
	raw := strings.TrimSpace(r.URL.Query().Get("pid"))
	if raw == "" {
		return 0
	}
	pid, err := strconv.Atoi(raw)
	if err != nil {
		return 0
	}
	return pid
}
