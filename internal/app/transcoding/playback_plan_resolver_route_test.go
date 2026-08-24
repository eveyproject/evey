package transcoding

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/transcoding"
	pkgcache "github.com/evey-project/evey/internal/pkg/cache"
)

// The admission SPILL: a hardware-natural stream whose GPU pool is full, on a
// host whose CPU pool still has room, is served in software instead of being
// refused. Three things must move together or the cache hash lies - the
// fingerprint (so the spawn bills the CPU pool), the ladder (the software
// producer's shape), and the producer the segment path picks.
//
// ProducerFor is the third one, and it was at 0%: a stream whose ladder was
// shaped by the software producer but whose segments spawn on the hardware one
// produces bytes under a hash nobody will ever look up.

// named wraps stubProducer with an identity so a test can say WHICH producer
// came back.
type namedProducer struct {
	stubProducer
	name string
}

func newNamedProducer(name string) *namedProducer {
	return &namedProducer{name: name}
}

func routeSelector(t *testing.T, hwUsable bool) (*ProducerSelector, *namedProducer, *namedProducer) {
	t.Helper()
	sw, hw := newNamedProducer("software"), newNamedProducer("hardware")
	sel := NewProducerSelector(sw, hw, func(transcoding.PlaybackDecision, SourceDims) bool { return hwUsable })
	return sel, sw, hw
}

func routeStreams() []*library.MediaStream {
	w, h := 3840, 2160
	return []*library.MediaStream{{
		StreamType: library.StreamTypeVideo,
		Codec:      "hevc",
		Width:      &w,
		Height:     &h,
	}}
}

func TestProducerFor(t *testing.T) {
	decision := transcoding.PlaybackDecision{VideoCodec: "h264", VideoDecision: "transcode"}

	t.Run("a spilled stream spawns on the software producer despite a usable GPU", func(t *testing.T) {
		sel, sw, hw := routeSelector(t, true) // the hw gate says yes...
		pr := &PlanResolver{
			Selector: sel,
			// ...but admission pinned this stream to the CPU pool.
			AdmissionPinnedClass: func(string) (LadderClass, bool) { return LadderClassSoftware, true },
		}

		got := pr.ProducerFor("m1", routeStreams(), decision, false)
		assert.Same(t, sw, got, "the pin must win over the hw gate - the ladder was shaped for software")
		assert.NotSame(t, hw, got)
	})

	t.Run("a stream pinned to HW follows the gate", func(t *testing.T) {
		sel, _, hw := routeSelector(t, true)
		pr := &PlanResolver{
			Selector:             sel,
			AdmissionPinnedClass: func(string) (LadderClass, bool) { return LadderClassHW, true },
		}

		assert.Same(t, hw, pr.ProducerFor("m1", routeStreams(), decision, false),
			"an HW pin is not a spill: the gate decides as usual")
	})

	t.Run("no pin at all follows the gate", func(t *testing.T) {
		sel, sw, hw := routeSelector(t, true)
		pr := &PlanResolver{
			Selector:             sel,
			AdmissionPinnedClass: func(string) (LadderClass, bool) { return LadderClassSoftware, false },
		}

		got := pr.ProducerFor("m1", routeStreams(), decision, false)
		assert.Same(t, hw, got, "an absent pin must not be read as a software pin")
		assert.NotSame(t, sw, got)
	})

	t.Run("a copy request takes the copy producer, pin or not", func(t *testing.T) {
		sel, sw, hw := routeSelector(t, true)
		copyProd := newNamedProducer("copy")
		pr := &PlanResolver{
			Selector:             sel,
			CopyProducer:         copyProd,
			AdmissionPinnedClass: func(string) (LadderClass, bool) { return LadderClassSoftware, true },
		}

		got := pr.ProducerFor("m1", routeStreams(), decision, true)
		assert.Same(t, copyProd, got, "a copy rung never encodes, so neither pool serves it")
		assert.NotSame(t, sw, got)
		assert.NotSame(t, hw, got)
	})

	t.Run("no selector falls back to the orchestrator", func(t *testing.T) {
		orch := newNamedProducer("orchestrator")
		pr := &PlanResolver{Orchestrator: orch}

		assert.Same(t, orch, pr.ProducerFor("m1", routeStreams(), decision, false))
	})

	t.Run("nothing wired returns nil rather than a wrong producer", func(t *testing.T) {
		assert.Nil(t, (&PlanResolver{}).ProducerFor("m1", routeStreams(), decision, false))
	})
}

// The copy rung's segments duplicate roughly the source size on disk, and the
// unpaced copy produces them well ahead of the viewer. Below the source size
// plus 10% headroom the rung must stand down to the segment-bounded transcode
// ladder rather than risk ENOSPC on a small box.
func TestIsCopy_DiskGuard(t *testing.T) {
	streams := ladderStreams(1080)
	streams[0].Keyframes = []int64{0, 6000, 12000}
	streams[0].Profile = "High"
	streams[0].Level = new(int)
	*streams[0].Level = 40

	decision := transcoding.PlaybackDecision{VideoCodec: "h264", VideoDecision: "copy"}
	const sourceBytes = 1_000_000

	newResolver := func(free int64, statErr error) *PlanResolver {
		return &PlanResolver{
			CopyProducer: &stubProducer{},
			FreeBytes:    func() (int64, error) { return free, statErr },
		}
	}

	t.Run("comfortably above the source plus headroom", func(t *testing.T) {
		assert.True(t, newResolver(sourceBytes*10, nil).IsCopy(decision, streams, sourceBytes))
	})

	t.Run("exactly the source plus its 10 percent headroom is enough", func(t *testing.T) {
		// The guard refuses strictly BELOW the requirement, so landing exactly on
		// it still serves the copy rung.
		assert.True(t, newResolver(sourceBytes+sourceBytes/10, nil).IsCopy(decision, streams, sourceBytes))
	})

	t.Run("one byte short falls back to the transcode ladder", func(t *testing.T) {
		assert.False(t, newResolver(sourceBytes+sourceBytes/10-1, nil).IsCopy(decision, streams, sourceBytes))
	})

	t.Run("fails OPEN when statfs fails", func(t *testing.T) {
		// An unreadable filesystem must not silently disable direct play; the mux
		// still fails gracefully downstream if the disk really is full.
		assert.True(t, newResolver(0, assert.AnError).IsCopy(decision, streams, sourceBytes))
	})

	t.Run("fails OPEN when the source size is unknown", func(t *testing.T) {
		pr := newResolver(1, nil) // a byte free: only the unknown size saves it
		assert.True(t, pr.IsCopy(decision, streams, 0))
	})

	t.Run("no probe wired means no guard", func(t *testing.T) {
		pr := &PlanResolver{CopyProducer: &stubProducer{}}
		assert.True(t, pr.IsCopy(decision, streams, sourceBytes))
	})
}

// The ladder freeze and the producer pick read the SAME pin key. If they
// drifted, a stream could be frozen under one key and routed under another -
// the manifest and the segments would then disagree with nothing to show for it.
func TestPinKey_IsStableAndDiscriminating(t *testing.T) {
	pr := &PlanResolver{}
	streams := routeStreams()
	transcodeH264 := transcoding.PlaybackDecision{VideoCodec: "h264", VideoDecision: "transcode"}

	base := pr.PinKey("m1", streams, transcodeH264)
	assert.Equal(t, base, pr.PinKey("m1", streams, transcodeH264), "the key must be a pure function of its inputs")

	assert.NotEqual(t, base, pr.PinKey("m2", streams, transcodeH264), "distinct titles must never collide")
	assert.NotEqual(t, base, pr.PinKey("m1", streams,
		transcoding.PlaybackDecision{VideoCodec: "hevc", VideoDecision: "transcode"}),
		"distinct output codecs must never collide")
	assert.NotEqual(t, base, pr.PinKey("m1", streams,
		transcoding.PlaybackDecision{VideoCodec: "h264", VideoDecision: "copy"}),
		"copy and transcode of the same title are different streams")
}

// PinKey keys on the EFFECTIVE codec, so an AV1 request that will really be
// encoded as H.264 must share the H.264 stream's pin - otherwise admission
// tracks two ceilings for what is one encode.
func TestPinKey_FollowsTheEffectiveCodec(t *testing.T) {
	pr := &PlanResolver{AV1Enabled: func() bool { return false }} // no HW, AV1 off
	streams := routeStreams()

	av1 := pr.PinKey("m1", streams, transcoding.PlaybackDecision{VideoCodec: "av1", VideoDecision: "transcode"})
	h264 := pr.PinKey("m1", streams, transcoding.PlaybackDecision{VideoCodec: "h264", VideoDecision: "transcode"})

	assert.Equal(t, h264, av1, "an AV1 request served as H.264 is the same stream to admission")
}

// The class lookup is cached per media id, INCLUDING the default: an
// unclassified title must not re-query the repo on every segment.
//
// The TTL must be non-zero. TTLCache.Set stamps expiresAt = now + ttl and Get
// treats a passed expiry as a miss, so a 0 TTL caches NOTHING - the opposite of
// "cache forever". Written with 0 first, this test failed with 3 repo calls
// instead of 1.
func TestShapeLadder_ClassLookupCachesTheResolvedDefault(t *testing.T) {
	repo := &countingComplexityRepo{}
	pr := &PlanResolver{
		ComplexityRepo: repo,
		ClassLookup:    newTestClassCache(t),
		ClassLookupTTL: time.Minute,
	}
	req := ladderRequest(2160)
	decision := transcoding.PlaybackDecision{VideoCodec: "h264", VideoDecision: "transcode"}

	pr.ShapeLadder(context.Background(), req, decision)
	pr.ShapeLadder(context.Background(), req, decision)
	pr.ShapeLadder(context.Background(), req, decision)

	assert.Equal(t, 1, repo.calls,
		"an unclassified title must be cached as the DEFAULT, not re-queried on every segment")
}

// Without a media id there is nothing to key the cache on, so the repo must not
// be consulted at all - a blank key would collide across every title.
func TestShapeLadder_NoMediaIDNeverTouchesTheComplexityRepo(t *testing.T) {
	repo := &countingComplexityRepo{}
	pr := &PlanResolver{ComplexityRepo: repo, ClassLookup: newTestClassCache(t)}
	req := ladderRequest(2160)
	req.MediaID = ""

	pr.ShapeLadder(context.Background(), req,
		transcoding.PlaybackDecision{VideoCodec: "h264", VideoDecision: "transcode"})

	assert.Zero(t, repo.calls, "no media id ⇒ no cache key ⇒ no lookup")
}

// countingComplexityRepo answers Get and counts the calls; the write side is
// never reached from the ladder path, so it refuses rather than pretending.
type countingComplexityRepo struct {
	calls int
	entry transcoding.ComplexityEntry
	found bool
}

func (r *countingComplexityRepo) Get(context.Context, string) (transcoding.ComplexityEntry, bool, error) {
	r.calls++
	return r.entry, r.found, nil
}

func (r *countingComplexityRepo) Set(context.Context, string, transcoding.ContentClass, transcoding.ComplexityMetrics, time.Time) error {
	panic("countingComplexityRepo: the ladder path must never write a classification")
}

func (r *countingComplexityRepo) ListUnclassified(context.Context, int) ([]string, error) {
	panic("countingComplexityRepo: the ladder path must never enumerate the queue")
}

func newTestClassCache(t *testing.T) *pkgcache.TTLCache[transcoding.ContentClass] {
	t.Helper()
	c, stop := pkgcache.New[transcoding.ContentClass](time.Minute)
	t.Cleanup(stop)
	return c
}

// A classified title's persisted class must reach the ladder - that is the
// whole point of the background ComplexityWorker.
func TestShapeLadder_UsesThePersistedComplexityClass(t *testing.T) {
	repo := &countingComplexityRepo{
		entry: transcoding.ComplexityEntry{Class: transcoding.ClassAnimation},
		found: true,
	}
	pr := &PlanResolver{ComplexityRepo: repo, ClassLookup: newTestClassCache(t)}
	decision := transcoding.PlaybackDecision{VideoCodec: "h264", VideoDecision: "transcode"}

	classified := pr.ShapeLadder(context.Background(), ladderRequest(2160), decision)

	plain := (&PlanResolver{}).ShapeLadder(context.Background(), ladderRequest(2160), decision)
	require.NotEmpty(t, classified)
	assert.NotEqual(t, plain, classified,
		"a persisted class must shape a different ladder than the LadderFilmNormal default")
}

// The other side of the same trap, pinned so a future refactor cannot quietly
// reintroduce a per-segment DB hit: a zero TTL means the entry is already
// expired when it is written, so nothing is cached.
func TestShapeLadder_ZeroClassLookupTTLCachesNothing(t *testing.T) {
	repo := &countingComplexityRepo{}
	pr := &PlanResolver{
		ComplexityRepo: repo,
		ClassLookup:    newTestClassCache(t),
		// ClassLookupTTL deliberately left at 0.
	}
	req := ladderRequest(2160)
	decision := transcoding.PlaybackDecision{VideoCodec: "h264", VideoDecision: "transcode"}

	pr.ShapeLadder(context.Background(), req, decision)
	pr.ShapeLadder(context.Background(), req, decision)

	assert.Equal(t, 2, repo.calls,
		"a 0 TTL is NOT 'cache forever' - it writes an already-expired entry, so every call re-queries")
}

// A spilled AV1 stream must be FINGERPRINTED as H.264.
//
// AV1 rides hardware only. When admission pins a hw-eligible stream to the CPU
// pool (GPU full), the software encoder produces H.264 - but the fingerprint
// is the cache key AND what the manifest's CODECS attribute is derived from.
// Leave "av1" in it and the master advertises av01.* while the producer writes
// H.264 under that very hash: the player fetches segments it was told are AV1,
// gets H.264, and fails to decode. Nothing logs an error - the bytes were
// served 200, the cache hit, the ladder was correct. Exactly the class of
// silent break this repo keeps paying for.
//
// The resolver's own comment states the rule; nothing asserted it.
func TestFingerprint_SpilledAV1IsFingerprintedAsH264(t *testing.T) {
	av1Capable := vaapiCapable()
	av1Capable.HWCodecs = []string{"h264", "hevc", "av1"}

	newResolver := func(pinned LadderClass, hasPin bool) *PlanResolver {
		pr := hwResolver(av1Capable)
		pr.AV1Enabled = func() bool { return true }
		pr.AdmissionPinnedClass = func(string) (LadderClass, bool) { return pinned, hasPin }
		return pr
	}
	req := PlaybackRequest{MediaID: "m1", SourcePath: "/m.mkv", Streams: routeStreams()}
	decision := transcoding.PlaybackDecision{VideoCodec: "av1", VideoDecision: "transcode"}

	spilled := newResolver(LadderClassSoftware, true).Fingerprint(context.Background(), req, decision, nil)
	assert.Equal(t, "h264", spilled.VideoCodec,
		"a spilled AV1 stream is encoded in software as H.264 - the cache key and the CODECS attribute must say so")
	assert.Empty(t, spilled.HWAccel, "a spilled stream carries no hw accel in its fingerprint")

	// Control: without a spill the AV1 fingerprint stays AV1, or this test
	// would pass just as well on a resolver that always says h264.
	notSpilled := newResolver(LadderClassHW, true).Fingerprint(context.Background(), req, decision, nil)
	assert.Equal(t, "av1", notSpilled.VideoCodec, "an HW pin is not a spill: AV1 stays AV1")
}
