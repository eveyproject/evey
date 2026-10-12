package transcoding_test

import (
	"context"
	"sync"
	"testing"

	apptranscode "github.com/eveyproject/evey/internal/app/transcoding"
	sharedfakes "github.com/eveyproject/evey/internal/domain/shared/fakes"
	"github.com/eveyproject/evey/internal/domain/transcoding"
	tcfakes "github.com/eveyproject/evey/internal/domain/transcoding/fakes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newMiscSvc() *apptranscode.Service {
	tc := tcfakes.NewFakeTranscoder()
	hw := tcfakes.NewFakeHWDetector()
	jobRepo := tcfakes.NewFakeTranscodeJobRepository()
	profileRepo := tcfakes.NewFakeTranscodeProfileRepository()
	bus := sharedfakes.NewFakeEventBus()
	return apptranscode.NewService(tc, hw, jobRepo, profileRepo, bus, 2)
}

// ListActiveTranscodes returns the repo's active set.
func TestService_ListActiveTranscodes(t *testing.T) {
	t.Parallel()
	svc := newMiscSvc()
	jobs, err := svc.ListActiveTranscodes(context.Background())
	require.NoError(t, err)
	assert.Empty(t, jobs, "initially no active jobs")
}

// SetMaxConcurrent updates the field; no direct observable from outside, so
// we assert the call does not panic (covers the function).
func TestService_SetMaxConcurrent_NoPanic(t *testing.T) {
	t.Parallel()
	svc := newMiscSvc()
	svc.SetMaxConcurrent(10)
	svc.SetMaxConcurrent(1)
}

// DetectHardware delegates to the HW detector fake.
func TestService_DetectHardware_DelegatesToDetector(t *testing.T) {
	t.Parallel()
	tc := tcfakes.NewFakeTranscoder()
	hw := tcfakes.NewFakeHWDetector()
	hw.Devices = []transcoding.HWDevice{{Type: "nvidia", Name: "RTX 3080"}}
	jobRepo := tcfakes.NewFakeTranscodeJobRepository()
	profileRepo := tcfakes.NewFakeTranscodeProfileRepository()
	bus := sharedfakes.NewFakeEventBus()
	svc := apptranscode.NewService(tc, hw, jobRepo, profileRepo, bus, 2)

	devices, err := svc.DetectHardware(context.Background())
	require.NoError(t, err)
	assert.Len(t, devices, 1)
	assert.Equal(t, "nvidia", devices[0].Type)
}

// SetMaxConcurrent is called from the admin PUT handler while StartTranscode
// reads the same field on request goroutines - an unsynchronised int shared
// across goroutines. Run with -race: this is a genuine data race, not a
// theoretical one (the admin panel really does flip it under live playback).
func TestService_SetMaxConcurrent_NoRaceWithReaders(t *testing.T) {
	svc := apptranscode.NewService(nil, nil, nil, nil, nil, 2)

	var wg sync.WaitGroup
	for i := range 8 {
		wg.Go(func() {
			svc.SetMaxConcurrent(i%4 + 1)
		})
		wg.Go(func() {
			_ = svc.MaxConcurrent() // the read StartTranscode performs
		})
	}
	wg.Wait()
}
