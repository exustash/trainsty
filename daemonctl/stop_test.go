package daemonctl

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// fakeDaemon stands in for a running scheduler. Pointed at through the baseURL
// seam, so no test here binds the real port 45678.
func fakeDaemon(t *testing.T, status any, onShutdown func()) {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/status", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(status)
	})
	mux.HandleFunc("/shutdown", func(w http.ResponseWriter, r *http.Request) {
		if onShutdown != nil {
			onShutdown()
		}
		active := false
		if m, ok := status.(map[string]any); ok {
			active = m["job"] != nil
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"shuttingDown": true, "jobWasActive": active})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	old := baseURL
	baseURL = srv.URL
	t.Cleanup(func() { baseURL = old })
}

func runningStatus() map[string]any {
	return map[string]any{
		"job":   map[string]any{"pid": 4242, "repo": "capture.web", "elapsedSeconds": 372},
		"queue": []any{},
	}
}

func idleStatus() map[string]any {
	return map[string]any{"job": nil, "queue": []any{}}
}

// The safety behaviour that matters: a scripted `trainsty stop` must NOT assume yes
// while a suite is running. Assuming would silently orphan it (ADR-009), and the
// developer would find out when two suites next collided.
func TestStopRefusesWhenASuiteIsRunningAndThereIsNoTerminal(t *testing.T) {
	shutdownCalled := false
	fakeDaemon(t, runningStatus(), func() { shutdownCalled = true })
	var out, errOut strings.Builder

	// strings.Reader is not a character device, so isTerminal is false.
	code := Stop(false, strings.NewReader(""), &out, &errOut)

	if code != 1 {
		t.Fatalf("want exit 1, got %d", code)
	}
	if shutdownCalled {
		t.Fatal("stop shut the daemon down without consent while a suite was running")
	}
	if !strings.Contains(errOut.String(), "--force") {
		t.Errorf("the refusal must name the way forward; got %q", errOut.String())
	}
	// The warning has to say WHICH repo and WHAT the consequence is, before the
	// developer decides (FR-014).
	if !strings.Contains(out.String(), "capture.web") {
		t.Errorf("the warning must name the repository; got %q", out.String())
	}
	if !strings.Contains(out.String(), "does NOT stop that suite") {
		t.Errorf("the warning must state that the suite keeps running; got %q", out.String())
	}
}

func TestStopProceedsWithForceWhileASuiteIsRunning(t *testing.T) {
	shutdownCalled := false
	fakeDaemon(t, runningStatus(), func() { shutdownCalled = true })
	var out, errOut strings.Builder

	code := Stop(true, strings.NewReader(""), &out, &errOut)

	if code != 0 {
		t.Fatalf("want exit 0 with --force, got %d (%s)", code, errOut.String())
	}
	if !shutdownCalled {
		t.Fatal("--force must actually shut the daemon down")
	}
	// And it must still say what it left behind, so an orphaned suite is not a
	// surprise discovered later.
	if !strings.Contains(out.String(), "still running") {
		t.Errorf("want a notice that the suite survives; got %q", out.String())
	}
}

func TestStopIsSilentAndSucceedsWhenIdle(t *testing.T) {
	fakeDaemon(t, idleStatus(), nil)
	var out, errOut strings.Builder

	code := Stop(false, strings.NewReader(""), &out, &errOut)

	if code != 0 {
		t.Fatalf("want exit 0 when idle, got %d (%s)", code, errOut.String())
	}
	if strings.Contains(out.String(), "?") {
		t.Errorf("an idle daemon must not prompt; got %q", out.String())
	}
}

// A no-op a script may run unconditionally: nothing to stop is not a failure.
func TestStopSucceedsWhenNoDaemonIsReachable(t *testing.T) {
	old := baseURL
	baseURL = "http://127.0.0.1:1" // nothing listens there
	t.Cleanup(func() { baseURL = old })
	var out, errOut strings.Builder

	code := Stop(false, strings.NewReader(""), &out, &errOut)

	if code != 0 {
		t.Fatalf("want exit 0 when there is nothing to stop, got %d", code)
	}
	if !strings.Contains(out.String(), "nothing to stop") {
		t.Errorf("want a plain explanation; got %q", out.String())
	}
}

func TestIsTerminalIsFalseForANonDevice(t *testing.T) {
	if isTerminal(strings.NewReader("")) {
		t.Fatal("a strings.Reader is not a terminal")
	}
}

// The warning that a suite was left running is the whole point of ADR-009 reaching
// the developer, and --force skipped the pre-flight one. An unreadable answer from
// the daemon must not be the thing that silently drops it.
func TestStopStillWarnsWhenTheShutdownAnswerCannotBeRead(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/status", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(runningStatus())
	})
	mux.HandleFunc("/shutdown", func(w http.ResponseWriter, r *http.Request) {
		// Truncated: what a daemon that exits mid-write leaves on the wire.
		fmt.Fprint(w, `{"shuttingDown":tr`)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	old := baseURL
	baseURL = srv.URL
	t.Cleanup(func() { baseURL = old })
	var out, errOut strings.Builder

	code := Stop(true, strings.NewReader(""), &out, &errOut)

	if code != 0 {
		t.Fatalf("want exit 0, got %d (stderr %q)", code, errOut.String())
	}
	if !strings.Contains(out.String(), "unsupervised") {
		t.Errorf("an unreadable answer dropped the orphan warning; got %q", out.String())
	}
}
