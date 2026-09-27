package web

import (
	"net/http"
	"strconv"
	"strings"

	"hyperdns/internal/httpx"
	"hyperdns/presets"
)

// handlePolicyIcon serves the embedded, theme-adaptive SVG for a built-in
// policy preset: GET /icons/policy/<id>.svg, below the admin prefix like every
// other dashboard asset. The glyph is the same geometry the cards used to show
// as hard-coded Feather icons — the dashboard swaps the Feather fallback for
// this image when it loads, so a preset that ships before its icon (a channel
// delivery) simply keeps the old glyph rather than showing a broken image.
//
// The lookup is by allow-listed id into the compile-time embedded FS: no user
// path ever reaches a filesystem, and the body is a fixed, generator-written
// file (no operator or subscriber input can shape it).
func (ws *WebServer) handlePolicyIcon(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		httpx.WriteMethodNotAllowed(w, "GET, HEAD")
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/icons/policy/")
	id = strings.TrimSuffix(id, ".svg")
	raw, ok := presets.Icon(id)
	if !ok {
		http.NotFound(w, r)
		return
	}
	// Icons are baked into the binary and change only with it: cache a day in
	// the browser so the policy tab's thirty glyphs cost one request per day,
	// and an etag so a reload after an update revalidates for free.
	w.Header().Set("Content-Type", "image/svg+xml")
	w.Header().Set("Cache-Control", "private, max-age=86400")
	w.Header().Set("ETag", `"`+strconv.Itoa(len(raw))+`-v2"`)
	if r.Method == http.MethodHead {
		w.Header().Set("Content-Length", strconv.Itoa(len(raw)))
		return
	}
	_, _ = w.Write(raw)
}
