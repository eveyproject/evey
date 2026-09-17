package transcoding

import (
	"fmt"
	"sort"
)

// route.go - the ROUTE is the unit of decision, pricing, calibration and
// failure (ADR-0030). This file holds its topology: the explicit pipeline
// shape a session runs, in place of the six booleans of EncodeRoute whose 64
// combinations hid the seven that are legal. Everything downstream is a
// function of the topology: the cost class admission prices on
// (RouteCostClass), the shapes the calibration measures (RuntimeRouteShapes),
// and what the ffmpeg builder must be able to express.
//
// NOT topology, on purpose: GPU deinterlacing and GPU subtitle compositing.
// They change the filtergraph, not the pools, the cost class or the
// calibration shape - they ride the resolved route as runtime options.

// Stage is where a pipeline step runs.
type Stage uint8

const (
	StageCPU Stage = iota
	StageGPU
)

func (s Stage) String() string {
	if s == StageGPU {
		return "gpu"
	}
	return "cpu"
}

// ToneMapPlacement is where the HDR->SDR fold runs, if at all.
type ToneMapPlacement uint8

const (
	ToneMapNone ToneMapPlacement = iota
	ToneMapGPU                   // folded on the accelerator (OpenCL / Vulkan / VPP)
	ToneMapCPU                   // folded on the CPU: the bridge behind a GPU decode, or the all-software chain
)

func (t ToneMapPlacement) String() string {
	switch t {
	case ToneMapGPU:
		return "tonemap-gpu"
	case ToneMapCPU:
		return "tonemap-cpu"
	}
	return "no-tonemap"
}

// OutputMode is the dynamic range the segments carry.
type OutputMode uint8

const (
	OutputSDR            OutputMode = iota
	OutputHDRPassthrough            // HEVC Main10 with the HDR kept
)

func (o OutputMode) String() string {
	if o == OutputHDRPassthrough {
		return "hdr-passthrough"
	}
	return "sdr"
}

// Topology is the pipeline shape of a session. Build one with TopologyFor,
// which refuses the shapes no route can run (a GPU tone-map on a CPU decode,
// a passthrough that tone-maps, a GPU decode ahead of a CPU encode...).
type Topology struct {
	Decode  Stage
	ToneMap ToneMapPlacement
	Encode  Stage
	Output  OutputMode
}

// The seven legal topologies. Named so tests, the calibration and the
// builder speak of shapes, not of field tuples.
var (
	TopologySoftware       = Topology{StageCPU, ToneMapNone, StageCPU, OutputSDR}
	TopologySoftwareHDR    = Topology{StageCPU, ToneMapCPU, StageCPU, OutputSDR}
	TopologyZeroCopy       = Topology{StageGPU, ToneMapNone, StageGPU, OutputSDR}
	TopologyGPUToneMap     = Topology{StageGPU, ToneMapGPU, StageGPU, OutputSDR}
	TopologyBridge         = Topology{StageGPU, ToneMapCPU, StageGPU, OutputSDR}
	TopologyDecoupled      = Topology{StageCPU, ToneMapNone, StageGPU, OutputSDR}
	TopologyHDRPassthrough = Topology{StageGPU, ToneMapNone, StageGPU, OutputHDRPassthrough}
)

// AllTopologies lists the legal shapes, in a stable order.
func AllTopologies() []Topology {
	return []Topology{
		TopologySoftware, TopologySoftwareHDR,
		TopologyZeroCopy, TopologyGPUToneMap, TopologyBridge, TopologyDecoupled, TopologyHDRPassthrough,
	}
}

// ErrIllegalTopology is returned by TopologyFor for a shape no route runs.
type ErrIllegalTopology struct {
	Topology Topology
	Why      string
}

func (e ErrIllegalTopology) Error() string {
	return fmt.Sprintf("transcoding: illegal topology %s: %s", e.Topology, e.Why)
}

// TopologyFor builds a topology and refuses the illegal ones. The rules are
// the resolver's and the builder's gates, stated once:
//   - a CPU encode is the all-software chain: it decodes on the CPU too and
//     never tone-maps on the GPU;
//   - a GPU tone-map needs the 10-bit surface a GPU decode leaves on the
//     accelerator: never behind a CPU decode (the upload folds to 8-bit);
//   - a CPU tone-map behind a GPU encode is the bridge, which downloads a GPU
//     frame: never behind a CPU decode;
//   - the HDR passthrough keeps the range, so it tone-maps nowhere, encodes
//     on the GPU (Main10 encoder) and decodes there (a 10-bit software decode
//     is refused for the decoupled route).
func TopologyFor(decode, encode Stage, tonemap ToneMapPlacement, output OutputMode) (Topology, error) {
	t := Topology{Decode: decode, ToneMap: tonemap, Encode: encode, Output: output}
	refuse := func(why string) (Topology, error) { return Topology{}, ErrIllegalTopology{Topology: t, Why: why} }
	if encode == StageCPU {
		if decode == StageGPU {
			return refuse("a CPU encode decodes on the CPU")
		}
		if tonemap == ToneMapGPU {
			return refuse("a CPU encode never tone-maps on the GPU")
		}
		if output == OutputHDRPassthrough {
			return refuse("the HDR passthrough needs a GPU Main10 encoder")
		}
		return t, nil
	}
	if output == OutputHDRPassthrough {
		if tonemap != ToneMapNone {
			return refuse("the HDR passthrough keeps the range, it tone-maps nowhere")
		}
		if decode == StageCPU {
			return refuse("a 10-bit source is not software-decoded ahead of the GPU")
		}
		return t, nil
	}
	if decode == StageCPU && tonemap != ToneMapNone {
		return refuse("the decoupled route neither tone-maps on the GPU (the upload folds to 8-bit) nor bridges (no GPU frame to download)")
	}
	return t, nil
}

// MustTopology is TopologyFor for the literals of tests and tables.
func MustTopology(decode, encode Stage, tonemap ToneMapPlacement, output OutputMode) Topology {
	t, err := TopologyFor(decode, encode, tonemap, output)
	if err != nil {
		panic(err)
	}
	return t
}

// Legal reports whether t is one of the seven shapes.
func (t Topology) Legal() bool {
	_, err := TopologyFor(t.Decode, t.Encode, t.ToneMap, t.Output)
	return err == nil
}

// HW reports whether the accelerator encodes.
func (t Topology) HW() bool { return t.Encode == StageGPU }

// SoftwareDecode reports a CPU decode ahead of a GPU encode (the decoupled route).
func (t Topology) SoftwareDecode() bool { return t.Decode == StageCPU && t.Encode == StageGPU }

// CostClass is the pricing and calibration class of the shape - the one
// dimension of a RouteCostKey that is not the encoder.
func (t Topology) CostClass() RouteCostClass {
	if t.Encode == StageGPU {
		switch {
		case t.Output == OutputHDRPassthrough:
			return RouteCostPassthrough
		case t.ToneMap == ToneMapGPU:
			return RouteCostTonemap
		case t.ToneMap == ToneMapCPU:
			return RouteCostBridge
		case t.Decode == StageCPU:
			return RouteCostDecoupled
		}
		return RouteCostSDR
	}
	if t.ToneMap == ToneMapCPU {
		return RouteCostSoftwareHDR
	}
	return RouteCostSDR
}

func (t Topology) String() string {
	return fmt.Sprintf("%s-decode/%s/%s-encode/%s", t.Decode, t.ToneMap, t.Encode, t.Output)
}

// Topology reads the shape off the boolean route, with the precedence
// CostKey always applied (passthrough, GPU tone-map, bridge, decoupled,
// plain) - so an over-specified route resolves the way it was priced.
func (r EncodeRoute) Topology() Topology {
	switch {
	case r.HWUsable && r.HDRPassthrough:
		return TopologyHDRPassthrough
	case r.HWUsable && r.HWTonemap:
		return TopologyGPUToneMap
	case r.HWUsable && r.Tonemap:
		return TopologyBridge
	case r.HWUsable && r.SoftwareDecode:
		return TopologyDecoupled
	case r.HWUsable:
		return TopologyZeroCopy
	case r.Tonemap:
		return TopologySoftwareHDR
	}
	return TopologySoftware
}

// RouteShape is one pipeline shape admission can be asked to price: the
// cost class of a topology and the codec the segments carry (the encoder
// follows from codec and accelerator). It is the unit the calibration must
// measure - a shape the runtime emits and nobody measured is refused by
// admission for lack of evidence (ADR-0029).
type RouteShape struct {
	Route RouteCostClass
	Codec string
	HW    bool
}

// routeShapeCodecs are the output codecs a plan can carry.
var routeShapeCodecs = []string{"h264", "hevc", "av1"}

// RuntimeRouteShapes enumerates every shape a resolved route can name: the
// legal topologies x the codecs, the HDR passthrough being HEVC by
// construction (codecWithPassthrough). Sorted, duplicates folded (two
// topologies may share a cost class only if they share everything priced).
func RuntimeRouteShapes() []RouteShape {
	seen := map[RouteShape]bool{}
	var out []RouteShape
	for _, t := range AllTopologies() {
		for _, codec := range routeShapeCodecs {
			if t.Output == OutputHDRPassthrough && codec != "hevc" {
				continue
			}
			sh := RouteShape{Route: t.CostClass(), Codec: codec, HW: t.HW()}
			if !seen[sh] {
				seen[sh] = true
				out = append(out, sh)
			}
		}
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.HW != b.HW {
			return !a.HW
		}
		if a.Route != b.Route {
			return a.Route < b.Route
		}
		return a.Codec < b.Codec
	})
	return out
}
