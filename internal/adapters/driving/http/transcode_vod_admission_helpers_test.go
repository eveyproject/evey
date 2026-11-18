package httpapi

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"

	apptranscoding "github.com/eveyproject/evey/internal/app/transcoding"
	domainlibrary "github.com/eveyproject/evey/internal/domain/library"
	domaintranscoding "github.com/eveyproject/evey/internal/domain/transcoding"
)

// A refusal is final only when no candidate height fits an idle host: one
// that fits keeps the retryable shed, and without the idle probe (or without
// a height to ask it) nothing is ever called too heavy.
func TestTooHeavyForIdleHost(t *testing.T) {
	fits := func(ok ...int) func(apptranscoding.DecisionFingerprint, int) bool {
		return func(_ apptranscoding.DecisionFingerprint, h int) bool { return slices.Contains(ok, h) }
	}
	cases := []struct {
		name    string
		probe   func(apptranscoding.DecisionFingerprint, int) bool
		heights []int
		want    bool
	}{
		{"no idle probe wired", nil, []int{1080, 720}, false},
		{"no candidate height", fits(), nil, false},
		{"none fits", fits(), []int{1080, 720, 480}, true},
		{"the last one fits", fits(480), []int{1080, 720, 480}, false},
		{"the first one fits", fits(1080), []int{1080, 720}, false},
		{"a height that fits is not a candidate", fits(240), []int{1080, 720}, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h := &hlsVODServe{videoFitsIdle: c.probe}
			assert.Equal(t, c.want, h.tooHeavyForIdleHost(apptranscoding.DecisionFingerprint{}, c.heights))
		})
	}
}

// "Still indexing" is said only of a source the copy rung would serve if its
// keyframe index existed: the client takes the video as it is, the video
// stream has no index yet, and everything else about it passes the copy gate.
func TestCopyBlockedOnlyByMissingIndex(t *testing.T) {
	copyable := func() *domainlibrary.MediaStream {
		level := 40
		return &domainlibrary.MediaStream{StreamType: "video", Codec: "h264", Profile: "High", Level: &level}
	}
	indexed := copyable()
	indexed.Keyframes = []int64{0, 2000}
	audio := &domainlibrary.MediaStream{StreamType: "audio", Codec: "aac"}
	copyDecision := domaintranscoding.PlaybackDecision{VideoDecision: "copy"}

	cases := []struct {
		name     string
		decision domaintranscoding.PlaybackDecision
		streams  []*domainlibrary.MediaStream
		want     bool
	}{
		{"copyable, no index yet", copyDecision, []*domainlibrary.MediaStream{audio, copyable()}, true},
		{"indexed already", copyDecision, []*domainlibrary.MediaStream{indexed}, false},
		{"the client cannot take the video as it is", domaintranscoding.PlaybackDecision{VideoDecision: "transcode"}, []*domainlibrary.MediaStream{copyable()}, false},
		{"no video stream", copyDecision, []*domainlibrary.MediaStream{audio, nil}, false},
		{"no stream at all", copyDecision, nil, false},
		{"a codec the copy rung does not carry", copyDecision, []*domainlibrary.MediaStream{{StreamType: "video", Codec: "mpeg2video"}}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, copyBlockedOnlyByMissingIndex(c.decision, c.streams))
		})
	}
}
