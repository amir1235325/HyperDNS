package presets

import (
	"strings"
	"testing"
)

// Every preset whose card the dashboard can show must have an embedded icon,
// and the icon field in the JSON must point at exactly that file — this is the
// contract the dashboard's data-preset upgrade relies on.
func TestEveryPresetHasAnEmbeddedIcon(t *testing.T) {
	for _, p := range All() {
		if p.Icon != "icons/"+p.ID+".svg" {
			t.Errorf("%s: icon field %q does not follow the icons/<id>.svg convention", p.ID, p.Icon)
		}
		raw, ok := Icon(p.ID)
		if !ok {
			t.Errorf("%s: no embedded icon at icons/%s.svg", p.ID, p.ID)
			continue
		}
		s := string(raw)
		if !strings.HasPrefix(s, "<svg") || !strings.HasSuffix(s, "</svg>\n") {
			t.Errorf("%s: icon is not a standalone SVG document", p.ID)
		}
		if !strings.Contains(s, "viewBox=\"0 0 24 24\"") {
			t.Errorf("%s: icon is not on the 24x24 grid", p.ID)
		}
		if !strings.Contains(s, "currentColor") {
			t.Errorf("%s: icon does not follow the theme (no currentColor)", p.ID)
		}
	}
}

// The lookup refuses anything that is not a generator-written id, so a
// caller-supplied string can never name a file.
func TestIconRejectsBadIDs(t *testing.T) {
	for _, bad := range []string{"", "../riot", "riot.svg", "RIOT", "riot;drop", string(make([]byte, 65))} {
		if _, ok := Icon(bad); ok {
			t.Errorf("Icon(%q) must be refused", bad)
		}
	}
	if !HasIcon("riot") {
		t.Error("HasIcon(riot) = false; the embedded set is broken")
	}
}
