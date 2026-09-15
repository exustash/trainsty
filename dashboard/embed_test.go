package dashboard

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPageIsServedAsHTML(t *testing.T) {
	rec := httptest.NewRecorder()

	ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Fatalf("want text/html, got %q", ct)
	}
	if rec.Body.Len() == 0 {
		t.Fatal("the embedded page is empty — go:embed did not include it")
	}
}

// A grep-able guard that survives future edits. `repo` is developer-supplied and
// lands on a page that can terminate process groups, so every value must be
// written with textContent.
//
// Matched on USAGE — the leading dot or paren — rather than on the bare word. The
// first version of this test grepped for "innerHTML" and failed on the comment
// that explains the rule, which is the same false positive the glossary warns
// about: a banned word may legitimately appear in the sentence banning it.
func TestPageNeverUsesInnerHTML(t *testing.T) {
	src := string(Page())
	for _, banned := range []string{".innerHTML", ".outerHTML", ".insertAdjacentHTML(", "document.write("} {
		if strings.Contains(src, banned) {
			t.Errorf("the dashboard uses %s — every value must be written with textContent", banned)
		}
	}
}

// The daemon must work with no network at all: developers run local CI on planes,
// and a page that silently loses its layout offline is worse than a plain one.
func TestPageLoadsNothingRemote(t *testing.T) {
	// The SVG namespace is an identifier the browser never fetches, and the
	// inline logo and the favicon both carry it. Same false positive the
	// innerHTML guard above records: strip it, then scan for the rest.
	src := strings.ReplaceAll(string(Page()), "http://www.w3.org/2000/svg", "")
	for _, banned := range []string{"http://", "https://", "//cdn", "integrity="} {
		if strings.Contains(src, banned) {
			t.Errorf("the dashboard references %q — it must load nothing remote", banned)
		}
	}
}

// The CSRF floor reaches the page as well as the server: the Stop request must
// carry the JSON content type, and must NOT name a target — a stale tab cannot be
// allowed to stop a suite that started after it rendered (FR-032).
func TestStopRequestCarriesTheJSONContentTypeAndNoPid(t *testing.T) {
	src := string(Page())

	if !strings.Contains(src, `"Content-Type": "application/json"`) {
		t.Error("the Stop request must set Content-Type: application/json (SDR-001)")
	}
	if strings.Contains(src, "/stop?pid") || strings.Contains(src, "pid=") {
		t.Error("the Stop request must not name a pid — a stale page would target the wrong suite")
	}
	if !strings.Contains(src, "confirm(") {
		t.Error("Stop must confirm: it is the one destructive control on the page")
	}
}

// Distinguishing the two is the difference between a trusted page and a lying one.
func TestPageDistinguishesUnreachableFromIdle(t *testing.T) {
	src := string(Page())
	if !strings.Contains(src, "Cannot reach the scheduler") {
		t.Error("the page must say when it cannot reach the scheduler")
	}
	if !strings.Contains(src, "Nothing holds the lock") {
		t.Error("the page must say when nothing is running")
	}
}

// An advancing elapsed time rather than a static badge (ADR-005): the page can be
// seconds behind reality, and a counter tells the reader what they are looking at.
func TestPageRendersAnAdvancingElapsedTime(t *testing.T) {
	src := string(Page())
	if !strings.Contains(src, "elapsedSeconds") {
		t.Error("the page must render the job's elapsed time")
	}
	if !strings.Contains(src, "setInterval") {
		t.Error("the page must poll, so the elapsed time advances")
	}
}
