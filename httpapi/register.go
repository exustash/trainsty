package httpapi

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/exustash/trainsty/process"
)

// maxRepoLen bounds the one developer-supplied string that reaches a log line and
// an HTML page.
const maxRepoLen = 128

// handleRegister holds an SSE stream open until the Lock is granted.
//
// Four things here are each a way to ship this broken, and none of them shows up
// in a short test:
//
//  1. The write deadline is removed PER REQUEST via http.ResponseController, not by
//     zeroing Server.WriteTimeout. A 30-minute wait survives and the other four
//     routes keep their timeout (research.md → R1).
//  2. Flush after the headers AND after the event. Without the second, the Grant
//     sits in a buffer and the Runner waits for ever while everything looks healthy.
//  3. The select covers r.Context().Done() as well as the Grant — a dropped
//     Registration is the PRIMARY release signal, not an edge case (ADR-008).
//  4. After the Grant the stream STAYS OPEN. That is what makes it the liveness
//     signal; closing it here would make the probe the only detector and turn a
//     2-second release into a 5-second one.
func (s *Server) handleRegister(w http.ResponseWriter, r *http.Request) {
	pid, repo, code := parseRegistration(r)
	if code != "" {
		s.logf("register refused: %s (pid=%q repo=%q)", code, r.URL.Query().Get("pid"), r.URL.Query().Get("repo"))
		writeJSONError(w, http.StatusBadRequest, code)
		return
	}

	rc := http.NewResponseController(w)
	// A zero value means no deadline. If the ResponseWriter cannot do this, refuse
	// rather than accept a registration whose wait will be severed mid-queue.
	if err := rc.SetWriteDeadline(time.Time{}); err != nil {
		s.logf("register refused: this ResponseWriter cannot hold a stream open: %v", err)
		writeJSONError(w, http.StatusInternalServerError, codeStreamingUnsupported)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	if err := rc.Flush(); err != nil {
		return // the client is already gone
	}

	waiter := s.sched.Register(pid, repo)
	s.logf("register: pid %d (%s)", pid, repo)

	select {
	case <-waiter.Granted():
	case <-r.Context().Done():
		// Withdrawn while waiting: it held nothing, so nothing is released.
		s.sched.Withdraw(waiter)
		s.logf("withdraw: pid %d (%s) left the queue", pid, repo)
		return
	}

	grantedAt := time.Now()
	if job := s.sched.JobFor(waiter); job != nil {
		grantedAt = job.GrantedAt
	}
	fmt.Fprintf(w, "event: grant\ndata: {\"pid\":%d,\"grantedAt\":%q}\n\n", pid, grantedAt.UTC().Format(time.RFC3339))
	if err := rc.Flush(); err != nil {
		// The Grant could not be delivered. Release rather than hold the Lock for a
		// Runner that will never know it has it.
		s.logf("release: pid %d (%s) — the grant could not be delivered: %v", pid, repo, err)
		s.sched.Withdraw(waiter)
		return
	}
	s.logf("grant: pid %d (%s)", pid, repo)

	// Hold the stream. This is release path 2: the terminal closing, the process
	// dying, or Ctrl+C all end the request context.
	<-r.Context().Done()
	if s.sched.Withdraw(waiter) {
		s.logf("release: pid %d (%s) — registration dropped", pid, repo)
	}
}

// parseRegistration validates before anything enters the Queue. A malformed
// Registration that reached the Queue would become a Grant to nobody — a Lock held
// by an entry that can never release (FR-012).
//
// The order is the contract's order, so a caller fixing one problem at a time gets
// the same answers in the same sequence.
func parseRegistration(r *http.Request) (pid int, repo string, code string) {
	q := r.URL.Query()

	raw := strings.TrimSpace(q.Get("pid"))
	if raw == "" {
		return 0, "", codeInvalidPID
	}
	pid, err := strconv.Atoi(raw)
	if err != nil || pid <= 0 {
		return 0, "", codeInvalidPID
	}

	repo = strings.TrimSpace(q.Get("repo"))
	if repo == "" || len(repo) > maxRepoLen || strings.ContainsFunc(repo, isControl) {
		return 0, "", codeInvalidRepo
	}

	// The pid must be SIGNALABLE, not merely existent, and this check is not
	// redundant with the leader check below — getpgid(2) needs no permission, so
	// another user's process passes it. Accepting one would be unreleasable by the
	// probe: Alive reports EPERM, which correctly means ALIVE, so the Lock would be
	// held until the stream dropped. Refused here instead (ADR-008).
	alive, err := process.Alive(pid)
	if err != nil || !alive {
		return 0, "", codeNoSuchProcess
	}

	leader, err := process.IsGroupLeader(pid)
	if err != nil {
		// Any error here is effectively ESRCH: getpgid(2) needs no permission, so
		// the only thing that fails is a pid that has gone since the check above.
		return 0, "", codeNoSuchProcess
	}
	if !leader {
		// The refusal that prevents a killed shell: KillGroup signals the group this
		// pid belongs to, and a non-leader's group is somebody else's (ADR-002).
		return 0, "", codeNotGroupLeader
	}
	return pid, repo, ""
}

func isControl(r rune) bool { return r < 0x20 || r == 0x7f }
