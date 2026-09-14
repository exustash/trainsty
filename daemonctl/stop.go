package daemonctl

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
)

// Stop shuts the Daemon down. The one interactive prompt in the product.
//
// Shutting down releases the Lock but does NOT terminate the Job (ADR-009): the
// suite keeps running, unsupervised, and the next Daemon knows nothing about it.
// That is a real hazard, so the developer is told which repository is affected
// before they confirm rather than after.
func Stop(force bool, stdin io.Reader, stdout, stderr io.Writer) int {
	snap, err := fetchStatus()
	if err != nil {
		// Nothing to stop is not a failure — `trainsty stop` on a machine with no
		// daemon is a no-op a script may run unconditionally.
		fmt.Fprintf(stdout, "trainsty: no scheduler reachable on port %d — nothing to stop\n", port())
		return 0
	}

	if snap.Job != nil && !force {
		fmt.Fprintf(stdout, "trainsty: %s is running (pid %d, %s elapsed).\n",
			snap.Job.Repo, snap.Job.PID, humanSeconds(snap.Job.ElapsedSeconds))
		fmt.Fprintln(stdout, "          Shutting down frees the lock but does NOT stop that suite —")
		fmt.Fprintln(stdout, "          it keeps running, and the next scheduler will not know about it.")
		ok, asked := confirm(stdin, stdout)
		if !asked {
			// Not a terminal and no --force: refuse rather than assume yes. Assuming
			// would make a scripted `trainsty stop` silently orphan a running suite.
			fmt.Fprintln(stderr, "trainsty: a suite is running and this is not a terminal — re-run with --force to shut down anyway")
			return 1
		}
		if !ok {
			fmt.Fprintln(stdout, "trainsty: left running")
			return 0
		}
	}

	resp, err := post("/shutdown")
	if err != nil {
		fmt.Fprintf(stderr, "trainsty: could not reach the scheduler: %v\n", err)
		return 1
	}
	defer resp.Body.Close()
	var payload struct {
		JobWasActive bool `json:"jobWasActive"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&payload)

	if payload.JobWasActive {
		fmt.Fprintln(stdout, "trainsty: stopped — the suite that held the lock is still running, unsupervised")
	} else {
		fmt.Fprintln(stdout, "trainsty: stopped")
	}
	return 0
}

// confirm reads a yes/no answer. asked is false when there is no terminal to ask.
func confirm(stdin io.Reader, stdout io.Writer) (yes, asked bool) {
	if !isTerminal(stdin) {
		return false, false
	}
	fmt.Fprint(stdout, "Shut down anyway? [y/N] ")
	line, err := bufio.NewReader(stdin).ReadString('\n')
	if err != nil {
		return false, true
	}
	answer := strings.ToLower(strings.TrimSpace(line))
	return answer == "y" || answer == "yes", true
}

// isTerminal reports whether r is an interactive terminal.
//
// Implemented with a Stat on the file mode rather than a terminal library: one
// syscall's worth of information does not justify the project's first dependency
// (Principle I).
func isTerminal(r io.Reader) bool {
	f, ok := r.(*os.File)
	if !ok {
		return false
	}
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}
