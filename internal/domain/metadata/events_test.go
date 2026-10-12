package metadata_test

import (
	"testing"

	"github.com/eveyproject/evey/internal/domain/metadata"
)

func TestEventNames(t *testing.T) {
	t.Parallel()

	cases := []struct {
		event interface{ EventName() string }
		want  string
	}{
		{metadata.MetadataRefreshed{}, "metadata.refreshed"},
		{metadata.MetadataOverrideSet{}, "metadata.override.set"},
		{metadata.MetadataOverrideRemoved{}, "metadata.override.removed"},
		{metadata.ArtworkDownloaded{}, "metadata.artwork.downloaded"},
	}

	for _, c := range cases {
		got := c.event.EventName()
		if got != c.want {
			t.Errorf("EventName() = %q, want %q", got, c.want)
		}
	}
}
