package plugin_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/eveyproject/evey/internal/domain/plugin"
)

func TestDisplayName_YAMLForms(t *testing.T) {
	t.Parallel()
	var m plugin.PluginManifest
	require.NoError(t, yaml.Unmarshal([]byte("name: cshell\ndisplay_name: CShell\n"), &m))
	assert.Equal(t, plugin.DisplayName{"en": "CShell"}, m.DisplayName, "the string form is filed under English")

	m = plugin.PluginManifest{}
	require.NoError(t, yaml.Unmarshal([]byte("name: dark-blue\ndisplay_name:\n  en: Dark Blue\n  fr: Bleu sombre\n"), &m))
	assert.Equal(t, plugin.DisplayName{"en": "Dark Blue", "fr": "Bleu sombre"}, m.DisplayName)

	m = plugin.PluginManifest{}
	require.NoError(t, yaml.Unmarshal([]byte("name: bare\n"), &m))
	assert.Nil(t, m.DisplayName)

	m = plugin.PluginManifest{}
	assert.Error(t, yaml.Unmarshal([]byte("name: x\ndisplay_name: [a, b]\n"), &m))
}

func TestDisplayName_Validate(t *testing.T) {
	t.Parallel()
	ok := []plugin.DisplayName{
		nil,
		{"en": "Dark Blue"},
		{"fr": "Bleu sombre", "pt-BR": "Azul escuro", "zh": "深蓝"},
	}
	for _, d := range ok {
		require.NoError(t, d.Validate(), "%v", d)
	}
	bad := map[string]plugin.DisplayName{
		"key not a tag": {"French": "Bleu"},
		"upper base":    {"EN": "Blue"},
		"empty":         {"en": ""},
		"padded":        {"en": " Blue"},
		"control":       {"en": "Blue\nRed"},
		"too long":      {"en": strings.Repeat("a", 41)},
		"key too long":  {"en-" + strings.Repeat("abcdefgh-", 5) + "x1": "Blue"},
		"rtl override":  {"en": "Dark\u202eeulB"},
		"zero width":    {"en": "Da\u200brk"},
		"bom":           {"en": "\ufeffDark"},
	}
	for name, d := range bad {
		require.Error(t, d.Validate(), name)
	}
	many := plugin.DisplayName{}
	for i := range 33 {
		many["l"+string(rune('a'+i%26))+string(rune('a'+i/26))] = "x"
	}
	require.Error(t, many.Validate(), "more than 32 languages")
}

func TestManifestValidate_RejectsBadDisplayName(t *testing.T) {
	t.Parallel()
	m := plugin.PluginManifest{Name: "t", Version: "1.0.0", Kind: plugin.KindTheme, DisplayName: plugin.DisplayName{"en": ""}}
	require.ErrorContains(t, m.Validate(), "display_name")

	m.DisplayName = plugin.DisplayName{"en": "Dark Blue"}
	require.NoError(t, m.Validate())
	p, err := plugin.NewPluginFromManifest("plg-1", "h", &m)
	require.NoError(t, err)
	assert.Equal(t, m.DisplayName, p.DisplayName)
}

func TestManifestValidate_BoundsTheName(t *testing.T) {
	t.Parallel()
	m := plugin.PluginManifest{Name: strings.Repeat("a", 101), Version: "1.0.0", Kind: plugin.KindTheme}
	require.ErrorContains(t, m.Validate(), "name")
	m.Name = "weather\u202e"
	require.ErrorContains(t, m.Validate(), "name")
	m.Name = "weather"
	require.NoError(t, m.Validate())
}
