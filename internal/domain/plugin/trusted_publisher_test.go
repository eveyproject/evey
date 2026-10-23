package plugin

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func pemOf(t *testing.T, pub any) []byte {
	t.Helper()
	der, err := x509.MarshalPKIXPublicKey(pub)
	require.NoError(t, err)
	return pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der})
}

func TestParsePublisherKey(t *testing.T) {
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	raw := pemOf(t, &priv.PublicKey)

	fp, canonical, err := ParsePublisherKey(raw)
	require.NoError(t, err)
	assert.Regexp(t, `^sha256:[0-9a-f]{64}$`, fp)
	assert.Equal(t, string(raw), canonical)

	// Whitespace around the block does not change the identity.
	fp2, canonical2, err := ParsePublisherKey(append(append([]byte("\n  "), raw...), "\n\n"...))
	require.NoError(t, err)
	assert.Equal(t, fp, fp2)
	assert.Equal(t, canonical, canonical2)

	other, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	fp3, _, err := ParsePublisherKey(pemOf(t, &other.PublicKey))
	require.NoError(t, err)
	assert.NotEqual(t, fp, fp3)
}

func TestParsePublisherKey_RefusesWhatIsNotP256(t *testing.T) {
	p384, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	require.NoError(t, err)
	rsaKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	for name, data := range map[string][]byte{
		"p384":     pemOf(t, &p384.PublicKey),
		"rsa":      pemOf(t, &rsaKey.PublicKey),
		"garbage":  []byte("hello"),
		"empty":    nil,
		"wrongTag": pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: []byte{1}}),
		"badDER":   pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: []byte{1, 2, 3}}),
	} {
		_, _, err := ParsePublisherKey(data)
		assert.Error(t, err, name)
	}
}

func TestSignatureStatus_ContinuityAndState(t *testing.T) {
	signed := func(fp string, trusted bool) *SignatureStatus {
		return &SignatureStatus{Verified: true, KeyFingerprint: fp, Trusted: trusted}
	}

	// No pin: never a continuity, whatever the candidate is.
	s := signed("sha256:a", false)
	s.ApplyContinuity("")
	assert.Empty(t, s.Continuity)

	s = signed("sha256:a", true)
	s.ApplyContinuity("sha256:a")
	assert.Equal(t, ContinuitySame, s.Continuity)
	assert.Equal(t, SigStateSigned, s.State())

	s = signed("sha256:b", true)
	s.ApplyContinuity("sha256:a")
	assert.Equal(t, ContinuityChanged, s.Continuity)
	assert.Equal(t, SigStatePublisherChanged, s.State(), "a changed key outranks a trusted one")

	for _, lost := range []*SignatureStatus{
		{Error: SigErrNoKey},
		{Error: SigErrNoSignature, KeyFingerprint: "sha256:a"},
		{Error: "signature does not match", KeyFingerprint: "sha256:a"},
	} {
		lost.ApplyContinuity("sha256:a")
		assert.Equal(t, ContinuityLost, lost.Continuity)
		assert.Equal(t, SigStateSignatureLost, lost.State())
	}

	var nilStatus *SignatureStatus
	nilStatus.ApplyContinuity("sha256:a")
	assert.Empty(t, nilStatus.State())
}

func TestSignatureStatus_StateWithoutContinuity(t *testing.T) {
	assert.Equal(t, SigStateSigned, (&SignatureStatus{Verified: true, Trusted: true}).State())
	assert.Equal(t, SigStateSignedUntrusted, (&SignatureStatus{Verified: true}).State())
	assert.Equal(t, SigStateUnsigned, (&SignatureStatus{Error: SigErrNoKey}).State())
	assert.Equal(t, SigStateUnsigned, (&SignatureStatus{Error: SigErrNoSignature}).State())
	assert.Equal(t, SigStateInvalid, (&SignatureStatus{Error: "bad"}).State())
	assert.True(t, (&SignatureStatus{Error: SigErrNoKey}).Unsigned())
	assert.False(t, (&SignatureStatus{Verified: true}).Unsigned())
}
