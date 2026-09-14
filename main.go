package main

import (
	"fmt"
	"os"

	"github.com/exustash/trainsty/daemonctl"
	"github.com/exustash/trainsty/runner"
)

// Exit codes are part of the CLI contract, because scripts branch on them —
// specs/001-serialize-e2e-runs/contracts/cli.md.
//
// exitNoScheduler is the one that earns its own value: a Runner must be able to
// tell "no scheduler is reachable" from "the scheduler refused" (FR-018), and a
// bare non-zero cannot say which.
const (
	exitOK          = 0
	exitFailure     = 1
	exitNoScheduler = 3
)

// command is one subcommand. hidden keeps `serve` out of help: it is an
// implementation detail of start (research.md → R3), not something to reach for.
type command struct {
	name    string
	summary string
	hidden  bool
	run     func(args []string) int
}

// commands is the dispatch table and the source of help's output, so the two
// cannot disagree. Order is the order help prints.
//
// Populated in init rather than as a literal: help renders this table, so a
// literal would be a compile-time initialization cycle. Keeping one table and
// breaking the cycle is better than keeping two lists that can drift.
var commands []command

func init() {
	commands = []command{
		{name: "wrap", summary: "run a command under the lock: trainsty wrap -- <command> [args...]", run: runWrap},
		{name: "start", summary: "start the scheduler in the background", run: runStart},
		{name: "stop", summary: "stop the scheduler (--force to skip the prompt)", run: runStop},
		{name: "status", summary: "print what holds the lock and how many are waiting", run: runStatus},
		{name: "ui", summary: "open the dashboard in a browser", run: runUI},
		{name: "version", summary: "print the version, commit and platform of this binary", run: runVersion},
		{name: "help", summary: "print this message", run: runHelp},
		{name: "serve", summary: "run the scheduler in the foreground", hidden: true, run: runServe},
	}
}

// runWrap strips a leading `--` so both `wrap -- cmd` and `wrap cmd` work. The
// separator is what the documented form uses, and it is what lets a suite take
// flags of its own without wrap trying to parse them.
func runWrap(args []string) int {
	return runner.Wrap(stripSeparator(args), os.Stderr)
}

// stripSeparator drops a leading `--` so both `wrap -- cmd` and `wrap cmd` work. The
// separator is the documented form, and it is what lets a suite take flags of its own
// without wrap trying to parse them.
func stripSeparator(args []string) []string {
	if len(args) > 0 && args[0] == "--" {
		return args[1:]
	}
	return args
}

// hasForce reports whether the caller asked to skip the confirmation prompt.
func hasForce(args []string) bool {
	for _, a := range args {
		if a == "--force" || a == "-f" {
			return true
		}
	}
	return false
}

func runStart(args []string) int  { return daemonctl.Start(os.Stdout, os.Stderr) }
func runServe(args []string) int  { return daemonctl.Serve(os.Stderr) }
func runStatus(args []string) int { return daemonctl.Status(os.Stdout, os.Stderr) }
func runUI(args []string) int     { return daemonctl.UI(os.Stdout, os.Stderr) }

func runStop(args []string) int {
	return daemonctl.Stop(hasForce(args), os.Stdin, os.Stdout, os.Stderr)
}

func runVersion(args []string) int {
	fmt.Fprintln(os.Stdout, versionLine())
	return exitOK
}

func runHelp(args []string) int {
	usage(os.Stdout)
	return exitOK
}

func main() {
	os.Exit(dispatch(os.Args[1:]))
}

// dispatch routes argv to a subcommand. Composition only: no logic beyond
// choosing what to call (CLAUDE.md → Repository Architecture).
func dispatch(args []string) int {
	if len(args) == 0 {
		usage(os.Stderr)
		return exitFailure
	}
	name := args[0]
	for _, c := range commands {
		if c.name == name {
			return c.run(args[1:])
		}
	}
	fmt.Fprintf(os.Stderr, "trainsty: unknown command %q\n\n", name)
	usage(os.Stderr)
	return exitFailure
}

// usage prints the one-line description and the visible subcommands. The
// description is not decoration: "trainsty" gives nothing away about what the
// tool does, where the requirements note's "e2e-scheduler" did (ADR-010).
func usage(w *os.File) {
	fmt.Fprint(w, "trainsty — serialize local end-to-end test runs so two suites never collide\n\n")
	fmt.Fprint(w, "usage: trainsty <command> [arguments]\n\n")
	for _, c := range commands {
		if c.hidden {
			continue
		}
		fmt.Fprintf(w, "  %-8s %s\n", c.name, c.summary)
	}
	// Stated because it is a real limitation with a non-obvious cause: a suite in a
	// background process group that reads the terminal receives SIGTTIN and stops.
	// A queued batch run is not an interactive session (research.md → R2).
	fmt.Fprint(w, "\nwrap is for batch suites. An interactive or watch-mode suite that reads\n")
	fmt.Fprint(w, "the terminal will stop, because a queued run is not a foreground job.\n")
}
