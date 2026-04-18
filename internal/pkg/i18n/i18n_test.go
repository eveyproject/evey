package i18n

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTestDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	// en
	enDir := filepath.Join(dir, "en")
	require.NoError(t, os.Mkdir(enDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(enDir, "errors.yaml"), []byte(`
auth:
  invalid_credentials: "Invalid username or password"
  password_too_short: "Password must be at least {min} characters"
general:
  not_found: "Resource not found"
`), 0o644))

	// fr
	frDir := filepath.Join(dir, "fr")
	require.NoError(t, os.Mkdir(frDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(frDir, "errors.yaml"), []byte(`
auth:
  invalid_credentials: "Nom d'utilisateur ou mot de passe invalide"
  password_too_short: "Le mot de passe doit contenir au moins {min} caractères"
general:
  not_found: "Ressource introuvable"
`), 0o644))

	return dir
}

func TestTranslate_English(t *testing.T) {
	tr, err := NewFromDir(setupTestDir(t))
	require.NoError(t, err)

	assert.Equal(t, "Invalid username or password", tr.T("en", "errors.auth.invalid_credentials"))
	assert.Equal(t, "Resource not found", tr.T("en", "errors.general.not_found"))
}

func TestTranslate_French(t *testing.T) {
	tr, err := NewFromDir(setupTestDir(t))
	require.NoError(t, err)

	assert.Equal(t, "Nom d'utilisateur ou mot de passe invalide", tr.T("fr", "errors.auth.invalid_credentials"))
	assert.Equal(t, "Ressource introuvable", tr.T("fr", "errors.general.not_found"))
}

func TestTranslate_Fallback(t *testing.T) {
	tr, err := NewFromDir(setupTestDir(t))
	require.NoError(t, err)

	// Unknown locale falls back to en
	assert.Equal(t, "Invalid username or password", tr.T("de", "errors.auth.invalid_credentials"))
	assert.Equal(t, "errors.auth.missing_key", tr.T("fr", "errors.auth.missing_key"))
}

func TestTranslate_MissingKey(t *testing.T) {
	tr, err := NewFromDir(setupTestDir(t))
	require.NoError(t, err)

	// Key doesn't exist in any locale — returns the key itself
	assert.Equal(t, "nonexistent.key", tr.T("en", "nonexistent.key"))
	assert.Equal(t, "nonexistent.key", tr.T("fr", "nonexistent.key"))
}

func TestTranslate_WithParams(t *testing.T) {
	tr, err := NewFromDir(setupTestDir(t))
	require.NoError(t, err)

	msg := tr.T("en", "errors.auth.password_too_short", map[string]string{"min": "8"})
	assert.Equal(t, "Password must be at least 8 characters", msg)

	msg = tr.T("fr", "errors.auth.password_too_short", map[string]string{"min": "8"})
	assert.Equal(t, "Le mot de passe doit contenir au moins 8 caractères", msg)
}

func TestParseLocale(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"fr-FR, en;q=0.9", "fr"},
		{"en-US", "en"},
		{"zh-CN", "zh"},
		{"hi", "hi"},
		{"es-MX,en;q=0.5", "es"},
		{"de-DE", "de"},
		{"", "en"},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, ParseLocale(tt.input), "input: %q", tt.input)
	}
}

func TestContextLocale(t *testing.T) {
	ctx := context.Background()
	assert.Equal(t, "en", LocaleFromCtx(ctx))

	ctx = ContextWithLocale(ctx, "fr")
	assert.Equal(t, "fr", LocaleFromCtx(ctx))
}

func TestNewFromDir_MissingFallback(t *testing.T) {
	dir := t.TempDir()
	frDir := filepath.Join(dir, "fr")
	require.NoError(t, os.Mkdir(frDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(frDir, "errors.yaml"), []byte(`auth: {}`), 0o644))

	_, err := NewFromDir(dir)
	assert.ErrorContains(t, err, "fallback locale")
}

func TestNewFromDir_InvalidDir(t *testing.T) {
	_, err := NewFromDir("/nonexistent/path")
	assert.ErrorContains(t, err, "read dir")
}

// Regression for the silent-skip bug: an overnight audit added a
// duplicate top-level `confirm:` key to common.yaml across all 6
// locales. yaml.v3 rejected the file but the loader only logged a
// warn, so every common.* string disappeared from the live API and
// users saw raw key strings in the UI. NewFromDir must now refuse
// to boot on any malformed file so the regression cannot reach prod.
func TestNewFromDir_FailsOnDuplicateKey(t *testing.T) {
	dir := t.TempDir()
	enDir := filepath.Join(dir, "en")
	require.NoError(t, os.Mkdir(enDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(enDir, "common.yaml"), []byte(`
confirm: First
other: Other
confirm: Second
`), 0o644))

	_, err := NewFromDir(dir)
	require.Error(t, err)
	assert.ErrorContains(t, err, "invalid YAML")
	assert.ErrorContains(t, err, "common.yaml")
}

func TestAllMessages(t *testing.T) {
	tr, err := NewFromDir(setupTestDir(t))
	require.NoError(t, err)

	msgs := tr.AllMessages("en")
	assert.Equal(t, "Invalid username or password", msgs["errors.auth.invalid_credentials"])
	assert.Equal(t, "Resource not found", msgs["errors.general.not_found"])

	frMsgs := tr.AllMessages("fr")
	assert.Equal(t, "Nom d'utilisateur ou mot de passe invalide", frMsgs["errors.auth.invalid_credentials"])

	// Unknown locale returns fallback
	deMsgs := tr.AllMessages("de")
	assert.Equal(t, "Invalid username or password", deMsgs["errors.auth.invalid_credentials"])
}

func TestNewFromDir_RealYAML(t *testing.T) {
	root := "../../../i18n"
	if _, err := os.Stat(root); os.IsNotExist(err) {
		t.Skip("i18n dir not found")
	}
	tr, err := NewFromDir(root)
	require.NoError(t, err, "real i18n YAML must parse without errors")
	assert.NotEmpty(t, tr.T("en", "errors.auth.unauthenticated"))
	assert.NotEmpty(t, tr.T("en", "errors.library.name_required"))
}
