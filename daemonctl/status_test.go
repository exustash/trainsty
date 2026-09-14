package daemonctl

import (
	"strings"
	"testing"
)

func TestStatusReportsIdle(t *testing.T) {
	fakeDaemon(t, idleStatus(), nil)
	var out, errOut strings.Builder

	code := Status(&out, &errOut)

	if code != 0 {
		t.Fatalf("want exit 0, got %d", code)
	}
	if !strings.Contains(out.String(), "idle") {
		t.Errorf("want an idle line; got %q", out.String())
	}
	if !strings.Contains(out.String(), "waiting: 0") {
		t.Errorf("want the queue length; got %q", out.String())
	}
}

func TestStatusReportsTheJobAndTheQueue(t *testing.T) {
	status := map[string]any{
		"job": map[string]any{"pid": 4242, "repo": "capture.web", "elapsedSeconds": 372},
		"queue": []any{
			map[string]any{"pid": 4300, "repo": "capture.desk", "waitingSeconds": 321},
			map[string]any{"pid": 4311, "repo": "postman", "waitingSeconds": 62},
		},
	}
	fakeDaemon(t, status, nil)
	var out, errOut strings.Builder

	code := Status(&out, &errOut)

	if code != 0 {
		t.Fatalf("want exit 0, got %d", code)
	}
	got := out.String()
	for _, want := range []string{"capture.web", "4242", "6m12s", "waiting: 2", "capture.desk", "postman"} {
		if !strings.Contains(got, want) {
			t.Errorf("output is missing %q; got %q", want, got)
		}
	}
	// Grant order must be visible, because it is what a waiting developer is asking
	// about: the position, not just the count.
	if strings.Index(got, "capture.desk") > strings.Index(got, "postman") {
		t.Error("the queue must print in grant order")
	}
}

// Exit 3, not 1. A Runner must be able to tell "no scheduler" from "the scheduler
// said no", and a bare non-zero cannot say which (FR-018).
func TestStatusExitsThreeWhenUnreachable(t *testing.T) {
	old := baseURL
	baseURL = "http://127.0.0.1:1"
	t.Cleanup(func() { baseURL = old })
	var out, errOut strings.Builder

	code := Status(&out, &errOut)

	if code != 3 {
		t.Fatalf("want exit 3 for an unreachable scheduler, got %d", code)
	}
	if !strings.Contains(errOut.String(), "no scheduler reachable") {
		t.Errorf("want an explanation on stderr; got %q", errOut.String())
	}
}

func TestHumanSeconds(t *testing.T) {
	for _, tc := range []struct {
		in   int
		want string
	}{
		{0, "0s"}, {45, "45s"}, {60, "1m00s"}, {372, "6m12s"}, {3601, "60m01s"},
	} {
		if got := humanSeconds(tc.in); got != tc.want {
			t.Errorf("humanSeconds(%d) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
