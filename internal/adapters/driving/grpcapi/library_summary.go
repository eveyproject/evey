package grpcapi

import eveyv1 "github.com/eveyproject/evey/gen/evey/v1"

// summarizeMediaItems trims list items to what a card, a row or a grid shows
// (the summary flag of ListMediaItems and SearchMedia): every scalar field, one
// artwork per type as type and blurhash, the video stream's size, codec and
// colour, the audio and subtitle languages. Trailers, certifications, studios,
// tags, external ratings and ids go, and so does the cast unless the list is a
// person's filmography. The full item was several KB per card, parsed before the first poster showed.

func summarizeMediaItems(items []*eveyv1.MediaItem, keepPeople bool) {
	for _, it := range items {
		if it == nil {
			continue
		}
		it.Tagline = ""
		it.ProviderIds = nil
		it.Studios = nil
		it.Tags = nil
		it.Ratings = nil
		it.Trailers = nil
		it.CertificationsByCountry = nil
		if !keepPeople {
			it.People = nil
		}
		it.Artwork = summarizeArtwork(it.Artwork)
		it.Streams = summarizeStreams(it.Streams)
	}
}

// One entry per artwork type, the selected one when there is a choice.
func summarizeArtwork(in []*eveyv1.Artwork) []*eveyv1.Artwork {
	if len(in) == 0 {
		return in
	}
	byType := make(map[eveyv1.ArtworkType]*eveyv1.Artwork, 4)
	order := make([]eveyv1.ArtworkType, 0, 4)
	for _, a := range in {
		if a == nil {
			continue
		}
		prev, seen := byType[a.Type]
		if !seen {
			order = append(order, a.Type)
		}
		if !seen || (a.IsSelected && !prev.IsSelected) {
			byType[a.Type] = a
		}
	}
	out := make([]*eveyv1.Artwork, 0, len(order))
	for _, t := range order {
		out = append(out, &eveyv1.Artwork{Type: t, Blurhash: byType[t].Blurhash})
	}
	return out
}

// The first video stream as a badge reads it, then the language of each audio
// and subtitle stream.
func summarizeStreams(in []*eveyv1.MediaStream) []*eveyv1.MediaStream {
	if len(in) == 0 {
		return in
	}
	out := make([]*eveyv1.MediaStream, 0, len(in))
	video := false
	for _, s := range in {
		if s == nil {
			continue
		}
		switch s.Type {
		case eveyv1.MediaStreamType_MEDIA_STREAM_TYPE_VIDEO:
			if video {
				continue
			}
			video = true
			out = append(out, &eveyv1.MediaStream{
				Type:          s.Type,
				Codec:         s.Codec,
				Width:         s.Width,
				Height:        s.Height,
				ColorSpace:    s.ColorSpace,
				ColorTransfer: s.ColorTransfer,
				BitDepth:      s.BitDepth,
			})
		case eveyv1.MediaStreamType_MEDIA_STREAM_TYPE_AUDIO, eveyv1.MediaStreamType_MEDIA_STREAM_TYPE_SUBTITLE:
			out = append(out, &eveyv1.MediaStream{Type: s.Type, Language: s.Language})
		}
	}
	return out
}
