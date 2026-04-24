package user

import (
	"errors"
	"strings"
)

// ErrInvalidEmail is returned by NewEmail when the input does not satisfy the
// minimal RFC 5321 shape we enforce (presence of '@', non-empty local-part and
// domain, no whitespace, total length <= 254).
var ErrInvalidEmail = errors.New("invalid email")

// maxEmailLen is the maximum forward-path length per RFC 5321 §4.5.3.1.3.
const maxEmailLen = 254

// Email is a normalised email address value object. Construction is forced
// through NewEmail so the zero value of Email outside this package is unusable
// (the underlying struct field is unexported); this prevents callers from
// fabricating an "invalid Email" by raw struct literal.
type Email struct {
	value string
}

// NewEmail parses raw into a normalised Email. Whitespace is trimmed, the
// domain part is lowercased (per RFC 5321 conventions; the local-part casing
// is preserved because it MAY be significant to the receiving server), and
// the result is validated. Returns ErrInvalidEmail on any failure.
func NewEmail(raw string) (Email, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return Email{}, ErrInvalidEmail
	}
	if len(trimmed) > maxEmailLen {
		return Email{}, ErrInvalidEmail
	}
	if strings.ContainsAny(trimmed, " \t\r\n") {
		return Email{}, ErrInvalidEmail
	}
	at := strings.LastIndex(trimmed, "@")
	if at <= 0 || at == len(trimmed)-1 {
		return Email{}, ErrInvalidEmail
	}
	local := trimmed[:at]
	domain := strings.ToLower(trimmed[at+1:])
	return Email{value: local + "@" + domain}, nil
}

// String returns the normalised form of the email.
func (e Email) String() string {
	return e.value
}

// Equal reports whether two emails are equivalent in their normalised form.
func (e Email) Equal(other Email) bool {
	return e.value == other.value
}
