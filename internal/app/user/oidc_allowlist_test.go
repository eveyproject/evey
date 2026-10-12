package user_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	appuser "github.com/eveyproject/evey/internal/app/user"
)

func TestIsEmailAllowed(t *testing.T) {
	cases := []struct {
		name           string
		email          string
		allowedDomains []string
		want           bool
	}{
		{"no restriction", "user@any.com", nil, true},
		{"empty restriction", "user@any.com", []string{}, true},
		{"matching domain", "user@example.com", []string{"example.com"}, true},
		{"non-matching domain", "user@other.com", []string{"example.com"}, false},
		{"case-insensitive domain", "user@EXAMPLE.COM", []string{"example.com"}, true},
		{"case-insensitive allowlist", "user@example.com", []string{"EXAMPLE.COM"}, true},
		{"multiple domains match", "user@company.org", []string{"example.com", "company.org"}, true},
		{"multiple domains no match", "user@evil.com", []string{"example.com", "company.org"}, false},
		{"invalid email no @", "invalidemail", []string{"example.com"}, false},
		{"empty email", "", []string{"example.com"}, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := appuser.IsEmailAllowed(tc.email, tc.allowedDomains)
			assert.Equal(t, tc.want, got)
		})
	}
}
