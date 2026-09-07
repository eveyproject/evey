package ffmpeg

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// The override names the concrete AAC implementation the session was
// FINGERPRINTED with. It must win for the AAC family and be ignored everywhere
// else: asking for a libfdk build of ac3 or opus is meaningless, and honouring
// it would emit an encoder name ffmpeg does not have.
func TestResolveAudioEncoder_Override(t *testing.T) {
	assert.Equal(t, "aac", resolveAudioEncoder("aac"), "no override keeps the native encoder")
	assert.Equal(t, "aac", resolveAudioEncoder("aac", ""), "an empty override is not an override")
	assert.Equal(t, "libfdk_aac", resolveAudioEncoder("aac", "libfdk_aac"))
	assert.Equal(t, "libfdk_aac", resolveAudioEncoder("", "libfdk_aac"), "empty codec is the AAC default")

	// Everything outside the AAC family ignores it.
	assert.Equal(t, "ac3", resolveAudioEncoder("ac3", "libfdk_aac"))
	assert.Equal(t, "eac3", resolveAudioEncoder("eac3", "libfdk_aac"))
	assert.Equal(t, "libopus", resolveAudioEncoder("opus", "libfdk_aac"))
	// A passthrough must stay a passthrough - an override here would turn a
	// zero-CPU copy into a re-encode.
	assert.Equal(t, "copy", resolveAudioEncoder("copy", "libfdk_aac"))
}

// The probe is memoised, so it must at least be self-consistent and never
// answer something the arg builder cannot use.
func TestAACEncoder_AnswersAKnownEncoder(t *testing.T) {
	got := AACEncoder()
	assert.Contains(t, []string{NativeAACEncoder, PreferredAACEncoder}, got)
	assert.Equal(t, got, AACEncoder(), "memoised: the answer is a property of the binary")
}
