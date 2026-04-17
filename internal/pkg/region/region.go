// Package region resolves the user-facing country for regional preferences
// (certifications, future date/number formats). The resolution is an explicit
// chain: profile override -> server default -> locale default -> US.
package region

import "strings"

// DefaultForLocale maps each supported UI locale to its canonical region.
// Callers fall back to this when neither profile nor server region is set.
var DefaultForLocale = map[string]string{
	"en": "US",
	"fr": "FR",
	"de": "DE",
	"es": "ES",
	"hi": "IN",
	"zh": "CN",
}

// Resolve returns the ISO 3166-1 alpha-2 region code for a user, following
// the explicit chain: profile override -> server default -> locale default -> "US".
// Empty inputs are ignored at each step.
func Resolve(profileRegion *string, serverRegion, locale string) string {
	if profileRegion != nil {
		if r := strings.ToUpper(strings.TrimSpace(*profileRegion)); r != "" {
			return r
		}
	}
	if r := strings.ToUpper(strings.TrimSpace(serverRegion)); r != "" {
		return r
	}
	if locale != "" {
		// "fr-BE" -> honor the region segment directly.
		parts := strings.Split(strings.ReplaceAll(locale, "_", "-"), "-")
		if len(parts) >= 2 && len(parts[1]) == 2 {
			return strings.ToUpper(parts[1])
		}
		if r, ok := DefaultForLocale[strings.ToLower(parts[0])]; ok {
			return r
		}
	}
	return "US"
}
