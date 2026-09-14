// Package dashboard holds the embedded web page.
//
// go:embed is what lets one static binary serve a UI with no asset directory, no
// bundler and no build step (ADR-001). The page is one file with inline CSS and
// JavaScript, small enough to read, and it loads nothing from a network — a daemon
// must work on a plane.
package dashboard

import (
	_ "embed"
	"net/http"
)

//go:embed index.html
var page []byte

// Page returns the dashboard's bytes.
func Page() []byte { return page }

// ServeHTTP writes the dashboard.
func ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	// No caching: a redeployed binary must not serve a stale page from a tab the
	// developer left open for a week.
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(page)
}
