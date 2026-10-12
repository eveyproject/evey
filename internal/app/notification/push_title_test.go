package notification

import (
	"testing"

	eveyi18n "github.com/eveyproject/evey/internal/pkg/i18n"
)

// The push title is the one line a viewer reads on the lock screen: translated
// by its key in the profile's language, and the English fallback whenever
// there is nothing to translate with or nothing to translate.
func TestPushSpec_ResolveTitle(t *testing.T) {
	tr, err := eveyi18n.NewFromDir("../../../i18n")
	if err != nil {
		t.Fatal(err)
	}
	spec := pushSpec{
		key:      "notifications.push.request_answered.title",
		params:   map[string]string{"title": "Some Movie"},
		fallback: "Your media request was updated: Some Movie",
	}
	cases := []struct {
		name string
		spec pushSpec
		tr   *eveyi18n.Translator
		want string
	}{
		{"translated in the profile language", spec, tr, "Votre demande de média a été mise à jour : Some Movie"},
		{"no translator", spec, nil, spec.fallback},
		{"no key", pushSpec{fallback: "plain"}, tr, "plain"},
		{"unknown key", pushSpec{key: "notifications.push.no_such_key", fallback: "plain"}, tr, "plain"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.spec.resolveTitle(c.tr, "fr"); got != c.want {
				t.Errorf("got %q, want %q", got, c.want)
			}
		})
	}
}
