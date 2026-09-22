package httpapi

import (
	"context"
	"testing"
	"time"

	domainlibrary "github.com/evey-project/evey/internal/domain/library"
)

// Which tracks are worth extracting ahead of time, and in which order. Getting
// this wrong is expensive in a way nothing reports: each track is a full read of
// the container (82s measured on a 15 GB file), so an image track queued by
// mistake burns that for a result the route will never serve.

func sub(codec, lang string, external bool) *domainlibrary.MediaStream {
	return &domainlibrary.MediaStream{
		StreamType: domainlibrary.StreamTypeSubtitle,
		Codec:      codec,
		Language:   lang,
		IsExternal: external,
	}
}

func video() *domainlibrary.MediaStream {
	return &domainlibrary.MediaStream{StreamType: domainlibrary.StreamTypeVideo, Codec: "h264"}
}

func audio() *domainlibrary.MediaStream {
	return &domainlibrary.MediaStream{StreamType: domainlibrary.StreamTypeAudio, Codec: "eac3"}
}

func indices(ts []prewarmTrack) []int {
	out := make([]int, 0, len(ts))
	for _, t := range ts {
		out = append(out, t.index)
	}
	return out
}

// The indices must be SUBTITLE-relative, counted over subtitle streams only -
// the same ordinal the route and the cache key use. Counting over all streams
// would prewarm the wrong tracks and cache them under the wrong keys.
func TestTextSubtitleTracks_CountsSubtitlesOnly(t *testing.T) {
	streams := []*domainlibrary.MediaStream{
		video(), audio(), audio(),
		sub("subrip", "fre", false),
		sub("subrip", "eng", false),
	}
	got := indices(textSubtitleTracks(streams, ""))
	if len(got) != 2 || got[0] != 0 || got[1] != 1 {
		t.Fatalf("indices = %v, want [0 1] counted over subtitles alone", got)
	}
}

// A bitmap track is never served as text (the route answers 422), so extracting
// one reads the whole film to produce something nobody can use.
// PGS is prewarmed now that it is served as decoded images instead of being
// burnt into the video - and it is the track that most needs the head start,
// being the one a retail rip flags DEFAULT. VobSub stays out: nothing decodes
// it, so warming it would fill the cache for a menu entry that draws nothing.
func TestTextSubtitleTracks_KeepsPGSButNotVobSub(t *testing.T) {
	streams := []*domainlibrary.MediaStream{
		sub("subrip", "fre", false),
		sub("hdmv_pgs_subtitle", "fre", false),
		sub("dvd_subtitle", "eng", false),
		sub("ass", "eng", false),
	}
	got := indices(textSubtitleTracks(streams, ""))
	if len(got) != 3 || got[0] != 0 || got[1] != 1 || got[2] != 3 {
		t.Fatalf("indices = %v, want [0 1 3]: text and PGS at their real ordinals, VobSub dropped", got)
	}
}

// An external subtitle is already a file on disk; the route serves it without
// extracting anything.
func TestTextSubtitleTracks_SkipsExternalFiles(t *testing.T) {
	streams := []*domainlibrary.MediaStream{
		sub("subrip", "fre", true),
		sub("subrip", "eng", false),
	}
	got := indices(textSubtitleTracks(streams, ""))
	if len(got) != 1 || got[0] != 1 {
		t.Fatalf("indices = %v, want [1]", got)
	}
}

// The viewer's language goes first: it is the one they are about to ask for.
// Everything else keeps FILE order behind it, which is the order the menu lists.
func TestTextSubtitleTracks_PreferredLanguageFirst(t *testing.T) {
	streams := []*domainlibrary.MediaStream{
		sub("subrip", "eng", false),
		sub("subrip", "ger", false),
		sub("subrip", "fre", false),
		sub("subrip", "spa", false),
	}
	got := indices(textSubtitleTracks(streams, "fre"))
	want := []int{2, 0, 1, 3}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("order = %v, want %v (preferred first, then file order)", got, want)
		}
	}
}

func TestTextSubtitleTracks_NoPreferenceKeepsFileOrder(t *testing.T) {
	streams := []*domainlibrary.MediaStream{
		sub("subrip", "eng", false),
		sub("subrip", "fre", false),
	}
	got := indices(textSubtitleTracks(streams, ""))
	if got[0] != 0 || got[1] != 1 {
		t.Fatalf("order = %v, want file order", got)
	}
}

func TestTextSubtitleTracks_NothingToDo(t *testing.T) {
	if got := textSubtitleTracks(nil, "fre"); len(got) != 0 {
		t.Fatalf("got %v, want none", got)
	}
	// VobSub alone still gives nothing to do: it is a bitmap track no decoder
	// here can read.
	onlyVobSub := []*domainlibrary.MediaStream{sub("dvd_subtitle", "fre", false)}
	if got := textSubtitleTracks(onlyVobSub, "fre"); len(got) != 0 {
		t.Fatalf("got %v, want none", got)
	}
}

// A player re-requests the master playlist on every seek, quality change and
// token refresh. Each one must not re-queue the same film.
func TestPrewarmer_EnqueuesAMediaOnlyOnce(t *testing.T) {
	p := newSubtitlePrewarmer(newSubtitleCache(t.TempDir()), subtitleIndexCache{})
	streams := []*domainlibrary.MediaStream{sub("subrip", "fre", false)}
	for range 5 {
		p.Enqueue("m1", "/media/film.mkv", streams, "")
	}
	if got := len(p.jobs); got != 1 {
		t.Fatalf("queued %d jobs, want 1", got)
	}
}

// Without a data dir there is nowhere to put the result: extracting would read
// the film and throw it away.
func TestPrewarmer_WithoutACacheDoesNothing(t *testing.T) {
	p := newSubtitlePrewarmer(newSubtitleCache(""), subtitleIndexCache{})
	p.Enqueue("m1", "/media/film.mkv", []*domainlibrary.MediaStream{sub("subrip", "fre", false)}, "")
	if got := len(p.jobs); got != 0 {
		t.Fatalf("queued %d jobs, want 0", got)
	}
}

// Enqueue must never hold up the request that triggered it - it runs inside the
// master-playlist response.
func TestPrewarmer_AFullQueueDropsRatherThanBlocks(t *testing.T) {
	p := newSubtitlePrewarmer(newSubtitleCache(t.TempDir()), subtitleIndexCache{})
	streams := []*domainlibrary.MediaStream{sub("subrip", "fre", false)}
	for i := range subtitlePrewarmQueue + 10 {
		p.Enqueue(string(rune('a'+i%26))+string(rune('0'+i/26)), "/media/film.mkv", streams, "")
	}
	if got := len(p.jobs); got > subtitlePrewarmQueue {
		t.Fatalf("queue holds %d, cap is %d", got, subtitlePrewarmQueue)
	}
}

// The index is a full pass of the container. Building it for a job that will
// extract a single track pays that pass to save nothing - the one extraction
// reads the file anyway. 227 of the 1503 files measured on a real library carry
// exactly one text track, so this is not a corner case.
func TestIndexWorthBuilding_OnlyFromTheSecondTrack(t *testing.T) {
	cases := []struct {
		name   string
		tracks int
		want   bool
	}{
		{"aucune piste", 0, false},
		{"une seule piste - la passe d'index est pure perte", 1, false},
		{"deux pistes - la passe est amortie", 2, true},
		{"quatre pistes", 4, true},
	}
	for _, c := range cases {
		job := prewarmJob{tracks: make([]prewarmTrack, c.tracks)}
		if got := indexWorthBuilding(job); got != c.want {
			t.Errorf("%s: indexWorthBuilding(%d tracks) = %v, want %v", c.name, c.tracks, got, c.want)
		}
	}
}

// One extraction that panics (a corrupt container tripping the parser) must
// not kill the server, nor the worker: the next film still gets its warm-up.
func TestPrewarmer_ASinglePanicDoesNotStopTheWorker(t *testing.T) {
	p := newSubtitlePrewarmer(newSubtitleCache(t.TempDir()), subtitleIndexCache{})
	ran := make(chan string, 2)
	p.runFn = func(_ context.Context, job prewarmJob) {
		ran <- job.mediaID
		if job.mediaID == "bad" {
			panic("corrupt container")
		}
	}
	p.Start(t.Context())
	streams := []*domainlibrary.MediaStream{sub("subrip", "fre", false)}
	p.Enqueue("bad", "/media/bad.mkv", streams, "")
	p.Enqueue("good", "/media/good.mkv", streams, "")

	var got []string
	for range 2 {
		select {
		case id := <-ran:
			got = append(got, id)
		case <-time.After(2 * time.Second):
			t.Fatalf("worker died after the panic; ran %v", got)
		}
	}
}
