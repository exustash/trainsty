package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"syscall"
	"testing"

	"github.com/exustash/trainsty/scheduler"
)

// ADR-009 is a BEHAVIOUR, so it is asserted rather than assumed: a shutdown drops
// the Lock and leaves the suite running. Conflating this with Stop would destroy
// work the developer never asked to lose.
func TestShutdownReleasesTheLockWithoutKillingTheJob(t *testing.T) {
	cmd := exec.Command("sleep", "30")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	pid := cmd.Process.Pid
	t.Cleanup(func() { _ = syscall.Kill(-pid, syscall.SIGKILL); _ = cmd.Wait() })

	sched := scheduler.New()
	s := New(sched, DiscardLogger())
	sched.Register(pid, "running")

	req := httptest.NewRequest(http.MethodPost, "/shutdown", nil)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	var payload struct {
		ShuttingDown bool `json:"shuttingDown"`
		JobWasActive bool `json:"jobWasActive"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !payload.ShuttingDown {
		t.Error("want shuttingDown true")
	}
	// This is what lets `trainsty stop` warn BEFORE it confirms.
	if !payload.JobWasActive {
		t.Error("want jobWasActive true so the CLI can warn about the running suite")
	}

	// The suite must still be alive: the Lock is dropped, the process is not killed.
	if err := syscall.Kill(pid, 0); err != nil {
		t.Fatalf("the suite was killed by a shutdown — that is Stop's behaviour, not Shutdown's: %v", err)
	}
}

func TestShutdownReportsNoActiveJobWhenIdle(t *testing.T) {
	s := New(scheduler.New(), DiscardLogger())
	req := httptest.NewRequest(http.MethodPost, "/shutdown", nil)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	s.Handler().ServeHTTP(rec, req)

	var payload struct {
		JobWasActive bool `json:"jobWasActive"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &payload)
	if payload.JobWasActive {
		t.Error("want jobWasActive false on an idle daemon")
	}
}

// A no-op is a success: a Runner's trap fires on paths where the Lock is already
// free, and a trap that errors on the normal path is a trap developers delete.
func TestReleaseIsASuccessWhenNothingHoldsTheLock(t *testing.T) {
	s := New(scheduler.New(), DiscardLogger())
	req := httptest.NewRequest(http.MethodPost, "/release", nil)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	s.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200 for a no-op release, got %d", rec.Code)
	}
	var payload struct {
		Released bool `json:"released"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &payload)
	if payload.Released {
		t.Error("want released false when nothing held the Lock")
	}
}

// The optional pid is what makes a release identity-checked, closing the race
// where a late release from a finished holder revokes its successor's Lock.
func TestReleaseWithAMismatchedPidReleasesNothing(t *testing.T) {
	sched := scheduler.New()
	s := New(sched, DiscardLogger())
	sched.Register(101, "holder")

	req := httptest.NewRequest(http.MethodPost, "/release?pid=999", nil)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)

	var payload struct {
		Released bool `json:"released"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &payload)
	if payload.Released {
		t.Fatal("a release naming another pid must not release this holder")
	}
	if sched.Current() == nil {
		t.Fatal("the holder must still hold the Lock")
	}
}
