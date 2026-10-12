package transcoding_test

import (
	"testing"

	"github.com/eveyproject/evey/internal/domain/transcoding"
)

func TestEventNames(t *testing.T) {
	t.Parallel()

	cases := []struct {
		event interface{ EventName() string }
		want  string
	}{
		{transcoding.TranscodeStarted{}, "transcode.started"},
		{transcoding.TranscodeFailed{}, "transcode.failed"},
		{transcoding.TranscodeCancelled{}, "transcode.cancelled"},
		{transcoding.DerivedCacheFull{}, "transcoding.derived_cache_full"},
	}

	for _, c := range cases {
		got := c.event.EventName()
		if got != c.want {
			t.Errorf("EventName() = %q, want %q", got, c.want)
		}
	}
}
