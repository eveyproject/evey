package mkv

import "context"

// SubtitleFormat is the on-disk format of an extracted subtitle file.
type SubtitleFormat string

const (
	// SubtitleFormatSRT is SubRip text (UTF-8, *.srt). Supported for all
	// subtitle tracks with a text-based codec (srt, utf8, tx3g …).
	SubtitleFormatSRT SubtitleFormat = "srt"

	// SubtitleFormatASS is Advanced SubStation Alpha (*.ass / *.ssa).
	// Supported only when the source track codec is "ass" or "ssa";
	// the extractor returns ErrUnsupportedCodec for other codecs.
	SubtitleFormatASS SubtitleFormat = "ass"
)

// SubtitleExtractor extracts an embedded subtitle track from a Matroska
// container to a standalone file. Only EBML containers are supported;
// non-EBML paths return ErrNotEBML.
type SubtitleExtractor interface {
	// ExtractSubtitle extracts subtitle track trackID from srcPath and writes it
	// to outPath. The format is inferred from outPath's extension when format is
	// ""; otherwise the caller requests a specific format explicitly.
	// Returns ErrNotEBML for non-Matroska files, ErrTrackNotFound when the
	// track is absent, and ErrUnsupportedCodec when the requested format
	// is incompatible with the track codec.
	ExtractSubtitle(ctx context.Context, req ExtractSubtitleRequest) error
}

// ExtractSubtitleRequest parameterises a subtitle extraction.
type ExtractSubtitleRequest struct {
	// SrcPath is the absolute path to the source MKV/MKA/MKS file.
	SrcPath string

	// TrackID is the Matroska TrackNumber (1-based) of the subtitle track.
	TrackID uint64

	// OutPath is the destination file path. The parent directory must exist.
	OutPath string

	// Format selects the output format. When empty the implementation infers
	// the format from OutPath's extension (.srt → SRT, .ass/.ssa → ASS).
	// An explicit format overrides the extension.
	Format SubtitleFormat
}
