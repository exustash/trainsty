//go:build e2e

// The build tag keeps these out of `go test ./...`.
//
// Without it the acceptance suite runs twice CONCURRENTLY under
// scripts/ci-local.sh — once inside the `go test -race` job, which matches ./...,
// and once inside the dedicated acceptance job — and the two fight over port
// 45678. Found by the gate on its first full run, which is the argument for the
// gate running the whole set rather than the jobs a developer remembers.
//
// Run them with:  go test -tags e2e -race -p 1 -run TestAcceptance .

package main

// Acceptance tests. These drive the BUILT BINARY, not the packages, and they are
// the executable form of the specification: each one names a scenario in
// specs/001-serialize-e2e-runs/quickstart.md and the requirements it proves.
//
// They bind the real port 45678, so they must run serially. None of them calls
// t.Parallel(), and scripts/ci-local.sh runs this suite with -p 1.
//
// This is trainsty's own contention problem applied to trainsty's own test suite:
// the product exists because concurrent local suites collide over machine-wide
// resources, and this suite is one of those suites. Hence requirePortFree.

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

const (
	acceptancePort = 45678
	acceptanceBase = "http://127.0.0.1:45678"
)

// buildBinary compiles trainsty into the test's temp directory and returns its
// path. Built once per test rather than shared, so a test can never run against a
// stale binary.
func buildBinary(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "trainsty")
	cmd := exec.Command("go", "build", "-o", bin, ".")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go build failed: %v\n%s", err, out)
	}
	return bin
}

// requirePortFree skips rather than fails when a real daemon holds the port.
//
// Skipping is the correct outcome: the suite cannot run beside a live daemon, and
// reporting that as a code failure would send the reader looking for a bug that
// does not exist.
func requirePortFree(t *testing.T) {
	t.Helper()
	if portListening() {
		t.Skipf("port %d is already in use — stop the daemon first ('trainsty status', then 'trainsty stop')", acceptancePort)
	}
}

func portListening() bool {
	client := &http.Client{Timeout: 500 * time.Millisecond}
	resp, err := client.Get(acceptanceBase + "/status")
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return true
}

// startDaemon starts the daemon and guarantees it is gone afterwards, whatever the
// test does.
func startDaemon(t *testing.T, bin string) {
	t.Helper()
	out, err := exec.Command(bin, "start").CombinedOutput()
	if err != nil {
		t.Fatalf("trainsty start failed: %v\n%s", err, out)
	}
	t.Cleanup(func() {
		_ = exec.Command(bin, "stop", "--force").Run()
		// Leaving the port bound would make every later test skip, so confirm.
		deadline := time.Now().Add(3 * time.Second)
		for time.Now().Before(deadline) {
			if !portListening() {
				return
			}
			time.Sleep(50 * time.Millisecond)
		}
		t.Errorf("the daemon was still listening after stop — later tests will skip")
	})
	if !waitFor(3*time.Second, portListening) {
		t.Fatal("the daemon did not start listening within 3s")
	}
}

type statusPayload struct {
	Job *struct {
		PID            int    `json:"pid"`
		Repo           string `json:"repo"`
		ElapsedSeconds int    `json:"elapsedSeconds"`
	} `json:"job"`
	Queue []struct {
		PID            int    `json:"pid"`
		Repo           string `json:"repo"`
		WaitingSeconds int    `json:"waitingSeconds"`
	} `json:"queue"`
}

func fetchStatus(t *testing.T) statusPayload {
	t.Helper()
	resp, err := http.Get(acceptanceBase + "/status")
	if err != nil {
		t.Fatalf("GET /status: %v", err)
	}
	defer resp.Body.Close()
	var payload statusPayload
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode /status: %v", err)
	}
	return payload
}

// syncBuffer is a concurrency-safe sink for a child's output.
//
// A plain strings.Builder is not: os/exec copies into it from its own goroutine
// while the test polls it from another, which -race correctly reports as a data
// race. The race was in this harness rather than in the product, which is a fair
// reminder that -race is not only for the code under test.
type syncBuffer struct {
	mu sync.Mutex
	b  strings.Builder
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

// wrapped starts `trainsty wrap -- <args>` with its output captured, in its own
// process group so the test can signal it the way a terminal would.
type wrapped struct {
	cmd *exec.Cmd
	out *syncBuffer
}

func startWrapped(t *testing.T, bin string, args ...string) *wrapped {
	t.Helper()
	full := append([]string{"wrap", "--"}, args...)
	cmd := exec.Command(bin, full...)
	sb := &syncBuffer{}
	cmd.Stdout = sb
	cmd.Stderr = sb
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		t.Fatalf("start wrap %v: %v", args, err)
	}
	w := &wrapped{cmd: cmd, out: sb}
	t.Cleanup(func() {
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		_ = cmd.Wait()
	})
	return w
}

func (w *wrapped) output() string { return w.out.String() }

func waitFor(within time.Duration, cond func() bool) bool {
	deadline := time.Now().Add(within)
	for time.Now().Before(deadline) {
		if cond() {
			return true
		}
		time.Sleep(25 * time.Millisecond)
	}
	return false
}

// ---------------------------------------------------------------------------
// Scenario 1 — two runs queue, in order.
// quickstart.md scenario 1 · FR-001, FR-002, FR-003, FR-005 · US1 §1-4 · SC-001
// ---------------------------------------------------------------------------

func TestAcceptanceTwoRunsQueueInOrder(t *testing.T) {
	requirePortFree(t)
	bin := buildBinary(t)
	startDaemon(t, bin)

	first := startWrapped(t, bin, "sh", "-c", "echo FIRST-START; sleep 3; echo FIRST-DONE")
	if !waitFor(3*time.Second, func() bool { return strings.Contains(first.output(), "FIRST-START") }) {
		t.Fatalf("the first suite never started; output was %q", first.output())
	}

	second := startWrapped(t, bin, "sh", "-c", "echo SECOND-START")

	// The second must WAIT: no suite output while the first holds the Lock.
	time.Sleep(750 * time.Millisecond)
	if strings.Contains(second.output(), "SECOND-START") {
		t.Fatal("the second suite ran while the first held the lock — FR-001 is broken")
	}

	st := fetchStatus(t)
	if st.Job == nil {
		t.Fatal("want a job holding the lock")
	}
	if len(st.Queue) != 1 {
		t.Fatalf("want exactly one waiter, got %d", len(st.Queue))
	}

	// …and then it runs, after the first finishes.
	if !waitFor(15*time.Second, func() bool { return strings.Contains(second.output(), "SECOND-START") }) {
		t.Fatalf("the second suite never ran after the first finished; output was %q", second.output())
	}
	if !strings.Contains(first.output(), "FIRST-DONE") {
		t.Fatalf("the first suite did not finish before the second started: %q", first.output())
	}
}

// ---------------------------------------------------------------------------
// Scenario 3 — THE MANDATORY TEST.
// The holder is killed outright, with no chance to release. The next waiter must
// still be promoted. quickstart.md scenario 3 · FR-007, FR-008 · US1 §6 · SC-003
//
// The constitution names this test specifically. Everything else can work and the
// product is still unusable if this does not: a lock held by a dead job blocks
// every suite on the machine.
// ---------------------------------------------------------------------------

func TestAcceptanceKilledHolderReleasesTheLock(t *testing.T) {
	requirePortFree(t)
	bin := buildBinary(t)
	startDaemon(t, bin)

	holder := startWrapped(t, bin, "sleep", "300")
	if !waitFor(5*time.Second, func() bool { return fetchStatus(t).Job != nil }) {
		t.Fatal("the first suite never took the lock")
	}

	waiter := startWrapped(t, bin, "sh", "-c", "echo PROMOTED")
	if !waitFor(5*time.Second, func() bool { return len(fetchStatus(t).Queue) == 1 }) {
		t.Fatal("the second suite never queued")
	}

	// SIGKILL: no trap, no release, no goodbye.
	if err := syscall.Kill(-holder.cmd.Process.Pid, syscall.SIGKILL); err != nil {
		t.Fatalf("kill the holder's group: %v", err)
	}

	if !waitFor(10*time.Second, func() bool { return strings.Contains(waiter.output(), "PROMOTED") }) {
		t.Fatalf("the waiter was never promoted after the holder was killed — the lock is stuck; output was %q", waiter.output())
	}
}

// ---------------------------------------------------------------------------
// Scenario 2 — Ctrl+C frees the lock AND reaches the suite.
// quickstart.md scenario 2 · FR-006, FR-036 · US1 §5, §15 · SC-002 · research R2
//
// This is the test that fails if wrap does not forward signals: the terminal
// delivers SIGINT to the FOREGROUND process group only, and the suite is
// deliberately in its own group so it can be terminated as a unit.
// ---------------------------------------------------------------------------

func TestAcceptanceInterruptStopsTheSuiteAndFreesTheLock(t *testing.T) {
	requirePortFree(t)
	bin := buildBinary(t)
	startDaemon(t, bin)

	// The suite records its OWN pid, which is what this test must assert on.
	//
	// /status reports the REGISTERED pid, and that is wrap's own — wrap makes itself
	// a group leader and registers itself, because the suite cannot be started
	// before the Grant and so has no pid to register (see runner.Wrap). wrap is also
	// meant to SURVIVE the interrupt long enough to release the Lock, so asserting
	// that the registered pid died would assert the opposite of the design.
	pidFile := filepath.Join(t.TempDir(), "suite.pid")
	holder := startWrapped(t, bin, "sh", "-c",
		fmt.Sprintf("echo $$ > %s; sleep 300", pidFile))
	if !waitFor(5*time.Second, func() bool { return fetchStatus(t).Job != nil }) {
		t.Fatal("the first suite never took the lock")
	}
	var suitePID int
	if !waitFor(5*time.Second, func() bool {
		raw, err := os.ReadFile(pidFile)
		if err != nil {
			return false
		}
		suitePID, err = strconv.Atoi(strings.TrimSpace(string(raw)))
		return err == nil && suitePID > 0
	}) {
		t.Fatal("the suite never started, or never recorded its pid")
	}

	waiter := startWrapped(t, bin, "sh", "-c", "echo NEXT-RAN")

	// Interrupt wrap the way a terminal would.
	if err := holder.cmd.Process.Signal(syscall.SIGINT); err != nil {
		t.Fatalf("signal wrap: %v", err)
	}

	// 1. the lock is freed fast — via the dropped registration, not the 3-5s probe
	if !waitFor(4*time.Second, func() bool { return strings.Contains(waiter.output(), "NEXT-RAN") }) {
		t.Fatalf("the next suite did not run within 4s of the interrupt; output was %q", waiter.output())
	}
	// 2. and the interrupted SUITE is actually gone, not orphaned
	if !waitFor(4*time.Second, func() bool { return syscall.Kill(suitePID, 0) != nil }) {
		t.Errorf("suite pid %d survived the interrupt — wrap is not forwarding the signal to its process group", suitePID)
	}
}

// ---------------------------------------------------------------------------
// Scenario 9 — no daemon is not a blocker.
// quickstart.md scenario 9 · FR-018, FR-038 · contracts/cli.md
// ---------------------------------------------------------------------------

func TestAcceptanceNoDaemonStillRunsTheSuite(t *testing.T) {
	requirePortFree(t)
	bin := buildBinary(t)
	// Deliberately NO daemon.

	cmd := exec.Command(bin, "wrap", "--", "sh", "-c", "echo RAN-UNSCHEDULED; exit 0")
	out, err := cmd.CombinedOutput()

	if err != nil {
		t.Fatalf("wrap must not fail when no scheduler is reachable: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "RAN-UNSCHEDULED") {
		t.Fatalf("the suite did not run; output was %q", out)
	}
	// The exact wording is the contract: a developer must be able to see that a run
	// was unscheduled, because a silently unscheduled run is indistinguishable from
	// a scheduled one until two collide.
	if !strings.Contains(string(out), "NOT serialized") {
		t.Errorf("wrap must say the run was not serialized; output was %q", out)
	}
}

func TestAcceptanceWrapPropagatesTheSuiteExitStatus(t *testing.T) {
	requirePortFree(t)
	bin := buildBinary(t)
	startDaemon(t, bin)

	// A failing suite must stay failing: swallowing its status turns a red suite
	// green, which is worse than having no scheduler at all (FR-037).
	err := exec.Command(bin, "wrap", "--", "sh", "-c", "exit 7").Run()

	exitErr, ok := err.(*exec.ExitError)
	if !ok {
		t.Fatalf("want an exit error, got %v", err)
	}
	if exitErr.ExitCode() != 7 {
		t.Fatalf("want the suite's own exit code 7, got %d", exitErr.ExitCode())
	}
}

func TestAcceptanceStatusExitsThreeWhenNoDaemonIsReachable(t *testing.T) {
	requirePortFree(t)
	bin := buildBinary(t)

	err := exec.Command(bin, "status").Run()

	exitErr, ok := err.(*exec.ExitError)
	if !ok {
		t.Fatalf("want a non-zero exit, got %v", err)
	}
	// 3, not 1: a Runner must be able to tell "no scheduler" from "scheduler said
	// no" (FR-018).
	if exitErr.ExitCode() != 3 {
		t.Fatalf("want exit code 3 for an unreachable scheduler, got %d", exitErr.ExitCode())
	}
}
