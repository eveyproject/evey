package grpcapi

import (
	"encoding/json"
	"fmt"
	"maps"
	"strings"
	"time"

	eveyv1 "github.com/gravity-zero/evey/gen/evey/v1"
	"github.com/gravity-zero/evey/internal/domain/activity"
	"github.com/gravity-zero/evey/internal/domain/library"
	domainmetadata "github.com/gravity-zero/evey/internal/domain/metadata"
	"github.com/gravity-zero/evey/internal/domain/mkv"
	"github.com/gravity-zero/evey/internal/domain/playback"
	"github.com/gravity-zero/evey/internal/domain/plugin"
	"github.com/gravity-zero/evey/internal/domain/transcoding"
	"github.com/gravity-zero/evey/internal/domain/user"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// --- Timestamp helpers ---

func toTimestamppb(t time.Time) *timestamppb.Timestamp {
	if t.IsZero() {
		return nil
	}
	return timestamppb.New(t)
}

func toTimestamppbPtr(t *time.Time) *timestamppb.Timestamp {
	if t == nil {
		return nil
	}
	return timestamppb.New(*t)
}

func fromTimestamppb(t *timestamppb.Timestamp) time.Time {
	if t == nil {
		return time.Time{}
	}
	return t.AsTime()
}

func fromTimestamppbPtr(t *timestamppb.Timestamp) *time.Time {
	if t == nil {
		return nil
	}
	tt := t.AsTime()
	return &tt
}

// --- Library converters ---

var libraryTypeToProto = map[string]eveyv1.LibraryType{
	"movie":  eveyv1.LibraryType_LIBRARY_TYPE_MOVIES,
	"tvshow": eveyv1.LibraryType_LIBRARY_TYPE_SHOWS,
	"music":  eveyv1.LibraryType_LIBRARY_TYPE_MUSIC,
	"photo":  eveyv1.LibraryType_LIBRARY_TYPE_PHOTOS,
	"book":   eveyv1.LibraryType_LIBRARY_TYPE_BOOKS,
}

var libraryTypeFromProto = map[eveyv1.LibraryType]string{
	eveyv1.LibraryType_LIBRARY_TYPE_MOVIES: "movie",
	eveyv1.LibraryType_LIBRARY_TYPE_SHOWS:  "tvshow",
	eveyv1.LibraryType_LIBRARY_TYPE_MUSIC:  "music",
	eveyv1.LibraryType_LIBRARY_TYPE_PHOTOS: "photo",
	eveyv1.LibraryType_LIBRARY_TYPE_BOOKS:  "book",
}

func libraryToProto(lib *library.Library) *eveyv1.Library {
	return &eveyv1.Library{
		Id:                 lib.ID,
		Name:               lib.Name,
		Type:               libraryTypeToProto[lib.Type],
		Paths:              lib.Paths,
		ScanOnStartup:      lib.ScanOnStartup,
		RealtimeMonitoring: lib.RealtimeMonitoring,
		CreatedAt:          toTimestamppb(lib.CreatedAt),
		UpdatedAt:          toTimestamppb(lib.UpdatedAt),
	}
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
	"book":    eveyv1.MediaItemType_MEDIA_ITEM_TYPE_BOOK,
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
	eveyv1.MediaItemType_MEDIA_ITEM_TYPE_BOOK:    "book",
}

func mediaItemToProto(item *library.MediaItem) *eveyv1.MediaItem {
	pb := &eveyv1.MediaItem{
		Id:              item.ID,
		LibraryId:       item.LibraryID,
		ParentId:        item.ParentID,
		Type:            mediaItemTypeToProto[item.Type],
		Title:           item.Title,
		SortTitle:       item.SortTitle,
		OriginalTitle:   item.OriginalTitle,
		Overview:        item.Overview,
		Tagline:         item.Tagline,
		FilePath:        item.FilePath,
		FileSize:        item.FileSize,
		ContainerFormat: item.ContainerFormat,
		DurationMs:      item.DurationMs,
		Bitrate:         item.Bitrate,
		OfficialRating:  item.OfficialRating,
		ProviderIds:     item.ProviderIDs,
		Genres:          item.Genres,
		Studios:         item.Studios,
		Tags:            item.Tags,
		CreatedAt:       toTimestamppb(item.AddedAt),
		UpdatedAt:       toTimestamppb(item.UpdatedAt),
		PremiereDate:    toTimestamppbPtr(item.PremiereDate),
	}
	if item.IndexNumber != nil {
		pb.IndexNumber = int32(*item.IndexNumber)
	}
	if item.CommunityRating != nil {
		pb.CommunityRating = *item.CommunityRating
	}
	if item.Year != nil {
		pb.Year = int32(*item.Year)
	}
	for _, t := range item.Trailers {
		pb.Trailers = append(pb.Trailers, trailerToProto(t))
	}
	for source, value := range item.Ratings {
		pb.Ratings = append(pb.Ratings, ratingToProto(source, value))
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

// ratingMaxValues maps known sources to their scale max.
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

// --- User converters ---

func userToProto(u *user.User) *eveyv1.User {
	pb := &eveyv1.User{
		Id:        u.ID,
		Username:  u.Username,
		Email:     u.Email,
		IsAdmin:   u.IsAdmin,
		Enabled:   u.Enabled,
		CreatedAt: toTimestamppb(u.CreatedAt),
		UpdatedAt: toTimestamppb(u.UpdatedAt),
	}
	if u.MaxStreams != nil {
		pb.MaxStreams = int32(*u.MaxStreams)
	}
	return pb
}

func profileToProto(p *user.Profile) *eveyv1.Profile {
	prefs := make(map[string]string)
	maps.Copy(prefs, p.Preferences)
	if len(p.AudioLanguages) > 0 {
		prefs["audio_languages"] = strings.Join(p.AudioLanguages, ",")
	}
	if len(p.SubtitleLanguages) > 0 {
		prefs["subtitle_languages"] = strings.Join(p.SubtitleLanguages, ",")
	}
	if p.MaxBitrateKbps != nil {
		prefs["max_bitrate_kbps"] = fmt.Sprintf("%d", *p.MaxBitrateKbps)
	}
	pb := &eveyv1.Profile{
		Id:          p.ID,
		UserId:      p.UserID,
		Name:        p.Name,
		Preferences: prefs,
		IsDefault:   p.IsDefault,
		Locale:      p.Locale,
		CreatedAt:   toTimestamppb(p.CreatedAt),
	}
	if p.AvatarPath != nil {
		pb.AvatarPath = *p.AvatarPath
	}
	if p.MaxRating != nil {
		pb.MaxRating = *p.MaxRating
	}
	return pb
}

func inviteLinkToProto(l *user.InviteLink) *eveyv1.InviteLink {
	return &eveyv1.InviteLink{
		Id:        l.ID,
		Code:      l.Token,
		CreatedBy: l.CreatedBy,
		MaxUses:   int32(l.MaxUses),
		TimesUsed: int32(l.UsedCount),
		ExpiresAt: toTimestamppbPtr(l.ExpiresAt),
		CreatedAt: toTimestamppb(l.CreatedAt),
	}
}

func quickConnectToProto(q *user.QuickConnectRequest) *eveyv1.QuickConnectResponse {
	return &eveyv1.QuickConnectResponse{
		Code:      q.Code,
		Secret:    q.Secret,
		ExpiresAt: toTimestamppb(q.ExpiresAt),
	}
}

func apiKeyToProto(k *user.APIKeyModel) *eveyv1.APIKey {
	return &eveyv1.APIKey{
		Id:         k.ID,
		Name:       k.Name,
		UserId:     k.UserID,
		LastUsedAt: toTimestamppbPtr(k.LastUsedAt),
		CreatedAt:  toTimestamppb(k.CreatedAt),
	}
}

// --- Playback converters ---

var playMethodToProto = map[string]eveyv1.PlayMethod{
	"direct_play":   eveyv1.PlayMethod_PLAY_METHOD_DIRECT_PLAY,
	"direct_stream": eveyv1.PlayMethod_PLAY_METHOD_DIRECT_STREAM,
	"transcode":     eveyv1.PlayMethod_PLAY_METHOD_TRANSCODE,
}

var playMethodFromProto = map[eveyv1.PlayMethod]string{
	eveyv1.PlayMethod_PLAY_METHOD_DIRECT_PLAY:   "direct_play",
	eveyv1.PlayMethod_PLAY_METHOD_DIRECT_STREAM: "direct_stream",
	eveyv1.PlayMethod_PLAY_METHOD_TRANSCODE:     "transcode",
}

func watchProgressToProto(wp *playback.WatchProgress) *eveyv1.WatchProgress {
	return &eveyv1.WatchProgress{
		Id:           wp.ID,
		ProfileId:    wp.ProfileID,
		MediaItemId:  wp.MediaItemID,
		PositionMs:   wp.PositionMs,
		Played:       wp.Completed,
		PlayCount:    int32(wp.PlayCount),
		LastPlayedAt: toTimestamppb(wp.LastPlayedAt),
	}
}

func playSessionToProto(s *playback.PlaySession) *eveyv1.PlaySession {
	ps := &eveyv1.PlaySession{
		Id:          s.ID,
		ProfileId:   s.ProfileID,
		MediaItemId: s.MediaItemID,
		PlayMethod:  playMethodToProto[s.PlayMethod],
		DeviceName:  s.DeviceName,
		ClientName:  s.DeviceType,
		StartedAt:   toTimestamppb(s.StartedAt),
	}
	if s.PositionMs != nil {
		ps.PositionMs = *s.PositionMs
	}
	return ps
}

// --- Transcoding converters ---

var hwAccelTypeToProto = map[string]eveyv1.HWAccelType{
	"none":  eveyv1.HWAccelType_HW_ACCEL_TYPE_NONE,
	"vaapi": eveyv1.HWAccelType_HW_ACCEL_TYPE_VAAPI,
	"nvenc": eveyv1.HWAccelType_HW_ACCEL_TYPE_NVENC,
	"qsv":   eveyv1.HWAccelType_HW_ACCEL_TYPE_QSV,
}

var hwAccelTypeFromProto = map[eveyv1.HWAccelType]string{
	eveyv1.HWAccelType_HW_ACCEL_TYPE_NONE:  "none",
	eveyv1.HWAccelType_HW_ACCEL_TYPE_VAAPI: "vaapi",
	eveyv1.HWAccelType_HW_ACCEL_TYPE_NVENC: "nvenc",
	eveyv1.HWAccelType_HW_ACCEL_TYPE_QSV:   "qsv",
}

var transcodeStateToProto = map[string]eveyv1.TranscodeState{
	"queued":    eveyv1.TranscodeState_TRANSCODE_STATE_QUEUED,
	"running":   eveyv1.TranscodeState_TRANSCODE_STATE_RUNNING,
	"completed": eveyv1.TranscodeState_TRANSCODE_STATE_COMPLETED,
	"failed":    eveyv1.TranscodeState_TRANSCODE_STATE_FAILED,
	"cancelled": eveyv1.TranscodeState_TRANSCODE_STATE_CANCELLED,
}

func transcodeJobToProto(j *transcoding.TranscodeJob) *eveyv1.TranscodeJob {
	pb := &eveyv1.TranscodeJob{
		Id:          j.ID,
		MediaItemId: j.MediaItemID,
		State:       transcodeStateToProto[j.Status],
		VideoCodec:  j.OutputCodec,
		HwAccel:     hwAccelTypeToProto[j.HWAccelType],
		Bitrate:     j.BitrateOut,
		Speed:       j.Speed,
		Error:       j.ErrorMessage,
		StartedAt:   toTimestamppb(j.StartedAt),
	}
	if j.EndedAt != nil {
		pb.CompletedAt = toTimestamppb(*j.EndedAt)
	}
	return pb
}

func transcodeProfileToProto(p *transcoding.TranscodeProfile) *eveyv1.TranscodeProfile {
	return &eveyv1.TranscodeProfile{
		Id:         p.ID,
		Name:       p.Name,
		VideoCodec: p.VideoCodec,
		AudioCodec: p.AudioCodec,
		Container:  p.Container,
		MaxWidth:   int32(p.MaxWidth),
		MaxHeight:  int32(p.MaxHeight),
		MaxBitrate: p.VideoBitrate,
	}
}

func hwDeviceToProto(d transcoding.HWDevice) *eveyv1.HWDevice {
	return &eveyv1.HWDevice{
		Type:            hwAccelTypeToProto[d.Type],
		Name:            d.Name,
		SupportedCodecs: d.SupportedCodecs,
	}
}

// --- MKV converters ---

var mkvTrackTypeToProto = map[mkv.TrackType]eveyv1.TrackType{
	mkv.VideoTrack:    eveyv1.TrackType_TRACK_TYPE_VIDEO,
	mkv.AudioTrack:    eveyv1.TrackType_TRACK_TYPE_AUDIO,
	mkv.SubtitleTrack: eveyv1.TrackType_TRACK_TYPE_SUBTITLE,
}

func mkvContainerToProto(c *mkv.Container) *eveyv1.Container {
	pb := &eveyv1.Container{
		Path:       c.Path,
		DurationMs: c.DurationMs,
		Info: &eveyv1.ContainerInfo{
			MuxingApp:  c.Info.MuxingApp,
			WritingApp: c.Info.WritingApp,
			Title:      c.Info.Title,
			DurationMs: int64(c.Info.Duration),
		},
	}
	for _, t := range c.Tracks {
		pb.Tracks = append(pb.Tracks, mkvTrackToProto(t))
	}
	for _, ch := range c.Chapters {
		pb.Chapters = append(pb.Chapters, mkvChapterToProto(ch))
	}
	for _, a := range c.Attachments {
		pb.Attachments = append(pb.Attachments, mkvAttachmentToProto(a))
	}
	return pb
}

func mkvTrackToProto(t mkv.Track) *eveyv1.MkvTrack {
	pb := &eveyv1.MkvTrack{
		Id:        int32(t.ID),
		Type:      mkvTrackTypeToProto[t.Type],
		Codec:     t.Codec,
		Language:  t.Language,
		Name:      t.Name,
		IsDefault: t.IsDefault,
		IsForced:  t.IsForced,
		IsEnabled: true,
	}
	if t.Width != nil {
		pb.Width = int32(*t.Width)
	}
	if t.Height != nil {
		pb.Height = int32(*t.Height)
	}
	if t.Channels != nil {
		pb.Channels = int32(*t.Channels)
	}
	if t.SampleRate != nil {
		pb.SampleRate = int32(*t.SampleRate)
	}
	if t.BitDepth != nil {
		pb.BitDepth = int32(*t.BitDepth)
	}
	return pb
}

func mkvChapterToProto(ch mkv.Chapter) *eveyv1.Chapter {
	return &eveyv1.Chapter{
		Id:      fmt.Sprintf("%d", ch.ID),
		Title:   ch.Title,
		StartMs: ch.StartMs,
		EndMs:   ch.EndMs,
	}
}

func mkvAttachmentToProto(a mkv.Attachment) *eveyv1.Attachment {
	return &eveyv1.Attachment{
		Id:       int32(a.ID),
		Name:     a.Name,
		MimeType: a.MIMEType,
		Size:     a.Size,
	}
}

func mkvChapterFromProto(ch *eveyv1.Chapter) mkv.Chapter {
	return mkv.Chapter{
		Title:   ch.Title,
		StartMs: ch.StartMs,
		EndMs:   ch.EndMs,
	}
}

// --- Metadata converters ---

func metadataResultToProto(r domainmetadata.MetadataResult) *eveyv1.MetadataResult {
	pb := &eveyv1.MetadataResult{
		Title:         r.Title,
		OriginalTitle: r.OriginalTitle,
		Overview:      r.Overview,
		Genres:        r.Genres,
	}
	if r.Year != nil {
		pb.Year = int32(*r.Year)
	}
	if r.CommunityRating != nil {
		pb.Rating = *r.CommunityRating
	}
	return pb
}

func metadataOverrideToProto(o *domainmetadata.MetadataOverride) *eveyv1.MetadataOverride {
	cv := ""
	if o.CustomValue != nil {
		cv = fmt.Sprintf("%v", o.CustomValue)
	}
	return &eveyv1.MetadataOverride{
		Id:          o.ID,
		MediaItemId: o.MediaItemID,
		FieldName:   o.FieldName,
		CustomValue: cv,
		CreatedAt:   toTimestamppb(o.CreatedAt),
	}
}

var artworkTypeToProto = map[string]eveyv1.ArtworkType{
	"poster":   eveyv1.ArtworkType_ARTWORK_TYPE_PRIMARY,
	"backdrop": eveyv1.ArtworkType_ARTWORK_TYPE_BACKDROP,
	"banner":   eveyv1.ArtworkType_ARTWORK_TYPE_BANNER,
	"thumb":    eveyv1.ArtworkType_ARTWORK_TYPE_THUMB,
	"logo":     eveyv1.ArtworkType_ARTWORK_TYPE_LOGO,
	"disc":     eveyv1.ArtworkType_ARTWORK_TYPE_DISC,
}

func artworkRecordToProto(a *domainmetadata.ArtworkRecord) *eveyv1.Artwork {
	pb := &eveyv1.Artwork{
		Id:       a.ID,
		Provider: a.Provider,
		Type:     artworkTypeToProto[a.Type],
	}
	if a.SourceURL != nil {
		pb.Url = *a.SourceURL
	}
	if a.Width != nil {
		pb.Width = int32(*a.Width)
	}
	if a.Height != nil {
		pb.Height = int32(*a.Height)
	}
	if a.Language != nil {
		pb.Language = *a.Language
	}
	return pb
}

// --- Activity converters ---

func loginEntryToProto(e *activity.LoginEntry) *eveyv1.LoginEntry {
	pb := &eveyv1.LoginEntry{
		Id:        e.ID,
		UserId:    e.UserID,
		IpAddress: e.ClientIP,
		UserAgent: e.UserAgent,
		Success:   e.Success,
		Timestamp: toTimestamppb(e.CreatedAt),
	}
	if e.FailureReason != nil {
		pb.FailureReason = *e.FailureReason
	}
	return pb
}

func serverMetricToProto(m *activity.ServerMetric) *eveyv1.ServerMetric {
	memPercent := float64(0)
	if m.MemoryTotalBytes > 0 {
		memPercent = float64(m.MemoryUsedBytes) / float64(m.MemoryTotalBytes) * 100
	}
	return &eveyv1.ServerMetric{
		Timestamp:        toTimestamppb(m.RecordedAt),
		CpuPercent:       m.CPUPercent,
		MemoryPercent:    memPercent,
		MemoryUsedBytes:  m.MemoryUsedBytes,
		ActiveStreams:    int32(m.ActiveStreams),
		ActiveTranscodes: int32(m.ActiveTranscodes),
	}
}

func playHistoryEntryToProto(e *activity.PlayHistoryEntry) *eveyv1.PlayHistoryEntry {
	pb := &eveyv1.PlayHistoryEntry{
		Id:          e.ID,
		ProfileId:   e.ProfileID,
		MediaItemId: e.MediaItemID,
		MediaTitle:  e.MediaTitle,
		MediaType:   mediaItemTypeToProto[e.MediaType],
		DurationMs:  e.DurationMs,
		PositionMs:  e.PositionMs,
		Completed:   e.Completed,
		DeviceName:  e.DeviceName,
		ClientName:  e.ClientName,
		PlayMethod:  e.PlayMethod,
		StartedAt:   toTimestamppb(e.StartedAt),
		EndedAt:     toTimestamppbPtr(e.EndedAt),
	}
	return pb
}

func auditLogEntryToProto(e *activity.AuditLogEntry) *eveyv1.AuditLogEntry {
	pb := &eveyv1.AuditLogEntry{
		Id:           e.ID,
		UserId:       e.UserID,
		Username:     e.Username,
		Action:       e.Action,
		ResourceType: e.ResourceType,
		CreatedAt:    toTimestamppb(e.CreatedAt),
	}
	if e.ResourceID != nil {
		pb.ResourceId = *e.ResourceID
	}
	if e.ClientIP != nil {
		pb.IpAddress = *e.ClientIP
	}
	if len(e.Details) > 0 {
		if b, err := json.Marshal(e.Details); err == nil {
			pb.Details = string(b)
		}
	}
	return pb
}

// --- Plugin converters ---

var pluginStateToProto = map[string]eveyv1.PluginState{
	"stopped":      eveyv1.PluginState_PLUGIN_STATE_STOPPED,
	"initializing": eveyv1.PluginState(5), // PLUGIN_STATE_INITIALIZING — regenerate proto to use constant
	"running":      eveyv1.PluginState_PLUGIN_STATE_RUNNING,
	"error":        eveyv1.PluginState_PLUGIN_STATE_ERROR,
	"installed":    eveyv1.PluginState_PLUGIN_STATE_INSTALLED,
}

func pluginToProto(p *plugin.Plugin) *eveyv1.Plugin {
	return &eveyv1.Plugin{
		Id:           p.ID,
		Name:         p.Name,
		Description:  p.Description,
		Version:      p.Version,
		Author:       p.Author,
		Language:     p.Language,
		Enabled:      p.Enabled,
		State:        pluginStateToProto[p.Status],
		InstalledAt:  toTimestamppb(p.InstalledAt),
		Permissions:  p.Permissions,
		Capabilities: p.Capabilities,
	}
}

func pluginRuntimeToProto(rt *plugin.PluginRuntime) *eveyv1.PluginRuntime {
	return &eveyv1.PluginRuntime{
		Id:       rt.ID,
		Language: rt.Language,
		Version:  rt.Version,
	}
}

var streamTypeToProto = map[string]eveyv1.MediaStreamType{
	"video":      eveyv1.MediaStreamType_MEDIA_STREAM_TYPE_VIDEO,
	"audio":      eveyv1.MediaStreamType_MEDIA_STREAM_TYPE_AUDIO,
	"subtitle":   eveyv1.MediaStreamType_MEDIA_STREAM_TYPE_SUBTITLE,
	"attachment": eveyv1.MediaStreamType_MEDIA_STREAM_TYPE_ATTACHMENT,
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
	if s.ColorSpace != nil {
		pb.ColorSpace = *s.ColorSpace
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
