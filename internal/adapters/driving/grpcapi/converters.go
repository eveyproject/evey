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

// fromTimestamppb returns (decoded-time, true) when t is set, or
// (zero, false) when the proto field was nil. Callers distinguishing
// "absent" from "explicit zero time" should switch on the boolean
// instead of IsZero(), which aliases an unset proto field with the
// Unix epoch minus N years and quietly breaks future edge cases.
func fromTimestamppb(t *timestamppb.Timestamp) (time.Time, bool) {
	if t == nil {
		return time.Time{}, false
	}
	return t.AsTime(), true
}

func fromTimestamppbPtr(t *timestamppb.Timestamp) *time.Time {
	if t == nil {
		return nil
	}
	tt := t.AsTime()
	return &tt
}
