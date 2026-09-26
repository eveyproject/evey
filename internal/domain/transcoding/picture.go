package transcoding

// PicturePlan is the player's picture decision, detached from the HLS session:
// how ONE SDR picture of a source is drawn on this host - the accelerator and
// its device, the pipeline shape (where it decodes, tone-maps and encodes),
// and the source corrections the player applies (HDR fold, deinterlace
// verdict, inverse telecine, anamorphic SAR).
//
// It exists so a producer that is not a player session - the sync-preview
// clip an operator judges, the reduced download a phone keeps - draws the
// SAME frame the viewer sees, from the same resolution (PlanResolver.
// ResolvePicture), instead of each keeping its own copy of the tone-map
// string and none of the other corrections. It carries no ladder, no
// admission pin and no fingerprint: those belong to a session.
type PicturePlan struct {
	// Codec is the output codec ("h264").
	Codec string
	// Accel / Device / Features name the accelerator when Topology encodes on
	// the GPU; zero when the picture is drawn in software.
	Accel    HWAccelType
	Device   string
	Features []string
	// Topology is the pipeline shape (TopologySoftware when nothing else).
	Topology Topology
	// ToneMap: the source is HDR (or Dolby Vision) and the picture is folded to
	// SDR. Where it is folded is Topology.ToneMap.
	ToneMap bool
	// HDRTransfer is the source range as the resolver names it ("pq", "hlg",
	// "" for SDR or Dolby Vision).
	HDRTransfer string
	// DeinterlaceAll / Telecine / HWDeinterlace are the resolver's verdicts on
	// the source's field structure, as the player's fingerprint carries them.
	DeinterlaceAll bool
	Telecine       bool
	HWDeinterlace  bool
	// SourceSAR is the anamorphic sample aspect ("64:45"); "" or "1:1" = square.
	SourceSAR string
}

// HW reports whether the accelerator encodes the picture.
func (p PicturePlan) HW() bool { return p.Topology.HW() }

// Software is the same picture drawn by the CPU alone: the corrections stay,
// the accelerator goes. What a consumer falls back to when the GPU session
// fails to start - a preview must still be shown, and it never demotes the
// host's calibration the way a player session would.
func (p PicturePlan) Software() PicturePlan {
	p.Accel, p.Device, p.Features, p.HWDeinterlace = "", "", nil, false
	p.Topology = TopologySoftware
	if p.ToneMap {
		p.Topology = TopologySoftwareHDR
	}
	return p
}

// ApplyTo writes the picture decision into the process options the ffmpeg
// builders read, exactly as the player's fingerprint projects them
// (segment_orchestrator / software_producer): one projection, so a clip and
// a session can never disagree on what a field means.
func (p PicturePlan) ApplyTo(o *HLSProcessOpts) {
	o.VideoCodec = p.Codec
	o.HDRSupported = false
	o.ToneMap = p.ToneMap
	o.HDRTransfer = p.HDRTransfer
	o.DeinterlaceAll = p.DeinterlaceAll
	o.InverseTelecine = p.Telecine
	o.SourceSAR = p.SourceSAR
	if !p.HW() {
		return
	}
	o.HWAccel = p.Accel
	o.HWDevice = p.Device
	o.HWFeatures = p.Features
	o.HWSoftwareDecode = p.Topology.SoftwareDecode()
	o.HWTonemap = p.Topology.ToneMap == ToneMapGPU
	o.HWDeinterlace = p.HWDeinterlace
}
