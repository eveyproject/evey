package transcoding

import "testing"

// A seeked copy rendition used to start on the index point before its target
// and was stamped as starting on it: the segments already on disk carry audio
// seconds late. The copy's hash must have moved off the one those segments were
// cached under, or the fix would never reach a player.
func TestSeekedCopyFix_MovedTheCopyRenditionsOffTheirCache(t *testing.T) {
	fp := AudioFingerprint{MediaID: "m1", SourcePath: "/media/film.mkv", SourceTrackIdx: 0, AudioCodec: "copy", MaxAudioChannels: 6}
	const cachedBeforeTheFix = "33c52f17ab2baf17"
	if got := fp.Hash(); got == cachedBeforeTheFix {
		t.Fatalf("a copy rendition still hashes as before the fix (%s)", got)
	}
}
