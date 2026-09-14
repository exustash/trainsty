package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/exustash/trainsty/scheduler"
)

func newTestServer(t *testing.T) *Server {
	t.Helper()
	return New(scheduler.New(), DiscardLogger())
}

// errorCode reads the stable code out of a JSON error body.
func errorCode(t *testing.T, body string) string {
	t.Helper()
	var payload struct{ Error string }
	if err := json.Unmarshal([]byte(body), &payload); err != nil {
		t.Fatalf("body is not a JSON error object: %q", body)
	}
	return payload.Error
}

// A GET on a mutating route is refused. Not REST tidiness: a GET is reachable
// from an <img src> on any page the developer has open, and this daemon
// terminates process groups (SDR-001).
func TestMutatingRoutesRefuseGET(t *testing.T) {
	s := newTestServer(t)
	// /release is registered from US1 onward; the guard itself is what this tests,
	// so it is exercised through a handler the guard wraps directly.
	handler := guardMutating(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]bool{"reached": true})
	})

	for _, method := range []string{http.MethodGet, http.MethodPut, http.MethodDelete, http.MethodHead} {
		rec := httptest.NewRecorder()
		handler(rec, httptest.NewRequest(method, "/release", nil))

		if rec.Code != http.StatusMethodNotAllowed {
			t.Errorf("%s: want 405, got %d", method, rec.Code)
		}
		if got := rec.Header().Get("Allow"); got != http.MethodPost {
			t.Errorf("%s: want an Allow: POST header, got %q", method, got)
		}
		if code := errorCode(t, rec.Body.String()); code != codeMethodNotAllowed {
			t.Errorf("%s: want %s, got %s", method, codeMethodNotAllowed, code)
		}
	}
	_ = s
}

// The JSON content type is the CSRF defence: a cross-origin HTML form can only
// send three content types, none of them this one.
func TestMutatingRoutesRequireAJSONContentType(t *testing.T) {
	handler := guardMutating(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]bool{"reached": true})
	})

	for _, tc := range []struct {
		name        string
		contentType string
		wantStatus  int
	}{
		{"application/json is accepted", "application/json", http.StatusOK},
		{"a charset parameter is still accepted", "application/json; charset=utf-8", http.StatusOK},
		{"mixed case is accepted", "Application/JSON", http.StatusOK},
		{"a form post is refused", "application/x-www-form-urlencoded", http.StatusUnsupportedMediaType},
		{"multipart is refused", "multipart/form-data; boundary=x", http.StatusUnsupportedMediaType},
		{"text/plain is refused", "text/plain", http.StatusUnsupportedMediaType},
		{"an absent content type is refused", "", http.StatusUnsupportedMediaType},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/release", nil)
			if tc.contentType != "" {
				req.Header.Set("Content-Type", tc.contentType)
			}
			rec := httptest.NewRecorder()

			handler(rec, req)

			if rec.Code != tc.wantStatus {
				t.Fatalf("want %d, got %d (body %q)", tc.wantStatus, rec.Code, rec.Body.String())
			}
			if tc.wantStatus == http.StatusUnsupportedMediaType {
				if code := errorCode(t, rec.Body.String()); code != codeUnsupportedMedia {
					t.Errorf("want %s, got %s", codeUnsupportedMedia, code)
				}
			}
		})
	}
}

func TestOriginPolicy(t *testing.T) {
	handler := guardMutating(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]bool{"reached": true})
	})

	for _, tc := range []struct {
		name       string
		origin     string
		wantStatus int
	}{
		// An absent Origin is allowed deliberately: a shell client sends none, and
		// requiring it would break every hand-written Runner while stopping no browser.
		{"absent is allowed — shell clients send none", "", http.StatusOK},
		{"the daemon's own localhost origin", "http://localhost:45678", http.StatusOK},
		{"the daemon's own loopback ip", "http://127.0.0.1:45678", http.StatusOK},
		{"another site is refused", "https://evil.example", http.StatusForbidden},
		{"another local port is refused", "http://localhost:3000", http.StatusForbidden},
		{"a lookalike host is refused", "http://localhost.evil.example:45678", http.StatusForbidden},
		{"an unparseable origin is refused", "://nonsense", http.StatusForbidden},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/stop", nil)
			req.Header.Set("Content-Type", "application/json")
			if tc.origin != "" {
				req.Header.Set("Origin", tc.origin)
			}
			rec := httptest.NewRecorder()

			handler(rec, req)

			if rec.Code != tc.wantStatus {
				t.Fatalf("origin %q: want %d, got %d", tc.origin, tc.wantStatus, rec.Code)
			}
			if tc.wantStatus == http.StatusForbidden {
				if code := errorCode(t, rec.Body.String()); code != codeForbiddenOrigin {
					t.Errorf("want %s, got %s", codeForbiddenOrigin, code)
				}
			}
		})
	}
}

func TestReadOnlyRoutesRefuseNonGET(t *testing.T) {
	s := newTestServer(t)
	rec := httptest.NewRecorder()

	s.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/status", nil))

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("want 405 for POST /status, got %d", rec.Code)
	}
}

// No response body may carry an internal error string, a filesystem path, or an
// errno text. The detail belongs in the log.
func TestErrorBodiesLeakNothing(t *testing.T) {
	s := newTestServer(t)
	for _, target := range []string{"/status", "/nope", "/release"} {
		rec := httptest.NewRecorder()
		s.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodPut, target, nil))

		body := rec.Body.String()
		for _, leak := range []string{"/Users/", "/home/", "syscall", "ESRCH", "EPERM", "goroutine"} {
			if strings.Contains(body, leak) {
				t.Errorf("%s: response body leaks %q: %s", target, leak, body)
			}
		}
	}
}

func TestUnknownPathsAre404WithAnEmptyBody(t *testing.T) {
	s := newTestServer(t)
	rec := httptest.NewRecorder()

	s.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/admin", nil))

	if rec.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d", rec.Code)
	}
	if rec.Body.Len() != 0 {
		t.Fatalf("want an empty body, got %q", rec.Body.String())
	}
}
