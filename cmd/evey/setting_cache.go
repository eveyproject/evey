package main

import (
	"context"
	"time"

	domainshared "github.com/eveyproject/evey/internal/domain/shared"
	pkgcache "github.com/eveyproject/evey/internal/pkg/cache"
)

// settingGetter is the one read the cache needs from the server-config store.
type settingGetter interface {
	Get(ctx context.Context, key string) (*domainshared.ServerConfigEntry, error)
}

// cachedBoolSetting reads a boolean server_config key through a short cache:
// a knob the HLS path consults two to three times per segment (fingerprint,
// delivery, on-demand resource) was costing ~30 SELECTs per ten segments. A
// read error is not cached, so the next call asks again. stop releases the
// cache's sweeper.
func cachedBoolSetting(repo settingGetter, key string, ttl time.Duration) (get func(context.Context) bool, stop func()) {
	cache, stop := pkgcache.New[bool](time.Minute)
	get = func(ctx context.Context) bool {
		if v, ok := cache.Get(key); ok {
			return v
		}
		entry, err := repo.Get(ctx, key)
		if err != nil {
			return false
		}
		v := entry.Value == "true"
		cache.Set(key, v, ttl)
		return v
	}
	return get, stop
}
