package plugin

// metadata_source.go - asking the metadata plugins about an item.
//
// A plugin with the metadata.provider capability is sent the event
// "metadata.request" and answers in OnEvent's result. No new RPC: every SDK
// already serves OnEvent, so a plugin in any of its languages can answer.
//
// Request payload:
//
//	{"media_item_id", "type": "movie"|"series", "title", "original_title",
//	 "year" (absent when unknown), "provider_ids": {"tmdb", "imdb", ...},
//	 "language": the locale of the TMDB text, "locales": the server's locales}
//
// Answer (handled true):
//
//	{"locale": "fr", "title", "overview", "tagline",
//	 "translations": {"de": {"title", "overview", "tagline"}, ...}}
//
// Top-level fields are for "locale", or for the request's "language" when the
// answer names none; "translations" adds other locales. A field left out or
// empty is one the plugin does not cover. handled false, or an answer with no
// text, means the plugin knows nothing of the item.

import (
	"context"
	"log/slog"
	"slices"
	"strings"
	"time"

	"github.com/eveyproject/evey/internal/domain/metadata"
	"github.com/eveyproject/evey/internal/domain/plugin"
)

// MetadataRequestEvent is the event a metadata plugin answers.
const MetadataRequestEvent = "metadata.request"

// metadataAskTimeout is how long one plugin may take to answer: it goes out to
// its own service, which a notification never does.
const metadataAskTimeout = 15 * time.Second

// pluginWaker resumes a plugin the low-memory profile suspended
// (IdleManager).
type pluginWaker interface {
	EnsureStarted(ctx context.Context, pluginID string)
	Touch(pluginID string)
}

// MetadataSource is metadata.PluginMetadataSource over the installed plugins.
type MetadataSource struct {
	plugins plugin.PluginRepository
	runner  plugin.PluginRunner
	waker   pluginWaker
	timeout time.Duration
}

// NewMetadataSource builds the source; waker may be nil (no low-memory
// profile).
func NewMetadataSource(plugins plugin.PluginRepository, runner plugin.PluginRunner, waker pluginWaker) *MetadataSource {
	return &MetadataSource{plugins: plugins, runner: runner, waker: waker, timeout: metadataAskTimeout}
}

// SetWaker sets the low-memory profile's waker after construction: the
// enricher needs the source before the plugin subsystem is built. Wire-time
// only, before any enrichment runs.
func (m *MetadataSource) SetWaker(w pluginWaker) { m.waker = w }

// LookupTexts is metadata.PluginMetadataSource: each enabled, running
// metadata plugin in turn, in id order.
func (m *MetadataSource) LookupTexts(ctx context.Context, req metadata.PluginTextRequest) []metadata.PluginTextAnswer {
	providers := m.providers(ctx)
	if len(providers) == 0 {
		return nil
	}
	payload := metadataRequestPayload(req)
	var answers []metadata.PluginTextAnswer
	for _, p := range providers {
		if answer, ok := m.ask(ctx, p, payload, req); ok {
			answers = append(answers, answer)
		}
	}
	return answers
}

// LookupTextsFrom is metadata.PluginMetadataSource: one plugin, when it is an
// enabled, running metadata plugin.
func (m *MetadataSource) LookupTextsFrom(ctx context.Context, pluginID string, req metadata.PluginTextRequest) (metadata.PluginTextAnswer, bool) {
	for _, p := range m.providers(ctx) {
		if p.ID == pluginID {
			return m.ask(ctx, p, metadataRequestPayload(req), req)
		}
	}
	return metadata.PluginTextAnswer{}, false
}

// providers lists the enabled, running metadata plugins in id order.
func (m *MetadataSource) providers(ctx context.Context) []*plugin.Plugin {
	all, err := m.plugins.List(ctx)
	if err != nil {
		slog.WarnContext(ctx, "metadata plugins: list plugins", slog.Any("err", err))
		return nil
	}
	providers := make([]*plugin.Plugin, 0, len(all))
	for _, p := range all {
		if p.Enabled && p.Status == plugin.Running && slices.Contains(p.Capabilities, plugin.CapabilityMetadataProvider) {
			providers = append(providers, p)
		}
	}
	slices.SortFunc(providers, func(a, b *plugin.Plugin) int { return strings.Compare(a.ID, b.ID) })
	return providers
}

// ask sends one plugin the request; ok is false when it did not answer.
func (m *MetadataSource) ask(ctx context.Context, p *plugin.Plugin, payload map[string]any, req metadata.PluginTextRequest) (metadata.PluginTextAnswer, bool) {
	if m.waker != nil {
		m.waker.EnsureStarted(ctx, p.ID)
		m.waker.Touch(p.ID)
	}
	result, handled, err := m.runner.Ask(ctx, p.ID, MetadataRequestEvent, payload, m.timeout)
	if err != nil {
		slog.WarnContext(ctx, "metadata plugin did not answer",
			slog.String("plugin", p.Name), slog.String("media_item_id", req.MediaItemID), slog.Any("err", err))
		return metadata.PluginTextAnswer{}, false
	}
	answer := metadata.PluginTextAnswer{PluginID: p.ID}
	if handled {
		answer.Texts = parseMetadataAnswer(result, req.Language)
	}
	return answer, true
}

// metadataRequestPayload is the request as the plugin reads it.
func metadataRequestPayload(req metadata.PluginTextRequest) map[string]any {
	ids := make(map[string]any, len(req.ProviderIDs))
	for k, v := range req.ProviderIDs {
		if v != "" {
			ids[k] = v
		}
	}
	locales := make([]any, 0, len(req.Locales))
	for _, l := range req.Locales {
		locales = append(locales, l)
	}
	p := map[string]any{
		"media_item_id":  req.MediaItemID,
		"type":           req.Type,
		"title":          req.Title,
		"original_title": req.OriginalTitle,
		"provider_ids":   ids,
		"language":       req.Language,
		"locales":        locales,
	}
	if req.Year > 0 {
		p["year"] = float64(req.Year)
	}
	return p
}

// parseMetadataAnswer reads a plugin's answer, one text per locale with at
// least one field. Anything not shaped as documented is ignored, not an
// error: a plugin's answer is data from outside.
func parseMetadataAnswer(result map[string]any, language string) []metadata.PluginLocaleText {
	byLocale := map[string]metadata.PluginLocaleText{}
	add := func(locale string, fields map[string]any) {
		locale = normalizeLocale(locale)
		if locale == "" {
			return
		}
		t := byLocale[locale]
		t.Locale = locale
		if s := answerString(fields, "title"); s != "" {
			t.Title = s
		}
		if s := answerString(fields, "overview"); s != "" {
			t.Overview = s
		}
		if s := answerString(fields, "tagline"); s != "" {
			t.Tagline = s
		}
		byLocale[locale] = t
	}
	top := answerString(result, "locale")
	if top == "" {
		top = language
	}
	add(top, result)
	if tr, ok := result["translations"].(map[string]any); ok {
		for locale, v := range tr {
			if fields, ok := v.(map[string]any); ok {
				add(locale, fields)
			}
		}
	}
	out := make([]metadata.PluginLocaleText, 0, len(byLocale))
	for _, t := range byLocale {
		if t.Title != "" || t.Overview != "" || t.Tagline != "" {
			out = append(out, t)
		}
	}
	slices.SortFunc(out, func(a, b metadata.PluginLocaleText) int { return strings.Compare(a.Locale, b.Locale) })
	return out
}

// maxAnswerField bounds one field of an answer: an overview is a paragraph, a
// plugin sending megabytes is not a metadata source.
const maxAnswerField = 8 << 10

func answerString(m map[string]any, key string) string {
	s, _ := m[key].(string)
	s = strings.TrimSpace(s)
	if len(s) > maxAnswerField {
		return ""
	}
	return s
}

// normalizeLocale keeps the language of a locale tag, the form the server's
// locales take ("fr-FR" and "fr_FR" read as "fr").
func normalizeLocale(l string) string {
	l = strings.ToLower(strings.TrimSpace(l))
	if i := strings.IndexAny(l, "-_"); i >= 0 {
		l = l[:i]
	}
	if len(l) < 2 || len(l) > 3 {
		return ""
	}
	return l
}
