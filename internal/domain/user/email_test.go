package user

import (
	"errors"
	"strings"
	"testing"
)

func TestNewEmail_Valid(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"simple", "alice@example.com", "alice@example.com"},
		{"domain uppercased", "alice@Example.COM", "alice@example.com"},
		{"local case preserved", "Alice.Smith@example.com", "Alice.Smith@example.com"},
		{"trim leading", "  bob@example.com", "bob@example.com"},
		{"trim trailing", "bob@example.com\t", "bob@example.com"},
		{"trim both", "  bob@Example.com  ", "bob@example.com"},
		{"plus addressing", "carol+tag@SUB.Example.org", "carol+tag@sub.example.org"},
		{"local with at via lastindex", "weird@stuff@example.com", "weird@stuff@example.com"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := NewEmail(tc.in)
			if err != nil {
				t.Fatalf("NewEmail(%q) returned error: %v", tc.in, err)
			}
			if got.String() != tc.want {
				t.Fatalf("NewEmail(%q).String() = %q, want %q", tc.in, got.String(), tc.want)
			}
		})
	}
}

func TestNewEmail_Invalid(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		in   string
	}{
		{"empty", ""},
		{"whitespace only", "   "},
		{"no at", "alice.example.com"},
		{"no local", "@example.com"},
		{"no domain", "alice@"},
		{"space inside", "ali ce@example.com"},
		{"tab inside", "alice@exa\tmple.com"},
		{"newline inside", "alice@example.com\nrcpt"},
		{"too long", strings.Repeat("a", 250) + "@b.co"}, // 250 + 1 + 4 = 255 > 254
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := NewEmail(tc.in)
			if !errors.Is(err, ErrInvalidEmail) {
				t.Fatalf("NewEmail(%q) err = %v, want ErrInvalidEmail", tc.in, err)
			}
		})
	}
}

func TestNewEmail_BoundaryLength(t *testing.T) {
	t.Parallel()
	// exactly 254 chars must succeed: 248 + 1 + 5 = 254
	in := strings.Repeat("a", 248) + "@b.com"
	if len(in) != 254 {
		t.Fatalf("test setup wrong: len=%d", len(in))
	}
	if _, err := NewEmail(in); err != nil {
		t.Fatalf("NewEmail at 254 chars should succeed, got %v", err)
	}
}

func TestEmail_Equal(t *testing.T) {
	t.Parallel()
	a, err := NewEmail("alice@Example.com")
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	b, err := NewEmail("alice@EXAMPLE.COM")
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	if !a.Equal(b) {
		t.Fatalf("Equal should be true for differing-domain-case emails: %q vs %q", a, b)
	}

	c, err := NewEmail("bob@example.com")
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	if a.Equal(c) {
		t.Fatalf("Equal should be false for distinct addresses")
	}

	// local-part casing is significant in our normalisation.
	d, err := NewEmail("Alice@example.com")
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	if a.Equal(d) {
		t.Fatalf("Equal should be false when only local-part casing differs")
	}
}
