package ffmpeg

import (
	"context"
	"errors"
	"sync"

	"github.com/gravity-zero/evey/internal/domain/transcoding"
)

type Transcoder struct {
	binaryPath string
	jobs       sync.Map
}

func NewTranscoder(binaryPath string) *Transcoder {
	return &Transcoder{binaryPath: binaryPath}
}

func (t *Transcoder) Start(_ context.Context, job *transcoding.TranscodeJob) (string, error) {
	// TODO: build ffmpeg command args from job, exec.CommandContext, store PID in jobs map
	return "", errors.New("TODO: implement ffmpeg start")
}

func (t *Transcoder) Stop(_ context.Context, jobID string) error {
	// TODO: lookup PID in jobs map, send SIGTERM
	return errors.New("TODO: implement ffmpeg stop")
}

func (t *Transcoder) Progress(_ context.Context, jobID string) (*transcoding.TranscodeProgress, error) {
	// TODO: parse ffmpeg progress output
	return nil, errors.New("TODO: implement ffmpeg progress")
}
