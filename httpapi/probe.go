package httpapi

import (
	"context"
	"time"

	"github.com/exustash/trainsty/process"
	"github.com/exustash/trainsty/scheduler"
)

// probeInterval is the 3-5 second window the requirements note specifies.
const probeInterval = 4 * time.Second

// startProbe watches the Job's process group leader and releases the Lock if it
// disappears. This is release path 3.
//
// It is the BACKSTOP, not the primary signal (ADR-008): the open Registration
// detects a closed terminal in milliseconds, while this covers the case the stream
// cannot see — a SIGKILLed Runner whose socket the kernel has not yet torn down.
//
// The probe is owned by the Job and stopped by its Release. A probe that outlived
// its Job would release a healthy successor.
func (s *Server) startProbe(job *scheduler.Job) {
	if job == nil {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.sched.SetStopProbe(job, cancel)

	go func() {
		ticker := time.NewTicker(probeInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				alive, err := process.Alive(job.PID)
				if err != nil {
					// EPERM or something unexpected. The process is ALIVE; this is a
					// defect in what was registered, and it must be loud rather than
					// quietly freeing somebody's Lock. "The probe errored" is not
					// "the job is dead".
					s.logf("probe: pid %d (%s) is alive but not ours to signal — NOT releasing: %v", job.PID, job.Repo, err)
					continue
				}
				if !alive {
					if s.sched.Release(job) {
						s.logf("release: pid %d (%s) — process gone (probe)", job.PID, job.Repo)
					}
					return
				}
			}
		}
	}()
}
