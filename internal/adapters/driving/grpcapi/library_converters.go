package grpcapi

import (
	eveyv1 "github.com/eveyproject/evey/gen/evey/v1"
	"github.com/eveyproject/evey/internal/domain/library"
)

var libraryTypeToProto = map[string]eveyv1.LibraryType{
	"movie":  eveyv1.LibraryType_LIBRARY_TYPE_MOVIES,
	"tvshow": eveyv1.LibraryType_LIBRARY_TYPE_SHOWS,
	"music":  eveyv1.LibraryType_LIBRARY_TYPE_MUSIC,
	"photo":  eveyv1.LibraryType_LIBRARY_TYPE_PHOTOS,
}

var libraryTypeFromProto = map[eveyv1.LibraryType]string{
	eveyv1.LibraryType_LIBRARY_TYPE_MOVIES: "movie",
	eveyv1.LibraryType_LIBRARY_TYPE_SHOWS:  "tvshow",
	eveyv1.LibraryType_LIBRARY_TYPE_MUSIC:  "music",
	eveyv1.LibraryType_LIBRARY_TYPE_PHOTOS: "photo",
}

func libraryToProto(lib *library.Library) *eveyv1.Library {
	out := &eveyv1.Library{
		Id:                 string(lib.ID),
		Name:               lib.Name,
		Type:               libraryTypeToProto[string(lib.Type)],
		Paths:              lib.Paths,
		ScanOnStartup:      lib.ScanOnStartup,
		RealtimeMonitoring: lib.RealtimeMonitoring,
		CreatedAt:          toTimestamppb(lib.CreatedAt),
		UpdatedAt:          toTimestamppb(lib.UpdatedAt),
	}
	if lib.LastScannedAt != nil {
		out.LastScannedAt = toTimestamppb(*lib.LastScannedAt)
	}
	return out
}

var mediaItemTypeToProto = map[string]eveyv1.MediaItemType{
	"movie":   eveyv1.MediaItemType_MEDIA_ITEM_TYPE_MOVIE,
	"series":  eveyv1.MediaItemType_MEDIA_ITEM_TYPE_SERIES,
	"season":  eveyv1.MediaItemType_MEDIA_ITEM_TYPE_SEASON,
	"episode": eveyv1.MediaItemType_MEDIA_ITEM_TYPE_EPISODE,
	"album":   eveyv1.MediaItemType_MEDIA_ITEM_TYPE_ALBUM,
	"artist":  eveyv1.MediaItemType_MEDIA_ITEM_TYPE_ARTIST,
	"track":   eveyv1.MediaItemType_MEDIA_ITEM_TYPE_TRACK,
	"photo":   eveyv1.MediaItemType_MEDIA_ITEM_TYPE_PHOTO,
}

var mediaItemTypeFromProto = map[eveyv1.MediaItemType]string{
	eveyv1.MediaItemType_MEDIA_ITEM_TYPE_MOVIE:   "movie",
	eveyv1.MediaItemType_MEDIA_ITEM_TYPE_SERIES:  "series",
	eveyv1.MediaItemType_MEDIA_ITEM_TYPE_SEASON:  "season",
	eveyv1.MediaItemType_MEDIA_ITEM_TYPE_EPISODE: "episode",
	eveyv1.MediaItemType_MEDIA_ITEM_TYPE_ALBUM:   "album",
	eveyv1.MediaItemType_MEDIA_ITEM_TYPE_ARTIST:  "artist",
	eveyv1.MediaItemType_MEDIA_ITEM_TYPE_TRACK:   "track",
	eveyv1.MediaItemType_MEDIA_ITEM_TYPE_PHOTO:   "photo",
}

// mediaItemToProto converts a MediaItem to proto. When includeFilePath is false
// the absolute server path is omitted - admin-only (matches photoToProto): it
// leaks the host filesystem layout, and non-admin clients stream via the media
// endpoints, not the raw path (pentest 2026-05-26).
func mediaItemToProto(item *library.MediaItem, includeFilePath bool) *eveyv1.MediaItem {
	pb := &eveyv1.MediaItem{
		Id:            string(item.ID),
		LibraryId:     string(item.LibraryID),
		ParentId:      item.ParentID,
		Type:          mediaItemTypeToProto[string(item.Type)],
		Title:         item.Title,
		SortTitle:     item.SortTitle,
		OriginalTitle: item.OriginalTitle,
		Overview:      item.Overview,
		Tagline:       item.Tagline,
		CreatedAt:     toTimestamppb(item.AddedAt),
		UpdatedAt:     toTimestamppb(item.UpdatedAt),
	}
	if item.Catalog != nil {
		pb.ProviderIds = item.Catalog.ProviderIDs
		pb.Genres = item.Catalog.Genres
		pb.Studios = item.Catalog.Studios
		pb.Tags = item.Catalog.Tags
		pb.PremiereDate = toTimestamppbPtr(item.Catalog.PremiereDate)
		if item.Catalog.Year != nil {
			pb.Year = int32(*item.Catalog.Year)
		}
	}
	if item.File != nil {
		if includeFilePath {
			pb.FilePath = item.File.Path
		}
		pb.FileSize = item.File.Size
		pb.ContainerFormat = item.File.ContainerFormat
		pb.DurationMs = item.File.Duration.Milliseconds()
		pb.Bitrate = item.File.Bitrate
	}
	if item.Classification != nil {
		pb.OfficialRating = item.Classification.OfficialRating
		pb.CertificationsByCountry = item.Classification.CertificationsByCountry
		if item.Classification.CommunityRating != nil {
			pb.CommunityRating = *item.Classification.CommunityRating
		}
		for source, value := range item.Classification.ProviderRatings {
			pb.Ratings = append(pb.Ratings, ratingToProto(source, value))
		}
	}
	if item.IndexNumber != nil {
		pb.IndexNumber = int32(*item.IndexNumber)
	}
	for _, t := range item.Trailers {
		pb.Trailers = append(pb.Trailers, trailerToProto(t))
	}
	return pb
}

func trailerToProto(t library.Trailer) *eveyv1.Trailer {
	return &eveyv1.Trailer{
		Site:             t.Site,
		Key:              t.Key,
		Language:         t.Language,
		Official:         t.Official,
		Type:             t.Type,
		UseSubtitles:     t.UseSubtitles,
		SubtitleLanguage: t.SubtitleLanguage,
	}
}

var ratingMaxValues = map[string]float64{
	"imdb":       10,
	"tmdb":       10,
	"rt":         100,
	"metacritic": 100,
}

func ratingToProto(source string, value float64) *eveyv1.Rating {
	maxValue := ratingMaxValues[source]
	if maxValue == 0 {
		maxValue = 10
	}
	return &eveyv1.Rating{
		Source:   source,
		Value:    value,
		MaxValue: maxValue,
	}
}

var personRoleToProto = map[string]eveyv1.PersonType{
	"actor":    eveyv1.PersonType_PERSON_TYPE_ACTOR,
	"director": eveyv1.PersonType_PERSON_TYPE_DIRECTOR,
	"writer":   eveyv1.PersonType_PERSON_TYPE_WRITER,
	"producer": eveyv1.PersonType_PERSON_TYPE_PRODUCER,
}

func castMemberToProto(cm library.CastMember) *eveyv1.MediaPerson {
	pb := &eveyv1.MediaPerson{
		Id:        cm.PersonID,
		Name:      cm.PersonName,
		Role:      cm.RoleType,
		Type:      personRoleToProto[cm.RoleType],
		SortOrder: int32(cm.SortOrder),
	}
	if cm.CharacterName != nil {
		pb.Role = *cm.CharacterName
	}
	if cm.ThumbPath != nil {
		pb.ThumbUrl = *cm.ThumbPath
	}
	return pb
}

var mediaSortFieldFromProto = map[eveyv1.MediaSortField]string{
	eveyv1.MediaSortField_MEDIA_SORT_FIELD_TITLE:         "title",
	eveyv1.MediaSortField_MEDIA_SORT_FIELD_YEAR:          "year",
	eveyv1.MediaSortField_MEDIA_SORT_FIELD_RATING:        "rating",
	eveyv1.MediaSortField_MEDIA_SORT_FIELD_DATE_ADDED:    "date_added",
	eveyv1.MediaSortField_MEDIA_SORT_FIELD_PREMIERE_DATE: "premiere_date",
}

var sortOrderFromProto = map[eveyv1.SortOrder]string{
	eveyv1.SortOrder_SORT_ORDER_ASC:  "asc",
	eveyv1.SortOrder_SORT_ORDER_DESC: "desc",
}

var streamTypeToProto = map[library.StreamType]eveyv1.MediaStreamType{
	library.StreamTypeVideo:      eveyv1.MediaStreamType_MEDIA_STREAM_TYPE_VIDEO,
	library.StreamTypeAudio:      eveyv1.MediaStreamType_MEDIA_STREAM_TYPE_AUDIO,
	library.StreamTypeSubtitle:   eveyv1.MediaStreamType_MEDIA_STREAM_TYPE_SUBTITLE,
	library.StreamTypeAttachment: eveyv1.MediaStreamType_MEDIA_STREAM_TYPE_ATTACHMENT,
}

func mediaStreamToProto(s *library.MediaStream) *eveyv1.MediaStream {
	pb := &eveyv1.MediaStream{
		Index:         int32(s.StreamIndex),
		Type:          streamTypeToProto[s.StreamType],
		Codec:         s.Codec,
		CodecLongName: s.CodecLongName,
		Language:      s.Language,
		Title:         s.Title,
		IsDefault:     s.IsDefault,
		IsForced:      s.IsForced,
		IsExternal:    s.IsExternal,
		Profile:       s.Profile,
	}
	if s.Width != nil {
		pb.Width = int32(*s.Width)
	}
	if s.Height != nil {
		pb.Height = int32(*s.Height)
	}
	if s.Framerate != nil {
		pb.FrameRate = *s.Framerate
	}
	if s.PixelFormat != nil {
		pb.PixelFormat = *s.PixelFormat
	}
	if s.ColorSpace != nil {
		pb.ColorSpace = *s.ColorSpace
	}
	// color_transfer is the only field that tells PQ from HLG, and every client
	// reads it: the web and the two device apps label HDR from it, and mobile's
	// castSafeDirectPlay refuses a direct play of PQ/HLG toward a non-HDR
	// Chromecast on it. The domain has carried it all along and this converter
	// never put it on the wire, so it arrived empty everywhere - which made
	// every HLG title read as "HDR10" and made that cast gate unable to fire.
	// (color_primaries stays unwritten: the domain has no such field, and no
	// client reads it - that one is dead proto surface, not a broken contract.)
	if s.ColorTransfer != nil {
		pb.ColorTransfer = *s.ColorTransfer
	}
	if s.Channels != nil {
		pb.Channels = int32(*s.Channels)
	}
	if s.SampleRate != nil {
		pb.SampleRate = int32(*s.SampleRate)
	}
	if s.BitDepth != nil {
		pb.BitDepth = int32(*s.BitDepth)
	}
	if s.Bitrate != nil {
		pb.Bitrate = *s.Bitrate
	}
	if s.ExternalPath != nil {
		pb.ExternalPath = *s.ExternalPath
	}
	return pb
}
