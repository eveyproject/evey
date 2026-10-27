package pluginrunner

import (
	"archive/zip"
	"context"
	"crypto/ecdsa"
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

// publisherKeyEntry is the ZIP-root file that carries the signing public key.
// maxPublisherKeyBytes bounds it: a PEM P-256 key is ~180 bytes.
const (
	publisherKeyEntry    = "cosign.pub"
	maxPublisherKeyBytes = 4 << 10
)

// CosignVerifier checks cosign-compatible detached signatures in-process: the
// key-based flow (`cosign sign-blob --key`) is an ECDSA P-256 signature over
// the file's SHA-256, base64-encoded. No transparency log, no binary.
//
// The verification key is NOT configured here: it travels inside the package
// (cosign.pub at the ZIP root), like an Android APK carries its signing
// certificate. The signature therefore proves "this package was signed by the
// holder of that key"; whether the instance TRUSTS that key is the lookup's
// answer, and whether it is the SAME key as the installed version's is the
// installer's continuity check.
type CosignVerifier struct {
	policy string // "off", "warn", "enforce"
	lookup domainplugin.KeyLookup
}

// NewCosignVerifier returns a CosignVerifier. policy must be "off", "warn" or
// "enforce". A nil lookup trusts no key.
func NewCosignVerifier(policy string, lookup domainplugin.KeyLookup) *CosignVerifier {
	return &CosignVerifier{policy: policy, lookup: lookup}
}

// VerifySignature checks the cosign signature of the ZIP at zipPath against the
// key the ZIP carries. sigPath is the detached .sig file (empty -> zipPath+".sig").
//
// Behaviour by policy:
//   - "off": returns (nil, nil) - no-op
//   - "warn": always returns the computed status and a nil error
//   - "enforce": returns (status, error) unless the signature verifies AND the
//     key is in the trusted-publisher list
func (v *CosignVerifier) VerifySignature(ctx context.Context, zipPath, sigPath string) (*domainplugin.SignatureStatus, error) {
	if v.policy == "off" {
		return nil, nil
	}
	if sigPath == "" {
		sigPath = zipPath + ".sig"
	}
	status := v.verify(ctx, zipPath, sigPath)
	if v.policy == "enforce" {
		switch {
		case !status.Verified:
			return status, fmt.Errorf("plugin signature verification failed: %s", status.Error)
		case !status.Trusted:
			return status, fmt.Errorf("plugin signature verification failed: signing key %s is not a trusted publisher", status.KeyFingerprint)
		}
	}
	return status, nil
}

// InspectSignature computes the status whatever the policy, never refusing:
// the installer's key-continuity guard holds even with verification off.
func (v *CosignVerifier) InspectSignature(ctx context.Context, zipPath, sigPath string) *domainplugin.SignatureStatus {
	if sigPath == "" {
		sigPath = zipPath + ".sig"
	}
	return v.verify(ctx, zipPath, sigPath)
}

func (v *CosignVerifier) verify(ctx context.Context, zipPath, sigPath string) *domainplugin.SignatureStatus {
	keyPEM, err := readPackageKey(zipPath)
	if err != nil {
		return &domainplugin.SignatureStatus{Error: err.Error()}
	}
	if keyPEM == nil {
		return &domainplugin.SignatureStatus{Error: domainplugin.SigErrNoKey}
	}
	fingerprint, canonical, err := domainplugin.ParsePublisherKey(keyPEM)
	if err != nil {
		return &domainplugin.SignatureStatus{Error: err.Error()}
	}
	status := &domainplugin.SignatureStatus{KeyFingerprint: fingerprint, KeyPEM: canonical}

	sig, err := os.ReadFile(sigPath)
	if errors.Is(err, os.ErrNotExist) {
		status.Error = domainplugin.SigErrNoSignature
		return status
	}
	if err != nil {
		status.Error = fmt.Sprintf("read signature: %v", err)
		return status
	}
	if err := verifyBlobSignature(canonical, sig, zipPath); err != nil {
		status.Error = err.Error()
		return status
	}
	status.Verified = true
	status.VerifiedBy = fingerprint
	// Trust only follows a signature that verified: a forged package must not
	// borrow a trusted publisher's name by embedding its public key.
	if v.lookup != nil {
		status.Publisher, status.Trusted = v.lookup(ctx, fingerprint)
	}
	return status
}

// readPackageKey returns the root cosign.pub of the ZIP, or nil when absent.
// Only a root regular entry counts: a nested cosign.pub is plugin data, not
// the package's signing key.
func readPackageKey(zipPath string) ([]byte, error) {
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		return nil, fmt.Errorf("open plugin: %w", err)
	}
	defer zr.Close() //nolint:errcheck // read-only handle
	for _, f := range zr.File {
		if f.Name != publisherKeyEntry || f.FileInfo().IsDir() {
			continue
		}
		if f.UncompressedSize64 > maxPublisherKeyBytes {
			return nil, errors.New("signing key is larger than 4 KiB")
		}
		rc, err := f.Open()
		if err != nil {
			return nil, fmt.Errorf("read signing key: %w", err)
		}
		defer rc.Close()
		data, err := io.ReadAll(io.LimitReader(rc, maxPublisherKeyBytes+1))
		if err != nil {
			return nil, fmt.Errorf("read signing key: %w", err)
		}
		if len(data) > maxPublisherKeyBytes {
			return nil, errors.New("signing key is larger than 4 KiB")
		}
		return data, nil
	}
	return nil, nil
}

// verifyBlobSignature is `cosign verify-blob --key --insecure-ignore-tlog`
// without the process: PKIX PEM key (already validated P-256), base64 DER
// signature over SHA-256 of the blob.
func verifyBlobSignature(keyPEM string, rawSig []byte, blobPath string) error {
	block, _ := pem.Decode([]byte(keyPEM))
	if block == nil {
		return errors.New("public key: expected a PEM PUBLIC KEY block")
	}
	parsed, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return fmt.Errorf("parse public key: %w", err)
	}
	pub, ok := parsed.(*ecdsa.PublicKey)
	if !ok {
		return errors.New("public key: only ECDSA P-256 (cosign's default) is accepted")
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
		return errors.New("signature does not match the package and its signing key")
	}
	return nil
}
