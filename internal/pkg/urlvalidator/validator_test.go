package urlvalidator

import (
	"context"
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- ValidateWebhookURL ---

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

func TestValidateWebhookURL_AllowsPublicIP(t *testing.T) {
	t.Parallel()
	err := ValidateWebhookURL("https://8.8.8.8/hook")
	assert.NoError(t, err)
}

func TestValidateWebhookURL_BlocksBlockedIPLiteral(t *testing.T) {
	t.Parallel()
	err := ValidateWebhookURL("http://100.100.100.200/hook")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "blocked")
}

func TestValidateWebhookURL_BlocksLocalhostCaseInsensitive(t *testing.T) {
	t.Parallel()
	err := ValidateWebhookURL("http://LOCALHOST/hook")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "blocked")
}

// --- ValidateExternalURL ---

func TestValidateExternalURL_ValidHTTPS(t *testing.T) {
	t.Parallel()
	assert.NoError(t, ValidateExternalURL("https://example.com"))
	assert.NoError(t, ValidateExternalURL("https://example.com/path?q=1"))
	assert.NoError(t, ValidateExternalURL("https://sub.domain.example.com:8443/foo"))
}

func TestValidateExternalURL_RejectsHTTP(t *testing.T) {
	t.Parallel()
	err := ValidateExternalURL("http://example.com")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must be https")
}

func TestValidateExternalURL_RejectsNonHTTPSchemes(t *testing.T) {
	t.Parallel()
	for _, u := range []string{"ftp://example.com", "file:///etc/passwd", "javascript://x"} {
		err := ValidateExternalURL(u)
		require.Error(t, err, "expected blocked: %s", u)
		assert.Contains(t, err.Error(), "not allowed")
	}
}

func TestValidateExternalURL_RejectsEmptyHost(t *testing.T) {
	t.Parallel()
	err := ValidateExternalURL("https:///path")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty host")
}

func TestValidateExternalURL_RejectsInvalidURL(t *testing.T) {
	t.Parallel()
	err := ValidateExternalURL("://bad")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid URL")
}

func TestValidateExternalURL_BlocksLocalhost(t *testing.T) {
	t.Parallel()
	err := ValidateExternalURL("https://localhost/path")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "blocked")
}

func TestValidateExternalURL_BlocksLocalhostCaseInsensitive(t *testing.T) {
	t.Parallel()
	err := ValidateExternalURL("https://LOCALHOST/path")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "blocked")
}

func TestValidateExternalURL_BlocksMetadata(t *testing.T) {
	t.Parallel()
	err := ValidateExternalURL("https://metadata.google.internal/v1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "blocked")
}

func TestValidateExternalURL_BlocksBlockedIP(t *testing.T) {
	t.Parallel()
	err := ValidateExternalURL("https://100.100.100.200/path")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "blocked")
}

func TestValidateExternalURL_BlocksPrivateIPs(t *testing.T) {
	t.Parallel()
	for _, u := range []string{
		"https://127.0.0.1/path",
		"https://10.0.0.1/path",
		"https://192.168.1.1/path",
		"https://[::1]/path",
	} {
		err := ValidateExternalURL(u)
		require.Error(t, err, "expected blocked: %s", u)
		assert.Contains(t, err.Error(), "private")
	}
}

func TestValidateExternalURL_AllowsPublicIP(t *testing.T) {
	t.Parallel()
	assert.NoError(t, ValidateExternalURL("https://8.8.8.8/path"))
}

func TestValidateExternalURL_EmptyScheme(t *testing.T) {
	t.Parallel()
	err := ValidateExternalURL("example.com")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not allowed")
}

// --- ValidateRedirectURL ---

func TestValidateRedirectURL_ValidHTTPAndHTTPS(t *testing.T) {
	t.Parallel()
	assert.NoError(t, ValidateRedirectURL("https://example.com/callback"))
	assert.NoError(t, ValidateRedirectURL("http://example.com/callback"))
	assert.NoError(t, ValidateRedirectURL("https://example.com/path?state=abc&code=123"))
}

func TestValidateRedirectURL_RejectsNonHTTPSchemes(t *testing.T) {
	t.Parallel()
	for _, u := range []string{
		"ftp://example.com",
		"javascript://alert(1)",
		"file:///etc/passwd",
		"data:text/html,<h1>hi</h1>",
	} {
		err := ValidateRedirectURL(u)
		require.Error(t, err, "expected blocked: %s", u)
		assert.Contains(t, err.Error(), "not allowed")
	}
}

func TestValidateRedirectURL_RejectsEmptyHost(t *testing.T) {
	t.Parallel()
	err := ValidateRedirectURL("http:///path")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty host")
}

func TestValidateRedirectURL_RejectsInvalidURL(t *testing.T) {
	t.Parallel()
	err := ValidateRedirectURL("://bad")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid URL")
}

func TestValidateRedirectURL_BlocksPrivateIPs(t *testing.T) {
	t.Parallel()
	for _, u := range []string{
		"http://127.0.0.1/callback",
		"http://10.0.0.1/callback",
		"http://192.168.1.1/callback",
		"https://[::1]/callback",
		"http://0.0.0.0/callback",
	} {
		err := ValidateRedirectURL(u)
		require.Error(t, err, "expected blocked: %s", u)
		assert.Contains(t, err.Error(), "private")
	}
}

func TestValidateRedirectURL_AllowsPublicIP(t *testing.T) {
	t.Parallel()
	assert.NoError(t, ValidateRedirectURL("http://8.8.8.8/callback"))
}

func TestValidateRedirectURL_AllowsLocalhostHostname(t *testing.T) {
	t.Parallel()
	// localhost as hostname (not IP) is allowed for redirect - no SSRF risk
	assert.NoError(t, ValidateRedirectURL("http://localhost/callback"))
}

func TestValidateRedirectURL_EmptyScheme(t *testing.T) {
	t.Parallel()
	err := ValidateRedirectURL("example.com/path")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not allowed")
}

// A hostname (not an IP literal) is resolved and each address checked: one
// that points at loopback is refused. Only literals and blocked names were
// tested, so a slip sending names down the literal branch - which accepts
// them unresolved - went unseen (mutation 2026-09-28). ip6-localhost comes
// from /etc/hosts, no network needed; skipped where it does not resolve.
func TestValidateWebhookURL_HostnameResolvingToLoopbackRefused(t *testing.T) {
	if addrs, err := net.DefaultResolver.LookupIPAddr(context.Background(), "ip6-localhost"); err != nil || len(addrs) == 0 {
		t.Skip("ip6-localhost does not resolve locally here")
	}
	if err := ValidateWebhookURL("http://ip6-localhost:8080/hook"); err == nil {
		t.Error("a hostname resolving to loopback must be refused")
	}
}
