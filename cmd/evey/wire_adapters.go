package main

import (
	"context"

	apploadtest "github.com/eveyproject/evey/internal/app/loadtest"

	"github.com/eveyproject/evey/internal/adapters/driven/postgres"
	applibrary "github.com/eveyproject/evey/internal/app/library"
	appuser "github.com/eveyproject/evey/internal/app/user"
	"github.com/eveyproject/evey/internal/domain/library"
	domainplugin "github.com/eveyproject/evey/internal/domain/plugin"
	"github.com/eveyproject/evey/internal/domain/shared"
	domainuser "github.com/eveyproject/evey/internal/domain/user"
)

// wire_adapters.go holds the small local adapter types that bridge one app
// service's port to another's concrete type at composition time.

// pluginIDLister adapts postgres.PluginRepository to pluginrunner.PluginLister.
type pluginIDLister struct {
	repo interface {
		List(ctx context.Context) ([]*domainplugin.Plugin, error)
	}
}

func (a *pluginIDLister) ListIDs(ctx context.Context) ([]string, error) {
	plugins, err := a.repo.List(ctx)
	if err != nil {
		return nil, err
	}
	ids := make([]string, len(plugins))
	for i, p := range plugins {
		ids[i] = p.ID
	}
	return ids, nil
}

// musicTrackCheckerAdapter adapts the music TrackRepository to playlist.MusicTrackChecker.
// Postgres repo returns (nil, nil) when not found; we surface that as ok=false.
type musicTrackCheckerAdapter struct {
	repo *postgres.MusicTrackRepository
}

func (a musicTrackCheckerAdapter) Exists(ctx context.Context, id string) (bool, error) {
	t, err := a.repo.GetByID(ctx, id)
	if err != nil {
		return false, err
	}
	return t != nil, nil
}

// mediaItemCheckerAdapter adapts the library MediaItemQueryRepo (typed id) to
// playlist.MediaItemExistsChecker (string id). It returns the item so AddItem
// can read its LibraryID for the library-access gate.
type mediaItemCheckerAdapter struct {
	repo library.MediaItemQueryRepo
}

func (a mediaItemCheckerAdapter) GetByID(ctx context.Context, id string) (*library.MediaItem, error) {
	return a.repo.GetByID(ctx, shared.MediaItemID(id))
}

// playlistLibraryAccessAdapter implements playlist.PlaylistLibraryAccessChecker:
// it resolves the profile to its owning user, then consults library_access.
type playlistLibraryAccessAdapter struct {
	profiles domainuser.ProfileRepository
	access   domainuser.LibraryAccessRepository
}

func (a playlistLibraryAccessAdapter) HasAccessForProfile(ctx context.Context, profileID shared.ProfileID, libraryID shared.LibraryID) (bool, error) {
	p, err := a.profiles.GetByID(ctx, profileID)
	if err != nil {
		return false, err
	}
	return a.access.HasAccess(ctx, p.UserID, libraryID)
}

// mediaItemDurationGetter adapts the cached MediaItem query repo to the
// playback.MediaItemDurationGetter port. Returns 0 when the item has no
// file (container types) or the file probe never recorded a duration -
// the playback command service skips auto-completion in that case.
type mediaItemDurationGetter struct {
	cache *applibrary.CachedMediaItemQueryRepo
}

func (g mediaItemDurationGetter) GetMediaItemDurationMs(ctx context.Context, mediaItemID string) (int64, error) {
	item, err := g.cache.GetByID(ctx, shared.MediaItemID(mediaItemID))
	if err != nil || item == nil || item.File == nil {
		return 0, err
	}
	return item.File.Duration.Milliseconds(), nil
}

// compositeTokenRevoker layers stream-token purge on top of the JWT epoch
// bump. epoch is always present; purgeStreams is bound after the stream token
// store is constructed and stays nil-safe (a nil purge just preserves the
// epoch-only behaviour). Revoke bumps the epoch first (JWT kill) then cuts the
// user's live stream tokens - the order is not load-bearing, both are
// idempotent, but epoch-first keeps the fast JWT revocation on the critical
// path even if the stream purge ranges a large map.
type compositeTokenRevoker struct {
	epoch        appuser.TokenRevoker
	purgeStreams func(userID string) int
}

func (c *compositeTokenRevoker) Revoke(ctx context.Context, userID string) error {
	if err := c.epoch.Revoke(ctx, userID); err != nil {
		return err
	}
	if c.purgeStreams != nil {
		c.purgeStreams(userID)
	}
	return nil
}

// loadTestMediaListerFunc adapts a closure to apploadtest.MediaLister.
type loadTestMediaListerFunc func(ctx context.Context, limit int) ([]apploadtest.MediaCandidate, error)

func (f loadTestMediaListerFunc) ListLoadTestCandidates(ctx context.Context, limit int) ([]apploadtest.MediaCandidate, error) {
	return f(ctx, limit)
}
