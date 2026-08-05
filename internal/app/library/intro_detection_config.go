package library

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/evey-project/evey/internal/domain/shared"
)

// IntroDetectionConfigKey is the server_config key under which the JSON
// blob persists. Admins can set/get/delete via the existing
// SetServerConfigEntry / GetServerConfigEntry endpoints.
const IntroDetectionConfigKey = "intro_detection.config"

// introConfig holds every tunable threshold for the intro-detection pipeline.
// Defaults preserve the values previously hardcoded; admins can override any
// subset by writing a JSON object to server_config under
// IntroDetectionConfigKey. Missing JSON fields fall back to defaults so a
// partial override is fine.
type introConfig struct {
	// pHash matching
	MaxHamming       int     `json:"max_hamming"`        // bit distance tolerated when matching frame hashes (default 8)
	FreqMapThreshold float64 `json:"freq_map_threshold"` // fraction of episodes a hash must appear in to be considered an intro frame (default 0.50)

	// Audio confirmation
	PadSec        float64 `json:"pad_sec"`        // padding around target candidate window for audio extraction (default 10)
	SearchSec     float64 `json:"search_sec"`     // sibling search window pad to absorb intro position drift (default 30)
	BinMs         int     `json:"bin_ms"`         // RMS/Goertzel bin width in ms (default 250)
	NumBands      int     `json:"num_bands"`      // number of spectral bands per bin (default 12)
	CorrThreshold float64 `json:"corr_threshold"` // minimum Pearson correlation per sibling (default 0.45)
	DriftTolSec   float64 `json:"drift_tol_sec"`  // max distance from median drift to count a sibling as coherent (default 6)
	MinMatches    int     `json:"min_matches"`    // minimum siblings with corr ≥ threshold (default 3)
	MinRatio      float64 `json:"min_ratio"`      // minimum coherent/total ratio (default 0.6)

	// Window classification
	RecapMaxMs int64 `json:"recap_max_ms"` // max duration of an early window classified as a recap (default 60_000)

	// Frame range
	FrameCutoffPercent float64 `json:"frame_cutoff_percent"` // fraction of the episode searched for the intro (default 0.25)
	MaxIntroStartMs    int64   `json:"max_intro_start_ms"`   // reject candidates whose start ms is past this point (default 240_000 = 4min) - beyond that it's not an intro, it's a recurring chapter card or in-episode element

	// Season clustering (v2): fingerprint each episode once and cluster the
	// recurring intro shared across the season, located at each episode's own
	// offset. Replaces per-episode audio discovery when it carries the episode.
	SeasonClusterEnabled bool `json:"season_cluster_enabled"` // default true

	// MinServeConfidence is the serving-side abstention gate: segments below
	// this confidence are withheld from players (QueryService.GetSegments), so
	// a coin-flip detection never surfaces as a skip button. A wrong skip
	// (jumping over content) costs far more than a missing button. 0 disables
	// the gate. The default cuts exactly the last-resort silence fallback
	// (source "audio", confidence 0.5) while keeping every measured tier
	// (blackdetect 0.6 and up). Detection still STORES low-confidence segments -
	// they stay available for eval/training - only serving is gated.
	MinServeConfidence float64 `json:"min_serve_confidence"` // default 0.55

	// AudioDiscoveryEnabled gates the per-episode Chromaprint discovery fallback
	// (discoverIntroByAudio). Demoted to OFF by default: an eval against
	// community ground truth (tidb/introdb) showed it frequently locks onto the
	// WRONG recurring segment - a "previously on" recap or a mid-episode musical
	// sting - landing intros ~65s off on BOTH edges (start +65s, end +69s median).
	// The season cluster (SeasonClusterEnabled) covers its useful "variable
	// teaser" cases more robustly, and a wrong skip button is worse than none
	// (same rationale as the heuristic-floor guard). Re-enable explicitly to
	// restore the old behaviour.
	AudioDiscoveryEnabled bool `json:"audio_discovery_enabled"` // default false
}

// defaultIntroConfig returns the values previously hardcoded throughout the
// pipeline. Any field omitted from the persisted JSON falls back to one of
// these.
//
// FreqMapThreshold = 0.30: a frame is considered intro-candidate if it
// appears in ≥30% of the season's episodes. Picked empirically - at 0.50
// the Robin Hood S1 intro was missed because only 3 of 6 episodes carry it
// (cold-open variability). The audio confirmation pass (CorrThreshold +
// MinRatio) downstream still filters false positives, so a permissive
// visual threshold here is safe.
//
// MinRatio = 0.7: bumped from 0.6 after observing accepted candidates with
// drift_values spanning ±18s (e.g. [0.25 -13.5 4 -8 -3.5 2.75 -4 -2 2 6.5
// -10.25] → coherent=8/11=0.72 passes 0.6 but is clearly noise). 0.7
// requires the median drift to actually represent the bulk of siblings.
//
// MaxIntroStartMs = 240_000 (4 min): true intros sit near the start of the
// episode. Anything detected past 4 minutes is much more likely a
// recurring in-episode visual element (chapter card, flashback, recap
// segment) than an intro. Prevents false positives like Gen V "Leçons de
// Cuisine" detected at 9:20-10:00.
func defaultIntroConfig() introConfig {
	return introConfig{
		MaxHamming:            8,
		FreqMapThreshold:      0.30,
		PadSec:                10.0,
		SearchSec:             30.0,
		BinMs:                 250,
		NumBands:              12,
		CorrThreshold:         0.45,
		DriftTolSec:           6.0,
		MinMatches:            3,
		MinRatio:              0.7,
		RecapMaxMs:            60 * 1000,
		FrameCutoffPercent:    0.25,
		MaxIntroStartMs:       240_000,
		MinServeConfidence:    0.55,
		SeasonClusterEnabled:  true,
		AudioDiscoveryEnabled: false,
	}
}

// loadIntroConfig fetches and parses the override JSON; on any error
// (missing key, parse failure, nil repo) it returns the defaults so the
// pipeline never fails because of a config issue. A parse error is logged
// once at WARN - not at every pipeline run, but the caller invokes this
// once per detect call so the cadence is acceptable.
func loadIntroConfig(ctx context.Context, repo shared.ServerConfigRepository) introConfig {
	cfg := defaultIntroConfig()
	if repo == nil {
		return cfg
	}
	entry, err := repo.Get(ctx, IntroDetectionConfigKey)
	if err != nil || entry == nil || entry.Value == "" {
		return cfg
	}
	// Unmarshal into a copy so a partial JSON only overrides the listed fields.
	override := cfg
	if err := json.Unmarshal([]byte(entry.Value), &override); err != nil {
		slog.WarnContext(ctx, "intro_detection: invalid config JSON, using defaults",
			slog.Any("err", err),
		)
		return cfg
	}
	return override
}
