package grpcapi

import (
	"testing"
	"time"

	"github.com/evey-project/evey/internal/domain/activity"
	"github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/metadata"
	"github.com/evey-project/evey/internal/domain/playback"
	"github.com/evey-project/evey/internal/domain/plugin"
	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/evey-project/evey/internal/domain/transcoding"
	"github.com/evey-project/evey/internal/domain/user"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestToTimestamppb(t *testing.T) {
	t.Parallel()
	if got := toTimestamppb(time.Time{}); got != nil {
		t.Error("zero time should return nil")
	}
	now := time.Now()
	pb := toTimestamppb(now)
	if pb == nil {
		t.Fatal("expected non-nil timestamppb")
	}
}

func TestToTimestamppbPtr(t *testing.T) {
	t.Parallel()
	if got := toTimestamppbPtr(nil); got != nil {
		t.Error("nil should return nil")
	}
	now := time.Now()
	pb := toTimestamppbPtr(&now)
	if pb == nil {
		t.Fatal("expected non-nil timestamppb for non-nil time")
	}
}

func TestFromTimestamppb(t *testing.T) {
	t.Parallel()
	if got, ok := fromTimestamppb(nil); ok || !got.IsZero() {
		t.Errorf("nil should return (zero, false); got (%v, %v)", got, ok)
	}
	now := time.Now()
	pb := timestamppb.New(now)
	got, ok := fromTimestamppb(pb)
	if !ok {
		t.Error("non-nil should return ok=true")
	}
	if got.Unix() != now.Unix() {
		t.Errorf("expected %v, got %v", now, got)
	}
}

func TestFromTimestamppbPtr(t *testing.T) {
	t.Parallel()
	if got := fromTimestamppbPtr(nil); got != nil {
		t.Error("nil should return nil")
	}
	now := time.Now()
	pb := timestamppb.New(now)
	got := fromTimestamppbPtr(pb)
	if got == nil {
		t.Fatal("expected non-nil time")
	}
	if got.Unix() != now.Unix() {
		t.Errorf("expected %v, got %v", now, *got)
	}
}

func TestTrailerToProto(t *testing.T) {
	t.Parallel()
	tr := library.Trailer{
		Site:             "YouTube",
		Key:              "abc123",
		Language:         "en",
		Official:         true,
		Type:             "Trailer",
		UseSubtitles:     true,
		SubtitleLanguage: "fr",
	}
	pb := trailerToProto(tr)
	if pb.Site != "YouTube" || pb.Key != "abc123" || pb.Language != "en" {
		t.Error("basic fields mismatch")
	}
	if !pb.Official || !pb.UseSubtitles || pb.SubtitleLanguage != "fr" {
		t.Error("optional fields mismatch")
	}
}

func TestRatingToProto(t *testing.T) {
	t.Parallel()
	tests := []struct {
		source  string
		value   float64
		wantMax float64
	}{
		{"imdb", 7.5, 10},
		{"tmdb", 8.0, 10},
		{"rt", 85.0, 100},
		{"metacritic", 72.0, 100},
		{"unknown", 5.0, 10},
	}
	for _, tt := range tests {
		pb := ratingToProto(tt.source, tt.value)
		if pb.Source != tt.source || pb.Value != tt.value || pb.MaxValue != tt.wantMax {
			t.Errorf("ratingToProto(%q, %.1f) = {source:%q, val:%.1f, max:%.1f}, want max %.1f",
				tt.source, tt.value, pb.Source, pb.Value, pb.MaxValue, tt.wantMax)
		}
	}
}

func TestMediaItemToProto_FilePathAdminOnly(t *testing.T) {
	t.Parallel()
	item := &library.MediaItem{ID: "m-1", File: &library.MediaFile{Path: "/mnt/secret/x.mkv"}}
	if pb := mediaItemToProto(item, false); pb.FilePath != "" {
		t.Fatalf("non-admin must not receive the absolute file path, got %q", pb.FilePath)
	}
	if pb := mediaItemToProto(item, true); pb.FilePath != "/mnt/secret/x.mkv" {
		t.Fatalf("admin should receive the file path, got %q", pb.FilePath)
	}
}

func TestMediaItemToProto(t *testing.T) {
	t.Parallel()
	idx := 3
	rating := 8.5
	year := 2024
	premiere := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	item := &library.MediaItem{
		ID:             "m-1",
		LibraryID:      "lib-1",
		Type:           "movie",
		Title:          "Test Movie",
		IndexNumber:    &idx,
		Classification: &library.MediaClassification{CommunityRating: &rating, ProviderRatings: map[string]float64{"imdb": 7.5}},
		Catalog:        &library.MediaCatalog{Year: &year, PremiereDate: &premiere},
		Trailers:       []library.Trailer{{Site: "YouTube", Key: "k1"}},
		AddedAt:        time.Now(),
		UpdatedAt:      time.Now(),
	}

	pb := mediaItemToProto(item, true)
	if pb.Id != "m-1" || pb.Title != "Test Movie" {
		t.Error("basic fields mismatch")
	}
	if pb.IndexNumber != 3 {
		t.Errorf("IndexNumber = %d, want 3", pb.IndexNumber)
	}
	if pb.CommunityRating != 8.5 {
		t.Errorf("CommunityRating = %.1f, want 8.5", pb.CommunityRating)
	}
	if pb.Year != 2024 {
		t.Errorf("Year = %d, want 2024", pb.Year)
	}
	if pb.PremiereDate == nil {
		t.Error("PremiereDate should not be nil")
	}
	if len(pb.Trailers) != 1 {
		t.Errorf("expected 1 trailer, got %d", len(pb.Trailers))
	}
	if len(pb.Ratings) != 1 {
		t.Errorf("expected 1 rating, got %d", len(pb.Ratings))
	}
}

func TestMediaItemToProto_NilOptionals(t *testing.T) {
	t.Parallel()
	item := &library.MediaItem{ID: "m-2", Type: "episode"}
	pb := mediaItemToProto(item, true)
	if pb.IndexNumber != 0 || pb.CommunityRating != 0 || pb.Year != 0 {
		t.Error("nil optionals should result in zero values")
	}
	if pb.PremiereDate != nil {
		t.Error("nil PremiereDate should result in nil proto")
	}
}

func TestStringFromInterface(t *testing.T) {
	t.Parallel()
	if got := stringFromInterface(nil); got != "" {
		t.Errorf("nil should return empty, got %q", got)
	}
	if got := stringFromInterface("hello"); got != "hello" {
		t.Errorf("string should return itself, got %q", got)
	}
	if got := stringFromInterface(42); got != "42" {
		t.Errorf("int should return string repr, got %q", got)
	}
}

func TestLibraryToProto(t *testing.T) {
	t.Parallel()
	lib := &library.Library{
		ID:    "lib-1",
		Name:  "Movies",
		Type:  "movie",
		Paths: []string{"/data/movies"},
	}
	pb := libraryToProto(lib)
	if pb.Id != "lib-1" || pb.Name != "Movies" {
		t.Error("basic fields mismatch")
	}
}

func TestUserToProto(t *testing.T) {
	t.Parallel()
	maxStreams := 3
	u := &user.User{
		ID:         "u-1",
		Username:   "admin",
		Email:      "admin@test.com",
		IsAdmin:    true,
		Enabled:    true,
		MaxStreams: &maxStreams,
	}
	pb := userToProto(u)
	if pb.Id != "u-1" || pb.Username != "admin" || !pb.IsAdmin {
		t.Error("basic fields mismatch")
	}
	if pb.MaxStreams != 3 {
		t.Errorf("MaxStreams = %d, want 3", pb.MaxStreams)
	}
}

func TestUserToProto_NilMaxStreams(t *testing.T) {
	t.Parallel()
	u := &user.User{ID: "u-2"}
	pb := userToProto(u)
	if pb.MaxStreams != 0 {
		t.Errorf("MaxStreams = %d, want 0", pb.MaxStreams)
	}
}

func TestProfileToProto(t *testing.T) {
	t.Parallel()
	avatar := "/path/avatar.png"
	maxRating := "PG-13"
	maxBitrate := 10000
	p := &user.Profile{
		ID:                "p-1",
		UserID:            "u-1",
		Name:              "Main",
		IsDefault:         true,
		AvatarPath:        &avatar,
		MaxRating:         &maxRating,
		AudioLanguages:    []string{"en", "fr"},
		SubtitleLanguages: []string{"en"},
		MaxBitrateKbps:    &maxBitrate,
		Preferences:       map[string]string{"theme": "dark"},
	}
	pb := profileToProto(p)
	if pb.Id != "p-1" || !pb.IsDefault {
		t.Error("basic fields mismatch")
	}
	if pb.AvatarPath != "/path/avatar.png" {
		t.Errorf("AvatarPath = %q", pb.AvatarPath)
	}
	if pb.MaxRating != "PG-13" {
		t.Errorf("MaxRating = %q", pb.MaxRating)
	}
	if pb.Preferences["audio_languages"] != "en,fr" {
		t.Errorf("audio_languages = %q", pb.Preferences["audio_languages"])
	}
	if pb.Preferences["subtitle_languages"] != "en" {
		t.Errorf("subtitle_languages = %q", pb.Preferences["subtitle_languages"])
	}
	if pb.Preferences["max_bitrate_kbps"] != "10000" {
		t.Errorf("max_bitrate_kbps = %q", pb.Preferences["max_bitrate_kbps"])
	}
}

func TestInviteLinkToProto(t *testing.T) {
	t.Parallel()
	exp := time.Now().Add(time.Hour)
	l := &user.InviteLink{
		ID:        "inv-1",
		Token:     "abc",
		CreatedBy: "u-1",
		MaxUses:   5,
		UsedCount: 2,
		ExpiresAt: &exp,
	}
	pb := inviteLinkToProto(l)
	if pb.Id != "inv-1" || pb.Code != "abc" || pb.MaxUses != 5 || pb.TimesUsed != 2 {
		t.Error("basic fields mismatch")
	}
	if pb.ExpiresAt == nil {
		t.Error("ExpiresAt should not be nil")
	}
}

func TestQuickConnectToProto(t *testing.T) {
	t.Parallel()
	q := &user.QuickConnectRequest{
		Code:      "1234",
		Secret:    "secret",
		ExpiresAt: time.Now().Add(time.Minute),
	}
	pb := quickConnectToProto(q)
	if pb.Code != "1234" || pb.Secret != "secret" {
		t.Error("fields mismatch")
	}
}

func TestAPIKeyToProto(t *testing.T) {
	t.Parallel()
	lastUsed := time.Now()
	k := &user.APIKeyModel{
		ID:         "k-1",
		Name:       "test-key",
		UserID:     "u-1",
		LastUsedAt: &lastUsed,
	}
	pb := apiKeyToProto(k)
	if pb.Id != "k-1" || pb.Name != "test-key" {
		t.Error("fields mismatch")
	}
	if pb.LastUsedAt == nil {
		t.Error("LastUsedAt should not be nil")
	}
}

func TestWatchProgressToProto(t *testing.T) {
	t.Parallel()
	pos, err := shared.NewDurationMs(60000)
	if err != nil {
		t.Fatal(err)
	}
	wp := &playback.WatchProgress{
		ID:          "wp-1",
		ProfileID:   "p-1",
		MediaItemID: "m-1",
		Position:    pos,
		Completed:   true,
		PlayCount:   3,
	}
	pb := watchProgressToProto(wp)
	if pb.Id != "wp-1" || pb.PositionMs != 60000 || !pb.Played || pb.PlayCount != 3 {
		t.Error("fields mismatch")
	}
}

func TestPlaySessionToProto(t *testing.T) {
	t.Parallel()
	pos, err := shared.NewDurationMs(30000)
	if err != nil {
		t.Fatal(err)
	}
	s := &playback.PlaySession{
		ID:          "ps-1",
		ProfileID:   "p-1",
		MediaItemID: "m-1",
		PlayMethod:  "direct_play",
		DeviceName:  "Chrome",
		DeviceType:  "web",
		Position:    &pos,
	}
	pb := playSessionToProto(s)
	if pb.Id != "ps-1" || pb.PositionMs != 30000 {
		t.Error("fields mismatch")
	}
}

func TestPlaySessionToProto_NilPosition(t *testing.T) {
	t.Parallel()
	s := &playback.PlaySession{ID: "ps-2"}
	pb := playSessionToProto(s)
	if pb.PositionMs != 0 {
		t.Errorf("nil PositionMs should be 0, got %d", pb.PositionMs)
	}
}

func TestTranscodeJobToProto(t *testing.T) {
	t.Parallel()
	ended := time.Now()
	j := &transcoding.TranscodeJob{
		ID:          "tj-1",
		MediaItemID: "m-1",
		Status:      "running",
		OutputCodec: "h264",
		HWAccelType: "vaapi",
		EndedAt:     &ended,
	}
	pb := transcodeJobToProto(j)
	if pb.Id != "tj-1" || pb.CompletedAt == nil {
		t.Error("fields mismatch")
	}
}

func TestTranscodeJobToProto_NilEndedAt(t *testing.T) {
	t.Parallel()
	j := &transcoding.TranscodeJob{ID: "tj-2", Status: "queued"}
	pb := transcodeJobToProto(j)
	if pb.CompletedAt != nil {
		t.Error("nil EndedAt should give nil CompletedAt")
	}
}

func TestTranscodeProfileToProto(t *testing.T) {
	t.Parallel()
	p := &transcoding.TranscodeProfile{
		ID:           "tp-1",
		Name:         "1080p",
		VideoCodec:   "h264",
		AudioCodec:   "aac",
		Container:    "mp4",
		MaxWidth:     1920,
		MaxHeight:    1080,
		VideoBitrate: 5000000,
	}
	pb := transcodeProfileToProto(p)
	if pb.Id != "tp-1" || pb.MaxWidth != 1920 {
		t.Error("fields mismatch")
	}
}

func TestHWDeviceToProto(t *testing.T) {
	t.Parallel()
	d := transcoding.HWDevice{
		Type:            "vaapi",
		Name:            "/dev/dri/renderD128",
		SupportedCodecs: []string{"h264_vaapi"},
	}
	pb := hwDeviceToProto(d)
	if pb.Name != "/dev/dri/renderD128" {
		t.Error("Name mismatch")
	}
}

func TestCastMemberToProto(t *testing.T) {
	t.Parallel()
	charName := "John Doe"
	thumb := "/thumb.jpg"
	cm := library.CastMember{
		PersonID:      "per-1",
		PersonName:    "Actor Name",
		RoleType:      "actor",
		CharacterName: &charName,
		ThumbPath:     &thumb,
		SortOrder:     1,
	}
	pb := castMemberToProto(cm)
	if pb.Id != "per-1" || pb.Name != "Actor Name" || pb.Role != "John Doe" {
		t.Error("fields mismatch")
	}
	if pb.ThumbUrl != "/thumb.jpg" {
		t.Errorf("ThumbUrl = %q", pb.ThumbUrl)
	}
}

func TestCastMemberToProto_NilOptionals(t *testing.T) {
	t.Parallel()
	cm := library.CastMember{PersonID: "per-2", RoleType: "director"}
	pb := castMemberToProto(cm)
	if pb.Role != "director" {
		t.Errorf("Role = %q, want director", pb.Role)
	}
}

func TestMediaStreamToProto(t *testing.T) {
	t.Parallel()
	w, h := 1920, 1080
	fr := 23.976
	cs := "bt709"
	ch := 6
	sr := 48000
	bd := 24
	br := int64(1500000)
	ext := "/subs/en.srt"
	s := &library.MediaStream{
		StreamIndex:   0,
		StreamType:    "video",
		Codec:         "h264",
		CodecLongName: "H.264",
		Width:         &w,
		Height:        &h,
		Framerate:     &fr,
		ColorSpace:    &cs,
		Channels:      &ch,
		SampleRate:    &sr,
		BitDepth:      &bd,
		Bitrate:       &br,
		IsExternal:    true,
		ExternalPath:  &ext,
	}
	pb := mediaStreamToProto(s)
	if pb.Width != 1920 || pb.Height != 1080 || pb.Codec != "h264" {
		t.Error("basic fields mismatch")
	}
	if pb.ExternalPath != "/subs/en.srt" || !pb.IsExternal {
		t.Error("external fields mismatch")
	}
}

func TestMediaStreamToProto_NilOptionals(t *testing.T) {
	t.Parallel()
	s := &library.MediaStream{StreamIndex: 1, StreamType: "audio", Codec: "aac"}
	pb := mediaStreamToProto(s)
	if pb.Width != 0 || pb.Height != 0 || pb.Channels != 0 {
		t.Error("nil optionals should be 0")
	}
}

// color_transfer is the only field that separates PQ from HLG, and all three
// clients read it: two label HDR from it, and mobile's castSafeDirectPlay
// refuses a direct play of PQ/HLG toward a non-HDR Chromecast on it. The
// domain carried it from the start and this converter never put it on the
// wire, so every client saw "" - which made every HLG title read as HDR10 and
// left that cast gate unable to fire, with nothing failing anywhere.
func TestMediaStreamToProto_PutsColorTransferOnTheWire(t *testing.T) {
	t.Parallel()
	for _, transfer := range []string{"smpte2084", "arib-std-b67"} {
		ct := transfer
		pb := mediaStreamToProto(&library.MediaStream{
			StreamIndex: 0, StreamType: "video", Codec: "hevc", ColorTransfer: &ct,
		})
		if pb.ColorTransfer != transfer {
			t.Fatalf("ColorTransfer = %q, want %q", pb.ColorTransfer, transfer)
		}
	}
}

func TestLoginEntryToProto(t *testing.T) {
	t.Parallel()
	reason := "bad password"
	e := &activity.LoginEntry{
		ID:            "le-1",
		UserID:        "u-1",
		ClientIP:      "1.2.3.4",
		UserAgent:     "Chrome",
		Success:       false,
		FailureReason: &reason,
	}
	pb := loginEntryToProto(e)
	if pb.Id != "le-1" || pb.FailureReason != "bad password" || pb.Success {
		t.Error("fields mismatch")
	}
}

func TestLoginEntryToProto_NilReason(t *testing.T) {
	t.Parallel()
	e := &activity.LoginEntry{ID: "le-2", Success: true}
	pb := loginEntryToProto(e)
	if pb.FailureReason != "" {
		t.Error("nil reason should be empty")
	}
}

func TestServerMetricToProto(t *testing.T) {
	t.Parallel()
	m := &activity.ServerMetric{
		CPUPercent:       45.2,
		MemoryUsedBytes:  4 << 30,
		MemoryTotalBytes: 8 << 30,
		ActiveStreams:    2,
	}
	pb := serverMetricToProto(m)
	if pb.CpuPercent != 45.2 {
		t.Errorf("CpuPercent = %.1f", pb.CpuPercent)
	}
	if pb.MemoryPercent < 49 || pb.MemoryPercent > 51 {
		t.Errorf("MemoryPercent = %.1f, expected ~50", pb.MemoryPercent)
	}
}

func TestServerMetricToProto_ZeroTotal(t *testing.T) {
	t.Parallel()
	m := &activity.ServerMetric{MemoryTotalBytes: 0}
	pb := serverMetricToProto(m)
	if pb.MemoryPercent != 0 {
		t.Errorf("zero total should give 0%%, got %.1f", pb.MemoryPercent)
	}
}

func TestPlayHistoryEntryToProto(t *testing.T) {
	t.Parallel()
	ended := time.Now()
	dur, _ := shared.NewDurationMs(120000)
	pos, _ := shared.NewDurationMs(60000)
	e := &activity.PlayHistoryEntry{
		ID:          "ph-1",
		ProfileID:   "p-1",
		MediaItemID: "m-1",
		MediaTitle:  "Movie",
		MediaType:   "movie",
		Duration:    dur,
		Position:    pos,
		Completed:   false,
		EndedAt:     &ended,
	}
	pb := playHistoryEntryToProto(e)
	if pb.Id != "ph-1" || pb.Completed || pb.EndedAt == nil {
		t.Error("fields mismatch")
	}
}

func TestAuditLogEntryToProto(t *testing.T) {
	t.Parallel()
	resID := "m-1"
	ip := "10.0.0.1"
	e := &activity.AuditLogEntry{
		ID:           "al-1",
		UserID:       "u-1",
		Username:     "admin",
		Action:       "delete",
		ResourceType: "media_item",
		ResourceID:   &resID,
		ClientIP:     &ip,
		Details:      map[string]any{"reason": "cleanup"},
	}
	pb := auditLogEntryToProto(e)
	if pb.Id != "al-1" || pb.ResourceId != "m-1" || pb.IpAddress != "10.0.0.1" {
		t.Error("fields mismatch")
	}
	if pb.Details == "" {
		t.Error("Details should be populated from JSON")
	}
}

func TestAuditLogEntryToProto_NilOptionals(t *testing.T) {
	t.Parallel()
	e := &activity.AuditLogEntry{ID: "al-2", Action: "login"}
	pb := auditLogEntryToProto(e)
	if pb.ResourceId != "" || pb.IpAddress != "" || pb.Details != "" {
		t.Error("nil optionals should be empty")
	}
}

func TestPluginToProto(t *testing.T) {
	t.Parallel()
	p := &plugin.Plugin{
		ID:           "plg-1",
		Name:         "TestPlugin",
		Description:  "desc",
		Version:      "1.0.0",
		Author:       "test",
		Language:     "python",
		Enabled:      true,
		Status:       "running",
		Permissions:  []string{"network"},
		Capabilities: []string{"metadata.provider"},
	}
	pb := pluginToProto(p)
	if pb.Id != "plg-1" || pb.Name != "TestPlugin" || !pb.Enabled {
		t.Error("fields mismatch")
	}
}

func TestPluginRuntimeToProto(t *testing.T) {
	t.Parallel()
	rt := &plugin.PluginRuntime{ID: "rt-1", Language: "python", Version: "3.11"}
	pb := pluginRuntimeToProto(rt)
	if pb.Id != "rt-1" || pb.Language != "python" {
		t.Error("fields mismatch")
	}
}

func TestMetadataResultToProto(t *testing.T) {
	t.Parallel()
	year := 2024
	rating := 8.5
	r := metadata.MetadataResult{
		Title:           "Movie",
		OriginalTitle:   "Le Film",
		Overview:        "overview",
		Genres:          []string{"Action"},
		Year:            &year,
		CommunityRating: &rating,
	}
	pb := metadataResultToProto(r)
	if pb.Title != "Movie" || pb.Year != 2024 || pb.Rating != 8.5 {
		t.Error("fields mismatch")
	}
}

func TestMetadataResultToProto_NilOptionals(t *testing.T) {
	t.Parallel()
	r := metadata.MetadataResult{Title: "X"}
	pb := metadataResultToProto(r)
	if pb.Year != 0 || pb.Rating != 0 {
		t.Error("nil optionals should be 0")
	}
}

func TestMetadataOverrideToProto(t *testing.T) {
	t.Parallel()
	o := &metadata.MetadataOverride{
		ID:          "mo-1",
		MediaItemID: "m-1",
		FieldName:   "title",
		CustomValue: "New Title",
	}
	pb := metadataOverrideToProto(o)
	if pb.Id != "mo-1" || pb.CustomValue != "New Title" {
		t.Error("fields mismatch")
	}
}

func TestMetadataOverrideToProto_NilCustomValue(t *testing.T) {
	t.Parallel()
	o := &metadata.MetadataOverride{ID: "mo-2", CustomValue: nil}
	pb := metadataOverrideToProto(o)
	if pb.CustomValue != "" {
		t.Error("nil CustomValue should be empty")
	}
}

func TestArtworkRecordToProto(t *testing.T) {
	t.Parallel()
	url := "https://image.tmdb.org/poster.jpg"
	w := 500
	h := 750
	lang := "en"
	a := &metadata.ArtworkRecord{
		ID:        "art-1",
		Provider:  "tmdb",
		Type:      "poster",
		SourceURL: &url,
		Width:     &w,
		Height:    &h,
		Language:  &lang,
	}
	pb := artworkRecordToProto(a)
	if pb.Id != "art-1" || pb.Url != url || pb.Width != 500 {
		t.Error("fields mismatch")
	}
}

func TestArtworkRecordToProto_NilOptionals(t *testing.T) {
	t.Parallel()
	a := &metadata.ArtworkRecord{ID: "art-2", Provider: "tmdb", Type: "poster"}
	pb := artworkRecordToProto(a)
	if pb.Url != "" || pb.Width != 0 || pb.Language != "" {
		t.Error("nil optionals should be empty/zero")
	}
}
