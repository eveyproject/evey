package grpcapi

import (
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"
)

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
