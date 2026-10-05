package offlineaudio

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gravity-zero/ac3go/ac3"
	"github.com/gravity-zero/ac3go/pcm"
	"github.com/gravity-zero/mkvgo/matroska"
	"github.com/gravity-zero/mkvgo/mkv"
	"github.com/gravity-zero/mkvgo/mkv/reader"
	"github.com/gravity-zero/mkvgo/mkv/writer"
)

// ascAACLC48kStereo is a real AudioSpecificConfig (AOT 2, 48 kHz, 2 ch), the
// bytes MediaCodec hands back as csd-0 for that configuration.
var ascAACLC48kStereo = []byte{0x11, 0x90}

// ac3Frames cuts the elementary testdata stream into its syncframes.
func ac3Frames(t *testing.T, path string) [][]byte {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	fr := ac3.NewFrameReader(f)
	var frames [][]byte
	for {
		frame, err := fr.Next()
		if errors.Is(err, io.EOF) {
			return frames
		}
		if err != nil {
			t.Fatal(err)
		}
		frames = append(frames, append([]byte(nil), frame...))
	}
}

type fixtureTrack struct {
	track  mkv.Track
	blocks []mkv.Block
}

// writeFixtureMKV assembles a small but structurally honest MKV: real
// metadata, clustered blocks, Cues, sealed sizes - the same writer the
// session itself uses for its intermediate file.
func writeFixtureMKV(t *testing.T, path string, durationMs int64, fts ...fixtureTrack) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	var tracks []mkv.Track
	var all []mkv.Block
	for _, ft := range fts {
		tracks = append(tracks, ft.track)
		all = append(all, ft.blocks...)
	}
	c := &mkv.Container{
		Info:   mkv.SegmentInfo{TimecodeScale: 1_000_000, MuxingApp: "fixture", WritingApp: "fixture"},
		Tracks: tracks,
	}
	mw := writer.NewMKVWriter(f)
	if err := mw.WriteStart(); err != nil {
		t.Fatal(err)
	}
	if err := mw.WriteMetadata(c, tracks, durationMs); err != nil {
		t.Fatal(err)
	}
	// One cluster per second, blocks in timestamp order across tracks.
	for start := int64(0); ; start += 1000 {
		var batch []mkv.Block
		for _, b := range all {
			if b.Timecode >= start && b.Timecode < start+1000 {
				batch = append(batch, b)
			}
		}
		if len(batch) == 0 {
			break
		}
		if err := mw.WriteClusterWithCues(start, 1_000_000, batch); err != nil {
			t.Fatal(err)
		}
	}
	if err := mw.Finalize(); err != nil {
		t.Fatal(err)
	}
}

func audioTrack(id uint64, codec, lang string, isDefault bool, channels uint8, rate float64) mkv.Track {
	return mkv.Track{
		ID: id, Type: mkv.AudioTrack, Codec: codec, Language: lang,
		IsDefault: isDefault, Channels: &channels, SampleRate: &rate,
	}
}

// ac3FixtureMKV builds the standard test source: the 5.1 tone as track 1
// (French, default) plus a subtitle track that must survive the pass intact.
func ac3FixtureMKV(t *testing.T) (string, [][]byte) {
	t.Helper()
	frames := ac3Frames(t, filepath.Join("testdata", "tone_48k_5p1_448k.ac3"))
	if len(frames) == 0 {
		t.Fatal("no frames in testdata")
	}
	var audio []mkv.Block
	for i, frame := range frames {
		audio = append(audio, mkv.Block{
			TrackNumber: 1, Timecode: int64(i) * 32, Keyframe: true, Data: frame,
		})
	}
	sub := []mkv.Block{
		{TrackNumber: 2, Timecode: 10, Keyframe: true, Data: []byte("bonjour"), Duration: 100},
	}
	path := filepath.Join(t.TempDir(), "src.mkv")
	writeFixtureMKV(t, path, int64(len(frames))*32,
		fixtureTrack{track: audioTrack(1, "ac3", "fre", true, 6, 48000), blocks: audio},
		fixtureTrack{track: mkv.Track{ID: 2, Type: mkv.SubtitleTrack, Codec: "srt", Language: "fre"}, blocks: sub},
	)
	return path, frames
}

// directStereoDecode is the reference the session's PCM must match: the same
// decoder walked over the same frames, downmixed the same way.
func directStereoDecode(t *testing.T, frames [][]byte) []byte {
	t.Helper()
	d := ac3.NewDecoder()
	if err := d.SetDownmix(pcm.LayoutStereo); err != nil {
		t.Fatal(err)
	}
	var out []byte
	for _, frame := range frames {
		if err := d.DecodeFrame(frame); err != nil {
			t.Fatal(err)
		}
		l, r := d.Samples(0), d.Samples(1)
		for i := range l {
			lv, rv := s16le(l[i]), s16le(r[i])
			out = append(out, byte(lv), byte(lv>>8), byte(rv), byte(rv>>8))
		}
	}
	return out
}

func TestOpenPicksConvertibleOverDefaultAAC(t *testing.T) {
	frames := ac3Frames(t, filepath.Join("testdata", "tone_48k_stereo_192k.ac3"))
	var audio []mkv.Block
	for i, frame := range frames {
		audio = append(audio, mkv.Block{TrackNumber: 2, Timecode: int64(i) * 32, Keyframe: true, Data: frame})
	}
	path := filepath.Join(t.TempDir(), "twoaudio.mkv")
	writeFixtureMKV(t, path, int64(len(frames))*32,
		fixtureTrack{track: audioTrack(1, "aac", "eng", true, 2, 48000)},
		fixtureTrack{track: audioTrack(2, "ac3", "fre", false, 2, 48000), blocks: audio},
	)

	s, err := Open(path, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if s.SourceCodec() != "ac3" {
		t.Fatalf("picked codec %q, want ac3", s.SourceCodec())
	}
	if s.SampleRate() != 48000 {
		t.Fatalf("sample rate %d, want 48000", s.SampleRate())
	}
}

func TestOpenRefusesDTSOnly(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dts.mkv")
	writeFixtureMKV(t, path, 1000,
		fixtureTrack{track: audioTrack(1, "dts", "eng", true, 6, 48000)},
	)
	_, err := Open(path, t.TempDir())
	if err == nil || !strings.HasPrefix(err.Error(), "unsupported-codec:dts") {
		t.Fatalf("err = %v, want unsupported-codec:dts", err)
	}
}

func TestOpenRefusesNoAudio(t *testing.T) {
	path := filepath.Join(t.TempDir(), "subsonly.mkv")
	writeFixtureMKV(t, path, 1000,
		fixtureTrack{
			track:  mkv.Track{ID: 1, Type: mkv.SubtitleTrack, Codec: "srt", Language: "eng"},
			blocks: []mkv.Block{{TrackNumber: 1, Timecode: 0, Keyframe: true, Data: []byte("x"), Duration: 50}},
		},
	)
	_, err := Open(path, t.TempDir())
	if err == nil || !strings.HasPrefix(err.Error(), "no-audio") {
		t.Fatalf("err = %v, want no-audio", err)
	}
}

func TestOpenRefusesNonMatroska(t *testing.T) {
	path := filepath.Join(t.TempDir(), "junk.mkv")
	if err := os.WriteFile(path, []byte("this is not a matroska file at all"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Open(path, t.TempDir())
	if err == nil || !strings.HasPrefix(err.Error(), "not-matroska") {
		t.Fatalf("err = %v, want not-matroska", err)
	}
}

func TestReadPCMMatchesDirectDecode(t *testing.T) {
	path, frames := ac3FixtureMKV(t)
	s, err := Open(path, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	if s.SourceChannels() != 6 {
		t.Fatalf("source channels %d, want 6", s.SourceChannels())
	}
	var got []byte
	for {
		chunk, err := s.ReadPCM()
		if err != nil {
			t.Fatal(err)
		}
		if len(chunk) == 0 {
			break
		}
		got = append(got, chunk...)
	}
	want := directStereoDecode(t, frames)
	if !bytes.Equal(got, want) {
		t.Fatalf("PCM differs from direct decode: got %d bytes, want %d", len(got), len(want))
	}
	if s.DecodeProgressPermille() != 1000 {
		t.Fatalf("decode progress %d, want 1000", s.DecodeProgressPermille())
	}
}

// A downloaded E-AC-3 5.1 film comes out of the pass with its downmix, not
// silence: the AAC written over the source track is all the file keeps.
func TestReadPCMEAC3IsHeard(t *testing.T) {
	frames := ac3Frames(t, filepath.Join("testdata", "tones_48k_5p1_384k.eac3"))
	var audio []mkv.Block
	for i, frame := range frames {
		audio = append(audio, mkv.Block{TrackNumber: 1, Timecode: int64(i) * 32, Keyframe: true, Data: frame})
	}
	path := filepath.Join(t.TempDir(), "eac3.mkv")
	writeFixtureMKV(t, path, int64(len(frames))*32,
		fixtureTrack{track: audioTrack(1, "eac3", "fre", true, 6, 48000), blocks: audio},
	)
	s, err := Open(path, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	var got []byte
	for {
		chunk, err := s.ReadPCM()
		if err != nil {
			t.Fatal(err)
		}
		if len(chunk) == 0 {
			break
		}
		got = append(got, chunk...)
	}
	assertHeardAsMix(t, "offline pass", got, handMixedStereo(t, frames))
}

func TestFullPassReplacesAudioKeepsSubs(t *testing.T) {
	path, _ := ac3FixtureMKV(t)
	tmp := t.TempDir()
	s, err := Open(path, tmp)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	var totalSamples int64
	for {
		chunk, err := s.ReadPCM()
		if err != nil {
			t.Fatal(err)
		}
		if len(chunk) == 0 {
			break
		}
		totalSamples += int64(len(chunk)) / 4 // stereo s16le
	}

	if err := s.BeginAAC(ascAACLC48kStereo); err != nil {
		t.Fatal(err)
	}
	aacFrames := (totalSamples + aacFrameSamples - 1) / aacFrameSamples
	fakeFrame := bytes.Repeat([]byte{0xA5}, 64)
	for i := int64(0); i < aacFrames; i++ {
		if err := s.WriteAAC(fakeFrame); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.FinishAAC(); err != nil {
		t.Fatal(err)
	}

	out := filepath.Join(tmp, "out.mkv")
	if err := s.Remux(out); err != nil {
		t.Fatal(err)
	}
	if s.RemuxPermille() != 1000 {
		t.Fatalf("remux progress %d, want 1000", s.RemuxPermille())
	}

	c, err := matroska.OpenMeta(context.Background(), out)
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Tracks) != 2 {
		t.Fatalf("output has %d tracks, want 2", len(c.Tracks))
	}
	var aac, sub *mkv.Track
	for i := range c.Tracks {
		switch c.Tracks[i].Type {
		case mkv.AudioTrack:
			aac = &c.Tracks[i]
		case mkv.SubtitleTrack:
			sub = &c.Tracks[i]
		}
	}
	if aac == nil || sub == nil {
		t.Fatalf("missing track: audio=%v sub=%v", aac, sub)
	}
	if aac.Codec != "aac" {
		t.Fatalf("audio codec %q, want aac", aac.Codec)
	}
	if aac.Language != "fre" {
		t.Fatalf("audio language %q, want fre", aac.Language)
	}
	if !aac.IsDefault {
		t.Fatal("replacement audio track lost the default flag")
	}
	if !bytes.Equal(aac.CodecPrivate, ascAACLC48kStereo) {
		t.Fatalf("CodecPrivate %x, want %x", aac.CodecPrivate, ascAACLC48kStereo)
	}

	issues, err := matroska.Validate(context.Background(), out)
	if err != nil {
		t.Fatal(err)
	}
	for _, is := range issues {
		if is.Severity == mkv.SeverityError {
			t.Fatalf("output fails validation: %s", is.Message)
		}
	}

	// The audio blocks must all be there with monotonic timestamps.
	f, err := os.Open(out)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	br, err := reader.NewBlockReader(f, c.Info.TimecodeScale)
	if err != nil {
		t.Fatal(err)
	}
	br.KeepTracks(aac.ID)
	var n, lastTs int64 = 0, -1
	for {
		blk, err := br.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if blk.Timecode < lastTs {
			t.Fatalf("timestamps go backwards: %d after %d", blk.Timecode, lastTs)
		}
		lastTs = blk.Timecode
		n++
	}
	if n != aacFrames {
		t.Fatalf("output carries %d audio blocks, want %d", n, aacFrames)
	}
}

func TestWriteAACOutOfSequence(t *testing.T) {
	path, _ := ac3FixtureMKV(t)
	s, err := Open(path, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.WriteAAC([]byte{1, 2, 3}); err == nil || !strings.HasPrefix(err.Error(), "out-of-sequence") {
		t.Fatalf("err = %v, want out-of-sequence", err)
	}
	if err := s.Remux(filepath.Join(t.TempDir(), "out.mkv")); err == nil || !strings.HasPrefix(err.Error(), "out-of-sequence") {
		t.Fatalf("err = %v, want out-of-sequence", err)
	}
}

func TestCloseRemovesIntermediate(t *testing.T) {
	path, _ := ac3FixtureMKV(t)
	tmp := t.TempDir()
	s, err := Open(path, tmp)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.BeginAAC(ascAACLC48kStereo); err != nil {
		t.Fatal(err)
	}
	s.Close()
	if _, err := os.Stat(filepath.Join(tmp, "evey-offline-audio.mka")); !os.IsNotExist(err) {
		t.Fatalf("intermediate MKA still there after Close (stat err: %v)", err)
	}
}

// A stream carrying a SECOND independent programme (substream id 1) next to
// the first comes out as the first programme alone, and with no frame counted
// as damaged: the decoder refuses the second programme, so stepping over it
// only on a decode failure reported half of a healthy stream as skipped.
func TestReadPCMSkipsASecondIndependentProgramme(t *testing.T) {
	frames := ac3Frames(t, filepath.Join("testdata", "tones_48k_5p1_384k.eac3"))
	if len(frames) == 0 {
		t.Fatal("no frames in testdata")
	}
	var audio []mkv.Block
	for i, frame := range frames {
		// The same frame again, relabelled as programme 1: strmtyp stays
		// independent (00), substreamid (the next 3 bits of byte 2) becomes 1.
		other := append([]byte(nil), frame...)
		other[2] = other[2]&0b11000111 | 1<<3
		resealEAC3(other)
		var hdr ac3.Header
		if err := ac3.ParseHeader(other, &hdr); err != nil || hdr.Sync.Substreamid != 1 || hdr.Sync.Strmtyp == ac3.StrmtypDependent {
			t.Fatalf("fixture: the relabelled frame reads as strmtyp %d id %d (err %v), want an independent programme 1", hdr.Sync.Strmtyp, hdr.Sync.Substreamid, err)
		}
		audio = append(audio, mkv.Block{
			TrackNumber: 1, Timecode: int64(i) * 32, Keyframe: true,
			Data: append(append([]byte(nil), frame...), other...),
		})
	}
	// The fixture must be what it claims: both programmes reach the reader.
	fr := ac3.NewFrameReader(bytes.NewReader(audio[0].Data))
	for n := range 2 {
		if _, err := fr.Next(); err != nil {
			t.Fatalf("fixture: the reader handed out %d frame(s) of the first block, want 2 (%v)", n, err)
		}
	}
	path := filepath.Join(t.TempDir(), "two-programmes.mkv")
	writeFixtureMKV(t, path, int64(len(frames))*32,
		fixtureTrack{track: audioTrack(1, "eac3", "fre", true, 6, 48000), blocks: audio},
	)
	s, err := Open(path, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	var got []byte
	for {
		chunk, err := s.ReadPCM()
		if err != nil {
			t.Fatal(err)
		}
		if len(chunk) == 0 {
			break
		}
		got = append(got, chunk...)
	}
	want := directStereoDecode(t, frames)
	if len(got) != len(want) {
		t.Fatalf("PCM is %d bytes, want %d", len(got), len(want))
	}
	if !bytes.Equal(got, want) {
		t.Fatal("PCM differs from the first programme decoded alone")
	}
	if n := s.SkippedFrames(); n != 0 {
		t.Fatalf("%d frames counted as skipped on a healthy stream, want 0: the second programme is not damage", n)
	}
}

// resealEAC3 recomputes an E-AC-3 syncframe's trailing CRC after an edit, so
// the frame reader keeps treating it as a sound frame rather than as damage.
// CRC-16, polynomial 0x8005, over everything after the syncword.
func resealEAC3(frame []byte) {
	var crc uint16
	for _, b := range frame[2 : len(frame)-2] {
		crc ^= uint16(b) << 8
		for range 8 {
			if crc&0x8000 != 0 {
				crc = crc<<1 ^ 0x8005
			} else {
				crc <<= 1
			}
		}
	}
	frame[len(frame)-2], frame[len(frame)-1] = byte(crc>>8), byte(crc)
}
