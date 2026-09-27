package web

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"strings"
	"testing"
)

// The group-icon upload standard: an SVG that goes IN with hostile content must
// come OUT with none of it, because the stored bytes are a re-encode from the
// allowlist — and a PNG above the ceiling must be refused outright.
func TestSanitizeGroupSVGStripsTheDirtyHalf(t *testing.T) {
	// A logo-shaped payload: clean geometry plus every vector we know of.
	dirty := `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" onload="alert(1)">
		<title>brand</title>
		<script>alert(2)</script>
		<foreignObject width="24" height="24"><body>x</body></foreignObject>
		<use href="https://evil.example/x.svg"/>
		<circle cx="12" cy="12" r="10" fill="url(https://evil.example/grad)"/>
		<circle cx="12" cy="12" r="9" fill="url(#internal)"/>
		<path d="M4 4h16v16H4z" style="background:url(https://evil.example)"/>
		<image href="https://evil.example/x.png"/>
		<rect x="2" y="2" width="20" height="20" stroke="currentColor"/>
	</svg>`
	clean, err := sanitizeGroupSVG([]byte(dirty))
	if err == nil {
		t.Fatalf("a payload mixing rejected constructs must be refused, got: %s", clean)
	}

	// The same constructs, one per upload: each is rejected.
	for name, svg := range map[string]string{
		"script":        `<svg><script>alert(1)</script></svg>`,
		"foreignObject": `<svg><foreignObject><body>x</body></foreignObject></svg>`,
		"onload":        `<svg onload="alert(1)"><circle r="5"/></svg>`,
		"external href": `<svg><use href="https://evil.example/x.svg"/></svg>`,
		"external url":  `<svg><circle r="5" fill="url(https://evil.example/g)"/></svg>`,
		"style attr":    `<svg><circle r="5" style="fill:url(https://evil.example)"/></svg>`,
		"image element": `<svg><image href="https://evil.example/x.png"/></svg>`,
		"not xml":       `<svg><circle><oops></svg>`,
		"no svg root":   `<div>hi</div>`,
		"oversize":      `<svg>` + strings.Repeat("<circle r=\"1\"/>", 4000) + `</svg>`,
	} {
		if _, err := sanitizeGroupSVG([]byte(svg)); err == nil {
			t.Errorf("%s: must be rejected", name)
		}
	}
}

// Clean input survives the round trip with its geometry intact, including an
// internal gradient reference and an XML prolog.
func TestSanitizeGroupSVGKeepsCleanGeometry(t *testing.T) {
	clean := `<?xml version="1.0" encoding="UTF-8"?>
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24">
	<defs><linearGradient id="g"><stop offset="0" stop-color="#0ff"/></linearGradient></defs>
	<circle cx="12" cy="12" r="10" fill="url(#g)"/>
</svg>`
	out, err := sanitizeGroupSVG([]byte(clean))
	if err != nil {
		t.Fatalf("clean SVG rejected: %v", err)
	}
	s := string(out)
	for _, want := range []string{`<circle`, `url(#g)`, `linearGradient`, `stop-color="#0ff"`, `viewBox="0 0 24 24"`} {
		if !strings.Contains(s, want) {
			t.Errorf("sanitized output lost %q: %s", want, s)
		}
	}
	for _, absent := range []string{"<?xml", "script", "onload"} {
		if strings.Contains(s, absent) {
			t.Errorf("sanitized output still contains %q: %s", absent, s)
		}
	}
}

func TestValidateGroupPNG(t *testing.T) {
	// A real, small PNG passes.
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 24, 24))); err != nil {
		t.Fatalf("encode fixture: %v", err)
	}
	if err := validateGroupPNG(buf.Bytes()); err != nil {
		t.Fatalf("a 24x24 PNG must pass: %v", err)
	}

	// Not a PNG.
	if err := validateGroupPNG([]byte("GIF89a.....")); err == nil {
		t.Error("a GIF must be refused by the PNG validator")
	}

	// Over the dimension ceiling.
	big := image.NewRGBA(image.Rect(0, 0, maxGroupIconDimension+1, 10))
	buf.Reset()
	if err := png.Encode(&buf, big); err != nil {
		t.Fatalf("encode big fixture: %v", err)
	}
	if err := validateGroupPNG(buf.Bytes()); err == nil {
		t.Error("a PNG over 512px must be refused")
	}

	// The magic-byte sniff routes each format to its validator.
	if kind, _ := detectGroupIcon(buf.Bytes()); kind != "png" {
		t.Errorf("detectGroupIcon(big png) = %q, want png", kind)
	}
	if kind, _ := detectGroupIcon([]byte("<svg xmlns='x'/>")); kind != "svg" {
		t.Errorf("detectGroupIcon(svg) = %q, want svg", kind)
	}
	if _, err := detectGroupIcon([]byte("hello world")); err == nil {
		t.Error("random bytes must be refused")
	}
	// A colored pixel also exercises the decoder, not just the header parse.
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.RGBA{255, 0, 0, 255})
	buf.Reset()
	_ = png.Encode(&buf, img)
	if err := validateGroupPNG(buf.Bytes()); err != nil {
		t.Fatalf("2x2 PNG must pass: %v", err)
	}
}
