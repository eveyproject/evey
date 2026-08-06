package ffmpeg

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The wrapped command must keep the binary and its args intact, in order,
// behind the demotion prefix - and when ionice exists, the DISK demotion must
// lead (it wraps nice, which wraps the binary).
func TestLowPriorityCommand_ArgvShape(t *testing.T) {
	t.Parallel()
	cmd := lowPriorityCommand(context.Background(), "ffprobe", "-v", "quiet", "file.mkv")
	argv := cmd.Args
	require.GreaterOrEqual(t, len(argv), 6)

	switch argv[0] {
	case "ionice":
		assert.Equal(t, []string{"ionice", "-c", "3", "nice", "-n", "19", "ffprobe", "-v", "quiet", "file.mkv"}, argv)
	case "nice":
		assert.Equal(t, []string{"nice", "-n", "19", "ffprobe", "-v", "quiet", "file.mkv"}, argv)
	default:
		t.Fatalf("unexpected argv[0] %q - the demotion prefix must lead", argv[0])
	}
}
