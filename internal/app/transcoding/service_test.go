package transcoding_test

import (
	"context"
	"testing"

	apptranscode "github.com/gravity-zero/evey/internal/app/transcoding"
	"github.com/gravity-zero/evey/internal/domain/transcoding"
	tcfakes "github.com/gravity-zero/evey/internal/domain/transcoding/fakes"
	sharedfakes "github.com/gravity-zero/evey/internal/domain/shared/fakes"
	"github.com/stretchr/testify/assert"
)

func TestStartTranscode(t *testing.T) {
	tc := tcfakes.NewFakeTranscoder()
	hw := tcfakes.NewFakeHWDetector()
	jobRepo := tcfakes.NewFakeTranscodeJobRepository()
	profileRepo := tcfakes.NewFakeTranscodeProfileRepository()
	bus := sharedfakes.NewFakeEventBus()

	svc := apptranscode.NewService(tc, hw, jobRepo, profileRepo, bus, 2)

	job := &transcoding.TranscodeJob{
		ID:          "j1",
		MediaItemID: "m1",
		InputCodec:  "h264",
		OutputCodec: "h265",
	}

	err := svc.StartTranscode(context.Background(), job)
	assert.NoError(t, err)
	assert.Equal(t, string(transcoding.Running), job.Status)
	assert.Len(t, bus.PublishedEvents(), 1)
}
