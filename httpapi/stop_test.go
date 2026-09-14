package httpapi

import (
	"bufio"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/exustash/trainsty/scheduler"
)

func postStop(t *testing.T, s *Server) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/stop", nil)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	return rec
}

// The test that catches signalling a bare pid at the API level: a real child tree
// must be gone entirely, because an aborted suite's headless browsers are what hold
// the ports the next run needs.
func TestStopTerminatesTheWholeChildTreeAndFreesTheLock(t *testing.T) {
	script := `sleep 30 & echo $!; sleep 30 & echo $!; wait`
	cmd := exec.Command("sh", "-c", script)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	leader := cmd.Process.Pid
	t.Cleanup(func() { _ = syscall.Kill(-leader, syscall.SIGKILL); _ = cmd.Wait() })

	var children []int
	scan := bufio.NewScanner(stdout)
	for len(children) < 2 && scan.Scan() {
		pid, convErr := strconv.Atoi(strings.TrimSpace(scan.Text()))
		if convErr != nil {
			t.Fatalf("unexpected line: %q", scan.Text())
		}
		children = append(children, pid)
	}
	if len(children) != 2 {
		t.Fatalf("want 2 children, got %v", children)
	}

	sched := scheduler.New()
	s := New(sched, DiscardLogger())
	sched.Register(leader, "hung")
	waiter := sched.Register(999002, "next")

	rec := postStop(t, s)

	// Reaped here, before asserting: the leader is a child of THIS test process, so
	// after being killed it stays a zombie until waited on — and a zombie still
	// answers kill(pid, 0) successfully, which would read as "survived the stop".
	_, _ = cmd.Process.Wait()

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	var payload struct {
		Stopped bool `json:"stopped"`
		PID     int  `json:"pid"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !payload.Stopped || payload.PID != leader {
		t.Fatalf("unexpected payload: %+v", payload)
	}

	// Every process in the group, not just the leader.
	for _, pid := range append([]int{leader}, children...) {
		gone := false
		deadline := time.Now().Add(3 * time.Second)
		for time.Now().Before(deadline) {
			if syscall.Kill(pid, 0) != nil {
				gone = true
				break
			}
			time.Sleep(25 * time.Millisecond)
		}
		if !gone {
			t.Errorf("pid %d survived the stop — the signal went to a process, not a group", pid)
		}
	}

	// …and the queue moved on.
	select {
	case <-waiter.Granted():
	default:
		t.Fatal("the next waiter was not granted after the stop")
	}
}

// A stale page or a double click is ordinary, not a failure (FR-031).
func TestStopWithNoJobIsASuccessThatStopsNothing(t *testing.T) {
	s := New(scheduler.New(), DiscardLogger())

	rec := postStop(t, s)

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	var payload struct {
		Stopped bool `json:"stopped"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &payload)
	if payload.Stopped {
		t.Fatal("want stopped false when there is no job")
	}
}

// The stale-click case, and the reason /stop takes no pid: whatever the dashboard
// rendered, the request acts on the CURRENT job — and if that job is a successor,
// the caller learns which pid was actually stopped.
func TestStopActsOnTheCurrentJobNotTheOneAPageRendered(t *testing.T) {
	sched := scheduler.New()
	s := New(sched, DiscardLogger())
	// The job a stale page would have rendered.
	sched.Register(999003, "finished")
	stale := sched.Current()
	// It finishes on its own, and a successor takes the Lock.
	sched.Release(stale)
	sched.Register(999004, "successor")

	rec := postStop(t, s)

	var payload struct {
		Stopped bool `json:"stopped"`
		PID     int  `json:"pid"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &payload)
	// It reports the pid it actually acted on, which is the successor's — the API
	// cannot resurrect the stale job, and it must not pretend it stopped it.
	if payload.PID == 999003 {
		t.Fatal("the stop reported the stale pid rather than the current job")
	}
	if payload.PID != 999004 {
		t.Fatalf("want the current job's pid, got %d", payload.PID)
	}
}
