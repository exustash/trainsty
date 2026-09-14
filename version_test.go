package main

import (
	"runtime"
	"strings"
	"testing"
)

// A version line nobody can act on is worse than none: it invites a bug report that
// does not say what was running. These are the parts a report needs.
func TestVersionLineNamesTheBinaryAndThePlatform(t *testing.T) {
	line := versionLine()

	if !strings.HasPrefix(line, "trainsty ") {
		t.Errorf("the line must name the binary; got %q", line)
	}
	if !strings.Contains(line, runtime.GOOS+"/"+runtime.GOARCH) {
		t.Errorf("the line must name the platform; got %q", line)
	}
	if !strings.Contains(line, "go1.") {
		t.Errorf("the line must name the toolchain; got %q", line)
	}
	if strings.Contains(line, "unknown build") {
		t.Errorf("build info was unreadable, so the version cannot be reported: %q", line)
	}
}

// version is dispatched like any other subcommand, and must exit 0 — a script that
// probes `trainsty version` to decide whether trainsty is installed branches on it.
func TestVersionIsDispatchedAndSucceeds(t *testing.T) {
	if code := dispatch([]string{"version"}); code != exitOK {
		t.Errorf("want exit %d, got %d", exitOK, code)
	}
}
