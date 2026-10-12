package user

import (
	"errors"
	"testing"

	"github.com/eveyproject/evey/internal/domain/shared"
)

// The household rule this pins: once ANY kids profile exists, every non-kids
// sibling must hold a PIN, so switching away from the kids profile is always
// challenge-gated. It used to be re-implemented inside individual HTTP
// handlers, which meant any write that did not go through them (an avatar
// upload, a rename) could quietly leave the household in a state where a child
// could switch to an adult profile unchallenged.
//
// It lives in the domain precisely so it can be re-run before ANY profile
// write, whatever fields that write touches.
func profile(id string, kids bool, pin string) *Profile {
	p := &Profile{ID: shared.ProfileID(id), IsKids: kids}
	if pin != "" {
		p.Parental = &ParentalSettings{PINHash: pin}
	}
	return p
}

func TestCheckKidsPINInvariant(t *testing.T) {
	cases := []struct {
		name      string
		candidate *Profile
		siblings  []*Profile
		want      error
	}{
		{
			name:      "lone adult profile, no kids anywhere: no PIN required",
			candidate: profile("adult", false, ""),
			siblings:  nil,
			want:      nil,
		},
		{
			name:      "adult without PIN while a kids sibling exists: refused",
			candidate: profile("adult", false, ""),
			siblings:  []*Profile{profile("kid", true, "")},
			want:      ErrKidsRequiresPIN,
		},
		{
			name:      "adult WITH a PIN alongside a kids sibling: allowed",
			candidate: profile("adult", false, "hash"),
			siblings:  []*Profile{profile("kid", true, "")},
			want:      nil,
		},
		{
			name:      "flipping a profile to kids while an adult sibling has no PIN: refused",
			candidate: profile("kid", true, ""),
			siblings:  []*Profile{profile("adult", false, "")},
			want:      ErrKidsRequiresPIN,
		},
		{
			name:      "flipping to kids when every adult sibling holds a PIN: allowed",
			candidate: profile("kid", true, ""),
			siblings:  []*Profile{profile("adult", false, "hash")},
			want:      nil,
		},
		{
			name: "the candidate must not be judged against a stale copy of ITSELF",
			// The caller loads siblings with a repo call that also returns the
			// candidate's own pre-edit row. Counting it would refuse the very
			// edit that ADDS the missing PIN - the household could never be
			// repaired.
			candidate: profile("adult", false, "fresh-hash"),
			siblings:  []*Profile{profile("adult", false, ""), profile("kid", true, "")},
			want:      nil,
		},
		{
			name:      "nil sibling entries are skipped rather than panicking",
			candidate: profile("adult", false, "hash"),
			siblings:  []*Profile{nil, profile("kid", true, "")},
			want:      nil,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if err := CheckKidsPINInvariant(c.candidate, c.siblings); !errors.Is(err, c.want) {
				t.Errorf("CheckKidsPINInvariant = %v, want %v", err, c.want)
			}
		})
	}
}

// An admin gated out of their own library by their own profile flag has no way
// back in, so this one is refused outright rather than merely warned about.
func TestCheckAdminCannotBeKids(t *testing.T) {
	if err := CheckAdminCannotBeKids(true, profile("p", true, "")); !errors.Is(err, ErrAdminCannotBeKids) {
		t.Errorf("admin marked kids should be refused, got %v", err)
	}
	if err := CheckAdminCannotBeKids(true, profile("p", false, "hash")); err != nil {
		t.Errorf("admin with a non-kids profile is fine, got %v", err)
	}
	if err := CheckAdminCannotBeKids(false, profile("p", true, "")); err != nil {
		t.Errorf("a non-admin user may have a kids profile, got %v", err)
	}
}
