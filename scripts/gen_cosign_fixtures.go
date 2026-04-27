//go:build ignore

// gen_cosign_fixtures generates test plugin zips and cosign-compatible
// signatures for manual verification testing.
//
// Usage:
//
//	go run scripts/gen_cosign_fixtures.go
//
// Outputs in tests/plugins/cosign/:
//
//	test.pub              — PKIX PEM public key (set EVEY_PLUGINS_COSIGN_PUBLIC_KEY to this path)
//	test.key              — EC PEM private key  (keep secret; use with `cosign sign-blob --key`)
//	plugin-valid.zip      — valid plugin, correctly signed
//	plugin-valid.zip.sig  — valid signature
//	plugin-tampered.zip      — zip whose content was modified after signing
//	plugin-tampered.zip.sig  — original (now invalid) signature
//	plugin-wrongkey.zip      — valid plugin signed with a different key
//	plugin-wrongkey.zip.sig  — signature from the other key (won't verify against test.pub)
package main

import (
	"archive/zip"
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"
)

const outDir = "tests/plugins/cosign"

func main() {
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		fatal("mkdir", err)
	}

	// Primary key pair
	priv, pub := mustKeyPair()
	writeKey(priv, pub)

	// Second key pair for wrong-key test
	priv2, _ := mustKeyPair()

	// Build zips and sign them
	validZip := buildPlugin("plugin-cosign-valid", "1.0.0")
	writeFile("plugin-valid.zip", validZip)
	writeFile("plugin-valid.zip.sig", sign(priv, validZip))

	// Tampered: sign first, then modify content
	originalZip := buildPlugin("plugin-cosign-tampered", "1.0.0")
	originalSig := sign(priv, originalZip)
	tamperedZip := buildPlugin("plugin-cosign-tampered", "1.0.1") // different version = different bytes
	writeFile("plugin-tampered.zip", tamperedZip)
	writeFile("plugin-tampered.zip.sig", originalSig) // sig for original, not tampered

	// Wrong key: valid zip, signed with priv2 instead of priv
	wrongKeyZip := buildPlugin("plugin-cosign-wrongkey", "1.0.0")
	writeFile("plugin-wrongkey.zip", wrongKeyZip)
	writeFile("plugin-wrongkey.zip.sig", sign(priv2, wrongKeyZip))

	fmt.Println("Done. Output in", outDir)
	fmt.Println()
	fmt.Println("To test:")
	fmt.Printf("  export EVEY_PLUGINS_COSIGN_POLICY=enforce\n")
	fmt.Printf("  export EVEY_PLUGINS_COSIGN_PUBLIC_KEY=$(pwd)/%s/test.pub\n", outDir)
	fmt.Println()
	fmt.Println("  # Should pass:")
	fmt.Printf("  cosign verify-blob --key %s/test.pub --signature %s/plugin-valid.zip.sig --insecure-ignore-tlog %s/plugin-valid.zip\n", outDir, outDir, outDir)
	fmt.Println()
	fmt.Println("  # Should fail (tampered content):")
	fmt.Printf("  cosign verify-blob --key %s/test.pub --signature %s/plugin-tampered.zip.sig --insecure-ignore-tlog %s/plugin-tampered.zip\n", outDir, outDir, outDir)
	fmt.Println()
	fmt.Println("  # Should fail (wrong key):")
	fmt.Printf("  cosign verify-blob --key %s/test.pub --signature %s/plugin-wrongkey.zip.sig --insecure-ignore-tlog %s/plugin-wrongkey.zip\n", outDir, outDir, outDir)
}

func mustKeyPair() (*ecdsa.PrivateKey, *ecdsa.PublicKey) {
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		fatal("generate key", err)
	}
	return priv, &priv.PublicKey
}

func writeKey(priv *ecdsa.PrivateKey, pub *ecdsa.PublicKey) {
	privDER, err := x509.MarshalECPrivateKey(priv)
	if err != nil {
		fatal("marshal private key", err)
	}
	privPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: privDER})
	writeFile("test.key", privPEM)

	pubDER, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		fatal("marshal public key", err)
	}
	pubPEM := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubDER})
	writeFile("test.pub", pubPEM)
}

// sign produces a base64-encoded ASN1 ECDSA signature over SHA256(data),
// which is exactly what `cosign verify-blob --key <PEM>` expects.
func sign(priv *ecdsa.PrivateKey, data []byte) []byte {
	digest := sha256.Sum256(data)
	sig, err := ecdsa.SignASN1(rand.Reader, priv, digest[:])
	if err != nil {
		fatal("sign", err)
	}
	out := make([]byte, base64.StdEncoding.EncodedLen(len(sig)))
	base64.StdEncoding.Encode(out, sig)
	return out
}

// buildPlugin creates a minimal valid plugin zip in memory.
func buildPlugin(name, version string) []byte {
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)

	manifest := fmt.Sprintf("name: %s\nversion: %s\nlanguage: python\nentrypoint: main.py\n", name, version)
	addFile(w, "plugin.yaml", []byte(manifest))
	addFile(w, "main.py", []byte("# evey plugin stub\ndef run(ctx): pass\n"))

	if err := w.Close(); err != nil {
		fatal("close zip", err)
	}
	return buf.Bytes()
}

func addFile(w *zip.Writer, name string, data []byte) {
	f, err := w.Create(name)
	if err != nil {
		fatal("zip create "+name, err)
	}
	if _, err := f.Write(data); err != nil {
		fatal("zip write "+name, err)
	}
}

func writeFile(name string, data []byte) {
	path := filepath.Join(outDir, name)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		fatal("write "+path, err)
	}
	fmt.Printf("  wrote %s (%d bytes)\n", path, len(data))
}

func fatal(msg string, err error) {
	fmt.Fprintf(os.Stderr, "error: %s: %v\n", msg, err)
	os.Exit(1)
}
