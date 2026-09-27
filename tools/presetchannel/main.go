// presetchannel builds the signed preset channel: it validates every policy
// file, hashes each one, builds manifest.json, signs the manifest with the
// channel's ed25519 key, and writes a deployable directory tree.
//
// The release workflow runs this with PRESET_SIGNING_KEY set; locally, `verify`
// mode checks an existing manifest without any key.
//
// Usage:
//
//	presetchannel -in presets -out channel build   (requires PRESET_SIGNING_KEY)
//	presetchannel -in channel verify              (keyless: checks hashes)
package main

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"hyperdns/presets"
)

// manifest mirrors presetupd.Manifest — the daemon parses that shape, so keep
// the two in sync.
type manifest struct {
	Schema      int             `json:"schema"`
	GeneratedAt string          `json:"generated_at"`
	BaseURL     string          `json:"base_url"`
	Signature   string          `json:"signature"`
	Policies    []manifestEntry `json:"policies"`
}

type manifestEntry struct {
	ID        string `json:"id"`
	Version   int    `json:"version"`
	SHA256    string `json:"sha256"`
	UpdatedAt string `json:"updated_at"`
}

// signPayload must byte-match presetupd's — both derive it from generated_at
// plus one id:version:hash line per policy, sorted by ID.
func signPayload(m *manifest) []byte {
	lines := []string{m.GeneratedAt}
	sorted := append([]manifestEntry(nil), m.Policies...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].ID < sorted[j].ID })
	for _, e := range sorted {
		lines = append(lines, fmt.Sprintf("%s:%d:%s", e.ID, e.Version, e.SHA256))
	}
	return []byte(strings.Join(lines, "\n"))
}

func main() {
	in := flag.String("in", "presets", "input directory")
	out := flag.String("out", "channel", "output directory for build mode")
	flag.Parse()
	cmd := flag.Arg(0)

	switch cmd {
	case "build":
		build(*in, *out)
	case "verify":
		verify(*in)
	case "validate":
		validate(*in)
	default:
		fmt.Fprintln(os.Stderr, "usage: presetchannel [-in dir] [-out dir] build|verify")
		os.Exit(2)
	}
}

func build(in, out string) {
	b64 := os.Getenv("PRESET_SIGNING_KEY")
	if b64 == "" {
		fmt.Fprintln(os.Stderr, "PRESET_SIGNING_KEY is not set — refusing to publish an unsigned channel")
		os.Exit(1)
	}
	raw, err := base64.StdEncoding.DecodeString(b64)
	if err != nil || len(raw) != ed25519.PrivateKeySize {
		fmt.Fprintln(os.Stderr, "PRESET_SIGNING_KEY is not a valid ed25519 private key")
		os.Exit(1)
	}
	priv := ed25519.PrivateKey(raw)

	// Load the canonical files with the same loader the daemon embeds, so what
	// is published cannot diverge from what the binary compiled.
	all := presets.All()
	if len(all) == 0 {
		fmt.Fprintln(os.Stderr, "no policy files found")
		os.Exit(1)
	}

	if err := os.MkdirAll(out, 0o755); err != nil {
		panic(err)
	}
	entries := make([]manifestEntry, 0, len(all))
	for _, p := range all {
		data, err := json.MarshalIndent(p, "", "  ")
		if err != nil {
			panic(err)
		}
		data = append(data, '\n')
		sum := sha256.Sum256(data)
		entries = append(entries, manifestEntry{
			ID: p.ID, Version: p.Version, SHA256: hex.EncodeToString(sum[:]),
			UpdatedAt: p.UpdatedAt,
		})
		if err := os.WriteFile(filepath.Join(out, p.ID+".json"), data, 0o644); err != nil {
			panic(err)
		}
	}

	m := &manifest{
		Schema:      1,
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		BaseURL:     os.Getenv("PRESET_BASE_URL"),
		Policies:    entries,
	}
	m.Signature = "ed25519:" + base64.StdEncoding.EncodeToString(ed25519.Sign(priv, signPayload(m)))
	mdata, _ := json.MarshalIndent(m, "", "  ")
	if err := os.WriteFile(filepath.Join(out, "manifest.json"), append(mdata, '\n'), 0o644); err != nil {
		panic(err)
	}

	// Copy icons through unchanged when present.
	if icons, err := os.ReadDir(filepath.Join(in, "icons")); err == nil {
		_ = os.MkdirAll(filepath.Join(out, "icons"), 0o755)
		for _, f := range icons {
			raw, err := os.ReadFile(filepath.Join(in, "icons", f.Name()))
			if err != nil {
				continue
			}
			_ = os.WriteFile(filepath.Join(out, "icons", f.Name()), raw, 0o644)
		}
	}

	fmt.Printf("channel built: %d policies signed into %s\n", len(entries), out)
}

func verify(dir string) {
	raw, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		panic(err)
	}
	var m manifest
	if err := json.Unmarshal(raw, &m); err != nil {
		panic(err)
	}
	bad := 0
	for _, e := range m.Policies {
		fileRaw, err := os.ReadFile(filepath.Join(dir, e.ID+".json"))
		if err != nil {
			fmt.Printf("MISSING %s.json\n", e.ID)
			bad++
			continue
		}
		sum := sha256.Sum256(fileRaw)
		if hex.EncodeToString(sum[:]) != e.SHA256 {
			fmt.Printf("HASH MISMATCH %s.json\n", e.ID)
			bad++
		}
	}
	if bad != 0 {
		fmt.Printf("verify FAILED: %d problem(s)\n", bad)
		os.Exit(1)
	}
	fmt.Printf("verify OK: %d policies, hashes intact\n", len(m.Policies))
}

// validate is the PR gate: every policy file must parse, carry a coherent
// identity, and list well-formed, non-duplicated domains. Cross-file overlap is
// reported as a NOTE — the matcher handles co-owned domains on purpose — and is
// not a failure.
func validate(dir string) {
	claims := map[string]string{} // domain -> first policy id that claims it
	keys := map[string]string{}   // toggle key -> first policy id that uses it
	orders := map[int]string{}    // sort_order -> first policy id that holds it
	referencedIcons := map[string]bool{}
	problems := 0
	files, err := os.ReadDir(dir)
	if err != nil {
		panic(err)
	}
	count := 0
	for _, f := range files {
		if f.IsDir() || !strings.HasSuffix(f.Name(), ".json") || f.Name() == "manifest.json" {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(dir, f.Name()))
		if err != nil {
			panic(err)
		}
		var p struct {
			ID        string   `json:"id"`
			Name      string   `json:"name"`
			Key       string   `json:"key"`
			Category  string   `json:"category"`
			Kind      string   `json:"kind"`
			Icon      string   `json:"icon"`
			SortOrder int      `json:"sort_order"`
			Version   int      `json:"version"`
			UpdatedAt string   `json:"updated_at"`
			Domains   []string `json:"domains"`
		}
		if err := json.Unmarshal(raw, &p); err != nil {
			fmt.Printf("PARSE ERROR %s: %v\n", f.Name(), err)
			problems++
			continue
		}
		count++
		if p.ID+".json" != f.Name() {
			fmt.Printf("ID MISMATCH %s: id=%q\n", f.Name(), p.ID)
			problems++
		}
		if p.Name == "" || p.Version < 1 {
			fmt.Printf("INCOMPLETE %s: name=%q version=%d\n", f.Name(), p.Name, p.Version)
			problems++
		}
		if !keyRe.MatchString(p.Key) {
			fmt.Printf("BAD KEY %s: %q (want enable_<name>)\n", f.Name(), p.Key)
			problems++
		} else if owner, ok := keys[p.Key]; ok && owner != p.ID {
			fmt.Printf("DUPLICATE KEY %s and %s both use %s\n", owner, p.ID, p.Key)
			problems++
		} else {
			keys[p.Key] = p.ID
		}
		if p.Category == "" {
			fmt.Printf("NO CATEGORY %s\n", f.Name())
			problems++
		}
		if p.Kind != "proxy" && p.Kind != "block" && p.Kind != "veto" {
			fmt.Printf("BAD KIND %s: %q (proxy|block|veto)\n", f.Name(), p.Kind)
			problems++
		}
		if p.SortOrder < 0 {
			fmt.Printf("BAD SORT_ORDER %s: %d\n", f.Name(), p.SortOrder)
			problems++
		} else if owner, ok := orders[p.SortOrder]; ok && owner != p.ID {
			fmt.Printf("NOTE %s and %s share sort_order %d (display order becomes ambiguous)\n", owner, p.ID, p.SortOrder)
		} else {
			orders[p.SortOrder] = p.ID
		}
		if p.UpdatedAt == "" {
			fmt.Printf("NO UPDATED_AT %s\n", f.Name())
			problems++
		} else if _, err := time.Parse(time.RFC3339, p.UpdatedAt); err != nil {
			fmt.Printf("BAD UPDATED_AT %s: %q is not RFC 3339 (e.g. 2026-09-26T12:00:00Z)\n", f.Name(), p.UpdatedAt)
			problems++
		}
		// The icon must follow the generator's convention AND exist beside the
		// policy files — a preset whose icon ships later renders with the
		// Feather fallback, which is fine, but a hand-made preset pushed without
		// its file is always an oversight, never a choice.
		if p.Icon != "icons/"+p.ID+".svg" {
			fmt.Printf("BAD ICON FIELD %s: %q (want icons/%s.svg)\n", f.Name(), p.Icon, p.ID)
			problems++
		} else if iconPath := filepath.Join(dir, "icons", p.ID+".svg"); iconExists(iconPath) {
			referencedIcons[p.ID+".svg"] = true
		} else {
			fmt.Printf("MISSING ICON %s: icons/%s.svg not found in %s\n", f.Name(), p.ID, dir)
			problems++
		}
		if len(p.Domains) == 0 {
			fmt.Printf("NO DOMAINS %s\n", f.Name())
			problems++
		}
		seen := map[string]bool{}
		for _, d := range p.Domains {
			if !domainRe.MatchString(d) {
				fmt.Printf("BAD DOMAIN %s: %q\n", f.Name(), d)
				problems++
			}
			if seen[d] {
				fmt.Printf("DUPLICATE IN FILE %s: %q\n", f.Name(), d)
				problems++
			}
			seen[d] = true
			if owner, ok := claims[d]; ok && owner != p.ID {
				fmt.Printf("NOTE %s and %s both claim %s\n", owner, p.ID, d)
			} else {
				claims[d] = p.ID
			}
		}
	}
	// Icons nobody references: harmless, but usually a rename that forgot a file.
	if iconDir, err := os.ReadDir(filepath.Join(dir, "icons")); err == nil {
		for _, f := range iconDir {
			if f.IsDir() || !referencedIcons[f.Name()] {
				fmt.Printf("NOTE unreferenced icon file: icons/%s\n", f.Name())
			}
		}
	}
	if problems != 0 {
		fmt.Printf("validate FAILED: %d problem(s) across %d files\n", problems, count)
		os.Exit(1)
	}
	fmt.Printf("validate OK: %d policies, %d domain claims, %d icons\n", count, len(claims), len(referencedIcons))
}

func iconExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// keyRe pins the toggle-key convention the dashboard and config keys share.
var keyRe = regexp.MustCompile(`^enable_[a-z0-9_]+$`)

// domainRe matches a hostname entry: lowercase labels, an optional leading *.,
// at least one dot. It is deliberately strict — the harvest automation writes
// these files, and a malformed entry would silently match nothing.
var domainRe = regexp.MustCompile(`^(\*\.)?[a-z0-9]([a-z0-9-]*[a-z0-9])?(\.[a-z0-9]([a-z0-9-]*[a-z0-9])?)+$`)
