package plugin

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"time"
)

// TrustedPublisher is a signing key the instance accepts. Fingerprint is the
// identity; Name is what the admin sees in place of the hash.
type TrustedPublisher struct {
	Fingerprint  string
	Name         string
	PublicKeyPEM string
	AddedAt      time.Time
	// Seeded marks the key injected from EVEY_PLUGINS_COSIGN_PUBLIC_KEY at boot:
	// the configuration owns it, the panel cannot remove it.
	Seeded bool
}

// ErrSeededPublisher is returned when removing a seeded trusted publisher.
var ErrSeededPublisher = errors.New("seeded trusted publisher cannot be removed")

// ParsePublisherKey validates a PEM public key (ECDSA P-256 only, cosign's
// default - anything else is refused, not guessed) and returns its fingerprint
// ("sha256:<hex>" over the DER SubjectPublicKeyInfo) with the PEM re-encoded
// canonically, so two spellings of one key store identically.
func ParsePublisherKey(data []byte) (fingerprint, canonicalPEM string, err error) {
	block, _ := pem.Decode(bytes.TrimSpace(data))
	if block == nil || block.Type != "PUBLIC KEY" {
		return "", "", errors.New("public key: expected a PEM PUBLIC KEY block")
	}
	parsed, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return "", "", fmt.Errorf("parse public key: %w", err)
	}
	pub, ok := parsed.(*ecdsa.PublicKey)
	if !ok || pub.Curve != elliptic.P256() {
		return "", "", errors.New("public key: only ECDSA P-256 (cosign's default) is accepted")
	}
	der, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		return "", "", fmt.Errorf("marshal public key: %w", err)
	}
	sum := sha256.Sum256(der)
	return "sha256:" + hex.EncodeToString(sum[:]),
		string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der})), nil
}
