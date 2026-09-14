package httpapi

import (
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/exustash/trainsty/scheduler"
)

// recordingServer returns a Server whose log is captured, so what is written can be
// asserted rather than assumed.
func recordingServer(t *testing.T) (*Server, *scheduler.Scheduler, *strings.Builder) {
	t.Helper()
	var sb strings.Builder
	sched := scheduler.New()
	return New(sched, log.New(&sb, "", 0)), sched, &sb
}

// FR-020: every grant, release and refusal is recorded WITH ITS CAUSE. The cause is
// the point — a log that says "released" without saying why cannot answer the only
// question a stuck-lock diagnosis asks.
func TestLogRecordsGrantsReleasesAndRefusalsWithCauses(t *testing.T) {
	s, sched, logged := recordingServer(t)
	pid := leaderPID(t)

	// A refusal.
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/register?pid=0&repo=r", nil))

	// A grant, then a release with a cause.
	sched.Register(pid, "capture.web")
	req := httptest.NewRequest(http.MethodPost, "/release?pid="+itoa(pid), nil)
	req.Header.Set("Content-Type", "application/json")
	s.Handler().ServeHTTP(httptest.NewRecorder(), req)

	out := logged.String()
	for _, want := range []string{"register refused", "invalid_pid", "release", "requested by the runner"} {
		if !strings.Contains(out, want) {
			t.Errorf("the log is missing %q; got:\n%s", want, out)
		}
	}
}

// The stricter half, and the one worth a test: the daemon never sees a suite's
// output (ADR-003), so the log must never carry it — nor an environment variable,
// nor an absolute path inside the developer's home directory
// (CLAUDE.md → Logging).
func TestLogNeverCarriesSuiteContentOrHomePaths(t *testing.T) {
	s, sched, logged := recordingServer(t)
	pid := leaderPID(t)

	sched.Register(pid, "capture.web")
	req := httptest.NewRequest(http.MethodPost, "/release?pid="+itoa(pid), nil)
	req.Header.Set("Content-Type", "application/json")
	s.Handler().ServeHTTP(httptest.NewRecorder(), req)

	out := logged.String()
	home, err := os.UserHomeDir()
	if err == nil && home != "" && home != "/" && strings.Contains(out, home) {
		t.Errorf("the log contains an absolute path inside $HOME:\n%s", out)
	}
	for _, leak := range []string{"PATH=", "HOME=", "GOPATH", "-----BEGIN"} {
		if strings.Contains(out, leak) {
			t.Errorf("the log contains %q:\n%s", leak, out)
		}
	}
}

// DDR-002's load-bearing half, at the unit level: the log is opened APPEND-only and
// write-only. A daemon that could read it back could inherit a stale lock, which is
// exactly what DDR-001 exists to prevent.
func TestOpenLogAppendsAndNeverTruncates(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_STATE_HOME", "")

	first, path, why := OpenLog()
	if why != "" {
		t.Fatalf("unexpected fallback: %s", why)
	}
	first.Print("first line")

	second, _, _ := OpenLog()
	second.Print("second line")

	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read the log: %v", err)
	}
	got := string(body)
	if !strings.Contains(got, "first line") {
		t.Error("reopening the log truncated it — it must append")
	}
	if !strings.Contains(got, "second line") {
		t.Error("the second write did not land")
	}
	if info, statErr := os.Stat(path); statErr == nil {
		if perm := info.Mode().Perm(); perm != 0o600 {
			t.Errorf("want the log 0600, got %04o", perm)
		}
	}
	_ = filepath.Dir(path)
}

// A log that cannot be written must not stop the daemon: losing the record is bad,
// refusing to schedule because a file is unwritable is worse.
func TestOpenLogFallsBackRatherThanFailing(t *testing.T) {
	// A home directory that cannot hold the log directory.
	unwritable := filepath.Join(t.TempDir(), "locked")
	if err := os.MkdirAll(unwritable, 0o500); err != nil {
		t.Fatalf("setup: %v", err)
	}
	t.Setenv("HOME", unwritable)
	t.Setenv("XDG_STATE_HOME", "")

	logger, _, why := OpenLog()

	if logger == nil {
		t.Fatal("OpenLog must always return a usable logger")
	}
	if why == "" {
		t.Skip("this platform allowed the write, so there is no fallback to observe")
	}
	if !strings.Contains(why, "stderr") {
		t.Errorf("the fallback must say where output goes instead; got %q", why)
	}
	logger.Print("must not panic")
}
