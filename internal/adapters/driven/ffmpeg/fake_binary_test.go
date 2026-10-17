package ffmpeg

import (
	"os/exec"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// writeFakeBinary puts a shell script at path that the code under test will exec
// in place of ffmpeg or ffprobe, and returns that path.
//
// The script is written by a child sh, never by this process. A Go process that
// holds a file open for writing lends that descriptor to every child another
// goroutine forks in the meantime, and the descriptor only closes when that
// child reaches its exec. Until then the kernel refuses to exec the file
// ("text file busy", go.dev/issue/22315), and os/exec does not retry. This
// package runs dozens of parallel tests that each write a fake binary and run
// it at once, so the window was hit from time to time. With the write in a
// child that has exited before this returns, no descriptor on the script exists
// in this process, so none can be inherited.
func writeFakeBinary(t *testing.T, path, body string) string {
	t.Helper()
	cmd := exec.Command("sh", "-c", `cat > "$0" && chmod 755 "$0"`, path)
	cmd.Stdin = strings.NewReader(body)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, string(out))
	return path
}
