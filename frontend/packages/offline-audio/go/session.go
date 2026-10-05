// Package offlineaudio makes a downloaded Matroska file renderable on the
// phone by replacing its default AC-3/E-AC-3 audio track with a stereo AAC
// one, video and every other track copied verbatim.
//
// The package deliberately does NOT encode AAC: Android's MediaCodec is a
// hardware encoder the platform ships everywhere, and a pure-Go encoder would
// be both slower and worse. The pipeline is therefore driven from the Kotlin
// side as a pull loop:
//
//	s := Open(in, tmpDir)          // picks the track, refuses DTS/TrueHD
//	pcm := s.ReadPCM()             // stereo s16le, downmixed by ac3go
//	  ... feed MediaCodec, drain AAC frames ...
//	s.BeginAAC(csd0)               // AudioSpecificConfig from the encoder
//	s.WriteAAC(frame)              // timestamps derived from the frame count
//	s.FinishAAC()
//	s.Remux(outPath)               // mkvgo Mux: source tracks + the new AAC
//	s.Close()
//
// Every exported signature sticks to gomobile's type subset (basic types,
// []byte, string, error) so the same package binds to an AAR unchanged.
package offlineaudio

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"

	"github.com/gravity-zero/ac3go/ac3"
	"github.com/gravity-zero/ac3go/pcm"
	"github.com/gravity-zero/mkvgo/matroska"
	"github.com/gravity-zero/mkvgo/mkv"
	"github.com/gravity-zero/mkvgo/mkv/reader"
	"github.com/gravity-zero/mkvgo/mkv/writer"
)

// Stable error prefixes the JS side routes on. gomobile carries a Go error
// across JNI as an exception message, so the message IS the contract.
const (
	errNotMatroska   = "not-matroska"
	errNoAudio       = "no-audio"
	errUnsupported   = "unsupported-codec"
	errNoConvertible = "no-convertible-track"
	errNoDecodable   = "no-decodable-audio"
	errOutOfSequence = "out-of-sequence"
)

// codecs ac3go decodes. Anything else on the picked track is refused with a
// typed error so the caller can offer the server-side fallback instead.
var convertibleCodecs = map[string]bool{"ac3": true, "eac3": true}

// aacFrameSamples is the AAC-LC frame size MediaCodec produces. One encoded
// frame always spans 1024 PCM samples per channel, which is what lets the
// writer derive every timestamp from the frame count alone.
const aacFrameSamples = 1024

// pcmChunkTarget is how many bytes ReadPCM aims to return per call. Each JNI
// crossing copies the slice, so the chunk is sized to amortise the call
// without holding a large buffer: ~21 AC-3 frames at 48 kHz.
const pcmChunkTarget = 128 * 1024

// clusterSpanMs bounds the intermediate MKA's cluster length. SimpleBlock
// offsets are int16 milliseconds from the cluster start, and 1 s keeps the
// audio-only cue throttling (1 cue per 500 ms) meaningful.
const clusterSpanMs = 1000

// Session is one compatibility pass over one file. Not safe for concurrent
// method calls except the progress getters, which are atomic and meant to be
// polled from another thread while ReadPCM or Remux blocks.
type Session struct {
	inPath  string
	mkaPath string

	src        *matroska.Container
	track      *mkv.Track // the AC-3/E-AC-3 track being replaced
	sampleRate int
	srcChans   int

	file    *os.File
	counted *countingReadSeeker
	frames  *ac3.FrameReader
	dec     *ac3.Decoder
	blocks  *audioByteReader

	pending   []byte // decoded PCM not yet returned by ReadPCM
	eof       bool
	decoded   int64 // frames successfully decoded
	badFrames int64 // frames skipped (damaged or unsupported)

	mkaFile  *os.File
	mka      *writer.MKVWriter
	aacTrack mkv.Track
	aacCount int64 // AAC frames written, drives every timestamp
	batch    []mkv.Block
	batchTs  int64
	mkaDone  bool

	decodePermille atomic.Int64
	remuxPermille  atomic.Int64
}

// Open probes inPath, picks the audio track to convert and primes the decoder
// far enough to know the sample rate (the encoder needs it before any PCM
// flows). tmpDir hosts the intermediate audio-only MKA; the caller passes the
// app cache dir so partial passes never litter the public Downloads folder.
func Open(inPath, tmpDir string) (*Session, error) {
	ctx := context.Background()
	src, err := matroska.OpenMeta(ctx, inPath)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", errNotMatroska, err)
	}

	track, err := pickTrack(src.Tracks)
	if err != nil {
		return nil, err
	}

	f, err := os.Open(inPath)
	if err != nil {
		return nil, err
	}
	st, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, err
	}

	counted := &countingReadSeeker{f: f, size: st.Size()}
	br, err := reader.NewBlockReader(counted, src.Info.TimecodeScale)
	if err != nil {
		f.Close()
		return nil, fmt.Errorf("%s: %w", errNotMatroska, err)
	}
	br.KeepTracks(track.ID)
	br.SetTrackDefaultDurations(reader.TrackDefaultDurations(src.Tracks))

	s := &Session{
		inPath:  inPath,
		mkaPath: filepath.Join(tmpDir, "evey-offline-audio.mka"),
		src:     src,
		track:   track,
		file:    f,
		counted: counted,
		blocks:  &audioByteReader{br: br, track: track},
		dec:     ac3.NewDecoder(),
	}
	s.frames = ac3.NewFrameReader(s.blocks)
	if err := s.dec.SetDownmix(pcm.LayoutStereo); err != nil {
		f.Close()
		return nil, err
	}

	// Decode one access unit now: the sample rate lives in the bitstream, not
	// the container (the container value can lie), and the encoder must be
	// configured before the first ReadPCM.
	if err := s.decodeNext(); err != nil {
		f.Close()
		return nil, err
	}
	return s, nil
}

// pickTrack chooses the track the pass replaces: the default AC-3/E-AC-3
// audio track, else the first one. A file whose audio exists but is all
// beyond ac3go (DTS, TrueHD...) gets the unsupported error naming the codec,
// so the caller can surface the honest "needs the server" message.
func pickTrack(tracks []mkv.Track) (*mkv.Track, error) {
	var first, def *mkv.Track
	var audioSeen []string
	for i := range tracks {
		t := &tracks[i]
		if t.Type != mkv.AudioTrack {
			continue
		}
		audioSeen = append(audioSeen, t.Codec)
		if !convertibleCodecs[t.Codec] {
			continue
		}
		if first == nil {
			first = t
		}
		if def == nil && t.IsDefault {
			def = t
		}
	}
	if def != nil {
		return def, nil
	}
	if first != nil {
		return first, nil
	}
	if len(audioSeen) == 0 {
		return nil, errors.New(errNoAudio)
	}
	for _, c := range audioSeen {
		if c == "dts" || c == "truehd" {
			return nil, fmt.Errorf("%s:%s", errUnsupported, c)
		}
	}
	return nil, fmt.Errorf("%s:%s", errNoConvertible, strings.Join(audioSeen, ","))
}

// SampleRate is the decoded stream's rate in Hz, known from Open on.
func (s *Session) SampleRate() int { return s.sampleRate }

// SourceCodec is the codec of the track being replaced ("ac3" or "eac3").
func (s *Session) SourceCodec() string { return s.track.Codec }

// SourceChannels is the coded channel count before the stereo downmix.
func (s *Session) SourceChannels() int { return s.srcChans }

// DurationMs is the source's declared duration, for the caller's progress UI.
func (s *Session) DurationMs() int64 { return s.src.DurationMs }

// DecodeProgressPermille reports how far the demux+decode walk is through the
// source file, 0..1000. Safe to poll from any thread.
func (s *Session) DecodeProgressPermille() int64 { return s.decodePermille.Load() }

// RemuxProgressPermille reports the final mux's progress, 0..1000. Safe to
// poll from any thread while Remux blocks.
func (s *Session) RemuxPermille() int64 { return s.remuxPermille.Load() }

// ReadPCM returns the next chunk of downmixed stereo PCM, interleaved s16le,
// at SampleRate. It returns an empty slice at the end of the stream. Damaged
// frames are skipped, not fatal: a movie with one bad frame must not lose its
// sound over it.
func (s *Session) ReadPCM() ([]byte, error) {
	for !s.eof && len(s.pending) < pcmChunkTarget {
		if err := s.decodeNext(); err != nil {
			return nil, err
		}
	}
	out := s.pending
	s.pending = nil
	if s.counted.size > 0 {
		s.decodePermille.Store(s.counted.pos() * 1000 / s.counted.size)
	}
	return out, nil
}

// decodeNext advances by one syncframe: append its downmixed samples to the
// pending buffer, or record why it was skipped. Sets eof at stream end.
func (s *Session) decodeNext() error {
	for {
		frame, err := s.frames.Next()
		if errors.Is(err, io.EOF) {
			s.eof = true
			s.decodePermille.Store(1000)
			if s.decoded == 0 {
				return errors.New(errNoDecodable)
			}
			return nil
		}
		if err != nil {
			// The frame reader resyncs past damage on its own; an error here
			// is the underlying read failing, which no retry loop fixes.
			return err
		}
		var hdr ac3.Header
		if err := ac3.ParseHeader(frame, &hdr); err != nil {
			s.badFrames++
			continue
		}
		// A dependent substream only carries the side/back channels of a 7.1
		// programme. The frame reader hands substreams out one by one, so the
		// 5.1 core was already decoded and downmixed when this one shows up;
		// skipping it trades the 7.1 merge for a pipeline with no buffering,
		// which a stereo target can afford.
		// A second independent programme (substream id above 0) is skipped the
		// same way. The decoder refuses it anyway, but as a failure: a healthy
		// two-programme stream then reported half its frames as damaged.
		if !hdr.Sync.AdvancesTime() {
			continue
		}
		if err := s.dec.DecodeFrame(frame); err != nil {
			s.badFrames++
			continue
		}
		s.decoded++
		s.appendPCM()
		return nil
	}
}

// appendPCM interleaves the decoder's stereo planes into pending as s16le.
// One buffer grow per frame, zero per-sample allocations: the first on-device
// run built a 2-byte slice per sample (~184M allocations for one film) and
// spent its time in the allocator instead of the decoder.
func (s *Session) appendPCM() {
	if s.sampleRate == 0 {
		h := s.dec.Header()
		s.sampleRate = h.Sync.SampleRate
		s.srcChans = h.Channels()
	}
	s.pending = appendStereoS16(s.pending, s.dec.Samples(0), s.dec.Samples(1))
}

// appendStereoS16 appends the two planes to dst interleaved as s16le.
func appendStereoS16(dst []byte, l, r []float32) []byte {
	base := len(dst)
	dst = append(dst, make([]byte, len(l)*4)...)
	out := dst[base:]
	for i := range l {
		lv, rv := s16le(l[i]), s16le(r[i])
		out[i*4] = byte(lv)
		out[i*4+1] = byte(lv >> 8)
		out[i*4+2] = byte(rv)
		out[i*4+3] = byte(rv >> 8)
	}
	return dst
}

func s16le(v float32) uint16 {
	f := v * 32767
	if f > 32767 {
		f = 32767
	}
	if f < -32768 {
		f = -32768
	}
	return uint16(int16(f))
}

// BeginAAC opens the intermediate audio-only MKA with the encoder's
// AudioSpecificConfig (MediaCodec csd-0) as CodecPrivate. Must be called
// before the first WriteAAC; the encoder only reveals the config after it
// has consumed some PCM, which is why this is a separate step from Open.
func (s *Session) BeginAAC(asc []byte) error {
	if s.mka != nil {
		return errors.New(errOutOfSequence + ": BeginAAC called twice")
	}
	if s.sampleRate == 0 {
		return errors.New(errOutOfSequence + ": BeginAAC before any decoded frame")
	}
	f, err := os.Create(s.mkaPath)
	if err != nil {
		return err
	}
	chans := uint8(2)
	rate := float64(s.sampleRate)
	s.aacTrack = mkv.Track{
		ID:                1,
		Type:              mkv.AudioTrack,
		Codec:             "aac",
		Language:          s.track.Language,
		Name:              s.track.Name,
		IsDefault:         true,
		CodecPrivate:      append([]byte(nil), asc...),
		Channels:          &chans,
		SampleRate:        &rate,
		DefaultDurationNs: int64(aacFrameSamples) * 1_000_000_000 / int64(s.sampleRate),
	}
	mw := writer.NewMKVWriter(f)
	if err := mw.WriteStart(); err != nil {
		f.Close()
		return err
	}
	meta := &mkv.Container{
		Info: mkv.SegmentInfo{
			TimecodeScale: 1_000_000,
			MuxingApp:     "evey-offline-audio",
			WritingApp:    "evey-offline-audio",
		},
		Tracks: []mkv.Track{s.aacTrack},
	}
	if err := mw.WriteMetadata(meta, meta.Tracks, s.src.DurationMs); err != nil {
		f.Close()
		return err
	}
	s.mkaFile = f
	s.mka = mw
	return nil
}

// WriteAAC appends one encoded AAC frame. Its timestamp is the frame count
// times 1024 samples, offset by the source track's first block time so the
// replacement track starts where the original did (A/V sync preserved).
func (s *Session) WriteAAC(frame []byte) error {
	if s.mka == nil || s.mkaDone {
		return errors.New(errOutOfSequence + ": WriteAAC outside BeginAAC/FinishAAC")
	}
	ts := s.blocks.firstTs + s.aacCount*aacFrameSamples*1000/int64(s.sampleRate)
	if len(s.batch) > 0 && ts-s.batchTs >= clusterSpanMs {
		if err := s.flushBatch(); err != nil {
			return err
		}
	}
	if len(s.batch) == 0 {
		s.batchTs = ts
	}
	s.batch = append(s.batch, mkv.Block{
		TrackNumber: 1,
		Timecode:    ts,
		Keyframe:    true,
		Data:        append([]byte(nil), frame...), // gomobile slices die with the call
	})
	s.aacCount++
	return nil
}

func (s *Session) flushBatch() error {
	if len(s.batch) == 0 {
		return nil
	}
	err := s.mka.WriteClusterWithCues(s.batchTs, 1_000_000, s.batch)
	s.batch = s.batch[:0]
	return err
}

// FinishAAC seals the intermediate MKA (last cluster, Cues, sizes).
func (s *Session) FinishAAC() error {
	if s.mka == nil || s.mkaDone {
		return errors.New(errOutOfSequence + ": FinishAAC outside BeginAAC")
	}
	if err := s.flushBatch(); err != nil {
		return err
	}
	if err := s.mka.Finalize(); err != nil {
		return err
	}
	s.mkaDone = true
	return s.mkaFile.Close()
}

// AACFrames is how many encoded frames were written, for sanity logging.
func (s *Session) AACFrames() int64 { return s.aacCount }

// SkippedFrames is how many source frames were dropped as damaged or
// unsupported, for sanity logging on the caller side.
func (s *Session) SkippedFrames() int64 { return s.badFrames }

// Remux assembles outPath: every source track verbatim except the converted
// one, which is taken from the intermediate MKA. mkvgo's Mux interleaves with
// bounded memory and writes fresh Cues and per-track statistics.
func (s *Session) Remux(outPath string) error {
	if !s.mkaDone {
		return errors.New(errOutOfSequence + ": Remux before FinishAAC")
	}
	var inputs []mkv.TrackInput
	for i := range s.src.Tracks {
		t := &s.src.Tracks[i]
		if t.ID == s.track.ID {
			inputs = append(inputs, mkv.TrackInput{
				SourcePath: s.mkaPath,
				TrackID:    1,
				Language:   t.Language,
				Name:       t.Name,
				IsDefault:  t.IsDefault,
			})
			continue
		}
		inputs = append(inputs, mkv.TrackInput{
			SourcePath: s.inPath,
			TrackID:    t.ID,
			Language:   t.Language,
			Name:       t.Name,
			IsDefault:  t.IsDefault,
		})
	}
	opts := mkv.MuxOptions{
		OutputPath:  outPath,
		Tracks:      inputs,
		Title:       s.src.Info.Title,
		Chapters:    s.src.Chapters,
		Attachments: s.src.Attachments,
	}
	err := matroska.Mux(context.Background(), opts, mkv.Options{
		Progress: func(processed, total int64) {
			if total > 0 {
				s.remuxPermille.Store(processed * 1000 / total)
			}
		},
	})
	if err == nil {
		s.remuxPermille.Store(1000)
	}
	return err
}

// Close releases the source handle and removes the intermediate MKA. Always
// safe to call, including after an error mid-pass.
func (s *Session) Close() {
	if s.file != nil {
		s.file.Close()
		s.file = nil
	}
	if s.mkaFile != nil {
		s.mkaFile.Close()
		s.mkaFile = nil
	}
	os.Remove(s.mkaPath)
}

// audioByteReader adapts the kept track's block stream into the io.Reader
// ac3go's FrameReader consumes, restoring stripped headers and remembering
// the first block's timestamp (the replacement track's start offset).
type audioByteReader struct {
	br      *reader.BlockReader
	track   *mkv.Track
	buf     []byte
	firstTs int64
	seen    bool
}

func (a *audioByteReader) Read(p []byte) (int, error) {
	for len(a.buf) == 0 {
		blk, err := a.br.Next()
		if err != nil {
			return 0, err
		}
		if !a.seen {
			a.firstTs = blk.Timecode
			a.seen = true
		}
		a.buf = a.track.RestoreHeader(blk.Data)
	}
	n := copy(p, a.buf)
	a.buf = a.buf[n:]
	return n, nil
}

// countingReadSeeker serves the block walk from a 1 MiB window over the file
// and tracks the logical offset for progress. The window matters on Android:
// the source lives on FUSE-backed shared storage where every syscall costs
// orders of magnitude more than on a server filesystem, and the EBML walk is
// made of tiny header reads and short skips. Most skips (a video payload
// between two audio blocks) land inside the current window and cost nothing.
type countingReadSeeker struct {
	f       *os.File
	size    int64
	off     atomic.Int64 // logical position, read from other threads for progress
	buf     []byte       // window contents: file[bufStart : bufStart+len(buf)]
	bufOff  int64
	scratch []byte
}

const readWindowSize = 1 << 20

func (c *countingReadSeeker) Read(p []byte) (int, error) {
	pos := c.off.Load()
	if pos < c.bufOff || pos >= c.bufOff+int64(len(c.buf)) {
		if c.scratch == nil {
			c.scratch = make([]byte, readWindowSize)
		}
		if _, err := c.f.Seek(pos, io.SeekStart); err != nil {
			return 0, err
		}
		n, err := c.f.Read(c.scratch)
		if n == 0 {
			if err == nil {
				err = io.EOF
			}
			return 0, err
		}
		c.bufOff = pos
		c.buf = c.scratch[:n]
	}
	n := copy(p, c.buf[pos-c.bufOff:])
	c.off.Store(pos + int64(n))
	return n, nil
}

func (c *countingReadSeeker) Seek(offset int64, whence int) (int64, error) {
	var pos int64
	switch whence {
	case io.SeekStart:
		pos = offset
	case io.SeekCurrent:
		pos = c.off.Load() + offset
	case io.SeekEnd:
		pos = c.size + offset
	default:
		return 0, fmt.Errorf("unsupported whence %d", whence)
	}
	if pos < 0 {
		return 0, fmt.Errorf("negative seek %d", pos)
	}
	// Purely logical: the window makes the eventual read cheap, and a seek
	// inside it costs nothing at all.
	c.off.Store(pos)
	return pos, nil
}

func (c *countingReadSeeker) pos() int64 { return c.off.Load() }
