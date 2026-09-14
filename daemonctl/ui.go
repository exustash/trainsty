package daemonctl

import (
	"fmt"
	"io"
	"os/exec"
	"runtime"

	"github.com/exustash/trainsty/httpapi"
)

// UI opens the dashboard in the default browser.
//
// A failed browser launch is not a failed command: the URL is printed and the exit
// code stays 0, because the developer can click it (FR-016).
func UI(stdout, stderr io.Writer) int {
	url := fmt.Sprintf("http://localhost:%d", httpapi.Port)

	opener := "xdg-open"
	if runtime.GOOS == "darwin" {
		opener = "open"
	}
	// Audited: both the opener and the URL are compile-time constants for this
	// platform. No external input reaches this call, and there is no shell.
	// nosemgrep: go.lang.security.audit.dangerous-exec-command.dangerous-exec-command
	if err := exec.Command(opener, url).Start(); err != nil {
		fmt.Fprintf(stderr, "trainsty: could not launch a browser (%v)\n", err)
		fmt.Fprintf(stdout, "%s\n", url)
		return 0
	}
	fmt.Fprintf(stdout, "trainsty: opened %s\n", url)
	return 0
}
