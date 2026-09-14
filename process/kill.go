package process

import (
	"fmt"
	"syscall"
)

// KillGroup terminates the process group led by pid — the leader and every
// process it started, headless browsers included.
//
// pid MUST be a group leader. Reclaiming a suite's children is the entire reason
// this project is Unix-only (ADR-002), and killing the bare PID instead is a
// defect rather than a partial success.
//
// This is the only syscall.Kill call site in the codebase that sends a real
// signal. The negation is what makes it a group: kill(2) reads a negative pid as
// "the process group with that id".
func KillGroup(pid int) error {
	// pid <= 1 is refused, and this guard is not defensive padding — each value
	// is a distinct catastrophe:
	//
	//	pid == 0  → kill(0, sig) signals EVERY process in the CALLER'S OWN group,
	//	            which includes the daemon itself.
	//	pid == 1  → kill(-1, sig) signals every process the user is permitted to
	//	            signal. On a developer's machine that is their whole session.
	//
	// Neither is reachable through a validated Registration, and both are one
	// arithmetic slip away, so the check lives at the syscall rather than at the
	// callers.
	if pid <= 1 {
		return fmt.Errorf("refusing to signal process group %d: pid must be greater than 1", pid)
	}
	if err := syscall.Kill(-pid, syscall.SIGKILL); err != nil {
		return fmt.Errorf("kill process group %d: %w", pid, err)
	}
	return nil
}
