package config

import "testing"

func TestDemoLevel(t *testing.T) {
	for _, c := range []struct {
		env          string
		on, extended bool
	}{
		{"", false, false}, {"1", true, false}, {"true", true, false}, {"0", false, false},
		{"extended", true, true}, {" Extended ", true, true},
	} {
		t.Setenv("EVEY_DEMO", c.env)
		got := demoLevel()
		if got.on != c.on || got.extended != c.extended {
			t.Errorf("EVEY_DEMO=%q: got %+v, want on=%v extended=%v", c.env, got, c.on, c.extended)
		}
	}
}

func TestExtendedDemoAccountMustBeStrongAndTypeable(t *testing.T) {
	good := "Abcdefghij0123456789"
	for _, c := range []struct {
		user, pass string
		ok         bool
	}{
		{"guest", good, true},
		{"", good, false},
		{"gu est", good, false},
		{"guest", "short", false},
		{"guest", good[:19], false},
		{"guest", good + "!", false},
		{"guest", "demo", false},
	} {
		if errs := validateExtendedDemoAccount(c.user, c.pass); (len(errs) == 0) != c.ok {
			t.Errorf("user=%q pass=%q: errs=%v, want ok=%v", c.user, c.pass, errs, c.ok)
		}
	}
}
