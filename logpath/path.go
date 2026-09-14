// Package logpath resolves where the daemon appends its log.
//
// It exists to contain the project's only platform branch (DDR-002). Two lines of
// runtime.GOOS in their own package with their own test, rather than a conditional
// in main that grows.
package logpath

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// FileName is the log's basename on every platform.
const FileName = "trainsty.log"

// Resolve returns the log file's path and creates its parent directory.
//
// Darwin uses ~/Library/Logs, which is where the operating system expects a
// per-user log. Everything else follows the XDG state directory, honouring
// XDG_STATE_HOME when set and falling back to ~/.local/state.
//
// The directory is created 0700 and the caller opens the file 0600: the log holds
// repository names and process ids, which are nobody else's business.
func Resolve() (string, error) {
	dir, err := directory()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("create log directory %s: %w", dir, err)
	}
	return filepath.Join(dir, FileName), nil
}

func directory() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}
	if runtime.GOOS == "darwin" {
		return filepath.Join(home, "Library", "Logs"), nil
	}
	if state := os.Getenv("XDG_STATE_HOME"); state != "" {
		return filepath.Join(state, "trainsty"), nil
	}
	return filepath.Join(home, ".local", "state", "trainsty"), nil
}
