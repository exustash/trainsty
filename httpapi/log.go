package httpapi

import (
	"fmt"
	"io"
	"log"
	"os"

	"github.com/exustash/trainsty/logpath"
)

// OpenLog returns a write-only logger appending to the per-user log file, and the
// path it opened.
//
// The file is append-only and is NEVER read back. That is what lets DDR-002
// coexist with DDR-001: the test is not whether trainsty writes, but whether it
// reads anything back — anything read at startup is a stale lock waiting to be
// inherited, and nothing here reads.
//
// A log that cannot be opened must not stop the daemon. Losing the record is bad;
// refusing to schedule because a log file is unwritable is worse. On failure this
// returns a stderr logger, the reason, and no error.
func OpenLog() (logger *log.Logger, path string, why string) {
	path, err := logpath.Resolve()
	if err != nil {
		return newLogger(os.Stderr), "", fmt.Sprintf("could not resolve a log path (%v) — logging to stderr", err)
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return newLogger(os.Stderr), path, fmt.Sprintf("could not open %s (%v) — logging to stderr", path, err)
	}
	return newLogger(f), path, ""
}

func newLogger(w io.Writer) *log.Logger {
	// Microseconds because two release paths can race by less than a millisecond,
	// and the log is how that race gets diagnosed.
	return log.New(w, "", log.LstdFlags|log.Lmicroseconds)
}

// DiscardLogger is for tests that do not assert on output.
func DiscardLogger() *log.Logger { return newLogger(io.Discard) }
