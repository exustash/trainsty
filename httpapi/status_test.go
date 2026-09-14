package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/exustash/trainsty/scheduler"
)

func TestStatusIsIdleOnAFreshDaemon(t *testing.T) {
	s := New(scheduler.New(), DiscardLogger())
	rec := httptest.NewRecorder()

	s.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/status", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	body := rec.Body.String()
	// Queue must serialise as [] rather than null: no client should have to tell an
	// absent list from an empty one.
	if !strings.Contains(body, `"queue":[]`) {
		t.Fatalf("want an empty queue array, got %s", body)
	}
	if !strings.Contains(body, `"job":null`) {
		t.Fatalf("want a null job, got %s", body)
	}
}

func TestStatusReportsTheJobAndTheQueueInGrantOrder(t *testing.T) {
	sched := scheduler.New()
	s := New(sched, DiscardLogger())
	sched.Register(101, "first")
	sched.Register(102, "second")
	sched.Register(103, "third")

	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/status", nil))

	var snap scheduler.Snapshot
	if err := json.Unmarshal(rec.Body.Bytes(), &snap); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if snap.Job == nil || snap.Job.PID != 101 {
		t.Fatalf("want pid 101 holding, got %+v", snap.Job)
	}
	if len(snap.Queue) != 2 || snap.Queue[0].PID != 102 || snap.Queue[1].PID != 103 {
		t.Fatalf("want the queue in grant order [102 103], got %+v", snap.Queue)
	}
	// Invariant 2, over the wire.
	if snap.Queue[0].PID == snap.Job.PID || snap.Queue[1].PID == snap.Job.PID {
		t.Fatal("a pid appears as both the Job and a Waiter")
	}
}

func TestRootServesTheDashboardAndOtherPathsAre404(t *testing.T) {
	s := New(scheduler.New(), DiscardLogger())

	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200 at /, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "trainsty") {
		t.Fatal("want the dashboard at /")
	}

	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/admin", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("want 404 at /admin, got %d", rec.Code)
	}
	if rec.Body.Len() != 0 {
		t.Fatalf("want an empty 404 body, got %q", rec.Body.String())
	}
}
