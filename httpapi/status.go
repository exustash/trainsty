package httpapi

import "net/http"

// handleStatus reports the whole state: the Job and the Queue in grant order.
//
// The snapshot is taken under the scheduler's mutex and marshalled after it is
// released — marshalling live structs while another goroutine mutates them is a
// data race that -race finds and a reviewer does not.
//
// Elapsed times are computed server-side, so the Dashboard needs no clock and
// cannot disagree about "now"; and Queue serialises as [] rather than null, so no
// client has to tell an absent list from an empty one
// (specs/001-serialize-e2e-runs/contracts/http-api.md).
func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, s.sched.Snapshot())
}

// handleRoot serves the dashboard at / and 404s everything else with an empty
// body. The dashboard itself arrives with US2; until then / is a 404 too.
func (s *Server) handleRoot(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNotFound)
}
