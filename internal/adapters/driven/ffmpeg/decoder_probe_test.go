package ffmpeg

import "testing"

func TestHasAV1Decoder(t *testing.T) {
	// Abridged real `ffmpeg -decoders` shape: " V....D av1   ...".
	withDav1d := `Decoders:
 V..... = Video
 V....D h264                 H.264 / AVC
 V....D hevc                 H.265 / HEVC
 V....D libdav1d             dav1d AV1 decoder by VideoLAN
 A....D aac                  AAC`
	if !hasAV1Decoder(withDav1d) {
		t.Error("libdav1d present must be detected as an AV1 decoder")
	}

	withNative := ` V....D av1                  Alliance for Open Media AV1`
	if !hasAV1Decoder(withNative) {
		t.Error("native av1 decoder must be detected")
	}

	withCuvid := ` V....D av1_cuvid            Nvidia CUVID AV1 decoder`
	if !hasAV1Decoder(withCuvid) {
		t.Error("av1_cuvid must be detected")
	}

	noAV1 := `Decoders:
 V....D h264                 H.264 / AVC
 V....D hevc                 H.265 / HEVC
 V....D vp9                  Google VP9
 A....D aac                  AAC`
	if hasAV1Decoder(noAV1) {
		t.Error("a build without any AV1 decoder must not be detected as having one")
	}

	// "av1" appearing only inside a description column must not match (the
	// decoder name is the second field, not a substring of the description).
	descOnly := ` V....D someenc              fancy av1 lookalike encoder`
	if hasAV1Decoder(descOnly) {
		t.Error("an av1 mention in the description column must not count as a decoder")
	}

	if hasAV1Decoder("") {
		t.Error("empty output must report no AV1 decoder")
	}
}
