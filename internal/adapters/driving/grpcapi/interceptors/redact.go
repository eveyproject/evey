package interceptors

import (
	"context"

	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// RedactInterceptor blanks named string fields in every response message,
// wherever they sit: top level, nested, repeated, or inside a map value. It is
// generic - a list of proto field NAMES, not a list of RPCs - so a new response
// carrying a `client_ip` is covered the day it is added, and no handler learns
// about the instance it runs on.
//
// The demo composes it with demo.RedactedFields: a visitor there is an
// administrator and would otherwise read the addresses and user agents of
// every other visitor on the sessions page, the presence list and the shared
// account's login history.
type RedactInterceptor struct {
	names map[string]bool
}

func NewRedactInterceptor(fieldNames ...string) *RedactInterceptor {
	names := make(map[string]bool, len(fieldNames))
	for _, n := range fieldNames {
		names[n] = true
	}
	return &RedactInterceptor{names: names}
}

func (r *RedactInterceptor) Unary() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		resp, err := handler(ctx, req)
		if msg, ok := resp.(proto.Message); ok && msg != nil {
			r.redact(msg.ProtoReflect())
		}
		return resp, err
	}
}

func (r *RedactInterceptor) Stream() grpc.StreamServerInterceptor {
	return func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		return handler(srv, &redactingStream{ServerStream: ss, r: r})
	}
}

type redactingStream struct {
	grpc.ServerStream
	r *RedactInterceptor
}

func (s *redactingStream) SendMsg(m any) error {
	if msg, ok := m.(proto.Message); ok && msg != nil {
		s.r.redact(msg.ProtoReflect())
	}
	return s.ServerStream.SendMsg(m)
}

// redact walks the message. Range permits mutating the field it is currently
// visiting, which is all Clear needs.
func (r *RedactInterceptor) redact(m protoreflect.Message) {
	if !m.IsValid() {
		return
	}
	m.Range(func(fd protoreflect.FieldDescriptor, v protoreflect.Value) bool {
		switch {
		case fd.IsMap():
			if fd.MapValue().Kind() == protoreflect.MessageKind {
				v.Map().Range(func(_ protoreflect.MapKey, mv protoreflect.Value) bool {
					r.redact(mv.Message())
					return true
				})
			}
		case fd.IsList():
			if fd.Kind() == protoreflect.MessageKind {
				list := v.List()
				for i := range list.Len() {
					r.redact(list.Get(i).Message())
				}
			}
		case fd.Kind() == protoreflect.MessageKind:
			r.redact(v.Message())
		case fd.Kind() == protoreflect.StringKind && r.names[string(fd.Name())]:
			m.Clear(fd)
		}
		return true
	})
}
