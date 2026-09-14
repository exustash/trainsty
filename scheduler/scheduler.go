// Package scheduler owns the Lock and the Queue, and nothing else.
//
// It imports neither net/http nor syscall nor os/exec, deliberately: this is the
// part of trainsty that must not be wrong, and keeping it pure is what lets it be
// tested with no server, no child processes and no sleeping
// (knowledge/conventions/go.md → Package layout).
//
// All state lives in one Scheduler behind one mutex
// (.specify/memory/constitution.md → Principle V). Nothing is persisted: a
// restart means the Lock is free, which is the only answer that cannot be wrong
// for long (DDR-001).
package scheduler

import (
	"sync"
	"time"
)

// Job is the holder of the Lock, and the suite it is running. At most one exists.
type Job struct {
	// PID is the process group leader's id, as registered. The unit of
	// termination — a Stop signals the group, never this pid alone (ADR-002).
	PID int
	// Repo is a display label for the Dashboard and nothing more (OD-3).
	Repo      string
	GrantedAt time.Time

	// waiter is the Registration this Job was granted from. It is what makes a
	// Release identity-checked rather than positional — see Release.
	waiter *Waiter
	// stopProbe ends this Job's liveness probe. Owned by the Job, so it cannot
	// outlive it and release a healthy successor.
	stopProbe func()
}

// Waiter is a Registration in the Queue. Its PID is the Queue's identity
// (ADR-007), which is what lets a reconnecting Runner keep its place.
type Waiter struct {
	PID      int
	Repo     string
	QueuedAt time.Time

	// grant is buffered with capacity 1 so signalling a Grant can never block —
	// not even when the waiting handler has not yet reached its select. That is
	// what lets the granting path release the mutex before it signals.
	grant chan struct{}
}

// Granted closes the wait: a receive succeeds once this Waiter holds the Lock.
func (w *Waiter) Granted() <-chan struct{} { return w.grant }

// signal delivers the Grant. Non-blocking and idempotent: a second Grant for the
// same Waiter is a no-op rather than a panic or a block.
func (w *Waiter) signal() {
	select {
	case w.grant <- struct{}{}:
	default:
	}
}

// Scheduler is the whole of trainsty's mutable state.
type Scheduler struct {
	mu    sync.Mutex
	job   *Job
	queue []*Waiter

	// now is injectable so tests can assert on elapsed times without sleeping.
	now func() time.Time
	// onGrant runs just after a Job is installed, outside the mutex. httpapi uses
	// it to start the liveness probe; scheduler itself knows nothing about probes.
	onGrant func(*Job)
}

// New returns an empty Scheduler with the Lock free.
func New() *Scheduler { return &Scheduler{now: time.Now} }

// SetClock replaces the clock. Tests only.
func (s *Scheduler) SetClock(now func() time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.now = now
}

// SetOnGrant registers a hook run after each Grant, outside the mutex.
func (s *Scheduler) SetOnGrant(fn func(*Job)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onGrant = fn
}

// Register enrolls pid and returns its Waiter. The caller waits on Granted().
//
// Three cases, and the second two are what ADR-007 buys:
//
//   - a free Lock grants immediately;
//   - a pid already in the Queue keeps its position, rather than being appended
//     again — otherwise a Runner on a flaky connection is starved and FIFO stops
//     being true of anything observable;
//   - a pid that is already the Job is granted immediately, which makes a
//     Runner's retry safe to write naively.
func (s *Scheduler) Register(pid int, repo string) *Waiter {
	s.mu.Lock()

	if s.job != nil && s.job.PID == pid {
		w := s.newWaiterLocked(pid, repo)
		s.mu.Unlock()
		w.signal() // already holds it
		return w
	}
	for _, q := range s.queue {
		if q.PID == pid {
			s.mu.Unlock()
			return q // position preserved
		}
	}

	w := s.newWaiterLocked(pid, repo)
	s.queue = append(s.queue, w)
	granted := s.grantLocked()
	hook := s.onGrant
	job := s.job
	s.mu.Unlock()

	// Signalling and hooks happen outside the mutex: a send or a callback while
	// holding it is the deadlock this design is shaped to avoid.
	if granted != nil {
		granted.signal()
		if hook != nil {
			hook(job)
		}
	}
	return w
}

func (s *Scheduler) newWaiterLocked(pid int, repo string) *Waiter {
	return &Waiter{PID: pid, Repo: repo, QueuedAt: s.now(), grant: make(chan struct{}, 1)}
}

// grantLocked promotes the head of the Queue if the Lock is free.
//
// Removing from the Queue and installing the Job happen in ONE critical section,
// which is invariant 1: two sections leave a window where a second Grant sees a
// free Lock and two suites run at once.
//
// The caller must hold s.mu, and must signal the returned Waiter after releasing.
func (s *Scheduler) grantLocked() *Waiter {
	if s.job != nil || len(s.queue) == 0 {
		return nil
	}
	w := s.queue[0]
	s.queue = s.queue[1:]
	s.job = &Job{PID: w.PID, Repo: w.Repo, GrantedAt: s.now(), waiter: w}
	return w
}

// Current returns the Job holding the Lock, or nil.
//
// The returned pointer is the identity a later Release is checked against, which
// is how /stop can terminate a process group outside the mutex and still refuse
// to release whatever took the Lock while it was doing so.
func (s *Scheduler) Current() *Job {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.job
}

// Release frees the Lock held by job and Grants it to the head of the Queue.
// Reports whether it released anything.
//
// Identity-checked, and that is invariant 3 rather than caution: five paths can
// release one Job and at least two routinely race — a Runner's own release and
// its Registration dropping as the terminal closes. Releasing "whatever holds the
// Lock" would let the loser of that race revoke the WINNER'S SUCCESSOR's Lock.
// Plausible, wrong, and invisible until two suites collide.
//
// This check also makes a sync.Once redundant: a second Release for the same Job
// finds s.job no longer equal to it and does nothing. One mechanism, strictly
// stronger than two, since Once cannot prevent releasing a different Job.
func (s *Scheduler) Release(job *Job) bool {
	if job == nil {
		return false
	}
	s.mu.Lock()
	if s.job != job {
		s.mu.Unlock()
		return false
	}
	if job.stopProbe != nil {
		job.stopProbe()
	}
	s.job = nil
	granted := s.grantLocked()
	hook := s.onGrant
	next := s.job
	s.mu.Unlock()

	if granted != nil {
		granted.signal()
		if hook != nil {
			hook(next)
		}
	}
	return true
}

// ReleaseByPID releases the Lock if the Job's PID matches, and reports whether
// anything was released.
//
// pid <= 0 means "whatever holds it". That is the unavoidable path for a
// hand-written Runner which sends no pid, and it cannot be identity-checked — so
// it carries the race Release exists to prevent. `trainsty wrap` always sends its
// pid; see contracts/http-api.md.
func (s *Scheduler) ReleaseByPID(pid int) bool {
	s.mu.Lock()
	job := s.job
	s.mu.Unlock()
	if job == nil {
		return false
	}
	if pid > 0 && job.PID != pid {
		return false
	}
	return s.Release(job)
}

// Withdraw removes w. If w holds the Lock this releases it; if w is queued it is
// dropped without releasing anything, because a Waiter holds nothing.
//
// This is the path a dropped Registration takes, and it is the primary release
// signal (ADR-008).
func (s *Scheduler) Withdraw(w *Waiter) (released bool) {
	if w == nil {
		return false
	}
	s.mu.Lock()
	if s.job != nil && s.job.waiter == w {
		s.mu.Unlock()
		return s.Release(s.jobFor(w))
	}
	for i, q := range s.queue {
		if q == w {
			s.queue = append(s.queue[:i:i], s.queue[i+1:]...)
			break
		}
	}
	s.mu.Unlock()
	return false
}

// jobFor returns the Job granted from w, or nil.
func (s *Scheduler) jobFor(w *Waiter) *Job {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.job != nil && s.job.waiter == w {
		return s.job
	}
	return nil
}

// SetStopProbe attaches a cancel function to job, called when it is released.
func (s *Scheduler) SetStopProbe(job *Job, stop func()) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.job == job && job != nil {
		job.stopProbe = stop
	}
}

// JobView and WaiterView are the serialisable shapes /status reports. Elapsed
// times are computed server-side so the Dashboard needs no clock of its own and
// cannot disagree about "now" (contracts/http-api.md).
type JobView struct {
	PID            int       `json:"pid"`
	Repo           string    `json:"repo"`
	GrantedAt      time.Time `json:"grantedAt"`
	ElapsedSeconds int       `json:"elapsedSeconds"`
}

type WaiterView struct {
	PID            int       `json:"pid"`
	Repo           string    `json:"repo"`
	QueuedAt       time.Time `json:"queuedAt"`
	WaitingSeconds int       `json:"waitingSeconds"`
}

// Snapshot is the whole of /status. Queue is never nil, so a client never has to
// tell an absent list from an empty one.
type Snapshot struct {
	Job   *JobView     `json:"job"`
	Queue []WaiterView `json:"queue"`
}

// Snapshot copies the state under the mutex. The caller marshals afterwards:
// marshalling while another goroutine mutates is a data race that -race finds and
// a reviewer does not.
func (s *Scheduler) Snapshot() Snapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	snap := Snapshot{Queue: make([]WaiterView, 0, len(s.queue))}
	if s.job != nil {
		snap.Job = &JobView{
			PID:            s.job.PID,
			Repo:           s.job.Repo,
			GrantedAt:      s.job.GrantedAt,
			ElapsedSeconds: int(now.Sub(s.job.GrantedAt).Seconds()),
		}
	}
	for _, w := range s.queue {
		snap.Queue = append(snap.Queue, WaiterView{
			PID:            w.PID,
			Repo:           w.Repo,
			QueuedAt:       w.QueuedAt,
			WaitingSeconds: int(now.Sub(w.QueuedAt).Seconds()),
		})
	}
	return snap
}
