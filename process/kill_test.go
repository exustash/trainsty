package process

import (
	"bufio"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// TestKillGroupTerminatesTheLeaderAndEveryChild is the test that catches
// signalling a bare PID.
//
// A real E2E suite starts a fleet of headless browsers. If termination reaches
// only the leader, they survive — reparented to init, still holding the ports and
// the memory the next run needs — and the product has failed at the one thing
// that makes it Unix-only (ADR-002).
//
// The stand-in is a shell that starts three background children and prints their
// PIDs. In a non-interactive shell job control is off, so the children stay in
// the leader's process group, exactly as a test runner's children do.
func TestKillGroupTerminatesTheLeaderAndEveryChild(t *testing.T) {
	script := `sleep 30 & echo $!; sleep 30 & echo $!; sleep 30 & echo $!; wait`
	cmd := exec.Command("sh", "-c", script)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("stdout pipe: %v", err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	leader := cmd.Process.Pid
	t.Cleanup(func() {
		_ = KillGroup(leader)
		_ = cmd.Wait()
	})

	children := make([]int, 0, 3)
	scan := bufio.NewScanner(stdout)
	for len(children) < 3 && scan.Scan() {
		pid, convErr := strconv.Atoi(strings.TrimSpace(scan.Text()))
		if convErr != nil {
			t.Fatalf("unexpected line from the shell: %q", scan.Text())
		}
		children = append(children, pid)
	}
	if len(children) != 3 {
		t.Fatalf("want 3 child pids, got %v", children)
	}

	// Arrange is only complete once every process really exists — otherwise a
	// pass could mean "they were never started".
	for _, pid := range append([]int{leader}, children...) {
		alive, aliveErr := Alive(pid)
		if aliveErr != nil || !alive {
			t.Fatalf("precondition: pid %d should be alive (err=%v)", pid, aliveErr)
		}
	}

	if err := KillGroup(leader); err != nil {
		t.Fatalf("KillGroup: %v", err)
	}
	_, _ = cmd.Process.Wait() // reap the leader so its pid is not a zombie

	for _, pid := range append([]int{leader}, children...) {
		if !waitGone(t, pid, 3*time.Second) {
			t.Errorf("pid %d survived KillGroup — the signal went to a process, not a group", pid)
		}
	}
}

// pid 0 and pid 1 are the two arithmetic slips that would be catastrophic:
// kill(0) signals the daemon's own process group, and kill(-1) signals every
// process the user may signal.
func TestKillGroupRefusesPidsThatWouldSignalTooMuch(t *testing.T) {
	for _, pid := range []int{1, 0, -1, -5} {
		err := KillGroup(pid)
		if err == nil {
			t.Fatalf("KillGroup(%d) must be refused — it would signal far more than one job", pid)
		}
		if !strings.Contains(err.Error(), "refusing") {
			t.Errorf("KillGroup(%d): want a refusal, got %v", pid, err)
		}
	}
}

func TestKillGroupReportsAnErrorForAGroupThatIsGone(t *testing.T) {
	cmd := startLeader(t, "sleep", "30")
	pid := cmd.Process.Pid
	if err := KillGroup(pid); err != nil {
		t.Fatalf("first KillGroup: %v", err)
	}
	_, _ = cmd.Process.Wait()
	if !waitGone(t, pid, 2*time.Second) {
		t.Fatal("want the group gone")
	}

	// The caller decides what to do about it; this package does not silently
	// swallow the fact that there was nothing there.
	if err := KillGroup(pid); err == nil {
		t.Fatal("want an error when the group no longer exists")
	}
}
