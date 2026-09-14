package main

import (
	"fmt"
	"runtime"
	"runtime/debug"
	"strings"
)

// versionLine describes this binary well enough to act on a bug report: what it
// claims to be, which commit it was built from, and the toolchain and platform that
// produced it.
//
// Read from the build rather than injected with -ldflags, which is the whole reason
// it needs no release-time plumbing: debug.ReadBuildInfo reports the module version
// for a binary installed with `go install …@v1.2.3` and the VCS revision for one
// built from a checkout, so both distribution channels answer without the release
// step having to remember anything (OD-4). Stripping with -s -w does not remove it.
func versionLine() string {
	platform := runtime.GOOS + "/" + runtime.GOARCH

	info, ok := debug.ReadBuildInfo()
	if !ok {
		// Only reachable for a binary built without build info at all, which the Go
		// toolchain does not do by default. Say so rather than invent a version.
		return fmt.Sprintf("trainsty (unknown build) %s %s", runtime.Version(), platform)
	}

	version := info.Main.Version
	if version == "" {
		version = "(devel)"
	}

	// A dirty tree is worth saying out loud: it means the revision below does not
	// describe what is actually running.
	var revision string
	var dirty bool
	for _, setting := range info.Settings {
		switch setting.Key {
		case "vcs.revision":
			revision = setting.Value
		case "vcs.modified":
			dirty = setting.Value == "true"
		}
	}

	details := []string{}
	if revision != "" {
		short := revision
		if len(short) > 12 {
			short = short[:12]
		}
		// An untagged build's version is a pseudo-version that already embeds the
		// revision and its own +dirty marker, so repeating them is noise. A tagged
		// build's "v1.0.0" does not, and there the commit is the useful half.
		if !strings.Contains(version, short) {
			if dirty {
				short += "-dirty"
			}
			details = append(details, short)
		}
	}
	details = append(details, info.GoVersion, platform)

	return fmt.Sprintf("trainsty %s (%s)", version, strings.Join(details, ", "))
}
