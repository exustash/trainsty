package process

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
	"testing"
	"time"
)

// startLeader starts a short-lived child that leads its own process group, and
// registers cleanup that kills the whole group.
//
// Every process test starts its own children and reaps them. Nothing here ever
// probes or signals a PID the test did not create: a hardcoded PID in a test is a
// signal aimed at whatever the machine happens to be running.
func startLeader(t *testing.T, args ...string) *exec.Cmd {
	t.Helper()
	cmd := exec.Command(args[0], args[1:]...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true} // Pgid == 0 ⇒ the child leads
	if err := cmd.Start(); err != nil {
		t.Fatalf("start %v: %v", args, err)
	}
	t.Cleanup(func() {
		_ = KillGroup(cmd.Process.Pid)
		_ = cmd.Wait() // reap, so the test leaves no zombie behind
	})
	return cmd
}

// waitGone polls until pid is gone or the deadline passes. Deliberately a poll
// rather than a sleep: a killed group is gone "soon", and a magic interval is
// either flaky or slow.
func waitGone(t *testing.T, pid int, within time.Duration) bool {
	t.Helper()
	deadline := time.Now().Add(within)
	for time.Now().Before(deadline) {
		alive, err := Alive(pid)
		if err != nil {
			t.Fatalf("Alive(%d) errored while polling: %v", pid, err)
		}
		if !alive {
			return true
		}
		time.Sleep(10 * time.Millisecond)
	}
	return false
}

func TestAliveReportsTrueForALiveProcess(t *testing.T) {
	cmd := startLeader(t, "sleep", "30")

	alive, err := Alive(cmd.Process.Pid)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !alive {
		t.Fatal("want alive for a running child")
	}
}

func TestAliveReportsFalseWithNoErrorAfterTheProcessDies(t *testing.T) {
	cmd := startLeader(t, "sleep", "30")
	pid := cmd.Process.Pid

	if err := KillGroup(pid); err != nil {
		t.Fatalf("KillGroup: %v", err)
	}
	_, _ = cmd.Process.Wait() // reap now, so the pid is fully gone rather than a zombie

	if !waitGone(t, pid, 2*time.Second) {
		t.Fatal("want the pid gone within 2s")
	}
	alive, err := Alive(pid)
	// The contract that matters: a dead process is (false, nil). An error here
	// would make a caller unable to tell "dead" from "could not tell", and only
	// the first may release a Lock.
	if err != nil {
		t.Fatalf("a dead pid must not error, got %v", err)
	}
	if alive {
		t.Fatal("want not alive")
	}
}

// The distinction ADR-008 turns on: a PID that exists but is not ours reports
// ALIVE with an error, never dead. Treating the error as death would free a
// healthy job's Lock.
func TestAliveReportsAliveWithAnErrorForAProcessOwnedByAnotherUser(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root: every process is signalable, so EPERM is unreachable")
	}
	// pid 1 is init/launchd — it exists and is not ours to signal. It is also the
	// one pid safe to PROBE without having started it, because signal 0 sends
	// nothing. Nothing in this package would ever pass 1 to KillGroup: that is
	// guarded against explicitly.
	alive, err := Alive(1)

	if !alive {
		t.Fatal("pid 1 must report alive")
	}
	if err == nil {
		t.Fatal("want a loud error for a process we cannot signal")
	}
	if !errors.Is(err, syscall.EPERM) {
		t.Fatalf("want the error to wrap EPERM so callers can classify it, got %v", err)
	}
}

func TestAliveRejectsNonPositivePids(t *testing.T) {
	for _, pid := range []int{0, -1, -42} {
		if _, err := Alive(pid); err == nil {
			t.Errorf("Alive(%d): want an error", pid)
		}
	}
}

func TestIsGroupLeaderIsTrueForASetpgidChild(t *testing.T) {
	cmd := startLeader(t, "sleep", "30")

	leader, err := IsGroupLeader(cmd.Process.Pid)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !leader {
		t.Fatal("a Setpgid child must lead its own group")
	}
}

// The refusal that prevents a killed shell. This test's subject is the test
// process itself: `go test` runs in the shell's process group, so it is a real
// non-leader and needs nothing started to demonstrate one.
func TestIsGroupLeaderIsFalseForANonLeader(t *testing.T) {
	self := os.Getpid()
	pgid, err := syscall.Getpgid(self)
	if err != nil {
		t.Fatalf("getpgid(self): %v", err)
	}
	if pgid == self {
		t.Skip("this test process happens to lead its own group, so it is not a non-leader example")
	}

	leader, err := IsGroupLeader(self)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if leader {
		t.Fatal("want false for a process that does not lead its group")
	}
}

func TestIsGroupLeaderReportsESRCHForAPidThatIsGone(t *testing.T) {
	cmd := startLeader(t, "sleep", "30")
	pid := cmd.Process.Pid
	if err := KillGroup(pid); err != nil {
		t.Fatalf("KillGroup: %v", err)
	}
	_, _ = cmd.Process.Wait()
	if !waitGone(t, pid, 2*time.Second) {
		t.Fatal("want the pid gone")
	}

	_, err := IsGroupLeader(pid)

	if err == nil {
		t.Fatal("want an error for a pid that does not exist")
	}
	// Wrapped rather than flattened, so the API layer can answer
	// no_such_process instead of a generic failure.
	if !errors.Is(err, syscall.ESRCH) {
		t.Fatalf("want the error to wrap ESRCH, got %v", err)
	}
}

func TestIsGroupLeaderRejectsNonPositivePids(t *testing.T) {
	for _, pid := range []int{0, -1} {
		if _, err := IsGroupLeader(pid); err == nil {
			t.Errorf("IsGroupLeader(%d): want an error", pid)
		}
	}
}
