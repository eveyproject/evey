package grpcapi

import (
	"encoding/json"
	"fmt"

	"github.com/eveyproject/evey/internal/domain/music"
	"github.com/eveyproject/evey/internal/domain/shared"
)

// trackCursorSortValue is the payload packed into shared.Cursor.SortValue for
// track listings. The keyset needs (album_artist, album, disk_number,
// track_number, id) - shared.Cursor only carries one SortValue slot on its
// own, so the extra fields ride along as JSON inside that slot instead of a
// second cursor format.
type trackCursorSortValue struct {
	AlbumArtist string `json:"aa"`
	Album       string `json:"al"`
	DiskNumber  int    `json:"d"`
	TrackNumber int    `json:"t"`
}

// encodeTrackCursor builds the opaque cursor pointing to the page after last,
// the final track of the current page. Mirrors the album_artist, album,
// disk_number, track_number, id keyset order that music.TrackCursor seeks on.
func encodeTrackCursor(last music.Track) string {
	v := trackCursorSortValue{
		AlbumArtist: last.AlbumArtist,
		Album:       last.Album,
		DiskNumber:  last.DiskNumber,
		TrackNumber: last.TrackNumber,
	}
	b, err := json.Marshal(v)
	if err != nil {
		// Unreachable with this struct, but an empty cursor is the honest
		// failure: the client reads it as end-of-list and stops, rather than
		// paging on with a corrupt keyset and silently skipping rows.
		return ""
	}
	return shared.EncodeCursor(shared.Cursor{
		SortField: "album_artist",
		SortValue: string(b),
		LastID:    last.ID,
	})
}

// decodeTrackCursor parses an opaque ListTracksRequest.cursor back into the
// keyset the repository seeks from. Empty input is an error - callers must
// branch on req.Cursor != "" before calling this.
func decodeTrackCursor(s string) (*music.TrackCursor, error) {
	c, err := shared.DecodeCursor(s)
	if err != nil {
		return nil, err
	}
	var v trackCursorSortValue
	if err := json.Unmarshal([]byte(c.SortValue), &v); err != nil {
		return nil, fmt.Errorf("unmarshal track cursor: %w", err)
	}
	return &music.TrackCursor{
		AlbumArtist: v.AlbumArtist,
		Album:       v.Album,
		DiskNumber:  v.DiskNumber,
		TrackNumber: v.TrackNumber,
		ID:          c.LastID,
	}, nil
}
