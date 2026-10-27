package user

import "testing"

func TestProfile_LooserThan(t *testing.T) {
	seven, sixteen := 7, 16
	young := &Profile{IsKids: true, Parental: &ParentalSettings{MaxAge: &seven, BlockedTags: []string{"horror"}}}
	older := &Profile{IsKids: true, Parental: &ParentalSettings{MaxAge: &sixteen, BlockedTags: []string{"horror"}}}
	sameAgeNoTags := &Profile{IsKids: true, Parental: &ParentalSettings{MaxAge: &seven}}
	adult := &Profile{}

	cases := []struct {
		name     string
		p, other *Profile
		want     bool
	}{
		{"higher age cap", older, young, true},
		{"lower age cap", young, older, false},
		{"a blocked tag dropped", sameAgeNoTags, young, true},
		{"unrestricted target", adult, young, true},
		{"from an unrestricted profile", young, adult, false},
		{"same limits", young, young, false},
	}
	for _, c := range cases {
		if got := c.p.LooserThan(c.other); got != c.want {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}
