package user

import "errors"

// ErrKidsRequiresPIN is returned by CheckKidsPINInvariant when the candidate
// profile would leave the kids/PIN household rule broken: a non-kids profile
// without a PIN while a kids sibling exists, or a profile being created/
// flipped to kids while a non-kids sibling still lacks one.
var ErrKidsRequiresPIN = errors.New("non-kids profile must have a PIN when a kids profile exists")

// ErrAdminCannotBeKids is returned by CheckAdminCannotBeKids: an admin's own
// profile cannot be marked kids, since that would gate the admin out of their
// own library.
var ErrAdminCannotBeKids = errors.New("an admin user's profile cannot be a kids profile")

// CheckKidsPINInvariant enforces the household-wide data invariant: once any
// kids profile exists for a user, every non-kids sibling must hold a PIN (so
// switching to it from a kids profile is challenge-gated), and a profile
// cannot be created or flipped to kids while a non-kids sibling still lacks
// one (that would break the same symmetry from the other side).
//
// Pure domain rule: callers load siblings themselves (via
// ProfileRepository.GetByUserID) and pass them in - this function does no
// I/O, so it can be re-run as a cheap final gate immediately before ANY
// profile write, regardless of which fields that write actually touches.
func CheckKidsPINInvariant(candidate *Profile, siblings []*Profile) error {
	hasKidsSibling := false
	var pinlessNonKidsSibling *Profile
	for _, other := range siblings {
		if other == nil || string(other.ID) == string(candidate.ID) {
			continue // ignore self
		}
		if other.IsKids {
			hasKidsSibling = true
		} else if !other.HasPIN() {
			pinlessNonKidsSibling = other
		}
	}
	if (hasKidsSibling || candidate.IsKids) && !candidate.IsKids && !candidate.HasPIN() {
		return ErrKidsRequiresPIN
	}
	if candidate.IsKids && pinlessNonKidsSibling != nil {
		return ErrKidsRequiresPIN
	}
	return nil
}

// CheckAdminCannotBeKids enforces that an admin's own profile is never kids -
// filtering would otherwise gate the admin out of their own library.
func CheckAdminCannotBeKids(ownerIsAdmin bool, candidate *Profile) error {
	if ownerIsAdmin && candidate.IsKids {
		return ErrAdminCannotBeKids
	}
	return nil
}
