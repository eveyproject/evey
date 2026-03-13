package transcoding_test

import (
	"testing"

	"github.com/gravity-zero/evey/internal/domain/transcoding"
)

func TestEventNames(t *testing.T) {
	t.Parallel()

	cases := []struct {
		event interface{ EventName() string }
		want  string
	}{
		{transcoding.TranscodeStarted{}, "transcode.started"},
		{transcoding.TranscodeCompleted{}, "transcode.completed"},
		{transcoding.TranscodeFailed{}, "transcode.failed"},
		{transcoding.TranscodeCancelled{}, "transcode.cancelled"},
	}

	for _, c := range cases {
		got := c.event.EventName()
		if got != c.want {
			t.Errorf("EventName() = %q, want %q", got, c.want)
		}
	}
}
