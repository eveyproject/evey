package httpapi

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"time"

	extmkv "github.com/gravity-zero/mkvgo/mkv"
	"github.com/gravity-zero/mkvgo/mp4"

	domainlibrary "github.com/eveyproject/evey/internal/domain/library"
	domainplayback "github.com/eveyproject/evey/internal/domain/playback"
)

// The server's witness of a playback (domain/playback/served.go). Every route
// that sends media bytes runs under witnessServed; the handler that writes
// the response says what it is writing on the request's deliveryStamp
// (engine, encoded or copied, which source track), and the witness reads the
// rest from the bytes themselves: an init segment that goes out is parsed for
// its codecs, picture and channels, and a media segment is described by the
// init it decodes with. Only a response that carried bytes (200/206, a body)
// is recorded: a 404, a 410 or a 503 served nothing.
//
// Nothing here consults a verdict or a plan. A handler declares the machinery
// it actually ran, at the point it ran it.

// ServedRecorder receives what a response carried (CommandService.RecordServed,
// RecordServedCast when viaCast). viaCast: the request rode a cast token.
type ServedRecorder func(ctx context.Context, profileID, mediaID string, leg domainplayback.Leg, l domainplayback.ServedLeg, viaCast bool) error

// stampLeg is a leg a handler declares it wrote.
type stampLeg struct {
	set         bool
	engine      string
	copy        bool
	track       int // audio: source audio ordinal; -1 = read it from the init (fromInitTrack)
	sourceCodec string
	backend     string
	// codec/width/height/channels: given by the handler when no init describes
	// the bytes (the source file itself).
	codec         string
	width, height int
	channels      int
	fromInitTrack bool
}

// deliveryStamp is filled by the handler that writes the response.
type deliveryStamp struct {
	// startedAt is when the response began: what it carried was sent from
	// then on, and an observation is dated by it (see RecordServed's order).
	startedAt time.Time
	mediaID   string
	video     stampLeg
	audio     stampLeg
	// init: this response is the init segment named initKey; its bytes are
	// parsed. Otherwise initKey names what describes the bytes (the init they
	// decode with, or the source file), and describe reads it when the witness
	// has not seen it go out (an init a browser keeps in its cache is not
	// asked again).
	init     bool
	initKey  string
	describe func(context.Context) ([]initTrack, error)
	// sourceCodecs names, per kind, the source codec of the tracks the
	// response carries, read with the description (once per init) rather
	// than per response.
	sourceCodecs func(context.Context) map[string]string
	// audioOrdinal resolves the source track number an init names
	// (FileTrackID, which the packager publishes on each rendition) to the
	// source's audio ordinal.
	audioOrdinal func(fileTrackID int) (int, bool)
}

type deliveryStampKey struct{}

func deliveryStampOf(r *http.Request) *deliveryStamp {
	st, _ := r.Context().Value(deliveryStampKey{}).(*deliveryStamp)
	return st
}

// stampVideo declares the video leg this response carries.
func (r *deliveryStamp) stampVideo(mediaID, engine string, copy bool, backend, sourceCodec string) {
	if r == nil {
		return
	}
	r.mediaID = mediaID
	r.video = stampLeg{set: true, engine: engine, copy: copy, backend: backend, sourceCodec: sourceCodec, track: -1}
}

// stampAudio declares the audio leg this response carries, as source audio
// track `track` (0-based ordinal). fromInit: the handler cannot name the
// track itself (a packager presentation carrying several), the init's
// FileTrackID does.
func (r *deliveryStamp) stampAudio(mediaID, engine string, copy bool, track int, sourceCodec string, fromInit bool) {
	if r == nil {
		return
	}
	r.mediaID = mediaID
	r.audio = stampLeg{set: true, engine: engine, copy: copy, track: track, sourceCodec: sourceCodec, fromInitTrack: fromInit}
}

// stampInit marks the response as the init segment named key.
func (r *deliveryStamp) stampInit(key string) {
	if r == nil {
		return
	}
	r.init, r.initKey = true, key
}

// stampMedia names the init the response's bytes decode with; read fetches
// that init when the witness has not seen it go out.
func (r *deliveryStamp) stampMedia(key string, read func(context.Context) ([]byte, error)) {
	if r == nil {
		return
	}
	r.initKey = key
	r.describe = func(ctx context.Context) ([]initTrack, error) {
		b, err := read(ctx)
		if err != nil {
			return nil, err
		}
		return parseInit(ctx, b)
	}
}

// stampFile declares a response that sends byte ranges of the source file
// itself: both legs are the file's own, copied, and the file's streams
// describe them (keyed by key, the file's identity). The audio leg names no
// track and no codec - the file carries every track and the player picks.
func (r *deliveryStamp) stampFile(mediaID, key string, streams func(context.Context) ([]*domainlibrary.MediaStream, error)) {
	if r == nil {
		return
	}
	r.mediaID = mediaID
	r.video = stampLeg{set: true, engine: domainplayback.EngineFile, copy: true, track: -1}
	r.audio = stampLeg{set: true, engine: domainplayback.EngineFile, copy: true, track: -1}
	r.initKey = key
	r.describe = func(ctx context.Context) ([]initTrack, error) {
		ss, err := streams(ctx)
		if err != nil {
			return nil, err
		}
		var out []initTrack
		audio := false
		for _, st := range ss {
			switch {
			case st == nil:
			case st.StreamType == domainlibrary.StreamTypeVideo && !domainlibrary.IsStillImageCodec(st.Codec):
				if _, ok := trackOfKind(out, "video"); ok {
					continue
				}
				t := initTrack{kind: "video", codec: st.Codec}
				if st.Width != nil {
					t.width = *st.Width
				}
				if st.Height != nil {
					t.height = *st.Height
				}
				out = append(out, t)
			case st.StreamType == domainlibrary.StreamTypeAudio:
				audio = true
			}
		}
		if audio {
			out = append(out, initTrack{kind: "audio"})
		}
		return out, nil
	}
}

// initTrack is what an init segment says of one of its tracks.
type initTrack struct {
	kind        string // "video" | "audio"
	codec       string
	width       int
	height      int
	channels    int
	fileTrackID int
	sourceCodec string
}

// initMemo remembers what each init that went out contains, so a media
// segment is described without re-reading its init. Bounded: a full memo is
// emptied, and the next segments re-read their inits once.
type initMemo struct {
	mu sync.Mutex
	m  map[string][]initTrack
}

const initMemoMax = 1024

func (m *initMemo) get(key string) ([]initTrack, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	t, ok := m.m[key]
	return t, ok
}

func (m *initMemo) put(key string, t []initTrack) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.m == nil || len(m.m) >= initMemoMax {
		m.m = make(map[string][]initTrack)
	}
	m.m[key] = t
}

// parseInit reads an init segment's tracks: codec in the library's vocabulary
// (what the scan stores for a source), picture, channels, and the source
// track number a packager rendition carries.
func parseInit(ctx context.Context, b []byte) ([]initTrack, error) {
	c, _, err := mp4.ReadMeta(ctx, bytes.NewReader(b), "init.mp4")
	if err != nil {
		return nil, err
	}
	out := make([]initTrack, 0, len(c.Tracks))
	for i := range c.Tracks {
		t := &c.Tracks[i]
		it := initTrack{codec: c.TrackCodecName(t), fileTrackID: int(t.FileTrackID)}
		switch t.Type {
		case extmkv.VideoTrack:
			it.kind = "video"
			if t.Width != nil {
				it.width = int(*t.Width)
			}
			if t.Height != nil {
				it.height = int(*t.Height)
			}
		case extmkv.AudioTrack:
			it.kind = "audio"
			if t.Channels != nil {
				it.channels = int(*t.Channels)
			}
		default:
			continue
		}
		out = append(out, it)
	}
	return out, nil
}

// initCaptureMax bounds what the witness keeps of an init response: an init
// is a few KB (a moov with empty sample tables), the bound only guards a
// handler that stamped a segment as an init.
const initCaptureMax = 1 << 20

// witnessWriter counts what a response carried and keeps an init's bytes. A
// media response is observed at its first bytes, not its last: a direct play
// reads the whole film through one request, and its bytes are going out from
// the first write. An init is observed once whole, since its bytes are read.
type witnessWriter struct {
	http.ResponseWriter
	stamp    *deliveryStamp
	status   int
	written  int64
	capture  bytes.Buffer
	onFirst  func()
	observed bool
}

func (w *witnessWriter) firstBytes() {
	if w.observed || w.stamp.init || w.onFirst == nil {
		return
	}
	if w.status != http.StatusOK && w.status != http.StatusPartialContent {
		return
	}
	w.observed = true
	w.onFirst()
}

func (w *witnessWriter) WriteHeader(code int) {
	if w.status == 0 {
		w.status = code
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *witnessWriter) Write(p []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	if len(p) > 0 {
		w.firstBytes()
	}
	n, err := w.ResponseWriter.Write(p)
	w.written += int64(n)
	if w.stamp.init && w.capture.Len() < initCaptureMax {
		room := initCaptureMax - w.capture.Len()
		w.capture.Write(p[:min(n, room)])
	}
	return n, err
}

// ReadFrom keeps the init capture on the sendfile path http.ServeContent
// takes: a plain io.Copy through Write, which an init (a few KB) can afford.
func (w *witnessWriter) ReadFrom(src io.Reader) (int64, error) {
	if !w.stamp.init {
		if rf, ok := w.ResponseWriter.(io.ReaderFrom); ok {
			if w.status == 0 {
				w.status = http.StatusOK
			}
			w.firstBytes()
			n, err := rf.ReadFrom(src)
			w.written += n
			return n, err
		}
	}
	return io.Copy(struct{ io.Writer }{w}, src)
}

func (w *witnessWriter) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (w *witnessWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

// servedWitness wraps the media routes. memo is shared by every route.
type servedWitness struct {
	record ServedRecorder
	memo   initMemo
}

func (sw *servedWitness) wrap(next http.HandlerFunc) http.HandlerFunc {
	if sw == nil || sw.record == nil {
		return next
	}
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead {
			next(w, r)
			return
		}
		stamp := &deliveryStamp{startedAt: time.Now()}
		r = r.WithContext(context.WithValue(r.Context(), deliveryStampKey{}, stamp))
		ww := &witnessWriter{ResponseWriter: w, stamp: stamp}
		ww.onFirst = func() { sw.observe(r, stamp, nil) }
		next(ww, r)
		if !stamp.init || ww.written == 0 {
			return
		}
		if ww.status != http.StatusOK && ww.status != http.StatusPartialContent {
			return
		}
		sw.observe(r, stamp, ww.capture.Bytes())
	}
}

// observe turns one response's stamp and bytes into served legs.
func (sw *servedWitness) observe(r *http.Request, st *deliveryStamp, captured []byte) {
	if !st.video.set && !st.audio.set {
		return
	}
	claims := HTTPClaimsFromContext(r.Context())
	if claims == nil || claims.ProfileID == "" || st.mediaID == "" {
		return
	}
	// Recorded after the response, off the request's cancellation: a viewer
	// that closed the connection once the bytes were in still received them.
	ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), 3*time.Second)
	defer cancel()

	tracks, described := sw.tracksFor(ctx, st, captured)
	if st.initKey != "" && !described {
		// What describes these bytes could not be read: which legs they
		// carry is unknown, and the session keeps what it last saw rather
		// than a guess.
		return
	}
	// Dated by the response's start: an init a player fetched and dropped can
	// finish after the bytes that replaced it began, and must not pass for
	// the newer of the two.
	now := st.startedAt
	if now.IsZero() {
		now = time.Now()
	}
	viaCast := false
	if tok, ok := StreamTokenFromContext(r.Context()); ok && tok != nil {
		viaCast = tok.Cast
	}
	for _, leg := range []domainplayback.Leg{domainplayback.LegVideo, domainplayback.LegAudio} {
		decl := st.audio
		kind := "audio"
		if leg == domainplayback.LegVideo {
			decl, kind = st.video, "video"
		}
		if !decl.set {
			continue
		}
		l := domainplayback.ServedLeg{
			Engine: decl.engine, Copy: decl.copy, Track: decl.track,
			SourceCodec: decl.sourceCodec, Backend: decl.backend,
			Codec: decl.codec, Width: decl.width, Height: decl.height, Channels: decl.channels,
			At: now,
		}
		if described {
			t, ok := trackOfKind(tracks, kind)
			if !ok {
				// The init carries no track of this kind: the response did not
				// carry this leg (a video-only variant beside an audio rendition).
				continue
			}
			l.Codec, l.Width, l.Height, l.Channels = t.codec, t.width, t.height, t.channels
			if l.SourceCodec == "" {
				l.SourceCodec = t.sourceCodec
			}
			if decl.fromInitTrack {
				l.Track = -1
				if st.audioOrdinal != nil && t.fileTrackID > 0 {
					if ord, ok := st.audioOrdinal(t.fileTrackID); ok {
						l.Track = ord
					}
				}
			}
		}
		if l.Copy && l.SourceCodec == l.Codec {
			l.SourceCodec = ""
		}
		if err := sw.record(ctx, claims.ProfileID, st.mediaID, leg, l, viaCast); err != nil {
			slog.WarnContext(ctx, "served witness: record failed",
				slog.String("media_id", st.mediaID), slog.String("leg", string(leg)), slog.Any("err", err))
		}
	}
}

// tracksFor returns what the init behind this response contains. described is
// false when no init describes the bytes (the source file itself, whose legs
// the handler described) or when it could not be read: the legs then carry
// what the handler declared and no codec it did not.
func (sw *servedWitness) tracksFor(ctx context.Context, st *deliveryStamp, captured []byte) ([]initTrack, bool) {
	if st.initKey == "" {
		return nil, false
	}
	if st.init {
		tracks, err := parseInit(ctx, captured)
		if err != nil {
			slog.WarnContext(ctx, "served witness: init that went out does not parse",
				slog.String("media_id", st.mediaID), slog.String("init", st.initKey), slog.Any("err", err))
			return nil, false
		}
		withSourceCodecs(tracks, st.video.sourceCodec, st.audio.sourceCodec)
		sw.memo.put(st.initKey, tracks)
		return tracks, true
	}
	if tracks, ok := sw.memo.get(st.initKey); ok {
		return tracks, true
	}
	if st.describe == nil {
		return nil, false
	}
	tracks, err := st.describe(ctx)
	if err != nil {
		slog.DebugContext(ctx, "served witness: what describes the served bytes is unreadable",
			slog.String("media_id", st.mediaID), slog.String("key", st.initKey), slog.Any("err", err))
		return nil, false
	}
	if st.sourceCodecs != nil {
		sc := st.sourceCodecs(ctx)
		withSourceCodecs(tracks, sc["video"], sc["audio"])
	}
	sw.memo.put(st.initKey, tracks)
	return tracks, true
}

// withSourceCodecs files the source codec of each kind on the description.
func withSourceCodecs(tracks []initTrack, video, audio string) {
	for i := range tracks {
		switch {
		case tracks[i].kind == "video" && video != "":
			tracks[i].sourceCodec = video
		case tracks[i].kind == "audio" && audio != "":
			tracks[i].sourceCodec = audio
		}
	}
}

func trackOfKind(tracks []initTrack, kind string) (initTrack, bool) {
	for _, t := range tracks {
		if t.kind == kind {
			return t, true
		}
	}
	return initTrack{}, false
}
