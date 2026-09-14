package runner

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestExitCodePropagatesTheSuiteStatus(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		want int
	}{
		{"success is 0", []string{"true"}, 0},
		{"a failing suite keeps its code", []string{"sh", "-c", "exit 7"}, 7},
		{"a 1 stays a 1", []string{"false"}, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cmd := exec.Command(tc.args[0], tc.args[1:]...)
			err := cmd.Run()

			// Swallowing a failure would turn a red suite green, which is worse than
			// having no scheduler at all (FR-037).
			if got := exitCode(err); got != tc.want {
				t.Fatalf("exitCode = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestExitCodeIsOneForAFailureThatIsNotAnExitStatus(t *testing.T) {
	// A command that could not start at all has no exit status to propagate.
	err := exec.Command("this-command-does-not-exist-trainsty").Run()
	if got := exitCode(err); got != 1 {
		t.Fatalf("want 1 for a start failure, got %d", got)
	}
}

// becomeGroupLeader is what makes the registration acceptable at all: the Daemon
// refuses a pid that does not lead its own group, and wrap registers its own.
func TestBecomeGroupLeaderMakesThisProcessALeader(t *testing.T) {
	// Run in a child, because changing the test process's own process group would
	// detach the test binary from `go test`'s job control for every later test.
	if os.Getenv("TRAINSTY_WRAP_LEADER_CHILD") == "1" {
		if err := becomeGroupLeader(); err != nil {
			os.Exit(9)
		}
		pid := os.Getpid()
		pgid, err := syscall.Getpgid(pid)
		if err != nil || pgid != pid {
			os.Exit(8)
		}
		os.Exit(0)
	}

	// nosemgrep: go.lang.security.audit.dangerous-exec-command.dangerous-exec-command
	// Audited: os.Args[0] is this test binary and the arguments are literals.
	cmd := exec.Command(os.Args[0], "-test.run=TestBecomeGroupLeaderMakesThisProcessALeader")
	cmd.Env = append(os.Environ(), "TRAINSTY_WRAP_LEADER_CHILD=1")
	// Started in the test's group deliberately: the child must be a NON-leader so
	// becomeGroupLeader has something to do.
	out, err := cmd.CombinedOutput()

	if err != nil {
		t.Fatalf("the child failed to become a group leader: %v\n%s", err, out)
	}
}

func TestBecomeGroupLeaderIsANoOpWhenAlreadyLeading(t *testing.T) {
	if os.Getenv("TRAINSTY_WRAP_LEADER_TWICE") == "1" {
		if err := becomeGroupLeader(); err != nil {
			os.Exit(9)
		}
		// Second call: setpgid on a process that already leads its group must not be
		// treated as an error. An interactive shell has already done it, so this is
		// the common path.
		if err := becomeGroupLeader(); err != nil {
			os.Exit(7)
		}
		os.Exit(0)
	}

	// nosemgrep: go.lang.security.audit.dangerous-exec-command.dangerous-exec-command
	// Audited: os.Args[0] is this test binary and the arguments are literals.
	cmd := exec.Command(os.Args[0], "-test.run=TestBecomeGroupLeaderIsANoOpWhenAlreadyLeading")
	cmd.Env = append(os.Environ(), "TRAINSTY_WRAP_LEADER_TWICE=1")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("calling it twice must be safe: %v\n%s", err, out)
	}
}

func TestRepoLabelNamesSomething(t *testing.T) {
	label := repoLabel()
	if strings.TrimSpace(label) == "" {
		t.Fatal("a repo label must never be empty — the Daemon refuses one, and the dashboard shows it")
	}
	if strings.ContainsAny(label, "\n\r") {
		t.Fatalf("a label must not contain control characters: %q", label)
	}
}

// With no daemon reachable, acquire must not block and must not fail: the suite
// runs anyway and the developer is told once (FR-038).
func TestAcquireFallsBackWhenNoDaemonIsReachable(t *testing.T) {
	var sb strings.Builder

	release := acquire(os.Getpid(), "test", &sb)
	defer release()

	// The notice IS the assertion that nothing was waited for: acquire returns
	// without blocking, and says so once.
	if !strings.Contains(sb.String(), "NOT serialized") {
		t.Fatalf("want a notice that the run is unscheduled, got %q", sb.String())
	}
}

// forwardSignals relays to the whole process group, and the re-entry guard is what
// lets it: the signaller is itself in that group, so it receives its own copy. Without
// the guard it would forward for ever.
//
// Run in a child process, because the subject is what happens when THIS process is
// signalled — doing that to the test binary would end the run.
func TestForwardSignalsRelaysOnceAndSurvivesItsOwnSignal(t *testing.T) {
	if os.Getenv("TRAINSTY_FORWARD_CHILD") == "1" {
		forwardChild()
		return
	}

	// nosemgrep: go.lang.security.audit.dangerous-exec-command.dangerous-exec-command
	// Audited: os.Args[0] is this test binary and the arguments are literals.
	cmd := exec.Command(os.Args[0], "-test.run=TestForwardSignalsRelaysOnceAndSurvivesItsOwnSignal", "-test.v")
	cmd.Env = append(os.Environ(), "TRAINSTY_FORWARD_CHILD=1")
	// Its own group, so signalling it cannot reach the test runner.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	out, err := cmd.CombinedOutput()

	if err != nil {
		t.Fatalf("the child reported a failure: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "FORWARDED-ONCE") {
		t.Fatalf("the child did not confirm a single forward:\n%s", out)
	}
}

// forwardChild runs inside the child: it leads its own group, installs the
// forwarder, signals its own group, and reports that it survived and did not loop.
func forwardChild() {
	if err := becomeGroupLeader(); err != nil {
		fmt.Fprintln(os.Stderr, "child: could not lead its group:", err)
		os.Exit(9)
	}

	// A sacrificial child in the same group, so there is something the forwarded
	// signal can be observed killing.
	victim := exec.Command("sleep", "30")
	if err := victim.Start(); err != nil {
		fmt.Fprintln(os.Stderr, "child: could not start the victim:", err)
		os.Exit(9)
	}

	stop := forwardSignals()
	defer stop()

	// Deliver the signal the way a terminal would: to the group.
	if err := syscall.Kill(-os.Getpid(), syscall.SIGTERM); err != nil {
		fmt.Fprintln(os.Stderr, "child: could not signal its own group:", err)
		os.Exit(9)
	}

	// Death is observed by WAITING on the victim, not by probing its pid.
	//
	// A killed child of this process stays a zombie until it is reaped, and a zombie
	// still answers kill(pid, 0) successfully — so a probe loop reports "survived" for
	// a process that is already dead. This trap has now cost three tests in this
	// repository; knowledge/conventions/testing.md carries the rule.
	died := make(chan struct{})
	go func() { _ = victim.Wait(); close(died) }()

	// Surviving at all is the other half of the assertion: signal.Notify disabled the
	// default action, so this process must still be here to reap and release. Without
	// the re-entry guard it would forward its own copy for ever instead of returning.
	select {
	case <-died:
		fmt.Println("FORWARDED-ONCE")
		os.Exit(0)
	case <-time.After(5 * time.Second):
		fmt.Fprintln(os.Stderr, "child: the victim survived — the signal did not reach the group")
		_ = syscall.Kill(victim.Process.Pid, syscall.SIGKILL)
		os.Exit(8)
	}
}

// The release closure must be safe to call more than once: a deferred release and a
// dropped stream are two independent paths to the same place.
func TestReleaseClosureIsIdempotentWhenThereIsNoDaemon(t *testing.T) {
	var sb strings.Builder
	release := acquire(os.Getpid(), "test", &sb)

	release()
	release() // must not panic, and must not block
}
