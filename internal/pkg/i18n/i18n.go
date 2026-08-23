package i18n

import (
	"context"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

type localeKey struct{}

const FallbackLocale = "en"

var supportedLocales = map[string]bool{
	"en": true, "de": true, "es": true, "fr": true, "hi": true, "zh": true,
}

func SupportedLocales() []string {
	out := make([]string, 0, len(supportedLocales))
	for l := range supportedLocales {
		out = append(out, l)
	}
	return out
}

type Translator struct {
	mu       sync.RWMutex
	messages map[string]map[string]string // locale -> flat key -> message
	fallback string
}

// NewEmpty returns a translator with no messages - T() returns keys as-is.
func NewEmpty() *Translator {
	return &Translator{
		messages: map[string]map[string]string{"en": {}},
		fallback: FallbackLocale,
	}
}

func NewFromDir(dir string) (*Translator, error) {
	t := &Translator{
		messages: make(map[string]map[string]string),
		fallback: FallbackLocale,
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("i18n: read dir %s: %w", dir, err)
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		locale := entry.Name()
		localeDir := filepath.Join(dir, locale)
		files, err := os.ReadDir(localeDir)
		if err != nil {
			return nil, fmt.Errorf("i18n: read locale dir %s: %w", localeDir, err)
		}
		for _, f := range files {
			if f.IsDir() || !strings.HasSuffix(f.Name(), ".yaml") {
				continue
			}
			data, err := os.ReadFile(filepath.Join(localeDir, f.Name()))
			if err != nil {
				return nil, fmt.Errorf("i18n: read file %s/%s: %w", locale, f.Name(), err)
			}
			var raw map[string]any
			if err := yaml.Unmarshal(data, &raw); err != nil {
				// Fail-loud: a silently-skipped file (e.g. duplicate map key)
				// drops the entire namespace from every locale, which surfaces
				// to users as raw key strings on every screen using it. The
				// previous warn-and-continue path hid a real production
				// regression for hours - refuse to boot instead.
				return nil, fmt.Errorf("i18n: invalid YAML %s/%s: %w", locale, f.Name(), err)
			}
			if t.messages[locale] == nil {
				t.messages[locale] = make(map[string]string)
			}
			stem := strings.TrimSuffix(f.Name(), ".yaml")
			flatten(stem, raw, t.messages[locale])
		}
	}
	if _, ok := t.messages[FallbackLocale]; !ok {
		return nil, fmt.Errorf("i18n: fallback locale %q not found in %s", FallbackLocale, dir)
	}
	return t, nil
}

func flatten(prefix string, m map[string]any, out map[string]string) {
	for k, v := range m {
		key := k
		if prefix != "" {
			key = prefix + "." + k
		}
		switch val := v.(type) {
		case string:
			out[key] = val
		case map[string]any:
			flatten(key, val, out)
		}
	}
}

func (t *Translator) AllMessages(locale string) map[string]string {
	t.mu.RLock()
	defer t.mu.RUnlock()

	result := make(map[string]string)
	if fb, ok := t.messages[t.fallback]; ok {
		maps.Copy(result, fb)
	}
	if locale != t.fallback {
		if msgs, ok := t.messages[locale]; ok {
			maps.Copy(result, msgs)
		}
	}
	return result
}

func (t *Translator) T(locale, key string, params ...map[string]string) string {
	if t == nil {
		return key
	}
	t.mu.RLock()
	defer t.mu.RUnlock()

	msg := t.lookup(locale, key)
	if len(params) > 0 && params[0] != nil {
		for pk, pv := range params[0] {
			msg = strings.ReplaceAll(msg, "{"+pk+"}", pv)
		}
	}
	return msg
}

// Loaded reports whether this translator holds a real catalogue. An empty one
// (NewEmpty, installed when the load fails) answers every key with the key
// itself, so the whole product renders "errors.auth.invalid_pin" where a
// sentence belongs - the beta.38 raw-key regression, catalogue-wide. The boot
// path uses this to say so out loud instead of leaving it to a screenshot.
func (t *Translator) Loaded() bool {
	if t == nil {
		return false
	}
	t.mu.RLock()
	defer t.mu.RUnlock()
	return len(t.messages[t.fallback]) > 0
}

func (t *Translator) lookup(locale, key string) string {
	if msgs, ok := t.messages[locale]; ok {
		if msg, ok := msgs[key]; ok {
			return msg
		}
	}
	if locale != t.fallback {
		if msgs, ok := t.messages[t.fallback]; ok {
			if msg, ok := msgs[key]; ok {
				return msg
			}
		}
	}
	return key
}

func ContextWithLocale(ctx context.Context, locale string) context.Context {
	return context.WithValue(ctx, localeKey{}, locale)
}

func LocaleFromCtx(ctx context.Context) string {
	v, _ := ctx.Value(localeKey{}).(string)
	if v == "" {
		return FallbackLocale
	}
	return v
}

func ParseLocale(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return FallbackLocale
	}
	// Accept-Language can be "fr-FR, en;q=0.9" - take first tag, strip region
	parts := strings.Split(raw, ",")
	tag := strings.TrimSpace(parts[0])
	tag = strings.SplitN(tag, ";", 2)[0]
	tag = strings.SplitN(tag, "-", 2)[0]
	tag = strings.ToLower(strings.TrimSpace(tag))
	if supportedLocales[tag] {
		return tag
	}
	return FallbackLocale
}
