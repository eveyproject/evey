package transcoding

import "strings"

// hw_route_cost.go - the ROUTE evidence (ADR-0029): what one real, warm, single-session
// transcode of a given route measured on this host. It is the proof the
// admission cost model prices a route on, replacing the cumulative bare-preset
// HW rung whose shape and arguments the runtime never emits (measured
// 2026-09-12/13: that rung over-priced a GPU 4K session x1.65 and mis-classed
// the CPU tone-map bridge as a pure GPU cost). VideoCostModel and the P1/P2
// ladder models are untouched: this sits beside them, consulted first for a
// route it has evidence for, the software ladder table its fallback.

// RouteCostClass is the pipeline SHAPE a route sample was measured on - the
// dimension whose cost genuinely differs, kept apart from the ladder-class
// pool (LadderClass) and from the encoder. A zero-copy SDR encode, a CPU
// tone-map bridge and a software-decoded decoupled session cost nothing alike
// even on the same encoder.
type RouteCostClass string

const (
	RouteCostSDR         RouteCostClass = "sdr"         // zero-copy (or CPU) encode, no tone-map
	RouteCostTonemap     RouteCostClass = "tonemap"     // HDR->SDR folded on the GPU
	RouteCostBridge      RouteCostClass = "bridge"      // GPU decode, CPU tone-map, GPU encode
	RouteCostPassthrough RouteCostClass = "passthrough" // HEVC Main10, HDR kept
	RouteCostDecoupled   RouteCostClass = "decoupled"   // CPU decode, hwupload, GPU scale+encode
	RouteCostSoftwareHDR RouteCostClass = "sw-hdr"      // CPU tone-map in the all-software chain
)

// RouteCostKey identifies a measured route: the accelerator (empty = CPU), the
// concrete ffmpeg encoder (so x264 and x265, h264_vaapi and hevc_vaapi are
// never one price), and the pipeline shape.
type RouteCostKey struct {
	Backend string         `json:"backend"`
	Encoder string         `json:"encoder"`
	Route   RouteCostClass `json:"route"`
}

// RouteSample is one warm, stable, single-session measurement of a route at
// one rung. The session is the one the orchestrator spawns (BuildFFmpegArgs,
// single-variant, production args), measured after a warm-up and taken as the
// conservative min(fps)/max(cpu) over two runs, so the number is the route the
// runtime serves - not a cold first encode (which ran 2.5-6x slower for its
// whole window on NVENC) nor a cumulative ladder.
//
// SCOPE (v7): the evidence is route-faithful on the VIDEO side - decode,
// filters, scaler, encoder, rate control, tone-map/bridge, all as production
// emits them. It is NOT yet the exact TOTAL session cost: the mires carry no
// audio, so the ~0.05 core a muxed-AAC rendition adds is not in CPUCores (the
// route-bench CLI, run with an audio bed, measures that fuller figure). A
// follow-up may fold it in; until then admission slightly under-counts the CPU
// a muxed session spends, which is small and on the safe side of the video cost.
type RouteSample struct {
	Key    RouteCostKey `json:"key"`
	Height int          `json:"height"`
	// FPS is the measured single-session throughput; SourceFPS the frame rate
	// of the clip it was measured on, so a request at another fps prices by
	// scaling (the encoder does NOT get 2x throughput on a 2x-fps source - it
	// gained ~1.3x on Ada, nothing on the UHD 630 - so RequiredFPS scales with
	// the source fps while the measured route fps is what it is).
	FPS       float64 `json:"fps"`
	SourceFPS float64 `json:"source_fps"`
	// CPUCores is the host cores this session burnt (rusage user+sys / wall).
	// A zero-copy GPU encode still costs 0.2-0.85 core (demux, mux, AAC, the
	// driver); a decoupled or bridged one costs several (software decode /
	// CPU tone-map). Charged to the CPU pool so a GPU box does not starve its
	// software streams on cores a "pure GPU" cost pretended were free.
	CPUCores float64 `json:"cpu_cores"`
	// SoftwareDecode records whether this session decoded on the CPU: the
	// source-decode adjustment (CostForSource) may apply ONLY then, never to a
	// zero-copy route whose measured fps already includes the GPU decode.
	SoftwareDecode bool `json:"software_decode"`
}

// RouteCostHeadroom is the realtime margin a route must clear: required fps =
// sourceFPS x headroom. Matches DerivePolicy.Margin; kept a named constant so
// the route cost and the ladder derivation move together.
const RouteCostHeadroom = 1.2

// SoftwareReadRate is the input pacing of a software session (`-readrate`):
// the CPU encoder runs at this multiple of realtime in steady state, so that
// is the rate its cores are charged at - not the 1.2 headroom, which is the
// floor it must clear. One constant for the producer's default and the cost
// model, so the two cannot drift (they had: paced at 1.5x, priced at 1.2x).
const SoftwareReadRate = 1.5

// RouteCostKeyFor builds the cost key for a resolved route. accel empty ⇒ CPU.
func RouteCostKeyFor(accel HWAccelType, encoder string, route RouteCostClass) RouteCostKey {
	return RouteCostKey{Backend: strings.ToLower(string(accel)), Encoder: strings.ToLower(encoder), Route: route}
}
