package pluginrunner

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// cosignFixtureDir resolves the repo-root tests/plugins/cosign directory from
// this package's dir (go test runs with CWD = package dir). Skips the test if
// the fixtures are absent (e.g. a sparse checkout).
func cosignFixtureDir(t *testing.T) string {
	t.Helper()
	dir := filepath.Join("..", "..", "..", "..", "tests", "plugins", "cosign")
	if _, err := os.Stat(filepath.Join(dir, "test.pub")); err != nil {
		t.Skipf("cosign fixtures not found at %s: %v", dir, err)
	}
	return dir
}

// requireCosign skips the test when the cosign binary is not on $PATH (dev hosts
// without it). The official image ships cosign, so CI exercises these paths.
func requireCosign(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("cosign"); err != nil {
		t.Skip("cosign binary not in $PATH")
	}
}

// These two need neither cosign nor fixtures - they pin the no-op contract.
func TestCosignVerifier_PolicyOff_IsNoop(t *testing.T) {
	st, err := NewCosignVerifier("off", "/some/key.pub").VerifySignature(context.Background(), "/x.zip", "")
	require.NoError(t, err)
	assert.Nil(t, st, "policy=off must skip verification entirely")
}

// warn without a key is best-effort: nothing to verify against, so it degrades
// to a silent no-op rather than blocking installs.
func TestCosignVerifier_Warn_EmptyKey_IsNoop(t *testing.T) {
	st, err := NewCosignVerifier("warn", "").VerifySignature(context.Background(), "/x.zip", "")
	require.NoError(t, err)
	assert.Nil(t, st, "warn + empty key path skips verification (best-effort)")
}

// enforce without a key must FAIL CLOSED, not silently pass: an admin who asks
// for mandatory signatures but forgets the key must never have every plugin
// install quietly accepted (config validation also refuses to start in this
// state; this is the defense-in-depth backstop at the verifier).
func TestCosignVerifier_Enforce_EmptyKey_FailsClosed(t *testing.T) {
	st, err := NewCosignVerifier("enforce", "").VerifySignature(context.Background(), "/x.zip", "")
	require.Error(t, err, "enforce + empty key must block (fail-closed), never silently pass")
	require.NotNil(t, st)
	assert.False(t, st.Verified)
}

// Fail-closed when the cosign binary is missing under enforce - no binary needed
// (we force "not found" by emptying PATH), so this always runs.
func TestCosignVerifier_Enforce_MissingBinary_FailsClosed(t *testing.T) {
	dir := cosignFixtureDir(t)
	t.Setenv("PATH", "")
	v := NewCosignVerifier("enforce", filepath.Join(dir, "test.pub"))
	st, err := v.VerifySignature(context.Background(), filepath.Join(dir, "plugin-valid.zip"), "")
	require.Error(t, err, "enforce + missing cosign must block (fail-closed)")
	require.NotNil(t, st)
	assert.False(t, st.Verified)
}

// warn never blocks, even when the binary is missing.
func TestCosignVerifier_Warn_MissingBinary_DoesNotBlock(t *testing.T) {
	dir := cosignFixtureDir(t)
	t.Setenv("PATH", "")
	v := NewCosignVerifier("warn", filepath.Join(dir, "test.pub"))
	st, err := v.VerifySignature(context.Background(), filepath.Join(dir, "plugin-valid.zip"), "")
	require.NoError(t, err, "warn must never return a blocking error")
	require.NotNil(t, st)
	assert.False(t, st.Verified)
}

// The real verification paths against the committed fixtures (need cosign).
func TestCosignVerifier_Enforce_ValidSignature_Accepted(t *testing.T) {
	requireCosign(t)
	dir := cosignFixtureDir(t)
	v := NewCosignVerifier("enforce", filepath.Join(dir, "test.pub"))
	st, err := v.VerifySignature(context.Background(), filepath.Join(dir, "plugin-valid.zip"), "")
	require.NoError(t, err)
	require.NotNil(t, st)
	assert.True(t, st.Verified, "a validly-signed plugin must verify")
}

func TestCosignVerifier_Enforce_TamperedZip_Rejected(t *testing.T) {
	requireCosign(t)
	dir := cosignFixtureDir(t)
	v := NewCosignVerifier("enforce", filepath.Join(dir, "test.pub"))
	_, err := v.VerifySignature(context.Background(), filepath.Join(dir, "plugin-tampered.zip"), "")
	require.Error(t, err, "a ZIP modified after signing must be rejected")
}

func TestCosignVerifier_Enforce_WrongKey_Rejected(t *testing.T) {
	requireCosign(t)
	dir := cosignFixtureDir(t)
	v := NewCosignVerifier("enforce", filepath.Join(dir, "test.pub"))
	_, err := v.VerifySignature(context.Background(), filepath.Join(dir, "plugin-wrongkey.zip"), "")
	require.Error(t, err, "a signature from an untrusted key must be rejected")
}

// warn surfaces a failed verification (status recorded) without blocking.
func TestCosignVerifier_Warn_TamperedZip_RecordsButAllows(t *testing.T) {
	requireCosign(t)
	dir := cosignFixtureDir(t)
	v := NewCosignVerifier("warn", filepath.Join(dir, "test.pub"))
	st, err := v.VerifySignature(context.Background(), filepath.Join(dir, "plugin-tampered.zip"), "")
	require.NoError(t, err)
	require.NotNil(t, st)
	assert.False(t, st.Verified)
	assert.NotEmpty(t, st.Error)
}
