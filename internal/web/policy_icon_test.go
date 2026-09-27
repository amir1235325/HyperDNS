package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// The policy-icon route serves the embedded brand/category glyphs below the
// admin prefix. Pins: correct content type, 404 for unknown ids, no path games.
func TestPolicyIconRoute(t *testing.T) {
	ws := &WebServer{}

	get := func(path string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		ws.handlePolicyIcon(rec, httptest.NewRequest(http.MethodGet, path, nil))
		return rec
	}

	rec := get("/icons/policy/riot.svg")
	if rec.Code != http.StatusOK {
		t.Fatalf("riot.svg status = %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "image/svg+xml" {
		t.Fatalf("Content-Type = %q, want image/svg+xml", ct)
	}
	if !strings.HasPrefix(rec.Body.String(), "<svg") {
		t.Fatalf("body is not an SVG document: %.60s", rec.Body.String())
	}

	// The extension is optional — the trimmed id is what resolves.
	if rec2 := get("/icons/policy/riot"); rec2.Code != http.StatusOK {
		t.Fatalf("extensionless id status = %d, want 200", rec2.Code)
	}

	// Unknown and hostile ids are a plain 404, never a lookup.
	for _, bad := range []string{
		"/icons/policy/unknown-preset.svg",
		"/icons/policy/../server.go",
		"/icons/policy/RIOT.svg",
		"/icons/policy/riot.svg.svg",
		"/icons/policy/",
	} {
		if rec := get(bad); rec.Code != http.StatusNotFound {
			t.Errorf("%s: status = %d, want 404", bad, rec.Code)
		}
	}

	// POST is refused with the advertised verbs.
	rec = httptest.NewRecorder()
	ws.handlePolicyIcon(rec, httptest.NewRequest(http.MethodPost, "/icons/policy/riot.svg", nil))
	if rec.Code != http.StatusMethodNotAllowed || rec.Header().Get("Allow") != "GET, HEAD" {
		t.Fatalf("POST: status %d Allow %q, want 405 + GET, HEAD", rec.Code, rec.Header().Get("Allow"))
	}
}
