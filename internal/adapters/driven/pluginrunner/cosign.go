package pluginrunner

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	domainplugin "github.com/eveyproject/evey/internal/domain/plugin"
)

// CosignVerifier checks cosign-compatible detached signatures in-process: the
// key-based flow (`cosign sign-blob --key`) is an ECDSA P-256 signature over
// the file's SHA-256, base64-encoded. No transparency log, no binary.
type CosignVerifier struct {
	policy  string // "off", "warn", "enforce"
	keyPath string // path to PEM public key
}

// NewCosignVerifier returns a CosignVerifier. policy must be "off", "warn",
// or "enforce". When policy is "off" or keyPath is empty, VerifySignature
// is a no-op that always returns (nil, nil).
func NewCosignVerifier(policy, keyPath string) *CosignVerifier {
	return &CosignVerifier{policy: policy, keyPath: keyPath}
}

// VerifySignature checks the cosign signature of the ZIP at zipPath.
// sigPath is the detached .sig file (empty → zipPath+".sig" is tried).
//
// Behaviour by policy:
//   - "off": returns (nil, nil) - no-op
//   - empty keyPath under "warn": returns (nil, nil) - best-effort, can't verify
//   - empty keyPath under "enforce": fails closed (config validation already
//     refuses to start in this state; this is defense-in-depth so a hand-built
//     enforce verifier without a key can never silently pass a plugin)
//   - "warn": returns (status, nil) even on failure - caller logs
//   - "enforce": returns (status, error) when verification fails - caller blocks
func (v *CosignVerifier) VerifySignature(_ context.Context, zipPath, sigPath string) (*domainplugin.SignatureStatus, error) {
	if v.policy == "off" {
		return nil, nil
	}
	if v.keyPath == "" {
		// enforce without a key = the admin asked for mandatory signatures but
		// configured nothing to verify against. Never return a silent pass here:
		// that would defeat enforcement entirely. warn stays a no-op (best-effort).
		if v.policy == "enforce" {
			return &domainplugin.SignatureStatus{
					Verified: false,
					Error:    "cosign enforce policy is set but no public key is configured",
				},
				fmt.Errorf("plugin signature verification failed: enforce policy requires a public key but none is configured")
		}
		return nil, nil
	}
	if sigPath == "" {
		sigPath = zipPath + ".sig"
	}

	if err := verifyBlobSignature(v.keyPath, sigPath, zipPath); err != nil {
		status := &domainplugin.SignatureStatus{Verified: false, Error: err.Error()}
		if v.policy == "enforce" {
			return status, fmt.Errorf("plugin signature verification failed: %w", err)
		}
		return status, nil
	}

	return &domainplugin.SignatureStatus{
		Verified:   true,
		VerifiedBy: v.keyPath,
	}, nil
}

// verifyBlobSignature is `cosign verify-blob --key --insecure-ignore-tlog`
// without the process: PKIX PEM key, P-256 only (anything else is refused, not
// guessed), base64 DER signature over SHA-256 of the blob.
func verifyBlobSignature(keyPath, sigPath, blobPath string) error {
	pub, err := loadP256PublicKey(keyPath)
	if err != nil {
		return err
	}
	rawSig, err := os.ReadFile(sigPath)
	if err != nil {
		return fmt.Errorf("read signature: %w", err)
	}
	sig, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(rawSig)))
	if err != nil {
		return fmt.Errorf("decode signature: %w", err)
	}
	f, err := os.Open(blobPath)
	if err != nil {
		return fmt.Errorf("open plugin: %w", err)
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return fmt.Errorf("hash plugin: %w", err)
	}
	if !ecdsa.VerifyASN1(pub, h.Sum(nil), sig) {
		return errors.New("signature does not match the plugin or the configured key")
	}
	return nil
}

func loadP256PublicKey(path string) (*ecdsa.PublicKey, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read public key: %w", err)
	}
	block, _ := pem.Decode(data)
	if block == nil || block.Type != "PUBLIC KEY" {
		return nil, errors.New("public key: expected a PEM PUBLIC KEY block")
	}
	parsed, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse public key: %w", err)
	}
	pub, ok := parsed.(*ecdsa.PublicKey)
	if !ok || pub.Curve != elliptic.P256() {
		return nil, errors.New("public key: only ECDSA P-256 (cosign's default) is accepted")
	}
	return pub, nil
}
