package httpapi

// transcode_vod_source_helpers.go - the source-fact helpers moved to the app
// layer with the PlaybackPlanResolver (app/transcoding/source_facts.go).
// These aliases keep the adapter's remaining call sites (manifest input,
// on-demand paths, audio handler, tests) untouched; new code should call the
// app package directly.

import (
	apptranscoding "github.com/evey-project/evey/internal/app/transcoding"
	domainlibrary "github.com/evey-project/evey/internal/domain/library"
	domaintranscoding "github.com/evey-project/evey/internal/domain/transcoding"
)

func sourceCopyVideoRange(streams []*domainlibrary.MediaStream) string {
	return apptranscoding.SourceCopyVideoRange(streams)
}

func sourceCopyVideoCodec(streams []*domainlibrary.MediaStream) string {
	return apptranscoding.SourceCopyVideoCodec(streams)
}

func audioCopyEligible(decision domaintranscoding.PlaybackDecision, normalize bool, profileCount int, srcCodec string, client domaintranscoding.ClientProfile) bool {
	return apptranscoding.AudioCopyEligible(decision, normalize, profileCount, srcCodec, client)
}

func servedAudioCodec(copyEligible bool, srcCodec string, decision domaintranscoding.PlaybackDecision) string {
	return apptranscoding.ServedAudioCodec(copyEligible, srcCodec, decision)
}

func sourceAudioCodec(streams []*domainlibrary.MediaStream, audioIdx int) string {
	return apptranscoding.SourceAudioCodec(streams, audioIdx)
}

func sourceSubtitleCodec(streams []*domainlibrary.MediaStream, subIdx int) string {
	return apptranscoding.SourceSubtitleCodec(streams, subIdx)
}

func sourceVideoTenBit(streams []*domainlibrary.MediaStream) bool {
	return apptranscoding.SourceVideoTenBit(streams)
}

func streamIsTenBit(s *domainlibrary.MediaStream) bool {
	return apptranscoding.StreamIsTenBit(s)
}

func sourceDimsOf(streams []*domainlibrary.MediaStream) apptranscoding.SourceDims {
	return apptranscoding.SourceDimsOf(streams)
}

// sourceBytesOf is the on-disk size of the source file, or 0 when unknown. The
// copy-rung disk guard treats 0 as "can't size it" → fail-open (allow copy).
func sourceBytesOf(item *domainlibrary.MediaItem) int64 {
	if item == nil || item.File == nil {
		return 0
	}
	return item.File.Size
}

// containerOf is item.File.ContainerFormat, guarded against a nil file.
func containerOf(item *domainlibrary.MediaItem) string {
	if item == nil || item.File == nil {
		return ""
	}
	return item.File.ContainerFormat
}

// sourcePathOf mirrors containerOf: the media file path when known, "" when
// the item has no file (the Playability shadow is inert on "").
func sourcePathOf(item *domainlibrary.MediaItem) string {
	if item == nil || item.File == nil {
		return ""
	}
	return item.File.Path
}
