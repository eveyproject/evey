package library

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sharedfakes "github.com/evey-project/evey/internal/domain/shared/fakes"
)

func TestLoadIntroConfig_NilRepo_ReturnsDefaults(t *testing.T) {
	t.Parallel()
	cfg := loadIntroConfig(context.Background(), nil)
	assert.Equal(t, defaultIntroConfig(), cfg)
}

func TestLoadIntroConfig_KeyAbsent_ReturnsDefaults(t *testing.T) {
	t.Parallel()
	repo := sharedfakes.NewFakeServerConfigRepository()
	cfg := loadIntroConfig(context.Background(), repo)
	assert.Equal(t, defaultIntroConfig(), cfg)
}

func TestLoadIntroConfig_FullOverride(t *testing.T) {
	t.Parallel()
	repo := sharedfakes.NewFakeServerConfigRepository()
	require.NoError(t, repo.Set(context.Background(), IntroDetectionConfigKey, `{
		"max_hamming": 12,
		"freq_map_threshold": 0.7,
		"pad_sec": 5,
		"search_sec": 20,
		"bin_ms": 500,
		"num_bands": 6,
		"corr_threshold": 0.6,
		"drift_tol_sec": 3,
		"min_matches": 2,
		"min_ratio": 0.4,
		"recap_max_ms": 90000,
		"frame_cutoff_percent": 0.4
	}`))
	cfg := loadIntroConfig(context.Background(), repo)
	assert.Equal(t, 12, cfg.MaxHamming)
	assert.Equal(t, 0.7, cfg.FreqMapThreshold)
	assert.Equal(t, 5.0, cfg.PadSec)
	assert.Equal(t, 20.0, cfg.SearchSec)
	assert.Equal(t, 500, cfg.BinMs)
	assert.Equal(t, 6, cfg.NumBands)
	assert.Equal(t, 0.6, cfg.CorrThreshold)
	assert.Equal(t, 3.0, cfg.DriftTolSec)
	assert.Equal(t, 2, cfg.MinMatches)
	assert.Equal(t, 0.4, cfg.MinRatio)
	assert.Equal(t, int64(90_000), cfg.RecapMaxMs)
	assert.Equal(t, 0.4, cfg.FrameCutoffPercent)
}

func TestLoadIntroConfig_PartialOverride_MissingFieldsKeepDefaults(t *testing.T) {
	t.Parallel()
	repo := sharedfakes.NewFakeServerConfigRepository()
	require.NoError(t, repo.Set(context.Background(), IntroDetectionConfigKey, `{"max_hamming": 14, "min_ratio": 0.8}`))
	cfg := loadIntroConfig(context.Background(), repo)
	def := defaultIntroConfig()
	assert.Equal(t, 14, cfg.MaxHamming)
	assert.Equal(t, 0.8, cfg.MinRatio)
	// Untouched fields keep defaults
	assert.Equal(t, def.FreqMapThreshold, cfg.FreqMapThreshold)
	assert.Equal(t, def.PadSec, cfg.PadSec)
	assert.Equal(t, def.RecapMaxMs, cfg.RecapMaxMs)
}

func TestLoadIntroConfig_InvalidJSON_ReturnsDefaults(t *testing.T) {
	t.Parallel()
	repo := sharedfakes.NewFakeServerConfigRepository()
	require.NoError(t, repo.Set(context.Background(), IntroDetectionConfigKey, `{not valid json`))
	cfg := loadIntroConfig(context.Background(), repo)
	assert.Equal(t, defaultIntroConfig(), cfg)
}

func TestLoadIntroConfig_EmptyValue_ReturnsDefaults(t *testing.T) {
	t.Parallel()
	repo := sharedfakes.NewFakeServerConfigRepository()
	require.NoError(t, repo.Set(context.Background(), IntroDetectionConfigKey, ""))
	cfg := loadIntroConfig(context.Background(), repo)
	assert.Equal(t, defaultIntroConfig(), cfg)
}

// The silence fallback is COMPUTED and stored by default, whatever the serving
// gate withholds: the saving is opt-in, never the other way round (user
// decision 2026-08-22 - a value the sweep already paid for is eval material,
// and recovering it later costs a full re-sweep).
func TestDefaultIntroConfig_ComputesTheSilenceFallbackByDefault(t *testing.T) {
	t.Parallel()
	def := defaultIntroConfig()
	assert.False(t, def.SkipUnservableSilenceFallback)
	assert.True(t, shouldRunSilenceFallback(def))
}

func TestLoadIntroConfig_SkipUnservableSilenceFallbackOverride(t *testing.T) {
	t.Parallel()
	repo := sharedfakes.NewFakeServerConfigRepository()
	require.NoError(t, repo.Set(context.Background(), IntroDetectionConfigKey,
		`{"skip_unservable_silence_fallback": true}`))
	cfg := loadIntroConfig(context.Background(), repo)
	assert.True(t, cfg.SkipUnservableSilenceFallback)
	assert.False(t, shouldRunSilenceFallback(cfg),
		"opted in, the 0.5 answer the 0.55 gate withholds is not worth its scan")
}

// Guard on the FAKE, not on the code: fakeConfigRepo once answered the same
// trickplay entry to every key, which silently made config-driven branches
// untestable. Every cost test reads the new key through it.
func TestFakeConfigRepo_HonoursTheIntroConfigKey(t *testing.T) {
	t.Parallel()
	cfg := loadIntroConfig(context.Background(), &fakeConfigRepo{values: map[string]string{
		IntroDetectionConfigKey: `{"skip_unservable_silence_fallback": true}`,
	}})
	assert.True(t, cfg.SkipUnservableSilenceFallback)
}
