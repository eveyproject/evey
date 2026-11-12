package plugin

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

// DisplayName is the name a plugin shows people, keyed by language tag. Name
// stays the identifier an update is matched on; this one can change freely.
// A manifest gives either one string, filed under English and so used in
// every language, or a map:
//
//	display_name: CShell
//	display_name: {en: Dark Blue, fr: Bleu sombre}
type DisplayName map[string]string

const (
	displayNameDefaultLang = "en"
	maxDisplayNameRunes    = 40
	maxDisplayNameLangs    = 32
)

// A language tag: a 2-3 letter language and at most three subtags
// ("pt-BR", "zh-Hant-TW"), so a key cannot grow without bound.
var displayNameLangRe = regexp.MustCompile(`^[a-z]{2,3}(-[A-Za-z0-9]{2,8}){0,3}$`)

// UnmarshalYAML accepts the string form as well as the map.
func (d *DisplayName) UnmarshalYAML(unmarshal func(any) error) error {
	var s string
	if err := unmarshal(&s); err == nil {
		*d = DisplayName{displayNameDefaultLang: s}
		return nil
	}
	var m map[string]string
	if err := unmarshal(&m); err != nil {
		return errors.New("manifest: display_name must be a string or a map of language to string")
	}
	*d = m
	return nil
}

// Validate refuses what a theme picker cannot show: an unknown key shape, an
// empty or padded value, a control character, a value longer than a label.
func (d DisplayName) Validate() error {
	if len(d) > maxDisplayNameLangs {
		return fmt.Errorf("manifest: display_name has more than %d languages", maxDisplayNameLangs)
	}
	for lang, v := range d {
		if !displayNameLangRe.MatchString(lang) {
			return fmt.Errorf("manifest: display_name key %q is not a language tag (en, fr, pt-BR)", lang)
		}
		if v == "" || strings.TrimSpace(v) != v {
			return fmt.Errorf("manifest: display_name %q must be non-empty without surrounding spaces", lang)
		}
		if utf8.RuneCountInString(v) > maxDisplayNameRunes {
			return fmt.Errorf("manifest: display_name %q is longer than %d characters", lang, maxDisplayNameRunes)
		}
		if !plainText(v) {
			return fmt.Errorf("manifest: display_name %q contains a control, format or private-use character", lang)
		}
	}
	return nil
}

// plainText refuses what renders as something else than the text it is:
// control characters, format characters (zero-width spaces, text-direction
// overrides that reorder what the reader sees), private-use and surrogate
// code points. A label made of those could pass for another theme's.
func plainText(s string) bool {
	for _, r := range s {
		if unicode.IsControl(r) || unicode.In(r, unicode.Cf, unicode.Co, unicode.Cs) {
			return false
		}
	}
	return true
}
