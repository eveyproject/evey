package plugin_test

import (
	"strings"
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

// A link must name the repository it reaches: dot segments, encoded
// characters and empty segments could make the request land elsewhere.
func TestNormalizeRepoURL_RefusesLinksThatReachAnotherRepository(t *testing.T) {
	t.Parallel()
	for _, in := range []string{
		"https://github.com/acme/thing/releases/download/../../../mallory/evil/releases/download/v1/p.zip",
		"https://github.com/acme/thing/./releases/download/v1/p.zip",
		"https://github.com/acme//thing",
		"https://github.com/acme/thing%2F..%2Fevil",
		"https://github.com/acme/thing?x=1",
	} {
		_, err := plugin.NormalizeRepoURL(in)
		assert.ErrorIs(t, err, plugin.ErrUnsupportedRepository, in)
	}
}

func TestValidReleaseVersion(t *testing.T) {
	t.Parallel()
	for _, v := range []string{"1.0.0", "10.20.30", "1.2.0-beta.1"} {
		assert.True(t, plugin.ValidReleaseVersion(v), v)
	}
	for _, v := range []string{"", "v1.0.0", "1.0", "01.0.0", "1.0.0+build", "1.0.0-" + strings.Repeat("a", 60),
		"Plugin update approved: click here"} {
		assert.False(t, plugin.ValidReleaseVersion(v), v)
	}
	assert.True(t, plugin.IsPreRelease("1.2.0-alpha"))
	assert.False(t, plugin.IsPreRelease("1.2.0"))
}
