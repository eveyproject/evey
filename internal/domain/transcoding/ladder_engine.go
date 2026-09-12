package transcoding

import (
	"fmt"
	"strings"
)

// ladder_engine.go - the vocabulary of the ladder engine. A session's ladder
// is a pure function of four inputs that used to be smeared across the plan
// resolver, the producers and the ffmpeg builders:
//
//	SourceFacts  what the file is (dimensions, codec, depth, HDR, cadence)
//	Complexity   how hard it is to encode (the per-title vector, persisted)
//	EncodeRoute  what will encode it (codec, GPU or CPU, tone-map placement)
//	Delivery     what may reach the viewer (decoder, screen, line, host)
//
// plus a QualityPolicy that turns each rung into a rate-control target for
// the encoder. LadderBuilder assembles the rungs; the resolver only freezes
// the result on the session's admission pin.

// SourceFacts are the persisted properties of the source video stream the
// ladder depends on. Zero Width/Height = unprobed (no source cap).
type SourceFacts struct {
	Width, Height int
	Codec         string
	TenBit        bool
	// PixelFormat is ffprobe's pix_fmt ("" = unknown): the chroma/depth gate
	// of the GPU decoder (HWChromaDecodable).
	PixelFormat string
	// HDRRange is the transcode-side range: "PQ", "HLG" or "" (SDR).
	HDRRange string
	Telecine bool
	// BitrateKbps is the video stream's bitrate, else the file's; 0 = unknown.
	BitrateKbps int64
	// FPS is the source frame rate (0 = unknown, priced as film cadence).
	FPS float64
}

// ComplexityVector is the per-title complexity the ladder is shaped for. The
// persisted class stays the sole driver until the vector's features have been
// validated against measured bitrate and quality (plan P1): a feature enters
// the shaping only once it is known to predict something.
type ComplexityVector struct {
	Metrics ComplexityMetrics
	// Class is the persisted verdict; "" = never classified.
	Class ContentClass
}

// Cost is the cost model's answer for this title, when the sampling allows one.
func (c ComplexityVector) Cost() (TitleCost, bool) {
	return CostModelV2.Predict(c.Metrics)
}

// LadderClass is the class the base ladder is picked for: the persisted one
// when valid, the film_normal fallback otherwise.
func (c ComplexityVector) LadderClass() ContentClass {
	if c.Class.IsValid() {
		return c.Class
	}
	return ClassFilmNormal
}

// EncodeRoute names the pipeline the plan resolved: the codec the segments
// carry, the accelerator (empty = CPU) and where an HDR->SDR tone-map runs.
type EncodeRoute struct {
	Codec string
	Accel HWAccelType
	// HWUsable: the GPU encodes. SoftwareDecode: the CPU decodes ahead of it
	// (decoupled session), which the GPU ceiling was never benched for.
	HWUsable       bool
	SoftwareDecode bool
	// Tonemap: the session folds HDR to SDR. HWTonemap: on the GPU; otherwise
	// the CPU tone-maps, between a GPU decode and encode when HWUsable (the
	// bridge) or in the all-software chain.
	Tonemap   bool
	HWTonemap bool
	// Copy: the single remux rung, no encode at all.
	Copy bool
	// HDRPassthrough: HEVC Main10 with the HDR kept - an operating point the
	// efficiency model does not cover yet.
	HDRPassthrough bool
}

// EncoderKey is the efficiency model's key for this route.
func (r EncodeRoute) EncoderKey() EncoderKey {
	return EncoderKey{Codec: strings.ToLower(r.Codec), Accel: r.Accel, Preset: ProductionPreset(r.Accel), HDR: r.HDRPassthrough}
}

// Bridge reports the CPU tone-map between a GPU decode and a GPU encode.
func (r EncodeRoute) Bridge() bool { return r.HWUsable && r.Tonemap && !r.HWTonemap }

// DeliveryConstraints bound what the ladder may advertise: the viewer's
// decoder and screen, the line, the operator's ceilings and the host's
// calibrated throughput. Every field is a cap; zero = unconstrained.
type DeliveryConstraints struct {
	// ScreenHeight trims rungs a panel would only downscale. MaxHeight is a
	// decoder limit or an explicit quality pick. MaxH264Level bounds the frame
	// size an H.264 decoder accepts.
	ScreenHeight int
	MaxHeight    int
	MaxH264Level int
	// VideoBitrateCeilingKbps is the tightest of the request, profile,
	// decision and server ceilings; 0 = follow the source.
	VideoBitrateCeilingKbps int64
	// QoEMaxHeight is the adaptive ceiling learned from playback feedback.
	QoEMaxHeight int
	// AV1MaxHeight is the operator's cap on AV1 rungs.
	AV1MaxHeight int
	// Capability is the host's calibrated throughput - the hard gate: a rung
	// the host cannot encode in real time is never advertised.
	Capability EncodeCapability
}

// RateControlMode is how an encoder is asked to spend bits. The set is
// deliberately generic: a policy may add modes an encoder family maps to its
// own flags (x264 CRF, NVENC cq, VAAPI QVBR/ICQ, QSV ICQ...).
type RateControlMode string

const (
	// RateControlConstantQuality targets a quality index; the bitrate is what
	// that quality costs, bounded by MaxKbps for distribution.
	RateControlConstantQuality RateControlMode = "cq"
	// RateControlVBR targets an average bitrate with a bounded peak.
	RateControlVBR RateControlMode = "vbr"
	// RateControlCBR holds the bitrate constant.
	RateControlCBR RateControlMode = "cbr"
)

// QualityTarget is what a rung asks of its encoder. The zero value means
// "the encoder family's default", which the builders resolve themselves.
type QualityTarget struct {
	Mode RateControlMode
	// Quality is the encoder-scale quality index for constant-quality modes
	// (x264 CRF 0-51, NVENC cq 0-51, VAAPI/QSV quality); 0 = default.
	Quality int
	// TargetKbps is the average bitrate for the bitrate-targeted modes.
	TargetKbps int64
	// MaxKbps is the distribution ceiling (the manifest's promise); BufferKbps
	// the VBV window it is enforced over.
	MaxKbps    int64
	BufferKbps int64
}

// IsZero reports the "encoder default" target.
func (q QualityTarget) IsZero() bool { return q == QualityTarget{} }

// QualityPolicy turns a rung into a rate-control target for its route.
type QualityPolicy interface {
	TargetFor(rung HLSProfile, route EncodeRoute) QualityTarget
}

// LadderInput is everything a LadderBuilder reads.
type LadderInput struct {
	Source     SourceFacts
	Complexity ComplexityVector
	Route      EncodeRoute
	Delivery   DeliveryConstraints
}

// LadderBuilder shapes the rungs of a session. Pure: the same input yields
// the same ladder at manifest- and segment-time (the cache-hash invariant).
type LadderBuilder interface {
	Build(in LadderInput) []HLSProfile
}

// --- the static policy: today's numbers, expressed as targets ---

// StaticQualityPolicy is the rate control the builders applied before the
// engine existed: capped CRF 20 with 160 % headroom on the CPU (quality
// targeted, bitrate as a ceiling), VBR whose peak is the rung's bitrate on
// the GPU. Content-blind by construction; the adaptive policies replace it.
type StaticQualityPolicy struct{}

const (
	// StaticSoftwareCRF is the x264 quality target of the static policy.
	StaticSoftwareCRF = 20
	// StaticSoftwareHeadroomPct sizes the CPU ceiling above the rung's
	// advertised bitrate: the rung bitrate is what ABR budgets, the ceiling
	// only clips outliers (pinned together, CRF 20 starved grainy re-encodes).
	StaticSoftwareHeadroomPct = 160
)

func (StaticQualityPolicy) TargetFor(rung HLSProfile, route EncodeRoute) QualityTarget {
	if route.HWUsable {
		return QualityTarget{Mode: RateControlVBR, TargetKbps: rung.VideoBitrate, MaxKbps: rung.VideoBitrate, BufferKbps: rung.VideoBitrate * 2}
	}
	ceiling := rung.VideoBitrate * StaticSoftwareHeadroomPct / 100
	if rung.CostModelled {
		ceiling = rung.VideoBitrate // the margin is in the price already
	}
	return QualityTarget{Mode: RateControlConstantQuality, Quality: StaticSoftwareCRF, MaxKbps: ceiling, BufferKbps: ceiling * 2}
}

// --- the static builder: the class table plus the caps, in one place ---

// StaticLadderBuilder is the per-class table shaped by the delivery caps -
// the ladder every session got before the engine. It is the fallback the
// adaptive builders degrade to, and the reference the golden files pin.
// Cost and Efficiency default to the current model versions; tests inject.
type StaticLadderBuilder struct {
	Cost       *CostModel
	Efficiency *CodecEfficiencyModel
}

func (b StaticLadderBuilder) cost() CostModel {
	if b.Cost != nil {
		return *b.Cost
	}
	return CostModelV2
}

func (b StaticLadderBuilder) efficiency() CodecEfficiencyModel {
	if b.Efficiency != nil {
		return *b.Efficiency
	}
	return CodecEfficiencyModelV1
}

// Build reproduces, in order: the class table, the producer's own shape
// (copy rung / GPU ceiling / CPU ceiling with the top-rung boost), the QoE
// override, the source raise and the bitrate ceiling, the screen and level
// caps, the AV1 cap, the HEVC scaling, and the CPU tone-map ceiling of a
// bridged route.
func (b StaticLadderBuilder) Build(in LadderInput) []HLSProfile {
	base := LadderFor(in.Complexity.LadderClass())
	src := in.Source
	cap := in.Delivery.Capability
	route := in.Route

	var laddered []HLSProfile
	switch {
	case route.Copy:
		return CopyRung(src.Width, src.Height)
	case route.HWUsable:
		laddered = ShapeHardwareLadder(base, src, cap)
	default:
		laddered = ShapeSoftwareLadder(base, src, route.Codec, route.Tonemap, cap)
	}

	// A title with a cost prediction is priced by the models instead of the
	// class table: the CPU reference price (what CRF 20 costs on THIS title,
	// P90 margin included), converted to the route's encoder by its measured
	// efficiency. A route the efficiency table does not cover (no cell for
	// this encoder at any confidence, or an HDR passthrough) keeps the table.
	if cost, ok := b.cost().Predict(in.Complexity.Metrics); ok {
		key := route.EncoderKey()
		isReference := key.Accel == "" && key.Codec == "h264"
		if _, covered := b.efficiency().Lookup(key, laddered[len(laddered)-1].Height); isReference || covered {
			laddered = b.cost().PriceLadder(laddered, cost, src, StaticSoftwareCRF)
			if !isReference {
				laddered = b.efficiency().PriceLadder(laddered, key)
			}
		}
	}

	if in.Delivery.QoEMaxHeight > 0 && src.Height > 0 {
		laddered = CapLadderToHeight(laddered, in.Delivery.QoEMaxHeight)
	}
	ceiling := in.Delivery.VideoBitrateCeilingKbps
	laddered = RaiseLadderToSource(laddered, src.BitrateKbps, src.Width, src.Height, ceiling)
	laddered = CapLadderToVideoBitrate(laddered, ceiling)
	laddered = CapLadderToScreen(laddered, in.Delivery.ScreenHeight)
	laddered = CapLadderToHeight(laddered, in.Delivery.MaxHeight)
	if strings.EqualFold(route.Codec, "h264") {
		laddered = CapLadderToH264Level(laddered, in.Delivery.MaxH264Level)
	}
	if strings.EqualFold(route.Codec, "av1") {
		laddered = CapLadderToHeight(laddered, in.Delivery.AV1MaxHeight)
	}
	if strings.EqualFold(route.Codec, "hevc") && !laddered[0].CostModelled {
		// The legacy two-thirds rule, for table-priced ladders only: a modelled
		// ladder carries its measured HEVC efficiency already.
		laddered = ScaleLadderVideoBitrates(laddered, HEVCBitrateNum, HEVCBitrateDen)
	}
	if route.Bridge() {
		if cap.HWBridgeMaxHeight > 0 {
			laddered = CapLadderToHeight(laddered, cap.HWBridgeMaxHeight)
		} else if cap.SoftwareHDRMaxHeight > 0 {
			laddered = CapLadderToHeight(laddered, cap.SoftwareHDRMaxHeight)
		}
	}
	return laddered
}

// CopyRung is the single remux rung: the source resolution, video copied.
// The "-kf4" suffix is the cache GENERATION of the copy plan contract; bump
// it whenever produced segment content or labelling changes for the same
// inputs (see the software producer's copy mode for the history).
func CopyRung(width, height int) []HLSProfile {
	if width <= 0 || height <= 0 {
		width, height = 1920, 1080 // deterministic fingerprint for an unprobed source
	}
	return []HLSProfile{{
		Name:         fmt.Sprintf("%dp-copy-kf4", height),
		Width:        width,
		Height:       height,
		VideoBitrate: CopyRungPlaceholderKbps,
		AudioBitrate: 192,
		Copy:         true,
	}}
}

// CopyRungPlaceholderKbps is the copy rung's nominal bitrate: the real one is
// the source's, advertised from the file by the manifest.
const CopyRungPlaceholderKbps int64 = 5_000

// ShapeHardwareLadder: the full class ladder (a GPU parallelises rungs
// cheaply) capped to the source and to the host's GPU ceiling. A decoupled
// session (a codec this GPU cannot decode) keeps the CPU ceiling: the GPU
// ladder was benched behind a zero-copy decode, and a rung the mixed pipeline
// cannot sustain starves the player.
func ShapeHardwareLadder(base []HLSProfile, src SourceFacts, cap EncodeCapability) []HLSProfile {
	sourceCapped := CapLadderToSource(base, src.Width, src.Height)
	zeroCopyDecode := src.Codec == "" || (cap.HWCanDecodeSource(src.Codec, src.TenBit) && HWChromaDecodable(src.PixelFormat))
	return CapLadderToHeight(sourceCapped, cap.MaxHeightFor(zeroCopyDecode))
}

// ShapeSoftwareLadder: the class ladder capped to the source and to the
// codec's CPU ceiling (its own for AV1, the HDR one when tone-mapping), the
// top surviving rung inheriting the first dropped rung's bitrate budget.
func ShapeSoftwareLadder(base []HLSProfile, src SourceFacts, codec string, tonemap bool, cap EncodeCapability) []HLSProfile {
	sourceCapped := CapLadderToSource(base, src.Width, src.Height)
	maxH := cap.SoftwareCeilingFor(codec)
	if tonemap && cap.SoftwareHDRMaxHeight > 0 && cap.SoftwareHDRMaxHeight < maxH {
		maxH = cap.SoftwareHDRMaxHeight
	}
	capped := CapLadderToHeight(sourceCapped, maxH)
	return BoostCappedTopRung(sourceCapped, capped)
}
