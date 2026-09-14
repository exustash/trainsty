// Package daemonctl implements the client subcommands: start, stop, status, ui.
//
// Client-side, like runner (ADR-012). `start` re-executes the binary to detach the
// Daemon, which is the second of the two places os/exec may be imported —
// scripts/ci-local.sh enforces that list.
package daemonctl

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"syscall"
	"time"

	"github.com/exustash/trainsty/httpapi"
	"github.com/exustash/trainsty/logpath"
	"github.com/exustash/trainsty/scheduler"
)

// Start spawns the Daemon detached and returns once it is confirmed listening.
//
// Go's runtime is multi-threaded, so the classic double-fork daemon idiom is
// unavailable — only fork+exec is safe, which is what os/exec does. Setsid makes
// the child a session leader with no controlling terminal, so it survives the
// terminal closing (research.md → R3).
func Start(stdout, stderr io.Writer) int {
	if httpapi.Reachable() {
		fmt.Fprintf(stderr, "trainsty: already running on %s\n", httpapi.Addr)
		fmt.Fprintln(stderr, "          check it with:  trainsty status")
		return 1
	}

	self, err := os.Executable()
	if err != nil {
		fmt.Fprintf(stderr, "trainsty: cannot find my own binary: %v\n", err)
		return 1
	}

	logPath, logWhy := prepareLog()

	devNull, err := os.OpenFile(os.DevNull, os.O_RDWR, 0)
	if err != nil {
		fmt.Fprintf(stderr, "trainsty: cannot open %s: %v\n", os.DevNull, err)
		return 1
	}
	defer devNull.Close()

	logFile, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		// Not fatal: a daemon with nowhere to write is still a working daemon.
		fmt.Fprintf(stderr, "trainsty: cannot open %s (%v) — the daemon's output will be discarded\n", logPath, err)
		logFile = devNull
	} else {
		defer logFile.Close()
	}

	// Audited: the command is this binary's own path from os.Executable() plus one
	// literal argument. No external input reaches it, and there is no shell.
	// nosemgrep: go.lang.security.audit.dangerous-exec-command.dangerous-exec-command
	cmd := exec.Command(self, "serve")
	cmd.Stdin, cmd.Stdout, cmd.Stderr = devNull, logFile, logFile
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		fmt.Fprintf(stderr, "trainsty: could not start the daemon: %v\n", err)
		return 1
	}
	// Deliberately not waited on: this process is the parent of a daemon it is
	// about to outlive. Releasing the handle avoids holding a zombie entry.
	_ = cmd.Process.Release()

	// Verify the bind before claiming success. Without this, `start` reports 0 for
	// a daemon that died on a taken port, and ADR-011's carefully worded error
	// lands in a log nobody is watching.
	if !waitUntil(2*time.Second, httpapi.Reachable) {
		fmt.Fprintf(stderr, "trainsty: the daemon did not come up on %s\n", httpapi.Addr)
		fmt.Fprintf(stderr, "          something else may hold the port — check:  lsof -nP -iTCP:%d\n", httpapi.Port)
		if logPath != "" {
			fmt.Fprintf(stderr, "          the reason is in:  %s\n", logPath)
		}
		return 1
	}

	fmt.Fprintf(stdout, "trainsty: listening on http://localhost:%d\n", httpapi.Port)
	if logWhy != "" {
		fmt.Fprintf(stdout, "trainsty: %s\n", logWhy)
	} else {
		fmt.Fprintf(stdout, "trainsty: logging to %s\n", logPath)
	}
	return 0
}

// Serve runs the Daemon in the foreground. The hidden `serve` subcommand, and an
// implementation detail of Start rather than something to reach for.
func Serve(stderr io.Writer) int {
	logger, path, why := httpapi.OpenLog()
	if why != "" {
		logger.Printf("startup: %s", why)
	}
	ln, err := httpapi.Listen()
	if err != nil {
		switch {
		case errors.Is(err, httpapi.ErrAlreadyRunning):
			logger.Printf("startup refused: another trainsty daemon already holds %s", httpapi.Addr)
			fmt.Fprintf(stderr, "trainsty: already running on %s\n", httpapi.Addr)
		case errors.Is(err, httpapi.ErrPortTaken):
			logger.Printf("startup refused: port %d is held by another program", httpapi.Port)
			fmt.Fprintf(stderr, "trainsty: port %d is in use by another program — check:  lsof -nP -iTCP:%d\n", httpapi.Port, httpapi.Port)
		default:
			logger.Printf("startup failed: %v", err)
			fmt.Fprintf(stderr, "trainsty: %v\n", err)
		}
		return 1
	}
	logger.Printf("listening on %s (log %s)", httpapi.Addr, path)

	srv := httpapi.New(scheduler.New(), logger)
	if err := srv.Serve(ln); err != nil {
		logger.Printf("serve ended with an error: %v", err)
		return 1
	}
	logger.Printf("stopped cleanly")
	return 0
}

func prepareLog() (path, why string) {
	p, err := logpath.Resolve()
	if err != nil {
		return "", fmt.Sprintf("no log path available (%v) — the daemon's output will be discarded", err)
	}
	return p, ""
}

func waitUntil(within time.Duration, cond func() bool) bool {
	deadline := time.Now().Add(within)
	for time.Now().Before(deadline) {
		if cond() {
			return true
		}
		time.Sleep(25 * time.Millisecond)
	}
	return false
}

// post sends a JSON POST, which is what the guard requires (SDR-001).
func post(path string) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodPost, httpapi.BaseURL+path, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	return (&http.Client{Timeout: 3 * time.Second}).Do(req)
}
