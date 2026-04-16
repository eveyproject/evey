package grpcapi

import (
	"fmt"
	"maps"
	"strings"

	eveyv1 "github.com/evey-project/evey/gen/evey/v1"
	"github.com/evey-project/evey/internal/domain/user"
)

func userToProto(u *user.User) *eveyv1.User {
	pb := &eveyv1.User{
		Id:        string(u.ID),
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
		Id:          string(p.ID),
		UserId:      string(p.UserID),
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
		CreatedBy: string(l.CreatedBy),
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
		UserId:     string(k.UserID),
		LastUsedAt: toTimestamppbPtr(k.LastUsedAt),
		CreatedAt:  toTimestamppb(k.CreatedAt),
		Scopes:     k.Permissions,
	}
}
