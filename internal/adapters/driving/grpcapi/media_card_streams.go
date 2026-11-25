package grpcapi

import (
	"context"
	"log/slog"
	"sync"

	eveyv1 "github.com/eveyproject/evey/gen/evey/v1"
	"github.com/eveyproject/evey/internal/domain/library"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// mediaStreamsReader is the batch read the card badges come from.
type mediaStreamsReader interface {
	GetStreamsForItems(ctx context.Context, mediaItemIDs []string) (map[string][]*library.MediaStream, error)
}

var mediaStreamFullName = (&eveyv1.MediaStream{}).ProtoReflect().Descriptor().FullName()

// A card message is one that names a title (media_item_id) and declares a
// video_stream to show its resolution and HDR badges: FeedItem and
// PopularMediaItem today, any rail added later by declaring the same pair.
const (
	cardIDField     = protoreflect.Name("media_item_id")
	cardStreamField = protoreflect.Name("video_stream")
)

// MediaCardStreamsUnary fills video_stream on every card message an answer
// carries, in one read per answer. It sits on the server, like the watched
// mark, so a home rail cannot ship without its badges: each rail filling its
// own is how the web cards ended up with badge code no rail ever fed.
func MediaCardStreamsUnary(src mediaStreamsReader) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		resp, err := handler(ctx, req)
		if err != nil || src == nil {
			return resp, err
		}
		if msg, ok := resp.(proto.Message); ok {
			stampCardStreams(ctx, src, msg)
		}
		return resp, nil
	}
}

func stampCardStreams(ctx context.Context, src mediaStreamsReader, msg proto.Message) {
	var cards []protoreflect.Message
	collectCards(msg.ProtoReflect(), &cards)
	if len(cards) == 0 {
		return
	}
	ids := make([]string, 0, len(cards))
	seen := make(map[string]bool, len(cards))
	for _, c := range cards {
		id := c.Get(c.Descriptor().Fields().ByName(cardIDField)).String()
		if id != "" && !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return
	}
	streams, err := src.GetStreamsForItems(ctx, ids)
	if err != nil {
		slog.WarnContext(ctx, "media card streams", slog.Int("items", len(ids)), slog.Any("err", err))
		return
	}
	for _, c := range cards {
		fields := c.Descriptor().Fields()
		v := firstVideoStream(streams[c.Get(fields.ByName(cardIDField)).String()])
		if v == nil {
			continue
		}
		c.Set(fields.ByName(cardStreamField), protoreflect.ValueOfMessage(mediaStreamToProto(v, false).ProtoReflect()))
	}
}

func firstVideoStream(streams []*library.MediaStream) *library.MediaStream {
	for _, s := range streams {
		if s.StreamType == library.StreamTypeVideo {
			return s
		}
	}
	return nil
}

func isCardMessage(md protoreflect.MessageDescriptor) bool {
	id := md.Fields().ByName(cardIDField)
	vs := md.Fields().ByName(cardStreamField)
	return id != nil && id.Kind() == protoreflect.StringKind && !id.IsList() &&
		vs != nil && vs.Message() != nil && vs.Message().FullName() == mediaStreamFullName && !vs.IsList()
}

func collectCards(m protoreflect.Message, out *[]protoreflect.Message) {
	if !mayHoldCard(m.Descriptor()) {
		return
	}
	if isCardMessage(m.Descriptor()) {
		*out = append(*out, m)
		return
	}
	m.Range(func(fd protoreflect.FieldDescriptor, v protoreflect.Value) bool {
		if fd.Message() == nil {
			return true
		}
		switch {
		case fd.IsList():
			l := v.List()
			for i := range l.Len() {
				collectCards(l.Get(i).Message(), out)
			}
		case fd.IsMap():
			if fd.MapValue().Message() != nil {
				v.Map().Range(func(_ protoreflect.MapKey, mv protoreflect.Value) bool {
					collectCards(mv.Message(), out)
					return true
				})
			}
		default:
			collectCards(v.Message(), out)
		}
		return true
	})
}

var holdsCard sync.Map // protoreflect.FullName -> bool

func mayHoldCard(md protoreflect.MessageDescriptor) bool {
	if v, ok := holdsCard.Load(md.FullName()); ok {
		return v.(bool)
	}
	got := descriptorHoldsCard(md, map[protoreflect.FullName]bool{})
	holdsCard.Store(md.FullName(), got)
	return got
}

func descriptorHoldsCard(md protoreflect.MessageDescriptor, visiting map[protoreflect.FullName]bool) bool {
	if isCardMessage(md) {
		return true
	}
	if visiting[md.FullName()] {
		return false
	}
	visiting[md.FullName()] = true
	fields := md.Fields()
	for i := range fields.Len() {
		fd := fields.Get(i)
		sub := fd.Message()
		if fd.IsMap() {
			sub = fd.MapValue().Message()
		}
		if sub != nil && descriptorHoldsCard(sub, visiting) {
			return true
		}
	}
	return false
}
