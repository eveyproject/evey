package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestValidateFCMServiceAccount(t *testing.T) {
	t.Run("empty path", func(t *testing.T) {
		if err := ValidateFCMServiceAccount(""); err != nil {
			t.Fatalf("expected nil, got %v", err)
		}
	})

	t.Run("non-existent file", func(t *testing.T) {
		if err := ValidateFCMServiceAccount("/tmp/does-not-exist-fcm.json"); err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("invalid JSON", func(t *testing.T) {
		f := writeTmp(t, []byte("not json"))
		if err := ValidateFCMServiceAccount(f); err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("wrong type field", func(t *testing.T) {
		f := writeTmp(t, []byte(`{"type":"oauth_client","project_id":"p","private_key":"k","client_email":"e@e.com"}`))
		if err := ValidateFCMServiceAccount(f); err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("missing fields", func(t *testing.T) {
		f := writeTmp(t, []byte(`{"type":"service_account"}`))
		if err := ValidateFCMServiceAccount(f); err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("valid service account", func(t *testing.T) {
		payload := `{
			"type": "service_account",
			"project_id": "my-project",
			"private_key": "-----BEGIN RSA PRIVATE KEY-----\nfake\n-----END RSA PRIVATE KEY-----\n",
			"client_email": "firebase@my-project.iam.gserviceaccount.com"
		}`
		f := writeTmp(t, []byte(payload))
		if err := ValidateFCMServiceAccount(f); err != nil {
			t.Fatalf("expected nil, got %v", err)
		}
	})
}

func writeTmp(t *testing.T, data []byte) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "sa.json")
	if err := os.WriteFile(p, data, 0600); err != nil {
		t.Fatal(err)
	}
	return p
}
