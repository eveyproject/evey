package main

import (
	"strings"
	"testing"
)

func TestRefuseDevModeInProd(t *testing.T) {
	cases := []struct {
		name      string
		envVar    string
		wantErr   bool
		wantMatch string // substring expected in error message when wantErr=true
	}{
		{"empty allowed", "", false, ""},
		{"dev allowed", "dev", false, ""},
		{"development allowed", "development", false, ""},
		{"staging allowed", "staging", false, ""},
		{"random allowed", "preview", false, ""},
		{"production blocked", "production", true, "EVEY_ENV=production"},
		{"prod blocked", "prod", true, "EVEY_ENV=prod"},
		{"PRODUCTION blocked (case)", "PRODUCTION", true, "EVEY_ENV=production"},
		{"  prod   blocked (whitespace)", "  prod   ", true, "EVEY_ENV=prod"},
		{"Prod blocked (mixed case)", "Prod", true, "EVEY_ENV=prod"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := refuseDevModeInProd(c.envVar)
			if c.wantErr {
				if err == nil {
					t.Fatalf("expected error for env=%q, got nil", c.envVar)
				}
				if !strings.Contains(err.Error(), c.wantMatch) {
					t.Errorf("error message %q does not contain %q", err.Error(), c.wantMatch)
				}
			} else {
				if err != nil {
					t.Errorf("expected nil error for env=%q, got %v", c.envVar, err)
				}
			}
		})
	}
}
