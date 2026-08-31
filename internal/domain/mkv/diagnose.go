package mkv

import "context"

// DiagnosisFinding is one diagnosed defect with its remedy, mirroring mkvgo's
// ops.Finding. Kind is one of: no-index | index-misskeyed | index-sparse |
// index-stale-tracks | audio-delay | truncated | damaged | trailing-junk |
// streamed-size.
type DiagnosisFinding struct {
	Kind    string
	Detail  string
	Remedy  string
	Track   uint64
	DelayMs int64 // audio-delay only: how late this track starts (ms)
}

// Diagnosis is the one-call triage verdict for a media file, distilled from
// mkvgo's ops.Diagnose so the UI can route to the right repair without
// guessing: reindex/resync for an index defect, retime for an audio delay,
// re-download for a truncated (incomplete) source.
type Diagnosis struct {
	Healthy  bool
	Findings []DiagnosisFinding
	// MissingTailBytes is how many declared bytes are absent from the end of
	// the file (declared Segment end minus real size) when Truncated. The
	// MAGNITUDE is what routes the operator's decision: a 9 KB stub cut off a
	// 937 MB episode is invisible in playback, 41 MB is a missing final scene
	// - both were shown as the same bare "incomplete download" badge before.
	// 0 when not truncated or not measurable.
	MissingTailBytes int64
	// Truncated: the damage runs to the end of the file - an incomplete
	// download whose missing tail no tool can recover. The operator must
	// re-acquire the source; a resync would only produce a playable prefix.
	Truncated bool
	// DamagedBytes is what a resync repair would skip (0 when clean or only an
	// index/audio defect). Its approximate presentation times are in the
	// "damaged" findings.
	DamagedBytes int64
	// AudioDelaysMs is every audio track's start delay relative to the video
	// (ms), whether or not it crossed the finding threshold - the raw signal
	// the audio-remux repair acts on. Keyed by track number.
	AudioDelaysMs map[uint64]int64
}

// HasIndexDefect reports whether the diagnosis found a seek-index defect: a
// missing index, or one that is present but unusable for video seeking
// (misskeyed onto audio, too sparse to land a seek near its target, or
// referencing tracks the file no longer has). Such a file must be reindexed
// even though a Cues element physically exists.
func (d *Diagnosis) HasIndexDefect() bool {
	for _, f := range d.Findings {
		switch f.Kind {
		case "no-index", "index-misskeyed", "index-sparse", "index-stale-tracks":
			return true
		}
	}
	return false
}

// IndexSparse reports the one index defect a rebuild may be POWERLESS against:
// the cues are keyed on the video track (nothing misskeyed, nothing stale) but
// too far apart for a seek to land near its target. Usually the muxer simply
// cued one keyframe in ten and a reindex fixes it - but when the encode's own
// keyframes are that far apart, no index can be denser than the keyframes it
// names, and re-running the repair forever would achieve nothing. The scan
// pairs this with SeekStatus.ReindexAttempted to tell the two apart.
func (d *Diagnosis) IndexSparse() bool {
	return d.IndexSparseDetail() != ""
}

// IndexSparseDetail is mkvgo's own account of the sparse index - "the video
// cues leave a 44s hole - ..." - or "" when the index is not sparse. Shown to
// the operator as-is: measured live on a library, 3 of 5 "coarse" files had
// cues every 2-10 s and the hole was only the final credits (last cue to the
// declared end), so a generic "the encode's keyframes are too far apart"
// would have lied about them.
func (d *Diagnosis) IndexSparseDetail() string {
	for _, f := range d.Findings {
		if f.Kind == "index-sparse" {
			if f.Detail == "" {
				return "index-sparse"
			}
			return f.Detail
		}
	}
	return ""
}

// WrongContainer reports that the file's extension lies about its content: a
// .mkv whose bytes are really ISO base media (MP4/MOV). mkvgo (>= v0.25.0)
// returns this as a structured finding instead of erroring. It has no seek-index
// defect class to repair - a Matroska reindex would fail - so the remedy is to
// rename/remux, not reindex. The scan routes it to its own actionable bucket.
func (d *Diagnosis) WrongContainer() bool {
	for _, f := range d.Findings {
		if f.Kind == "wrong-container" {
			return true
		}
	}
	return false
}

// TrailingJunk reports surplus bytes lying entirely PAST the declared Segment
// end: a valid, complete file with extra tail bytes (leftover junk, or a
// crashed in-place-repair journal). Since mkvgo v0.25 this is its own verdict,
// split out of the truncated/damaged pair: the media is whole, so the remedy is
// a harmless rewrite (any reindex drops them), NOT a re-download or a resync
// that would imply lost content. It carries no seek-index defect - the file
// stays out of the reindex bucket and surfaces only through this flag.
func (d *Diagnosis) TrailingJunk() bool {
	for _, f := range d.Findings {
		if f.Kind == "trailing-junk" {
			return true
		}
	}
	return false
}

// Damaged reports mid-Segment corruption a resync repair can skip past (losing
// the damaged span). It is distinct from TrailingJunk: surplus bytes past the
// declared end also inflate the salvage walk's BytesSkipped since v0.25, so a
// caller must not treat that byte count as damage unless this finding is
// present - a complete file with trailing junk has BytesSkipped > 0 yet nothing
// to repair.
func (d *Diagnosis) Damaged() bool {
	for _, f := range d.Findings {
		if f.Kind == "damaged" {
			return true
		}
	}
	return false
}

// MaxAudioDelayMs is the largest audio start delay across the file's tracks
// (ms, 0 when aligned) - the value the audio-delay flag floor is applied to.
func (d *Diagnosis) MaxAudioDelayMs() int64 {
	var m int64
	for _, ns := range d.AudioDelaysMs {
		if ns > m {
			m = ns
		}
	}
	return m
}

// Diagnoser classifies a media file in one head-mostly call (a tolerant walk
// only when the declared size diverges). Matroska/WebM only; a non-EBML
// container has no seek-index defect class and reports Healthy. Implemented by
// the mkvgo adapter.
type Diagnoser interface {
	Diagnose(ctx context.Context, path string) (*Diagnosis, error)
}
