package urlvalidator

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidateWebhookURL_BlocksPrivateIPs(t *testing.T) {
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
	cases := []string{
		"ftp://example.com/hook",
		"file:///etc/passwd",
		"gopher://example.com",
	}
	for _, u := range cases {
		err := ValidateWebhookURL(u)
		assert.Error(t, err, "expected blocked: %s", u)
		assert.Contains(t, err.Error(), "not allowed")
	}
}

func TestValidateWebhookURL_BlocksLocalhost(t *testing.T) {
	err := ValidateWebhookURL("http://localhost/hook")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "blocked")
}

func TestValidateWebhookURL_BlocksMetadataEndpoint(t *testing.T) {
	err := ValidateWebhookURL("http://metadata.google.internal/computeMetadata/v1/")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "blocked")
}

func TestValidateWebhookURL_AllowsPublicURL(t *testing.T) {
	err := ValidateWebhookURL("https://example.com/webhook")
	assert.NoError(t, err)
}

func TestValidateWebhookURL_EmptyHost(t *testing.T) {
	err := ValidateWebhookURL("http:///hook")
	assert.Error(t, err)
}

func TestValidateWebhookURL_InvalidURL(t *testing.T) {
	err := ValidateWebhookURL("://bad")
	assert.Error(t, err)
}
