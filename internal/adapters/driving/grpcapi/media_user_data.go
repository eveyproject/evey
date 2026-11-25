package grpcapi

import (
	"context"
	"log/slog"
	"sync"

	eveyv1 "github.com/eveyproject/evey/gen/evey/v1"
	"github.com/eveyproject/evey/internal/adapters/driving/grpcapi/interceptors"
	"github.com/eveyproject/evey/internal/domain/playback"
	"github.com/eveyproject/evey/internal/domain/shared"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// mediaUserDataReader is the one read the watched mark comes from.
type mediaUserDataReader interface {
	UserData(ctx context.Context, profileID shared.ProfileID, mediaItemIDs []string) (map[string]playback.UserData, error)
}

var mediaItemFullName = (&eveyv1.MediaItem{}).ProtoReflect().Descriptor().FullName()

// MediaUserDataUnary fills MediaItem.user_data on every answer that carries a
// MediaItem, at any depth, for the caller's profile. It sits on the server
// rather than in each handler so no read can forget it: a new RPC returning
// titles gets the watched mark without a line of its own. Each handler
// stamping its own items is how the mark drifted before (a client list here,
// an inline SQL copy there).
func MediaUserDataUnary(src mediaUserDataReader) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		resp, err := handler(ctx, req)
		if err != nil || src == nil {
			return resp, err
		}
		if msg, ok := resp.(proto.Message); ok {
			stampMediaUserData(ctx, src, msg)
		}
		return resp, nil
	}
}

func stampMediaUserData(ctx context.Context, src mediaUserDataReader, msg proto.Message) {
	var items []*eveyv1.MediaItem
	collectMediaItems(msg.ProtoReflect(), &items)
	if len(items) == 0 {
		return
	}
	claims := interceptors.ClaimsFromContext(ctx)
	if claims == nil || claims.ProfileID == "" {
		return
	}
	ids := make([]string, 0, len(items))
	seen := make(map[string]bool, len(items))
	for _, it := range items {
		if it.GetId() != "" && !seen[it.GetId()] {
			seen[it.GetId()] = true
			ids = append(ids, it.GetId())
		}
	}
	byID, err := src.UserData(ctx, shared.ProfileID(claims.ProfileID), ids)
	if err != nil {
		slog.WarnContext(ctx, "media user data", slog.Int("items", len(ids)), slog.Any("err", err))
		return
	}
	for _, it := range items {
		ud, ok := byID[it.GetId()]
		if !ok {
			continue
		}
		it.UserData = mediaUserDataToProto(ud, it.GetDurationMs())
	}
}

func mediaUserDataToProto(ud playback.UserData, durationMs int64) *eveyv1.MediaUserData {
	out := &eveyv1.MediaUserData{
		Played:     ud.Played,
		PositionMs: ud.Position.Milliseconds(),
		PlayCount:  int32(ud.PlayCount),
	}
	if durationMs > 0 {
		out.DurationMs = durationMs
		// Clamped like applyMediaDuration: a position past a short header.
		out.PercentComplete = min(float64(out.PositionMs)/float64(durationMs)*100, 100)
	}
	return out
}

// collectMediaItems gathers every MediaItem under m. Messages that cannot hold
// one are skipped by descriptor, so most answers cost one map lookup.
func collectMediaItems(m protoreflect.Message, out *[]*eveyv1.MediaItem) {
	if !mayHoldMediaItem(m.Descriptor()) {
		return
	}
	if m.Descriptor().FullName() == mediaItemFullName {
		if it, ok := m.Interface().(*eveyv1.MediaItem); ok {
			*out = append(*out, it)
		}
		return
	}
	m.Range(func(fd protoreflect.FieldDescriptor, v protoreflect.Value) bool {
		if fd.Message() == nil {
			return true
		}
		switch {
		case fd.IsList():
			l := v.List()
			for i := 0; i < l.Len(); i++ {
				collectMediaItems(l.Get(i).Message(), out)
			}
		case fd.IsMap():
			if fd.MapValue().Message() != nil {
				v.Map().Range(func(_ protoreflect.MapKey, mv protoreflect.Value) bool {
					collectMediaItems(mv.Message(), out)
					return true
				})
			}
		default:
			collectMediaItems(v.Message(), out)
		}
		return true
	})
}

var holdsMediaItem sync.Map // protoreflect.FullName -> bool

func mayHoldMediaItem(md protoreflect.MessageDescriptor) bool {
	if v, ok := holdsMediaItem.Load(md.FullName()); ok {
		return v.(bool)
	}
	got := descriptorHoldsMediaItem(md, map[protoreflect.FullName]bool{})
	holdsMediaItem.Store(md.FullName(), got)
	return got
}

func descriptorHoldsMediaItem(md protoreflect.MessageDescriptor, visiting map[protoreflect.FullName]bool) bool {
	if md.FullName() == mediaItemFullName {
		return true
	}
	if visiting[md.FullName()] {
		return false
	}
	visiting[md.FullName()] = true
	fields := md.Fields()
	for i := 0; i < fields.Len(); i++ {
		fd := fields.Get(i)
		sub := fd.Message()
		if fd.IsMap() {
			sub = fd.MapValue().Message()
		}
		if sub != nil && descriptorHoldsMediaItem(sub, visiting) {
			return true
		}
	}
	return false
}
