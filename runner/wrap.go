// Package runner implements `trainsty wrap` — the Runner the product ships.
//
// It is a CLIENT (ADR-012). The Daemon spawns nothing; this is a separate process
// the developer starts, which happens to be compiled into the same binary. That
// distinction is why os/exec here does not violate Principle II, and
// scripts/ci-local.sh enforces that only this package and daemonctl import it.
package runner

import (
	"bufio"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/exustash/trainsty/httpapi"
)

// Wrap acquires the Lock, runs argv, releases, and returns argv's exit status.
//
// # Why this registers its OWN pid rather than the suite's
//
// specs/001-serialize-e2e-runs/research.md → R2 proposed registering the child's
// PID and forwarding signals to it. That design cannot work, and the reason is an
// ordering one: the Daemon validates that a registered PID leads its own process
// group, so the PID must exist before registering — but the suite must NOT run
// before the Grant. Registering the child means starting it first, which defeats
// the entire product.
//
// So: wrap makes ITSELF a process group leader, registers its own PID, waits, and
// then starts the suite as a child in its own group. Killing -wrapPID therefore
// reaches wrap and the whole suite tree, which is what /stop needs (FR-028).
//
// This also makes Ctrl+C work by construction in the common case. An interactive
// shell already puts wrap in its own foreground process group, so the terminal
// delivers SIGINT to wrap AND the suite together — no forwarding required for the
// suite to die. Forwarding is kept anyway for the case where wrap had to call
// setpgid itself and is no longer in the terminal's foreground group.
func Wrap(args []string, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "trainsty: wrap needs a command: trainsty wrap -- <command> [args...]")
		return 1
	}

	if err := becomeGroupLeader(); err != nil {
		// Not fatal: the suite still needs to run. What is lost is the ability to
		// terminate it as a group, and saying so is better than failing.
		fmt.Fprintf(stderr, "trainsty: could not lead my own process group (%v) — this run cannot be stopped from the dashboard\n", err)
	}

	repo := repoLabel()
	pid := os.Getpid()

	release, waited := acquire(pid, repo, stderr)
	defer release()

	// Running an arbitrary command IS this function's purpose — wrap is `env`, `time`
	// or `nohup` for a lock — so the static-command rule cannot apply. Audited
	// rather than suppressed, and the reasoning is structural rather than a promise:
	//
	//   * args come from the developer's own argv. wrap runs as that developer and
	//     crosses no privilege boundary; anything it can execute, they could type.
	//   * the HTTP API cannot reach here. runner imports httpapi, never the reverse,
	//     so the Daemon has no path to Wrap — and Principle II forbids it acquiring
	//     one. scripts/ci-local.sh enforces that only runner/ and daemonctl/ may
	//     import os/exec at all.
	//   * there is no shell. exec.Command with an explicit argv does not interpret
	//     metacharacters, so a repository name or a suite argument cannot inject a
	//     second command.
	//
	// nosemgrep: go.lang.security.audit.dangerous-exec-command.dangerous-exec-command
	cmd := exec.Command(args[0], args[1:]...)
	// Inherited directly, never piped. A pipe alone would make the suite believe it
	// is not a terminal and turn off colour, which is the workflow this product
	// exists to preserve (FR-037, ADR-003).
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := cmd.Start(); err != nil {
		fmt.Fprintf(stderr, "trainsty: could not start %s: %v\n", args[0], err)
		return 1
	}
	_ = waited

	stop := forwardSignals()
	defer stop()

	err := cmd.Wait()
	return exitCode(err)
}

// becomeGroupLeader makes this process a process group leader if it is not one.
//
// An interactive shell has already done this — job control puts each pipeline in
// its own group — so the common path is a no-op. A non-interactive script has not,
// and there the setpgid is what lets the Daemon accept the registration at all.
func becomeGroupLeader() error {
	pid := os.Getpid()
	pgid, err := syscall.Getpgid(pid)
	if err != nil {
		return fmt.Errorf("getpgid: %w", err)
	}
	if pgid == pid {
		return nil
	}
	if err := syscall.Setpgid(0, 0); err != nil {
		return fmt.Errorf("setpgid: %w", err)
	}
	return nil
}

// acquire blocks until the Lock is granted and returns a release function.
//
// When no Daemon is reachable it returns immediately with a no-op release and says
// so ONCE. A missing scheduler costs a convenience, not the developer's work
// (FR-038) — and a silently unscheduled run is indistinguishable from a scheduled
// one until two collide, which is why it is said at all.
func acquire(pid int, repo string, stderr io.Writer) (release func(), waited bool) {
	target := fmt.Sprintf("%s/register?pid=%d&repo=%s", httpapi.BaseURL, pid, url.QueryEscape(repo))

	// No client timeout, ever: a queue wait is unbounded, and a timeout here
	// defeats the entire reason the wait is a stream (ADR-004).
	client := &http.Client{}
	resp, err := client.Get(target)
	if err != nil {
		fmt.Fprintln(stderr, "trainsty: no scheduler reachable — this run is NOT serialized")
		return func() {}, false
	}
	if resp.StatusCode != http.StatusOK {
		body := make([]byte, 256)
		n, _ := resp.Body.Read(body)
		resp.Body.Close()
		fmt.Fprintf(stderr, "trainsty: the scheduler refused this run (%s %s) — running anyway, NOT serialized\n",
			resp.Status, strings.TrimSpace(string(body[:n])))
		return func() {}, false
	}

	// The stream stays open for the whole run: it is the Daemon's primary signal
	// that this Runner is alive (ADR-008). Closing it is release path 2.
	started := time.Now()
	announced := false
	reader := bufio.NewReader(resp.Body)
	for {
		line, readErr := reader.ReadString('\n')
		if readErr != nil {
			resp.Body.Close()
			fmt.Fprintln(stderr, "trainsty: lost the scheduler while waiting — running anyway, NOT serialized")
			return func() {}, false
		}
		if strings.HasPrefix(line, "event: grant") {
			break
		}
		if !announced && time.Since(started) > 300*time.Millisecond {
			// Only said when the run actually waits: wrapping a fast suite must not
			// add noise.
			fmt.Fprintln(stderr, "trainsty: waiting for the lock…")
			announced = true
		}
	}
	if announced {
		fmt.Fprintf(stderr, "trainsty: granted after %s\n", time.Since(started).Round(time.Second))
	}

	return func() {
		// Identity-checked on the server: sending the pid is what stops a late
		// release from revoking a successor's Lock.
		req, _ := http.NewRequest(http.MethodPost, fmt.Sprintf("%s/release?pid=%d", httpapi.BaseURL, pid), nil)
		if req != nil {
			req.Header.Set("Content-Type", "application/json")
			if rr, rerr := (&http.Client{Timeout: 2 * time.Second}).Do(req); rerr == nil {
				rr.Body.Close()
			}
		}
		// Closing the stream is the second, independent release path. A failure
		// above is therefore not worth reporting: the Lock is freed either way, and
		// an error printed on the normal path is an error developers learn to ignore.
		resp.Body.Close()
	}, true
}

// forwardSignals relays SIGINT and SIGTERM to the whole process group.
//
// # Why the group and not just the child
//
// Signalling only the direct child is not enough, and the acceptance test proved
// it: a POSIX shell waiting on a foreground child does NOT run its trap until that
// child exits. So `sh -c '...; sleep 300'` swallows the signal entirely — the
// sleep never sees it, the shell never returns, and Ctrl+C appears to do nothing
// while the Lock stays held. Real suites have the same shape: a wrapper script
// around a test runner around a browser.
//
// Signalling the group reaches every descendant directly, which is what a terminal
// does for a foreground job. wrap is itself in that group and would normally die,
// but signal.Notify has already disabled the default action — so the re-delivered
// signal arrives on this channel instead, and the once-guard drops it. wrap
// therefore survives to reap the suite and release the Lock, which is the whole
// point of catching the signal in the first place.
func forwardSignals() (stop func()) {
	sigs := make(chan os.Signal, 2)
	signal.Notify(sigs, os.Interrupt, syscall.SIGTERM)
	done := make(chan struct{})

	go func() {
		forwarded := map[os.Signal]bool{}
		for {
			select {
			case sig := <-sigs:
				if forwarded[sig] {
					continue // our own copy of the signal we just sent to the group
				}
				forwarded[sig] = true
				s, ok := sig.(syscall.Signal)
				if !ok {
					continue
				}
				// The group is ours: wrap made itself the leader before registering,
				// so this reaches the suite and everything it started, and nothing
				// outside this run.
				_ = syscall.Kill(-os.Getpid(), s)
			case <-done:
				return
			}
		}
	}()
	return func() { signal.Stop(sigs); close(done) }
}

// repoLabel names the repository for the dashboard. A display label only (OD-3).
func repoLabel() string {
	out, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err == nil {
		if top := strings.TrimSpace(string(out)); top != "" {
			return filepath.Base(top)
		}
	}
	wd, err := os.Getwd()
	if err != nil {
		return "unknown"
	}
	return filepath.Base(wd)
}

// exitCode returns the suite's own status. Swallowing a failure would turn a red
// suite green, which is worse than having no scheduler at all (FR-037).
func exitCode(err error) int {
	if err == nil {
		return 0
	}
	var exitErr *exec.ExitError
	if ok := asExitError(err, &exitErr); ok {
		return exitErr.ExitCode()
	}
	return 1
}

func asExitError(err error, target **exec.ExitError) bool {
	if e, ok := err.(*exec.ExitError); ok {
		*target = e
		return true
	}
	return false
}
