package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/exustash/trainsty/scheduler"
)

func TestHistoryIsAnEmptyListOnAFreshDaemon(t *testing.T) {
	s := New(scheduler.New(), DiscardLogger())
	rec := httptest.NewRecorder()

	s.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/history", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	if got := rec.Body.String(); got != "{\"jobs\":[]}\n" && got != `{"jobs":[]}` {
		t.Fatalf("want {\"jobs\":[]}, got %s", got)
	}
}

func TestHistoryReportsReleasedJobsNewestFirst(t *testing.T) {
	sched := scheduler.New()
	s := New(sched, DiscardLogger())
	sched.Register(101, "first")
	sched.Register(102, "second")
	sched.Release(sched.Current())
	sched.Release(sched.Current())
	rec := httptest.NewRecorder()

	s.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/history", nil))

	var history scheduler.History
	if err := json.Unmarshal(rec.Body.Bytes(), &history); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(history.Jobs) != 2 || history.Jobs[0].PID != 102 || history.Jobs[1].PID != 101 {
		t.Fatalf("want [102 101], got %+v", history.Jobs)
	}
}

func TestHistoryRefusesMutatingMethods(t *testing.T) {
	s := New(scheduler.New(), DiscardLogger())
	rec := httptest.NewRecorder()

	s.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/history", nil))

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("want 405, got %d", rec.Code)
	}
}
