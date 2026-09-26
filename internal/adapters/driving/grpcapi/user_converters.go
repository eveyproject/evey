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
	if u.MaxProfiles != nil {
		v := int32(*u.MaxProfiles)
		pb.MaxProfiles = &v
	}
	if u.MaxDevicesPerProfile != nil {
		v := int32(*u.MaxDevicesPerProfile)
		pb.MaxDevicesPerProfile = &v
	}
	if u.LastLogin != nil {
		pb.LastLogin = toTimestamppb(*u.LastLogin)
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
		IsKids:      p.IsKids,
		Locale:      p.Locale,
		CreatedAt:   toTimestamppb(p.CreatedAt),
	}
	if p.AvatarPath != nil {
		pb.AvatarPath = *p.AvatarPath
	}
	if p.LastActiveAt != nil {
		pb.LastActiveAt = toTimestamppb(*p.LastActiveAt)
	}
	if p.MaxRating != nil {
		pb.MaxRating = *p.MaxRating
	}
	if p.Region != nil {
		pb.Region = p.Region
	}
	if p.Parental != nil && p.Parental.MaxAge != nil {
		v := int32(*p.Parental.MaxAge)
		pb.MaxAge = &v
	}
	return pb
}

func inviteLinkToProto(l *user.InviteLink) *eveyv1.InviteLink {
	libs := make([]string, len(l.LibraryIDs))
	for i, id := range l.LibraryIDs {
		libs[i] = string(id)
	}
	pb := &eveyv1.InviteLink{
		Id:         l.ID,
		Code:       l.Token,
		CreatedBy:  string(l.CreatedBy),
		MaxUses:    int32(l.MaxUses),
		TimesUsed:  int32(l.UsedCount),
		ExpiresAt:  toTimestamppbPtr(l.ExpiresAt),
		CreatedAt:  toTimestamppb(l.CreatedAt),
		LibraryIds: libs,
		IsAdmin:    l.IsAdmin,
	}
	if l.MaxProfiles != nil {
		v := int32(*l.MaxProfiles)
		pb.MaxProfiles = &v
	}
	return pb
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
