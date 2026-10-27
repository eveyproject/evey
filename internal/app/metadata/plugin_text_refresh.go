package metadata

// plugin_text_refresh.go - asking a metadata plugin about the library it found
// when it was installed.
//
// New items are asked about as they are enriched (Enricher.enrichPluginTexts).
// The movies and series already there are not, until an administrator starts
// this walk: one plugin, one item after the other (the plugin's own service is
// the bottleneck, and most limit their callers), in the background, its
// progress kept in memory.

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	domainlib "github.com/eveyproject/evey/internal/domain/library"
)

// ErrPluginRefreshRunning refuses a second walk for a plugin already walking.
var ErrPluginRefreshRunning = errors.New("a metadata refresh is already running for this plugin")

// PluginTextRefreshStatus is where a plugin's walk stands.
type PluginTextRefreshStatus struct {
	Running    bool
	Done       int
	Total      int
	Covered    int
	Failed     int
	FinishedAt time.Time
}

// pluginTextRefreshPage is how many items a page of the walk reads.
const pluginTextRefreshPage = 200

// PluginTextRefresher walks the library for one plugin at a time.
type PluginTextRefresher struct {
	e        *Enricher
	parent   context.Context
	pageSize int
	mu       sync.Mutex
	statuses map[string]*PluginTextRefreshStatus
}

// NewPluginTextRefresher builds the refresher; parent is the server's
// lifetime, which cancels a walk at shutdown.
func NewPluginTextRefresher(parent context.Context, e *Enricher) *PluginTextRefresher {
	return &PluginTextRefresher{e: e, parent: parent, pageSize: pluginTextRefreshPage, statuses: map[string]*PluginTextRefreshStatus{}}
}

// Status is the plugin's walk since the server started, the zero status when
// none ran.
func (r *PluginTextRefresher) Status(pluginID string) PluginTextRefreshStatus {
	r.mu.Lock()
	defer r.mu.Unlock()
	if s := r.statuses[pluginID]; s != nil {
		return *s
	}
	return PluginTextRefreshStatus{}
}

// Start begins the plugin's walk in the background.
func (r *PluginTextRefresher) Start(pluginID string) error {
	if r.e.pluginSource == nil || r.e.pluginTexts == nil {
		return errors.New("metadata plugins are not wired")
	}
	r.mu.Lock()
	if s := r.statuses[pluginID]; s != nil && s.Running {
		r.mu.Unlock()
		return ErrPluginRefreshRunning
	}
	status := &PluginTextRefreshStatus{Running: true}
	r.statuses[pluginID] = status
	r.mu.Unlock()
	go r.walk(pluginID, status)
	return nil
}

// update changes the status under the lock.
func (r *PluginTextRefresher) update(fn func()) {
	r.mu.Lock()
	fn()
	r.mu.Unlock()
}

func (r *PluginTextRefresher) walk(pluginID string, status *PluginTextRefreshStatus) {
	ctx := r.parent
	defer r.update(func() {
		status.Running = false
		status.FinishedAt = time.Now()
	})
	types := []domainlib.MediaItemType{domainlib.MovieItem, domainlib.Series}
	total := 0
	for _, typ := range types {
		t := string(typ)
		n, err := r.e.mediaItems.Count(ctx, domainlib.ListMediaItemsFilter{Type: &t})
		if err != nil {
			slog.WarnContext(ctx, "metadata plugin refresh: count items", slog.String("plugin", pluginID), slog.Any("err", err))
			return
		}
		total += n
	}
	r.update(func() { status.Total = total })
	slog.InfoContext(ctx, "metadata plugin refresh started", slog.String("plugin", pluginID), slog.Int("items", total))
	for _, typ := range types {
		t := string(typ)
		after := ""
		for {
			if ctx.Err() != nil {
				return
			}
			page, err := r.e.mediaItems.List(ctx, domainlib.ListMediaItemsFilter{Type: &t, Limit: r.pageSize, AfterID: after, KeysetByID: true})
			if err != nil {
				slog.WarnContext(ctx, "metadata plugin refresh: list items", slog.String("plugin", pluginID), slog.Any("err", err))
				return
			}
			for _, item := range page {
				if ctx.Err() != nil {
					return
				}
				answer, ok := r.e.pluginSource.LookupTextsFrom(ctx, pluginID, r.e.pluginTextRequest(item))
				stored := ok && r.e.storePluginAnswer(ctx, string(item.ID), answer)
				r.update(func() {
					status.Done++
					switch {
					case !stored:
						status.Failed++
					case len(answer.Texts) > 0:
						status.Covered++
					}
				})
			}
			if len(page) < r.pageSize {
				break
			}
			after = string(page[len(page)-1].ID)
		}
	}
	final := r.Status(pluginID)
	slog.InfoContext(ctx, "metadata plugin refresh finished", slog.String("plugin", pluginID),
		slog.Int("items", final.Done), slog.Int("covered", final.Covered), slog.Int("failed", final.Failed))
}
