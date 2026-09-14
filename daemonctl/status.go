package daemonctl

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/exustash/trainsty/httpapi"
	"github.com/exustash/trainsty/scheduler"
)

func port() int { return httpapi.Port }

// Status prints what holds the Lock and who is waiting.
//
// Exit 3 — not 1 — when the scheduler is unreachable. A Runner must be able to
// tell "no scheduler" from "the scheduler said no", and a bare non-zero cannot say
// which (FR-018).
func Status(stdout, stderr io.Writer) int {
	snap, err := fetchStatus()
	if err != nil {
		fmt.Fprintf(stderr, "trainsty: no scheduler reachable on port %d\n", port())
		return 3
	}

	if snap.Job == nil {
		fmt.Fprintln(stdout, "idle: nothing holds the lock")
	} else {
		fmt.Fprintf(stdout, "running: %s (pid %d) for %s\n",
			snap.Job.Repo, snap.Job.PID, humanSeconds(snap.Job.ElapsedSeconds))
	}
	fmt.Fprintf(stdout, "waiting: %d\n", len(snap.Queue))
	for i, w := range snap.Queue {
		fmt.Fprintf(stdout, "  %d. %s (pid %d) for %s\n", i+1, w.Repo, w.PID, humanSeconds(w.WaitingSeconds))
	}
	return 0
}

func fetchStatus() (scheduler.Snapshot, error) {
	var snap scheduler.Snapshot
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(httpapi.BaseURL + "/status")
	if err != nil {
		return snap, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return snap, fmt.Errorf("unexpected status %s", resp.Status)
	}
	if err := json.NewDecoder(resp.Body).Decode(&snap); err != nil {
		return snap, err
	}
	return snap, nil
}

func humanSeconds(secs int) string {
	d := time.Duration(secs) * time.Second
	if d < time.Minute {
		return d.String()
	}
	return fmt.Sprintf("%dm%02ds", secs/60, secs%60)
}
