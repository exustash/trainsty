package scheduler

import (
	"sync"
	"testing"
	"time"
)

// granted reports whether w holds the Lock, without blocking. Every assertion
// here is about state the Scheduler already decided, so no test needs to wait.
func granted(w *Waiter) bool {
	select {
	case <-w.Granted():
		return true
	default:
		return false
	}
}

func mustGranted(t *testing.T, w *Waiter, who string) {
	t.Helper()
	if !granted(w) {
		t.Fatalf("%s: want the Lock, did not get it", who)
	}
}

func mustWaiting(t *testing.T, w *Waiter, who string) {
	t.Helper()
	if granted(w) {
		t.Fatalf("%s: want to be waiting, but holds the Lock", who)
	}
}

func TestRegisterGrantsImmediatelyWhenTheLockIsFree(t *testing.T) {
	s := New()

	w := s.Register(100, "capture.web")

	mustGranted(t, w, "the first Runner")
	if job := s.Current(); job == nil || job.PID != 100 || job.Repo != "capture.web" {
		t.Fatalf("want pid 100 holding the Lock, got %+v", job)
	}
}

func TestOnlyOneRegistrationHoldsTheLock(t *testing.T) {
	s := New()

	first := s.Register(100, "a")
	second := s.Register(200, "b")

	mustGranted(t, first, "the first Runner")
	mustWaiting(t, second, "the second Runner")
}

// FIFO is the whole scheduling policy (ADR-006). Table-driven because the cases
// differ only in how many wait and in which order they are released.
func TestQueueIsStrictlyFirstInFirstOut(t *testing.T) {
	for _, tc := range []struct {
		name string
		pids []int
	}{
		{"two waiters", []int{100, 200}},
		{"three waiters", []int{100, 200, 300}},
		{"five waiters", []int{100, 200, 300, 400, 500}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := New()
			waiters := make([]*Waiter, 0, len(tc.pids))
			for _, pid := range tc.pids {
				waiters = append(waiters, s.Register(pid, "repo"))
			}

			// Each release must promote exactly the next in arrival order.
			for i := range tc.pids {
				mustGranted(t, waiters[i], "waiter in position")
				for j := i + 1; j < len(tc.pids); j++ {
					mustWaiting(t, waiters[j], "a later waiter")
				}
				if job := s.Current(); job == nil || job.PID != tc.pids[i] {
					t.Fatalf("position %d: want pid %d holding, got %+v", i, tc.pids[i], job)
				}
				if !s.Release(s.Current()) {
					t.Fatalf("position %d: Release reported nothing released", i)
				}
			}
			if job := s.Current(); job != nil {
				t.Fatalf("want the Lock free after the last release, got %+v", job)
			}
		})
	}
}

// ADR-007: a reconnecting Runner keeps its place. Appending again would starve a
// Runner on a flaky connection and make FIFO untrue of anything observable.
func TestReRegisteringAQueuedPidKeepsItsPosition(t *testing.T) {
	s := New()
	holder := s.Register(100, "holder")
	second := s.Register(200, "second")
	third := s.Register(300, "third")
	mustGranted(t, holder, "the holder")

	again := s.Register(200, "second")

	if again != second {
		t.Fatal("re-registering a queued pid must return its existing Waiter, not a new one")
	}
	if snap := s.Snapshot(); len(snap.Queue) != 2 {
		t.Fatalf("want 2 waiters after a re-registration, got %d — a duplicate entry was added", len(snap.Queue))
	}
	// And it is still ahead of the one that arrived after it.
	s.Release(s.Current())
	mustGranted(t, second, "the reconnected waiter")
	mustWaiting(t, third, "the waiter behind it")
}

func TestReRegisteringTheCurrentHolderGrantsImmediately(t *testing.T) {
	s := New()
	first := s.Register(100, "holder")
	mustGranted(t, first, "the holder")

	again := s.Register(100, "holder")

	mustGranted(t, again, "the holder re-registering")
	if snap := s.Snapshot(); len(snap.Queue) != 0 {
		t.Fatalf("the holder must not be queued behind itself, queue=%d", len(snap.Queue))
	}
}

func TestReleaseIsIdempotent(t *testing.T) {
	s := New()
	s.Register(100, "a")
	job := s.Current()

	if !s.Release(job) {
		t.Fatal("first Release: want true")
	}
	if s.Release(job) {
		t.Fatal("second Release for the same Job must be a no-op")
	}
	if s.Release(nil) {
		t.Fatal("Release(nil) must be a no-op")
	}
}

// Invariant 3, and the reason Release takes a Job rather than releasing whatever
// holds the Lock. A late release from a finished holder must not revoke its
// SUCCESSOR's Lock — plausible, wrong, and invisible until two suites collide.
func TestALateReleaseDoesNotReleaseTheSuccessor(t *testing.T) {
	s := New()
	s.Register(100, "first")
	second := s.Register(200, "second")
	stale := s.Current() // the first Job, about to finish

	s.Release(stale)
	mustGranted(t, second, "the successor")
	successor := s.Current()

	if s.Release(stale) {
		t.Fatal("a stale Release reported that it released something")
	}
	if s.Current() != successor {
		t.Fatal("a stale Release revoked the successor's Lock — invariant 3 is broken")
	}
}

func TestReleaseByPIDOnlyReleasesTheMatchingJob(t *testing.T) {
	for _, tc := range []struct {
		name         string
		pid          int
		wantReleased bool
	}{
		{"the holder's own pid releases", 100, true},
		{"a different pid does not", 999, false},
		{"pid 0 means whatever holds it", 0, true},
		{"a negative pid means the same", -1, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := New()
			s.Register(100, "holder")

			released := s.ReleaseByPID(tc.pid)

			if released != tc.wantReleased {
				t.Fatalf("ReleaseByPID(%d) = %v, want %v", tc.pid, released, tc.wantReleased)
			}
			if tc.wantReleased && s.Current() != nil {
				t.Fatal("want the Lock free")
			}
			if !tc.wantReleased && s.Current() == nil {
				t.Fatal("want the Lock still held")
			}
		})
	}
}

func TestReleaseByPIDOnAFreeLockReleasesNothing(t *testing.T) {
	s := New()
	if s.ReleaseByPID(100) {
		t.Fatal("want false when nothing holds the Lock")
	}
}

// A dropped Registration is the primary release signal (ADR-008).
func TestWithdrawReleasesTheLockWhenTheHolderDisappears(t *testing.T) {
	s := New()
	holder := s.Register(100, "holder")
	second := s.Register(200, "second")
	mustGranted(t, holder, "the holder")

	released := s.Withdraw(holder)

	if !released {
		t.Fatal("withdrawing the holder must release the Lock")
	}
	mustGranted(t, second, "the next waiter")
}

// A Waiter holds nothing, so withdrawing one must not release the Lock from
// whoever does.
func TestWithdrawingAWaiterDoesNotReleaseTheLock(t *testing.T) {
	s := New()
	holder := s.Register(100, "holder")
	second := s.Register(200, "second")
	third := s.Register(300, "third")

	released := s.Withdraw(second)

	if released {
		t.Fatal("withdrawing a queued Waiter must not release anything")
	}
	if s.Current() == nil || s.Current().PID != 100 {
		t.Fatal("the holder must still hold the Lock")
	}
	if snap := s.Snapshot(); len(snap.Queue) != 1 || snap.Queue[0].PID != 300 {
		t.Fatalf("want only pid 300 queued, got %+v", snap.Queue)
	}
	// And the one behind it is promoted when the holder goes, not the withdrawn one.
	s.Release(s.Current())
	mustGranted(t, third, "the waiter behind the withdrawn one")
	_ = holder
}

func TestWithdrawIsSafeForAnUnknownOrNilWaiter(t *testing.T) {
	s := New()
	s.Register(100, "holder")
	stranger := &Waiter{PID: 999, grant: make(chan struct{}, 1)}

	if s.Withdraw(stranger) {
		t.Fatal("withdrawing a Waiter this Scheduler never issued must do nothing")
	}
	if s.Withdraw(nil) {
		t.Fatal("Withdraw(nil) must do nothing")
	}
	if s.Current() == nil {
		t.Fatal("the holder must be untouched")
	}
}

func TestStopProbeRunsOnRelease(t *testing.T) {
	s := New()
	s.Register(100, "holder")
	job := s.Current()
	stopped := false
	s.SetStopProbe(job, func() { stopped = true })

	s.Release(job)

	if !stopped {
		t.Fatal("releasing a Job must stop its liveness probe — one that outlives its Job releases a healthy successor")
	}
}

func TestOnGrantHookFiresForEachGrant(t *testing.T) {
	s := New()
	var got []int
	s.SetOnGrant(func(j *Job) {
		if j != nil {
			got = append(got, j.PID)
		}
	})

	s.Register(100, "a")
	s.Register(200, "b")
	s.Release(s.Current())

	if len(got) != 2 || got[0] != 100 || got[1] != 200 {
		t.Fatalf("want the hook for each grant in order [100 200], got %v", got)
	}
}

func TestSnapshotReportsTheJobTheQueueAndElapsedTimes(t *testing.T) {
	s := New()
	base := time.Date(2026, 9, 14, 10, 0, 0, 0, time.UTC)
	now := base
	s.SetClock(func() time.Time { return now })

	s.Register(100, "capture.web")
	now = base.Add(30 * time.Second)
	s.Register(200, "capture.desk")
	now = base.Add(90 * time.Second)

	snap := s.Snapshot()

	if snap.Job == nil {
		t.Fatal("want a Job in the snapshot")
	}
	if snap.Job.PID != 100 || snap.Job.Repo != "capture.web" {
		t.Fatalf("unexpected Job: %+v", snap.Job)
	}
	if snap.Job.ElapsedSeconds != 90 {
		t.Fatalf("want elapsed 90s computed server-side, got %d", snap.Job.ElapsedSeconds)
	}
	if len(snap.Queue) != 1 || snap.Queue[0].PID != 200 {
		t.Fatalf("unexpected queue: %+v", snap.Queue)
	}
	if snap.Queue[0].WaitingSeconds != 60 {
		t.Fatalf("want waiting 60s, got %d", snap.Queue[0].WaitingSeconds)
	}
}

// The Dashboard must never have to tell an absent list from an empty one.
func TestSnapshotQueueIsNeverNil(t *testing.T) {
	s := New()

	snap := s.Snapshot()

	if snap.Job != nil {
		t.Fatal("want no Job on a fresh Scheduler")
	}
	if snap.Queue == nil {
		t.Fatal("Queue must be an empty slice, never nil")
	}
	if len(snap.Queue) != 0 {
		t.Fatalf("want an empty queue, got %d", len(snap.Queue))
	}
}

// Invariant 2: one entry per PID across the Job and the Queue together.
func TestNoPidAppearsTwiceAcrossTheJobAndTheQueue(t *testing.T) {
	s := New()
	s.Register(100, "a")
	s.Register(200, "b")
	s.Register(100, "a") // the holder again
	s.Register(200, "b") // a queued waiter again

	snap := s.Snapshot()

	seen := map[int]bool{}
	if snap.Job != nil {
		seen[snap.Job.PID] = true
	}
	for _, w := range snap.Queue {
		if seen[w.PID] {
			t.Fatalf("pid %d appears more than once across the Job and the Queue", w.PID)
		}
		seen[w.PID] = true
	}
	if len(seen) != 2 {
		t.Fatalf("want 2 distinct pids, got %d", len(seen))
	}
}

// Concurrency is the whole risk in this package: a race here is a wrong Grant,
// which means two suites running at once. -race is what makes this meaningful.
func TestConcurrentRegisterAndReleaseNeverGrantsTwice(t *testing.T) {
	s := New()
	const runners = 32
	var wg sync.WaitGroup

	for i := 0; i < runners; i++ {
		wg.Add(1)
		go func(pid int) {
			defer wg.Done()
			w := s.Register(pid, "repo")
			if granted(w) {
				// Whoever gets it releases it, so the queue keeps draining.
				s.Withdraw(w)
			}
		}(1000 + i)
	}
	wg.Wait()

	// Drain whatever is left, asserting the invariant at every step.
	for {
		job := s.Current()
		if job == nil {
			break
		}
		snap := s.Snapshot()
		for _, w := range snap.Queue {
			if w.PID == job.PID {
				t.Fatalf("pid %d is both the Job and queued", job.PID)
			}
		}
		if !s.Release(job) {
			t.Fatal("Release of the current Job reported nothing")
		}
	}
	if snap := s.Snapshot(); len(snap.Queue) != 0 {
		t.Fatalf("want a drained queue, got %d waiters", len(snap.Queue))
	}
}
