package ffmpeg

import (
	"bufio"
	"context"
	"log/slog"
	"strings"

	"github.com/evey-project/evey/internal/pkg/procfork"
)

// av1DecoderNames are the ffmpeg decoder names that can decode an AV1 video
// stream. A build with any one of these can play AV1 sources; a build with
// none cannot, and every AV1 file fails at playback time.
var av1DecoderNames = map[string]struct{}{
	"av1":        {}, // native (dav1d-backed in recent ffmpeg) or libaom auto
	"libdav1d":   {}, // the fast standalone AV1 decoder (jellyfin-ffmpeg ships it)
	"libaom-av1": {}, // reference decoder, slow but valid
	"av1_cuvid":  {}, // NVDEC
	"av1_qsv":    {}, // Intel QSV
}

// hasAV1Decoder parses the output of `ffmpeg -decoders` and reports whether any
// AV1 decoder is registered. The decoder table has a leading flags column then
// the decoder name in the second field. Pure so it is unit-testable without
// invoking ffmpeg.
func hasAV1Decoder(decodersOutput string) bool {
	scanner := bufio.NewScanner(strings.NewReader(decodersOutput))
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 2 {
			continue
		}
		if _, ok := av1DecoderNames[fields[1]]; ok {
			return true
		}
	}
	return false
}

// WarnIfNoAV1Decoder probes `ffmpeg -decoders` once and logs an actionable
// warning when the build ships no AV1 decoder, so AV1 source files don't fail
// playback with a cryptic ffmpeg error on a host whose ffmpeg lacks dav1d.
//
// Deliberately non-fatal and best-effort: a probe failure (ffmpeg missing, slow
// to answer, killed by the caller's timeout) is logged at debug and swallowed.
// It never blocks or aborts boot. The caller is expected to run it detached
// with a short timeout.
func WarnIfNoAV1Decoder(ctx context.Context, ffmpegPath string, logger *slog.Logger) {
	if logger == nil {
		logger = slog.Default()
	}
	out, err := procfork.Output(ctx, procfork.Options{Class: procfork.Probe, Site: "decoder_probe.decoders"}, ffmpegPath, "-hide_banner", "-decoders")
	if err != nil {
		logger.Debug("av1 decoder probe: ffmpeg -decoders failed", slog.Any("err", err))
		return
	}
	if !hasAV1Decoder(string(out)) {
		logger.Warn("no AV1 decoder found in this ffmpeg build (-decoders lists none of dav1d/aom/cuvid/qsv); AV1 source files will fail to play. Deploy a jellyfin-ffmpeg or ffmpeg build with libdav1d.")
	}
}
