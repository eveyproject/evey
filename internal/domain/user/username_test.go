package user_test

import (
	"strings"
	"testing"

	"github.com/eveyproject/evey/internal/domain/user"
)

// A name is bounded in length and in what it may carry: the login path keys
// its counters and audit rows on it as typed.
func TestValidateUsername(t *testing.T) {
	cases := []struct {
		name string
		in   string
		ok   bool
	}{
		{"plain", "alice", true},
		{"accented", "zoé", true},
		{"at the bound", strings.Repeat("a", user.UsernameMaxRunes), true},
		{"past the bound", strings.Repeat("a", user.UsernameMaxRunes+1), false},
		{"past the bound in runes not bytes", strings.Repeat("é", user.UsernameMaxRunes), true},
		{"empty", "   ", false},
		{"control", "ali\x00ce", false},
		{"newline", "alice\n", false},
		{"bidi override", "alice‮ecila", false},
		{"zero width joiner", "ali‍ce", false},
		{"malformed utf8", "ali\xffce", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := user.ValidateUsername(c.in)
			if c.ok && err != nil {
				t.Fatalf("%q refused: %v", c.in, err)
			}
			if !c.ok && err == nil {
				t.Fatalf("%q accepted", c.in)
			}
		})
	}
}

// NewUser and an update carry the same rule.
func TestNewUser_RefusesAnOversizedName(t *testing.T) {
	if _, err := user.NewUser(strings.Repeat("a", user.UsernameMaxRunes+1), "", "hash"); err == nil {
		t.Fatal("an oversized name must be refused at creation")
	}
	u, err := user.NewUser("alice", "", "hash")
	if err != nil {
		t.Fatal(err)
	}
	u.Username = "ali‮ce"
	if err := u.Validate(); err == nil {
		t.Fatal("a format character must be refused on update")
	}
}
