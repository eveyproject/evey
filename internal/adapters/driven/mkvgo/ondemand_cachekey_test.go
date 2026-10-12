package mkvgo

import (
	"testing"

	"github.com/stretchr/testify/assert"

	apptranscoding "github.com/eveyproject/evey/internal/app/transcoding"
)

// The plan cache must not alias plans that produce different bytes. Encrypted
// segments depend on the exact AES key, so two keys for the same source (e.g.
// after a secret rotation) must yield distinct cache keys.
func TestOnDemandCacheKey_DistinguishesAESKeys(t *testing.T) {
	src := "/media/movie.mkv" // stat fails → size/mtime 0; we only assert on the key-tag difference
	plain := onDemandCacheKey(src, apptranscoding.OnDemandOptions{SegmentMs: 6000, URLPrefix: "od_"})
	encA := onDemandCacheKey(src, apptranscoding.OnDemandOptions{
		SegmentMs: 6000, URLPrefix: "od_", AESKey: []byte("aaaaaaaaaaaaaaaa"), AESKeyURI: "key",
	})
	encB := onDemandCacheKey(src, apptranscoding.OnDemandOptions{
		SegmentMs: 6000, URLPrefix: "od_", AESKey: []byte("bbbbbbbbbbbbbbbb"), AESKeyURI: "key",
	})

	assert.NotEqual(t, plain, encA, "encrypted and plaintext plans must not alias")
	assert.NotEqual(t, encA, encB, "different AES keys for the same source must not alias")

	// Determinism: same inputs → same key (cache hit).
	assert.Equal(t, encA, onDemandCacheKey(src, apptranscoding.OnDemandOptions{
		SegmentMs: 6000, URLPrefix: "od_", AESKey: []byte("aaaaaaaaaaaaaaaa"), AESKeyURI: "key",
	}), "same options → same cache key")
}

// A rotated presentation encrypts each period with a different key, so its plan
// must not alias onto the single-key plan or onto a different rotation schedule.
func TestOnDemandCacheKey_DistinguishesRotation(t *testing.T) {
	src := "/media/movie.mkv"
	single := onDemandCacheKey(src, apptranscoding.OnDemandOptions{
		SegmentMs: 6000, URLPrefix: "od_", AESKey: []byte("aaaaaaaaaaaaaaaa"), AESKeyURI: "key",
	})
	rot := func(every int, keys ...string) string {
		ks := make([]apptranscoding.OnDemandAESKey, len(keys))
		for i, k := range keys {
			ks[i] = apptranscoding.OnDemandAESKey{Key: []byte(k), URI: "key?i=" + string(rune('0'+i))}
		}
		return onDemandCacheKey(src, apptranscoding.OnDemandOptions{
			SegmentMs: 6000, URLPrefix: "od_", AESRotateSegments: every, AESKeys: ks,
		})
	}
	rotA := rot(10, "aaaaaaaaaaaaaaaa", "bbbbbbbbbbbbbbbb")
	rotDiffN := rot(20, "aaaaaaaaaaaaaaaa", "bbbbbbbbbbbbbbbb")
	rotDiffKeys := rot(10, "aaaaaaaaaaaaaaaa", "cccccccccccccccc")

	assert.NotEqual(t, single, rotA, "rotated plan must not alias the single-key plan")
	assert.NotEqual(t, rotA, rotDiffN, "different rotation period must not alias")
	assert.NotEqual(t, rotA, rotDiffKeys, "different key set must not alias")
	assert.Equal(t, rotA, rot(10, "aaaaaaaaaaaaaaaa", "bbbbbbbbbbbbbbbb"), "same rotation → same cache key")
}
