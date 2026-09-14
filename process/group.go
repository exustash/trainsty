// Package process is the only place in trainsty that makes a system call.
//
// It exposes intent rather than mechanism — Alive, IsGroupLeader, KillGroup —
// so no other package needs to know that a negative PID means "the group" or
// that ESRCH and EPERM answer different questions. Confining it here is what
// keeps the scheduler pure and testable with no child processes
// (knowledge/conventions/go.md → Package layout).
//
// Unix only. There is no Windows path and none is planned: whole-group
// termination is the product, not an implementation detail (ADR-002).
package process

import (
	"errors"
	"fmt"
	"syscall"
)

// ErrNotGroupLeader is returned when a PID does not lead its own process group.
//
// This is refused rather than tolerated because of what a Stop would do with it:
// KillGroup signals the group the PID belongs to, so a non-leader's group is
// somebody else's — in the common case the developer's own shell. See ADR-002 and
// specs/001-serialize-e2e-runs/contracts/http-api.md.
var ErrNotGroupLeader = errors.New("pid does not lead its own process group")

// IsGroupLeader reports whether pid leads its own process group.
//
// A pid that does not exist returns an error wrapping syscall.ESRCH, so a caller
// can tell "no such process" from "exists but is not a leader" — the API refuses
// them with different codes.
func IsGroupLeader(pid int) (bool, error) {
	if pid <= 0 {
		return false, fmt.Errorf("pid %d is not a valid process id", pid)
	}
	pgid, err := syscall.Getpgid(pid)
	if err != nil {
		return false, fmt.Errorf("getpgid %d: %w", pid, err)
	}
	return pgid == pid, nil
}

// Alive reports whether the process pid exists and can be signalled.
//
// The three-way answer is the point, and collapsing it to a boolean is the bug
// this function exists to prevent (ADR-008, research.md → R4):
//
//	err == nil, alive == true    the process exists and is ours to signal
//	err == nil, alive == false   ESRCH — it is gone. The ONLY case that releases a Lock
//	err != nil, alive == true    EPERM or something unexpected. Still alive; LOG IT
//
// EPERM means the process exists but belongs to another user, which on this
// product means a Registration named a PID that is not the developer's. That is a
// defect worth shouting about, and it must never free somebody's Lock — "the
// probe returned an error" is not "the job is dead".
func Alive(pid int) (bool, error) {
	if pid <= 0 {
		return false, fmt.Errorf("pid %d is not a valid process id", pid)
	}
	// Signal 0 performs the permission and existence checks without sending
	// anything, which is the portable liveness probe on both Darwin and Linux.
	err := syscall.Kill(pid, 0)
	switch {
	case err == nil:
		return true, nil
	case errors.Is(err, syscall.ESRCH):
		return false, nil
	case errors.Is(err, syscall.EPERM):
		return true, fmt.Errorf("probe pid %d: exists but is not ours to signal: %w", pid, err)
	default:
		return true, fmt.Errorf("probe pid %d: %w", pid, err)
	}
}
