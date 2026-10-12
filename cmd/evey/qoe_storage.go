package main

import (
	"context"

	"github.com/eveyproject/evey/internal/adapters/driven/postgres"
	appqoe "github.com/eveyproject/evey/internal/app/qoe"
)

// qoeStorageAdapter bridges the app-layer qoe.Storage interface to the
// postgres repo. Kept in the wiring package so the app package doesn't
// import the postgres adapter (clean hexagonal split).
type qoeStorageAdapter struct {
	repo *postgres.PlaybackQoERepo
}

func (a qoeStorageAdapter) InsertEvent(ctx context.Context, evt appqoe.Event) error {
	return a.repo.Insert(ctx, postgres.QoEEventRow{
		UserID:           nilIfEmpty(evt.UserID),
		MediaItemID:      nilIfEmpty(evt.MediaItemID),
		SessionID:        evt.Payload.SessionID,
		ContentID:        evt.Payload.ContentID,
		VariantIdx:       new(evt.VariantIdx),
		SegmentIdx:       intPtrIf(evt.SegmentIdx, !evt.IsInit && !evt.IsManifest),
		ObjectType:       evt.Payload.ObjectType,
		ObjectDurationMs: intPtrIfNonZero(evt.Payload.ObjectDuration),
		BufferLengthMs:   intPtrIfNonZero(evt.Payload.BufferLengthMs),
		EncodedKbps:      intPtrIfNonZero(evt.Payload.EncodedKbps),
		TopBitrateKbps:   intPtrIfNonZero(evt.Payload.TopBitrateKbps),
		MeasuredKbps:     intPtrIfNonZero(evt.Payload.MeasuredKbps),
		RequestedMaxKbps: intPtrIfNonZero(evt.Payload.RequestedMaxKbps),
		DeadlineMs:       intPtrIfNonZero(evt.Payload.DeadlineMs),
		PlaybackRate:     floatPtrIfNonZero(evt.Payload.PlaybackRate),
		StreamingFormat:  evt.Payload.StreamingFormat,
		StreamType:       evt.Payload.StreamType,
		CMCDVersion:      intPtrIfNonZero(evt.Payload.Version),
		BufferStarved:    evt.Payload.BufferStarved,
		Startup:          evt.Payload.Startup,
		Raw:              evt.Payload.Raw,
		DeviceClass:      evt.DeviceClass,
		RemoteIP:         evt.RemoteIP,
	})
}

func nilIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func intPtrIf(v int, cond bool) *int {
	if !cond {
		return nil
	}
	return &v
}

func intPtrIfNonZero(v int) *int {
	if v == 0 {
		return nil
	}
	return &v
}

func floatPtrIfNonZero(v float64) *float64 {
	if v == 0 {
		return nil
	}
	return &v
}
