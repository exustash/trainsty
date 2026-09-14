package main

import (
	"fmt"
	"os"
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
var commands = []command{
	{name: "wrap", summary: "run a command under the lock: trainsty wrap -- <command> [args...]", run: notImplemented("wrap")},
	{name: "start", summary: "start the scheduler in the background", run: notImplemented("start")},
	{name: "stop", summary: "stop the scheduler", run: notImplemented("stop")},
	{name: "status", summary: "print what holds the lock and how many are waiting", run: notImplemented("status")},
	{name: "ui", summary: "open the dashboard in a browser", run: notImplemented("ui")},
	{name: "help", summary: "print this message", run: notImplemented("help")},
	{name: "serve", summary: "run the scheduler in the foreground", hidden: true, run: notImplemented("serve")},
}

// notImplemented is the placeholder every subcommand starts as. It names the
// task that fills it in, so a stub reached by accident says what is missing
// rather than failing silently.
func notImplemented(name string) func([]string) int {
	return func([]string) int {
		fmt.Fprintf(os.Stderr, "trainsty: %s is not implemented yet\n", name)
		return exitFailure
	}
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
}
