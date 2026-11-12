package plugin_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/eveyproject/evey/internal/domain/plugin"
)

func TestNormalizeRepoURL(t *testing.T) {
	t.Parallel()
	ok := map[string]string{
		"https://github.com/eveyproject/evey-plugin-weather":                                            "https://github.com/eveyproject/evey-plugin-weather",
		"  https://github.com/eveyproject/evey-plugin-weather/  ":                                       "https://github.com/eveyproject/evey-plugin-weather",
		"https://github.com/eveyproject/evey-plugin-weather.git":                                        "https://github.com/eveyproject/evey-plugin-weather",
		"https://GitHub.com/eveyproject/evey-plugin-weather/releases/download/v1.0.0/weather-1.0.0.zip": "https://github.com/eveyproject/evey-plugin-weather",
		"": "",
	}
	for in, want := range ok {
		got, err := plugin.NormalizeRepoURL(in)
		require.NoError(t, err, in)
		assert.Equal(t, want, got, in)
	}
	bad := []string{
		"http://github.com/acme/thing",
		"https://gitlab.com/acme/thing",
		"https://github.com/acme",
		"https://user:pass@github.com/acme/thing",
		"https://github.com/acme/th%20ing",
		"https://github.com/../thing",
		"github.com/acme/thing",
	}
	for _, in := range bad {
		_, err := plugin.NormalizeRepoURL(in)
		assert.ErrorIs(t, err, plugin.ErrUnsupportedRepository, in)
	}
}
