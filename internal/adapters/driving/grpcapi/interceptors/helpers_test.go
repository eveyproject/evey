package interceptors_test

import (
	"context"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

type fakeServerStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (f *fakeServerStream) Context() context.Context { return f.ctx }
func (f *fakeServerStream) SetHeader(metadata.MD) error  { return nil }
func (f *fakeServerStream) SendHeader(metadata.MD) error  { return nil }
func (f *fakeServerStream) SetTrailer(metadata.MD)        {}
func (f *fakeServerStream) SendMsg(any) error              { return nil }
func (f *fakeServerStream) RecvMsg(any) error              { return nil }
