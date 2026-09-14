package logpath

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestResolveReturnsThePlatformPathAndCreatesTheDirectory(t *testing.T) {
	// A real home, redirected, so the test never writes to the developer's own
	// Library/Logs or state directory.
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_STATE_HOME", "")

	path, err := Resolve()

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if filepath.Base(path) != FileName {
		t.Fatalf("want basename %s, got %s", FileName, path)
	}
	wantDir := filepath.Join(home, ".local", "state", "trainsty")
	if runtime.GOOS == "darwin" {
		wantDir = filepath.Join(home, "Library", "Logs")
	}
	if filepath.Dir(path) != wantDir {
		t.Fatalf("want directory %s, got %s", wantDir, filepath.Dir(path))
	}
	info, err := os.Stat(filepath.Dir(path))
	if err != nil {
		t.Fatalf("the parent directory was not created: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o700 {
		t.Errorf("want the directory 0700, got %04o", perm)
	}
}

func TestResolveHonoursXDGStateHomeOffDarwin(t *testing.T) {
	if runtime.GOOS == "darwin" {
		t.Skip("darwin uses ~/Library/Logs by design, so XDG_STATE_HOME does not apply")
	}
	home := t.TempDir()
	state := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_STATE_HOME", state)

	path, err := Resolve()

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.HasPrefix(path, state) {
		t.Fatalf("want a path under %s, got %s", state, path)
	}
}

// The file is opened by the caller, but the directory must already be usable —
// resolving twice must be safe, because start runs after any previous start.
func TestResolveIsIdempotent(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	first, err := Resolve()
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	second, err := Resolve()
	if err != nil {
		t.Fatalf("second: %v", err)
	}
	if first != second {
		t.Fatalf("want a stable path, got %s then %s", first, second)
	}
}
