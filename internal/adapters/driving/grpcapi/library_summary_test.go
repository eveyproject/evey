package grpcapi

import (
	"testing"

	eveyv1 "github.com/eveyproject/evey/gen/evey/v1"
)

func TestSummarizeMediaItemsKeepsWhatCardsShow(t *testing.T) {
	item := &eveyv1.MediaItem{
		Id: "m1", Title: "Film", Year: 2024, CommunityRating: 7.1, Overview: "kept for hover previews",
		Genres: []string{"Drama"}, FilePath: "/films/f.mkv", FileSize: 42, ParentId: "s1", SeasonIndexNumber: 2,
		Tagline: "drop", Studios: []string{"drop"}, Tags: []string{"drop"},
		ProviderIds:             map[string]string{"tmdb": "1"},
		CertificationsByCountry: map[string]string{"FR": "12"},
		Ratings:                 []*eveyv1.Rating{{Source: "imdb"}},
		Trailers:                []*eveyv1.Trailer{{Key: "k"}},
		People:                  []*eveyv1.MediaPerson{{}},
		Artwork: []*eveyv1.Artwork{
			{Type: eveyv1.ArtworkType_ARTWORK_TYPE_PRIMARY, Url: "a", Blurhash: "first"},
			{Type: eveyv1.ArtworkType_ARTWORK_TYPE_PRIMARY, Url: "b", Blurhash: "chosen", IsSelected: true},
			{Type: eveyv1.ArtworkType_ARTWORK_TYPE_BACKDROP, Url: "c", Blurhash: "back"},
		},
		Streams: []*eveyv1.MediaStream{
			{Type: eveyv1.MediaStreamType_MEDIA_STREAM_TYPE_VIDEO, Codec: "hevc", Width: 3840, Height: 2160, ColorTransfer: "smpte2084", BitDepth: 10, Profile: "drop"},
			{Type: eveyv1.MediaStreamType_MEDIA_STREAM_TYPE_VIDEO, Codec: "mjpeg"},
			{Type: eveyv1.MediaStreamType_MEDIA_STREAM_TYPE_AUDIO, Language: "fra", Channels: 6, Title: "drop"},
			{Type: eveyv1.MediaStreamType_MEDIA_STREAM_TYPE_SUBTITLE, Language: "eng"},
			{Type: eveyv1.MediaStreamType_MEDIA_STREAM_TYPE_ATTACHMENT},
		},
	}
	summarizeMediaItems([]*eveyv1.MediaItem{item, nil}, false)

	if item.Title != "Film" || item.Overview == "" || item.FilePath == "" || item.FileSize != 42 || item.SeasonIndexNumber != 2 || len(item.Genres) != 1 {
		t.Fatalf("a scalar field a card reads was dropped: %+v", item)
	}
	if item.Tagline != "" || item.Studios != nil || item.Tags != nil || item.ProviderIds != nil ||
		item.CertificationsByCountry != nil || item.Ratings != nil || item.Trailers != nil || item.People != nil {
		t.Fatalf("a field no card reads was kept: %+v", item)
	}
	if len(item.Artwork) != 2 || item.Artwork[0].Blurhash != "chosen" || item.Artwork[0].Url != "" || item.Artwork[1].Blurhash != "back" {
		t.Fatalf("artwork: want one per type, the selected one, type and blurhash only; got %+v", item.Artwork)
	}
	if len(item.Streams) != 3 {
		t.Fatalf("streams: want the first video plus audio and subtitle; got %d", len(item.Streams))
	}
	v := item.Streams[0]
	if v.Height != 2160 || v.ColorTransfer != "smpte2084" || v.BitDepth != 10 || v.Codec != "hevc" || v.Profile != "" {
		t.Fatalf("video stream: badge fields kept, the rest dropped; got %+v", v)
	}
	if a := item.Streams[1]; a.Language != "fra" || a.Channels != 0 || a.Title != "" {
		t.Fatalf("audio stream: language only; got %+v", a)
	}
}

func TestSummarizeMediaItemsKeepsTheCastOfAFilmography(t *testing.T) {
	item := &eveyv1.MediaItem{People: []*eveyv1.MediaPerson{{}}}
	summarizeMediaItems([]*eveyv1.MediaItem{item}, true)
	if len(item.People) != 1 {
		t.Fatal("a person's filmography groups its items by role: the cast must stay")
	}
}
