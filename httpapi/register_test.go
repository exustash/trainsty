package httpapi

import (
	"bufio"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/exustash/trainsty/scheduler"
)

// leaderPID starts a real child that leads its own process group, because
// registration validates against the operating system and cannot be faked.
func leaderPID(t *testing.T) int {
	t.Helper()
	cmd := exec.Command("sleep", "30")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		t.Fatalf("start child: %v", err)
	}
	t.Cleanup(func() {
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		_ = cmd.Wait()
	})
	return cmd.Process.Pid
}

func TestRegisterRefusals(t *testing.T) {
	leader := leaderPID(t)

	for _, tc := range []struct {
		name  string
		query string
		want  string
	}{
		{"a missing pid", "?repo=r", codeInvalidPID},
		{"an unparseable pid", "?pid=abc&repo=r", codeInvalidPID},
		{"a zero pid", "?pid=0&repo=r", codeInvalidPID},
		{"a negative pid", "?pid=-5&repo=r", codeInvalidPID},
		{"a pid that does not exist", "?pid=4194303&repo=r", codeNoSuchProcess},
		{"a missing repo", "?pid=" + itoa(leader), codeInvalidRepo},
		{"a repo with a newline", "?pid=" + itoa(leader) + "&repo=a%0Ab", codeInvalidRepo},
		{"an over-long repo", "?pid=" + itoa(leader) + "&repo=" + strings.Repeat("x", maxRepoLen+1), codeInvalidRepo},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sched := scheduler.New()
			s := New(sched, DiscardLogger())
			rec := httptest.NewRecorder()

			s.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/register"+tc.query, nil))

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("want 400, got %d (%s)", rec.Code, rec.Body.String())
			}
			if got := errorCode(t, rec.Body.String()); got != tc.want {
				t.Fatalf("want %s, got %s", tc.want, got)
			}
			// Nothing malformed may reach the Queue: a bad Registration there becomes
			// a Grant to nobody, and a Lock that can never be released (FR-012).
			if snap := sched.Snapshot(); snap.Job != nil || len(snap.Queue) != 0 {
				t.Fatalf("a refused registration entered the scheduler: %+v", snap)
			}
		})
	}
}

// The refusal that prevents a killed shell. The test process is a real non-leader
// whenever it runs under a shell's job control.
func TestRegisterRefusesANonGroupLeader(t *testing.T) {
	self := os.Getpid()
	pgid, err := syscall.Getpgid(self)
	if err != nil {
		t.Fatalf("getpgid: %v", err)
	}
	if pgid == self {
		t.Skip("this test process leads its own group, so it is not a non-leader example")
	}
	s := New(scheduler.New(), DiscardLogger())
	rec := httptest.NewRecorder()

	s.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/register?pid="+itoa(self)+"&repo=r", nil))

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", rec.Code)
	}
	if got := errorCode(t, rec.Body.String()); got != codeNotGroupLeader {
		t.Fatalf("want %s, got %s", codeNotGroupLeader, got)
	}
}

// A process owned by another user must be refused, never accepted — EPERM is a
// defect in what was registered, not a liveness answer (ADR-008).
func TestRegisterRefusesAProcessOwnedByAnotherUser(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root: every process is ours, so EPERM is unreachable")
	}
	sched := scheduler.New()
	s := New(sched, DiscardLogger())
	rec := httptest.NewRecorder()

	s.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/register?pid=1&repo=r", nil))

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("want a refusal for pid 1, got %d", rec.Code)
	}
	if snap := sched.Snapshot(); snap.Job != nil {
		t.Fatal("pid 1 must never hold the Lock")
	}
}

// The SSE handshake and the Grant, over a real server: the headers must arrive
// before the wait, and the event must be flushed.
func TestRegisterStreamsTheGrant(t *testing.T) {
	leader := leaderPID(t)
	s := New(scheduler.New(), DiscardLogger())
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/register?pid=" + itoa(leader) + "&repo=capture.web")
	if err != nil {
		t.Fatalf("GET /register: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %s", resp.Status)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("want text/event-stream, got %q", ct)
	}
	if cc := resp.Header.Get("Cache-Control"); cc != "no-cache" {
		t.Errorf("want no-cache, got %q", cc)
	}

	// The Grant must arrive without the stream closing — a Flush that never happens
	// leaves it in a buffer and the Runner waits for ever.
	line, err := bufio.NewReader(resp.Body).ReadString('\n')
	if err != nil {
		t.Fatalf("no grant event arrived: %v", err)
	}
	if !strings.HasPrefix(line, "event: grant") {
		t.Fatalf("want an event: grant line, got %q", line)
	}
}

// Release path 2: the Registration dropping frees the Lock.
func TestRegisterReleasesTheLockWhenTheStreamDrops(t *testing.T) {
	leader := leaderPID(t)
	sched := scheduler.New()
	s := New(sched, DiscardLogger())
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"/register?pid="+itoa(leader)+"&repo=r", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /register: %v", err)
	}
	if _, err := bufio.NewReader(resp.Body).ReadString('\n'); err != nil {
		t.Fatalf("no grant: %v", err)
	}
	if sched.Current() == nil {
		t.Fatal("want the Lock held after the grant")
	}

	cancel() // the terminal closes
	resp.Body.Close()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if sched.Current() == nil {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("the Lock was not released within 2s of the stream dropping")
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	if neg {
		return "-" + string(b)
	}
	return string(b)
}
