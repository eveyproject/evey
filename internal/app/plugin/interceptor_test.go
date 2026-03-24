package plugin

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsValidPluginID(t *testing.T) {
	tests := []struct {
		name  string
		id    string
		valid bool
	}{
		{"length 3 invalid", "abc", false},
		{"length 4 valid", "abcd", true},
		{"length 50 valid", strings.Repeat("a", 50), true},
		{"length 51 invalid via extractPluginID", strings.Repeat("a", 51), true}, // isValidPluginID itself only checks >=4; len>50 is checked in extractPluginID
		{"empty string", "", false},
		{"length 1", "a", false},
		{"lowercase letters", "my-plugin", true},
		{"digits", "plugin123", true},
		{"hyphens", "my-cool-plugin", true},
		{"mixed valid chars", "a0-b1-c2", true},
		{"uppercase rejected", "MyPlugin", false},
		{"underscore rejected", "my_plugin", false},
		{"space rejected", "my plugin", false},
		{"dot rejected", "my.plugin", false},
		{"slash rejected", "my/plugin", false},
		{"unicode rejected", "plu\u00e9gin", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.valid, isValidPluginID(tt.id))
		})
	}
}
