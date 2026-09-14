// Package httpapi serves the five-endpoint local API and the dashboard.
//
// The API is a compatibility surface: its clients include hand-written Runner
// scripts in repositories this project cannot see and cannot upgrade
// (specs/001-serialize-e2e-runs/contracts/http-api.md).
package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// Stable error codes. The code is the machine-readable contract the Dashboard and
// the Runner branch on; the message is for humans. Adding one is a contract change.
const (
	codeInvalidPID           = "invalid_pid"
	codeNoSuchProcess        = "no_such_process"
	codeNotGroupLeader       = "not_group_leader"
	codeInvalidRepo          = "invalid_repo"
	codeMethodNotAllowed     = "method_not_allowed"
	codeUnsupportedMedia     = "unsupported_media_type"
	codeForbiddenOrigin      = "forbidden_origin"
	codeStreamingUnsupported = "streaming_unsupported"
)

// writeJSONError is the only way this package emits an error body.
//
// It writes a code and nothing else: never an internal error string, a filesystem
// path, or an errno text. The detail belongs in the log.
// The encode error is discarded deliberately, here and in writeJSON. Every value
// either function writes is primitives or a Snapshot of them, so marshalling cannot
// fail: the only way to get an error is a client that has already gone. The status
// line is sent, so there is nothing left to tell it — and nothing a log line would
// tell a developer either, because every outcome worth recording is logged by the
// handler that caused it. A dashboard tab closed mid-poll is not an event.
func writeJSONError(w http.ResponseWriter, status int, code string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": code})
}

// writeJSON writes a success payload. Its encode error is discarded for the reason
// given above writeJSONError.
func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

// guardMutating wraps a handler that changes state. It enforces the whole of the
// termination floor from SDR-001, and every clause earns its place:
//
//   - POST only. A GET is reachable from an <img src> on any page the developer
//     has open, and this daemon terminates process groups.
//   - Content-Type: application/json. A cross-origin HTML form can only send three
//     content types, none of them this one, so a form-based request cannot be
//     formed at all. A fetch that sets it triggers a preflight, which never
//     arrives here.
//   - A foreign Origin is refused. An ABSENT Origin is allowed, deliberately: a
//     shell client sends none, and requiring it would break every hand-written
//     Runner while stopping no browser.
//
// No shared secret. What remains reachable is another process running as the same
// developer, which could already signal the suite directly — see SDR-001 for the
// residual and the condition under which it must be revisited.
func guardMutating(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			writeJSONError(w, http.StatusMethodNotAllowed, codeMethodNotAllowed)
			return
		}
		if !hasJSONContentType(r) {
			writeJSONError(w, http.StatusUnsupportedMediaType, codeUnsupportedMedia)
			return
		}
		if !originAllowed(r) {
			writeJSONError(w, http.StatusForbidden, codeForbiddenOrigin)
			return
		}
		next(w, r)
	}
}

// guardReadOnly wraps a handler that only reads.
func guardReadOnly(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			writeJSONError(w, http.StatusMethodNotAllowed, codeMethodNotAllowed)
			return
		}
		next(w, r)
	}
}

// hasJSONContentType accepts application/json with or without parameters, so a
// client sending "application/json; charset=utf-8" is not refused for being polite.
func hasJSONContentType(r *http.Request) bool {
	ct := r.Header.Get("Content-Type")
	if ct == "" {
		return false
	}
	base := strings.TrimSpace(strings.SplitN(ct, ";", 2)[0])
	return strings.EqualFold(base, "application/json")
}

// originAllowed permits an absent Origin and the daemon's own loopback origins.
//
// The port is fixed (ADR-011), so the allowed set is small and hardcoded rather
// than derived from the request's Host — which an attacker controls.
func originAllowed(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true // a shell client, not a browser
	}
	u, err := url.Parse(origin)
	if err != nil {
		return false
	}
	if u.Scheme != "http" {
		return false
	}
	switch u.Host {
	case fmt.Sprintf("localhost:%d", Port), fmt.Sprintf("127.0.0.1:%d", Port):
		return true
	default:
		return false
	}
}
