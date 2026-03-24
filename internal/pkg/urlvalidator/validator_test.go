package urlvalidator

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateWebhookURL_BlocksPrivateIPs(t *testing.T) {
	t.Parallel()
	cases := []string{
		"http://127.0.0.1/hook",
		"http://10.0.0.1/hook",
		"http://172.16.0.1/hook",
		"http://192.168.1.1/hook",
		"http://169.254.169.254/latest/meta-data/",
		"http://0.0.0.0/hook",
		"http://[::1]/hook",
	}
	for _, u := range cases {
		err := ValidateWebhookURL(u)
		assert.Error(t, err, "expected blocked: %s", u)
	}
}

func TestValidateWebhookURL_BlocksNonHTTP(t *testing.T) {
	t.Parallel()
	cases := []string{
		"ftp://example.com/hook",
		"file:///etc/passwd",
		"gopher://example.com",
	}
	for _, u := range cases {
		err := ValidateWebhookURL(u)
		require.Error(t, err, "expected blocked: %s", u)
		assert.Contains(t, err.Error(), "not allowed")
	}
}

func TestValidateWebhookURL_BlocksLocalhost(t *testing.T) {
	t.Parallel()
	err := ValidateWebhookURL("http://localhost/hook")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "blocked")
}

func TestValidateWebhookURL_BlocksMetadataEndpoint(t *testing.T) {
	t.Parallel()
	err := ValidateWebhookURL("http://metadata.google.internal/computeMetadata/v1/")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "blocked")
}

func TestValidateWebhookURL_AllowsPublicURL(t *testing.T) {
	t.Parallel()
	err := ValidateWebhookURL("https://example.com/webhook")
	assert.NoError(t, err)
}

func TestValidateWebhookURL_EmptyHost(t *testing.T) {
	t.Parallel()
	err := ValidateWebhookURL("http:///hook")
	assert.ErrorContains(t, err, "empty host")
}

func TestValidateWebhookURL_InvalidURL(t *testing.T) {
	t.Parallel()
	err := ValidateWebhookURL("://bad")
	assert.ErrorContains(t, err, "invalid URL")
}
