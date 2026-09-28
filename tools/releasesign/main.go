// tools/releasesign signs (or verifies) the release checksums manifest with the
// ed25519 keypair whose public half is pinned in internal/selfupdate
// (ReleasePubKey). It exists for the release workflow only: nothing in the
// daemon or the installer imports it, and the private key never enters the
// repository — RELEASE_SIGNING_KEY comes from the Actions secret as base64 of
// the raw 32-byte ed25519 key.
//
// The signature is detached: it covers exactly the bytes of checksums.txt as
// published. The release workflow raw-base64's it into a file, and the daemon
// reads the same bytes from the asset response's X-HyperDNS-Checksums-Signature
// header. Signing a re-serialisation instead of the published bytes would break
// this, so nothing here may reformat the input.
//
//	sign  -in checksums.txt -header out.sig   # writes the raw base64 signature
//	verify -in checksums.txt -header out.sig   # re-verifies a produced file
package main

import (
	"bufio"
	"crypto/ed25519"
	"encoding/base64"
	"flag"
	"fmt"
	"os"
	"strings"

	"hyperdns/internal/selfupdate"
)

func main() {
	in := flag.String("in", "dist/checksums.txt", "the checksums manifest")
	headerFile := flag.String("header", "dist/checksums.sig", "the detached signature file")
	flag.Parse()
	cmd := flag.Arg(0)

	raw, err := os.ReadFile(*in)
	if err != nil {
		fmt.Fprintf(os.Stderr, "read manifest: %v\n", err)
		os.Exit(1)
	}

	switch cmd {
	case "sign":
		b64 := os.Getenv("RELEASE_SIGNING_KEY")
		if b64 == "" {
			fmt.Fprintln(os.Stderr, "RELEASE_SIGNING_KEY is not set — refusing to sign")
			os.Exit(1)
		}
		key, err := decodeKey(b64)
		if err != nil {
			fmt.Fprintf(os.Stderr, "RELEASE_SIGNING_KEY is not a valid ed25519 private key: %v\n", err)
			os.Exit(1)
		}
		sig := base64.StdEncoding.EncodeToString(ed25519.Sign(key, raw))
		if err := os.WriteFile(*headerFile, []byte(sig), 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "write signature: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("signed %s (%d bytes) -> %s\n", *in, len(raw), *headerFile)

	case "verify":
		sigRaw, err := os.ReadFile(*headerFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "read signature: %v\n", err)
			os.Exit(1)
		}
		// Verify through the same code path the daemon uses, so a green result
		// here is the same check the binary will perform.
		if err := selfupdate.VerifyChecksumsSignatureForTest(raw, strings.TrimSpace(string(sigRaw))); err != nil {
			fmt.Fprintf(os.Stderr, "verification FAILED: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("verification OK against the pinned release key\n")

	default:
		fmt.Fprintln(os.Stderr, "usage: releasesign [-in checksums.txt] [-header out.sig] sign|verify")
		os.Exit(2)
	}
}

func decodeKey(b64 string) (ed25519.PrivateKey, error) {
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(b64))
	if err != nil {
		return nil, err
	}
	if len(raw) != ed25519.PrivateKeySize {
		return nil, fmt.Errorf("expected %d raw bytes, got %d", ed25519.PrivateKeySize, len(raw))
	}
	return ed25519.PrivateKey(raw), nil
}

var _ = bufio.NewReader // reserved for a future streaming verify mode
