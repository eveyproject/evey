package extfetch

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidate_EmptyList(t *testing.T) {
	require.NoError(t, Validate([]string{}))
}

func TestValidate_NilList(t *testing.T) {
	require.NoError(t, Validate(nil))
}

func TestValidate_ValidEntries(t *testing.T) {
	cases := []struct {
		name  string
		entry string
	}{
		{"simple domain", "api.tmdb.org"},
		{"wildcard subdomain", "*.tmdb.org"},
		{"domain with path", "api.tmdb.org/v3/*"},
		{"https explicit", "https://api.tmdb.org"},
		{"https with path", "https://api.tmdb.org/v3/movie"},
		{"https wildcard subdomain", "https://*.tmdb.org"},
		{"deep path wildcard", "api.example.com/some/deep/path/*"},
		{"multi-level subdomain", "cdn.static.example.com"},
		{"wildcard multi-level", "*.static.example.com"},
		{"two-label domain", "example.com"},
		{"uppercase normalised", "API.TMDB.ORG"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := Validate([]string{tc.entry})
			assert.NoError(t, err, "expected %q to be valid", tc.entry)
		})
	}
}

func TestValidate_InvalidEntries(t *testing.T) {
	cases := []struct {
		name        string
		entry       string
		errContains string
	}{
		{"empty string", "", "empty entry"},
		{"bare wildcard", "*", "at least two labels"},
		{"http scheme", "http://api.tmdb.org", "only https is permitted"},
		{"ftp scheme", "ftp://example.com", "only https is permitted"},
		{"http wildcard", "http://*", "only https is permitted"},
		{"https bare wildcard", "https://*", "at least two labels"},
		{"wildcard tld", "*.com", "wildcard in TLD or SLD"},
		{"double wildcard", "*.*.com", "wildcard in TLD or SLD"},
		{"mid wildcard", "cdn.*.tmdb.org", "must be the leftmost label"},
		{"ipv4 literal", "192.168.1.1", "IP address"},
		{"ipv4 loopback", "127.0.0.1", "IP address"},
		{"ipv4 private rfc1918 10", "10.0.0.1", "IP address"},
		{"ipv4 private rfc1918 172", "172.16.0.1", "IP address"},
		{"ipv4 link local", "169.254.1.1", "IP address"},
		{"ipv6 loopback", "::1", "IP address"},
		{"ipv6 bracketed", "[::1]", "IP address"},
		{"localhost", "localhost", "localhost is not allowed"},
		{"bare path no host", "/api/*", "must specify a host"},
		{"single label", "intranet", "at least two labels"},
		{"embedded wildcard label", "foo*.example.com", "embedded wildcard"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := Validate([]string{tc.entry})
			assert.ErrorContains(t, err, tc.errContains, "expected %q to fail with %q", tc.entry, tc.errContains)
		})
	}
}

func TestValidate_MultipleEntries_FirstInvalidReported(t *testing.T) {
	err := Validate([]string{"api.tmdb.org", "http://evil.com", "*.tmdb.org"})
	assert.ErrorContains(t, err, "http://evil.com")
	assert.ErrorContains(t, err, "only https is permitted")
}

func TestValidate_MultipleValidEntries(t *testing.T) {
	err := Validate([]string{
		"api.themoviedb.org",
		"*.themoviedb.org",
		"https://image.tmdb.org/t/p/*",
	})
	require.NoError(t, err)
}
