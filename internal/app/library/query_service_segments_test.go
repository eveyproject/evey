package library

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	domainlibrary "github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/shared"
	sharedfakes "github.com/evey-project/evey/internal/domain/shared/fakes"
)

type fakeSegmentQueryRepo struct {
	segs []*domainlibrary.MediaSegment
}

func (f *fakeSegmentQueryRepo) GetByMediaItemID(_ context.Context, _ shared.MediaItemID) ([]*domainlibrary.MediaSegment, error) {
	return f.segs, nil
}

func (f *fakeSegmentQueryRepo) GetByMediaItemIDs(_ context.Context, _ []shared.MediaItemID) (map[shared.MediaItemID][]*domainlibrary.MediaSegment, error) {
	return nil, nil
}

func segQuerySvc(segs []*domainlibrary.MediaSegment, opts ...QueryServiceOption) *QueryService {
	opts = append([]QueryServiceOption{WithSegments(&fakeSegmentQueryRepo{segs: segs})}, opts...)
	return NewQueryService(nil, nil, nil, nil, opts...)
}

// The serving gate withholds low-confidence segments from players: a stored
// coin-flip detection (the silence fallback at 0.5) must not surface as a
// skip button, while every measured tier keeps flowing.
func TestGetSegments_WithholdsLowConfidence(t *testing.T) {
	t.Parallel()
	segs := []*domainlibrary.MediaSegment{
		{Type: "intro", StartMs: 10_000, EndMs: 40_000, Source: "audio_cluster", Confidence: 0.9},
		{Type: "intro", StartMs: 0, EndMs: 30_000, Source: "audio", Confidence: 0.5},
		{Type: "end_credits", StartMs: 2_400_000, EndMs: 2_580_000, Source: "blackdetect", Confidence: 0.6},
	}
	got, err := segQuerySvc(segs).GetSegments(context.Background(), "ep-1")
	require.NoError(t, err)
	require.Len(t, got, 2)
	for _, sg := range got {
		assert.GreaterOrEqual(t, sg.Confidence, 0.55)
	}
}

// min_serve_confidence: 0 disables the gate entirely - an admin can opt back
// into serving everything.
func TestGetSegments_ZeroThresholdServesEverything(t *testing.T) {
	t.Parallel()
	cfgRepo := sharedfakes.NewFakeServerConfigRepository()
	require.NoError(t, cfgRepo.Set(context.Background(), IntroDetectionConfigKey, `{"min_serve_confidence": 0}`))
	segs := []*domainlibrary.MediaSegment{
		{Type: "intro", StartMs: 0, EndMs: 30_000, Source: "audio", Confidence: 0.5},
	}
	got, err := segQuerySvc(segs, WithServerConfig(cfgRepo)).GetSegments(context.Background(), "ep-1")
	require.NoError(t, err)
	require.Len(t, got, 1)
}

// The threshold is admin-tunable upward too: raising it withholds tiers that
// the default lets through.
func TestGetSegments_RaisedThresholdWithholdsMore(t *testing.T) {
	t.Parallel()
	cfgRepo := sharedfakes.NewFakeServerConfigRepository()
	require.NoError(t, cfgRepo.Set(context.Background(), IntroDetectionConfigKey, `{"min_serve_confidence": 0.8}`))
	segs := []*domainlibrary.MediaSegment{
		{Type: "intro", StartMs: 10_000, EndMs: 40_000, Source: "audio_cluster", Confidence: 0.9},
		{Type: "end_credits", StartMs: 2_400_000, EndMs: 2_580_000, Source: "blackdetect", Confidence: 0.6},
	}
	got, err := segQuerySvc(segs, WithServerConfig(cfgRepo)).GetSegments(context.Background(), "ep-1")
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "audio_cluster", got[0].Source)
}

// Measured in prod 2026-08-22: 43.5% of the served end_credits start more than
// 4 minutes before the end of the file (median lead 162s, p90 472s, a pile of
// them at 600s - the scan bound), so players raise "skip the credits" in the
// middle of the content. A blackdetect credits row carries EndMs = file
// duration, so EndMs-StartMs IS the lead: the implausible ones can be refused
// at the single serving decision point, for the whole catalogue, with no extra
// query and no re-scan.
func TestGetSegments_WithholdsImplausiblyEarlyCredits(t *testing.T) {
	t.Parallel()
	const durMs = 42 * 60 * 1000 // 42-min episode
	segs := []*domainlibrary.MediaSegment{
		{Type: "end_credits", StartMs: durMs - 600_000, EndMs: durMs, Source: "blackdetect", Confidence: 0.6},
	}
	got, err := segQuerySvc(segs).GetSegments(context.Background(), "ep-1")
	require.NoError(t, err)
	assert.Empty(t, got, "credits claiming to start 10 minutes before the end of a 42-min episode are not credits")
}

// The bound is max(4 min, 12% of the runtime): a feature film really can roll
// 12 minutes of credits, and refusing those would trade one defect for another.
func TestGetSegments_KeepsLongCreditsOnFeatureFilm(t *testing.T) {
	t.Parallel()
	const durMs = 130 * 60 * 1000 // 2h10 film: 12% = 15min36
	segs := []*domainlibrary.MediaSegment{
		{Type: "end_credits", StartMs: durMs - 12*60*1000, EndMs: durMs, Source: "blackdetect", Confidence: 0.6},
	}
	got, err := segQuerySvc(segs).GetSegments(context.Background(), "ep-1")
	require.NoError(t, err)
	require.Len(t, got, 1, "12 minutes of credits on a 2h10 film stay plausible")
}

// Same knob shape as min_serve_confidence: admin-tunable, 0 disables the gate.
func TestGetSegments_ZeroCreditsLeadDisablesTheGate(t *testing.T) {
	t.Parallel()
	cfgRepo := sharedfakes.NewFakeServerConfigRepository()
	require.NoError(t, cfgRepo.Set(context.Background(), IntroDetectionConfigKey, `{"max_serve_credits_lead_ms": 0}`))
	const durMs = 42 * 60 * 1000
	segs := []*domainlibrary.MediaSegment{
		{Type: "end_credits", StartMs: durMs - 600_000, EndMs: durMs, Source: "blackdetect", Confidence: 0.6},
	}
	got, err := segQuerySvc(segs, WithServerConfig(cfgRepo)).GetSegments(context.Background(), "ep-1")
	require.NoError(t, err)
	require.Len(t, got, 1)
}

// An operator mark is ground truth, not a detector guess: the plausibility
// bound polices detectors, it does not overrule someone who watched the file.
func TestGetSegments_KeepsOperatorMarkedCredits(t *testing.T) {
	t.Parallel()
	const durMs = 42 * 60 * 1000
	segs := []*domainlibrary.MediaSegment{
		{Type: "end_credits", StartMs: durMs - 600_000, EndMs: durMs, Source: "manual", Confidence: 1.0},
	}
	got, err := segQuerySvc(segs).GetSegments(context.Background(), "ep-1")
	require.NoError(t, err)
	require.Len(t, got, 1)
}

// The span is the lead only for blackdetect, whose EndMs is the file duration.
// A chapter row can end before the file does, so EndMs-StartMs UNDERSTATES its
// lead and the bound cannot judge it without the real runtime - which this call
// does not load. Such a row is served rather than judged on a wrong number.
// Documents a known coverage hole, not a desirable outcome: 100% of the
// end_credits measured in prod are blackdetect, so nothing is lost today.
func TestGetSegments_LeavesNonBlackdetectCreditsToTheirOwnSource(t *testing.T) {
	t.Parallel()
	segs := []*domainlibrary.MediaSegment{
		// Span far past the bound, but EndMs is a chapter end, not the duration.
		{Type: "end_credits", StartMs: 600_000, EndMs: 1_800_000, Source: "mkv_chapter", Confidence: 0.9},
	}
	got, err := segQuerySvc(segs).GetSegments(context.Background(), "ep-1")
	require.NoError(t, err)
	require.Len(t, got, 1, "the gate must not judge a row whose EndMs is not the file duration")
}

// User decision 2026-08-06: below 5 seconds there is NO skip-intro button,
// whatever the source and however confident - a 3s mkv chapter labelled
// "intro" (9-1-1 S02E02, stored because chapters are exempt from the
// detection-side floor) must not reach a player. Enforced at the single
// serving decision point; the confidence gate being disabled does not
// disable it. Credits are out of scope: the rule is about the skip button.
func TestGetSegments_WithholdsSubFiveSecondIntros(t *testing.T) {
	t.Parallel()
	cfgRepo := sharedfakes.NewFakeServerConfigRepository()
	require.NoError(t, cfgRepo.Set(context.Background(), IntroDetectionConfigKey, `{"min_serve_confidence": 0}`))
	segs := []*domainlibrary.MediaSegment{
		{Type: "intro", StartMs: 389_000, EndMs: 392_000, Source: "mkv_chapter", Confidence: 1.0},
		{Type: "intro", StartMs: 10_000, EndMs: 40_000, Source: "audio_cluster", Confidence: 0.9},
		{Type: "end_credits", StartMs: 2_576_000, EndMs: 2_580_000, Source: "mkv_chapter", Confidence: 1.0},
	}
	got, err := segQuerySvc(segs, WithServerConfig(cfgRepo)).GetSegments(context.Background(), "ep-1")
	require.NoError(t, err)
	require.Len(t, got, 2, "the 3s chapter intro is withheld even with the confidence gate disabled")
	for _, sg := range got {
		if sg.Type == "intro" {
			assert.GreaterOrEqual(t, sg.EndMs-sg.StartMs, int64(5_000))
		}
	}
}

// Plancher de 12 s pour tout ce qui n'est PAS un chapitre MKV. Mesure du
// 2026-08-24 sur ce que la PROD sert vraiment (19 intros servies, dump lecture
// seule) : les trois plus courtes font 5 000, 5 347 et 10 000 ms et sont les
// TROIS faux positifs du gold (deux [tidb], un [phash]) ; la suivante fait
// 18 015 ms et est un
// vrai positif. La marge sure est donc [10 001, 18 014] ms - 12 000 est au
// milieu. Un chapitre MKV reste exempt : c'est le muxeur qui l'a ecrit, pas un
// detecteur qui a devine, et il garde le plancher absolu de 5 s (decision user
// du 2026-08-06).
func TestGetSegments_WithholdsShortDetectedIntrosButKeepsChapters(t *testing.T) {
	t.Parallel()
	cfgRepo := sharedfakes.NewFakeServerConfigRepository()
	require.NoError(t, cfgRepo.Set(context.Background(), IntroDetectionConfigKey, `{"min_serve_confidence": 0}`))
	segs := []*domainlibrary.MediaSegment{
		// Les trois faux positifs mesures en prod, a la milliseconde.
		{Type: "intro", StartMs: 0, EndMs: 5_000, Source: "tidb", Confidence: 0.97},
		{Type: "intro", StartMs: 3_000, EndMs: 8_347, Source: "phash", Confidence: 0.95},
		{Type: "intro", StartMs: 0, EndMs: 10_000, Source: "tidb", Confidence: 0.97},
		// Le premier vrai positif au-dessus du plancher : intact.
		{Type: "intro", StartMs: 121_000, EndMs: 139_015, Source: "audio_cluster", Confidence: 0.75},
		// Chapitre MKV de 8 s : exempt du plancher de 12 s.
		{Type: "intro", StartMs: 5_000, EndMs: 13_000, Source: "mkv_chapter", Confidence: 1.0},
	}
	got, err := segQuerySvc(segs, WithServerConfig(cfgRepo)).GetSegments(context.Background(), "ep-1")
	require.NoError(t, err)
	require.Len(t, got, 2, "les 3 faux positifs sous 12 s tombent, le vrai positif et le chapitre restent")
	sources := []string{got[0].Source, got[1].Source}
	assert.Contains(t, sources, "audio_cluster")
	assert.Contains(t, sources, "mkv_chapter")
}

// Le plancher absolu de 5 s (2026-08-06) vaut TOUJOURS, chapitre compris : le
// plancher de 12 s s'ajoute pour les detecteurs, il ne remplace pas l'autre.
func TestGetSegments_ChapterExemptionStopsAtTheAbsoluteFloor(t *testing.T) {
	t.Parallel()
	cfgRepo := sharedfakes.NewFakeServerConfigRepository()
	require.NoError(t, cfgRepo.Set(context.Background(), IntroDetectionConfigKey, `{"min_serve_confidence": 0}`))
	segs := []*domainlibrary.MediaSegment{
		{Type: "intro", StartMs: 389_000, EndMs: 392_000, Source: "mkv_chapter", Confidence: 1.0},
	}
	got, err := segQuerySvc(segs, WithServerConfig(cfgRepo)).GetSegments(context.Background(), "ep-1")
	require.NoError(t, err)
	require.Empty(t, got, "un chapitre de 3 s reste refuse")
}

// Community rows (tidb/introdb) are never served - user decision 2026-08-25.
// Their timestamps describe someone else's copy: the same source scored IoU
// 1.00 on one gold title and 232 seconds off on another the same day. The rows
// stay stored (the sweep replaces them, the eval scores them); the players
// just never see them - whatever the type, span or confidence.
func TestGetSegments_NeverServesCommunityRows(t *testing.T) {
	t.Parallel()
	segs := []*domainlibrary.MediaSegment{
		// High confidence, long span: passes every other gate on its own.
		{Type: "intro", StartMs: 80_000, EndMs: 170_000, Source: "tidb", Confidence: 0.9},
		{Type: "intro", StartMs: 0, EndMs: 33_000, Source: "introdb", Confidence: 0.9},
		{Type: "recap", StartMs: 0, EndMs: 40_000, Source: "tidb", Confidence: 0.9},
		// The file-derived row must keep flowing.
		{Type: "intro", StartMs: 80_000, EndMs: 170_000, Source: "audio_cluster", Confidence: 0.9},
	}
	got, err := segQuerySvc(segs).GetSegments(context.Background(), "ep-1")
	require.NoError(t, err)
	require.Len(t, got, 1, "only the file-derived detection may reach a player")
	assert.Equal(t, "audio_cluster", got[0].Source)
}
