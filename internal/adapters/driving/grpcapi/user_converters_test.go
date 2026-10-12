package grpcapi

import (
	"testing"
	"time"

	"github.com/eveyproject/evey/internal/domain/shared"
	"github.com/eveyproject/evey/internal/domain/user"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUserToProto_FullFields(t *testing.T) {
	t.Parallel()
	max := 3
	now := time.Date(2024, 5, 1, 12, 0, 0, 0, time.UTC)
	u := &user.User{
		ID:         shared.UserID("usr-1"),
		Username:   "alice",
		Email:      "a@b.c",
		IsAdmin:    true,
		Enabled:    true,
		MaxStreams: &max,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	pb := userToProto(u)
	assert.Equal(t, "usr-1", pb.Id)
	assert.Equal(t, "alice", pb.Username)
	assert.Equal(t, "a@b.c", pb.Email)
	assert.True(t, pb.IsAdmin)
	assert.True(t, pb.Enabled)
	assert.Equal(t, int32(3), pb.MaxStreams)
	require.NotNil(t, pb.CreatedAt)
	assert.Equal(t, now.Unix(), pb.CreatedAt.AsTime().Unix())
}

func TestProfileToProto_FullFieldsAndPrefs(t *testing.T) {
	t.Parallel()
	avatar := "/a.png"
	rating := "PG-13"
	max := 5000
	p := &user.Profile{
		ID:                shared.ProfileID("prof-1"),
		UserID:            shared.UserID("usr-1"),
		Name:              "kids",
		AvatarPath:        &avatar,
		Preferences:       map[string]string{"theme": "dark"},
		MaxRating:         &rating,
		IsDefault:         true,
		AudioLanguages:    []string{"fr", "en"},
		SubtitleLanguages: []string{"en"},
		MaxBitrateKbps:    &max,
		Locale:            "fr",
		CreatedAt:         time.Now(),
	}
	pb := profileToProto(p)
	assert.Equal(t, "prof-1", pb.Id)
	assert.Equal(t, "usr-1", pb.UserId)
	assert.Equal(t, "kids", pb.Name)
	assert.Equal(t, "/a.png", pb.AvatarPath)
	assert.Equal(t, "PG-13", pb.MaxRating)
	assert.True(t, pb.IsDefault)
	assert.Equal(t, "fr", pb.Locale)
	assert.Equal(t, "fr,en", pb.Preferences["audio_languages"])
	assert.Equal(t, "en", pb.Preferences["subtitle_languages"])
	assert.Equal(t, "5000", pb.Preferences["max_bitrate_kbps"])
	assert.Equal(t, "dark", pb.Preferences["theme"])
}

func TestProfileToProto_NilOptionals(t *testing.T) {
	t.Parallel()
	p := &user.Profile{ID: shared.ProfileID("p"), UserID: shared.UserID("u"), Name: "n", Preferences: map[string]string{}}
	pb := profileToProto(p)
	assert.Empty(t, pb.AvatarPath)
	assert.Empty(t, pb.MaxRating)
	_, hasAudio := pb.Preferences["audio_languages"]
	_, hasSub := pb.Preferences["subtitle_languages"]
	_, hasBr := pb.Preferences["max_bitrate_kbps"]
	assert.False(t, hasAudio)
	assert.False(t, hasSub)
	assert.False(t, hasBr)
}

func TestInviteLinkToProto_FullFields(t *testing.T) {
	t.Parallel()
	exp := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	l := &user.InviteLink{
		ID:        "inv-1",
		CreatedBy: shared.UserID("usr-1"),
		Token:     "tok",
		MaxUses:   10,
		UsedCount: 2,
		ExpiresAt: &exp,
		CreatedAt: time.Now(),
	}
	pb := inviteLinkToProto(l)
	assert.Equal(t, "inv-1", pb.Id)
	assert.Equal(t, "tok", pb.Code)
	assert.Equal(t, "usr-1", pb.CreatedBy)
	assert.Equal(t, int32(10), pb.MaxUses)
	assert.Equal(t, int32(2), pb.TimesUsed)
	require.NotNil(t, pb.ExpiresAt)
	assert.Equal(t, exp.Unix(), pb.ExpiresAt.AsTime().Unix())
}

func TestInviteLinkToProto_NilExpires(t *testing.T) {
	t.Parallel()
	l := &user.InviteLink{ID: "i", CreatedBy: shared.UserID("u"), Token: "t", CreatedAt: time.Now()}
	pb := inviteLinkToProto(l)
	assert.Nil(t, pb.ExpiresAt)
}

func TestQuickConnectToProto_RoundTrip(t *testing.T) {
	t.Parallel()
	exp := time.Date(2024, 6, 1, 12, 0, 0, 0, time.UTC)
	q := &user.QuickConnectRequest{Code: "ABC123", Secret: "s3cr3t", ExpiresAt: exp}
	pb := quickConnectToProto(q)
	assert.Equal(t, "ABC123", pb.Code)
	assert.Equal(t, "s3cr3t", pb.Secret)
	require.NotNil(t, pb.ExpiresAt)
	assert.Equal(t, exp.Unix(), pb.ExpiresAt.AsTime().Unix())
}

func TestAPIKeyToProto_FullFields(t *testing.T) {
	t.Parallel()
	last := time.Date(2024, 7, 1, 0, 0, 0, 0, time.UTC)
	k := &user.APIKeyModel{
		ID:         "key-1",
		UserID:     shared.UserID("usr-1"),
		Name:       "ci",
		LastUsedAt: &last,
		CreatedAt:  time.Now(),
	}
	pb := apiKeyToProto(k)
	assert.Equal(t, "key-1", pb.Id)
	assert.Equal(t, "ci", pb.Name)
	assert.Equal(t, "usr-1", pb.UserId)
	require.NotNil(t, pb.LastUsedAt)
	assert.Equal(t, last.Unix(), pb.LastUsedAt.AsTime().Unix())
}

func TestAPIKeyToProto_NilLastUsed(t *testing.T) {
	t.Parallel()
	k := &user.APIKeyModel{ID: "k", UserID: shared.UserID("u"), Name: "n", CreatedAt: time.Now()}
	pb := apiKeyToProto(k)
	assert.Nil(t, pb.LastUsedAt)
}
