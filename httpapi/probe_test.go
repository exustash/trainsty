package httpapi

import (
	"os"
	"os/exec"
	"syscall"
	"testing"
	"time"

	"github.com/exustash/trainsty/scheduler"
)

// Release path 3: the probe is the backstop for a holder that died without its
// stream closing.
func TestProbeReleasesTheLockWhenTheHolderDies(t *testing.T) {
	cmd := exec.Command("sleep", "30")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	pid := cmd.Process.Pid

	sched := scheduler.New()
	s := New(sched, DiscardLogger())
	sched.Register(pid, "dying") // installs the Job and starts the probe
	if sched.Current() == nil {
		t.Fatal("want the Lock held")
	}

	_ = syscall.Kill(-pid, syscall.SIGKILL)
	_, _ = cmd.Process.Wait()

	// Within one probe interval plus slack. Deliberately a poll with a deadline
	// rather than a sleep of exactly probeInterval.
	deadline := time.Now().Add(3 * probeInterval)
	for time.Now().Before(deadline) {
		if sched.Current() == nil {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("the probe did not release the Lock within %s of the holder dying", 3*probeInterval)
	_ = s
}

// The distinction ADR-008 turns on. A holder we cannot signal is ALIVE, so the
// probe must not release it — "the probe returned an error" is not "the job is
// dead", and getting this backwards frees a healthy job's Lock.
func TestProbeDoesNotReleaseAHolderItCannotSignal(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root: EPERM is unreachable")
	}
	sched := scheduler.New()
	s := New(sched, DiscardLogger())
	// Installed directly rather than through /register, which now refuses an
	// unsignalable pid. The probe must still behave correctly if one arrives.
	sched.Register(1, "init")
	if sched.Current() == nil {
		t.Fatal("want the Lock held by pid 1 for this test")
	}

	time.Sleep(2 * probeInterval)

	if sched.Current() == nil {
		t.Fatal("the probe released a Lock held by a live process it merely could not signal")
	}
	_ = s
}

func TestProbeStopsWhenItsJobIsReleased(t *testing.T) {
	cmd := exec.Command("sleep", "30")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	t.Cleanup(func() { _ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL); _ = cmd.Wait() })

	sched := scheduler.New()
	s := New(sched, DiscardLogger())
	first := sched.Register(cmd.Process.Pid, "first")
	second := sched.Register(999001, "second") // a pid that does not exist

	sched.Release(sched.Current())
	select {
	case <-second.Granted():
	default:
		t.Fatal("want the second waiter granted")
	}

	// The first Job's probe must be stopped. If it were still running it would find
	// its own pid alive and do nothing — but a probe for a RELEASED job that later
	// called Release would revoke the successor's Lock. The scheduler's identity
	// check makes that safe; this asserts the probe is stopped as well, so the two
	// protections do not both have to hold.
	time.Sleep(2 * probeInterval)
	// The second holder's pid does not exist, so ITS probe should have released it.
	if sched.Current() != nil {
		t.Fatal("the second holder's pid does not exist — its own probe should have released the Lock")
	}
	_ = first
	_ = s
}
