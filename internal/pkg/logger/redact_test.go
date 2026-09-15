package logger_test

import (
	"errors"
	"log/slog"
	"os"
	"strings"
	"testing"

	"github.com/evey-project/evey/internal/pkg/logger"
)

// A credential that reaches a log call must not reach the file: the file is
// what the admin log routes serve, what gets pasted in bug reports, what a
// backup carries. Scrubbed at the write, not only at the read.
func TestNew_RedactsCredentialsAtWrite(t *testing.T) {
	dir := t.TempDir()
	l := logger.New(logger.Config{Dir: dir, Level: "info"})
	l.Info("proxy dump",
		slog.String("authorization", "Bearer abc.def"),
		slog.String("token", "s3cr3t"),
		slog.String("password", "hunter2"),
		slog.String("url", "/hls/master.m3u8?token=q-secret&w=1"),
		slog.String("user", "bob"),
		slog.Int("token_count", 3),
	)
	l.Info("stream url http://h/x?jwt=eyJhbGciOiJSUzI1NiJ9.eyJzdWIiOiJ1MSJ9.c2lnbmF0dXJl failed")
	l.Info("login", slog.Group("req", slog.String("pin", "1234"), slog.String("profile", "kid")))
	l.Warn("download failed", slog.Any("err", errors.New("GET http://h/plugin.zip?token=err-secret: 404")))

	data, err := os.ReadFile(todayLogFile(dir))
	if err != nil {
		t.Fatal(err)
	}
	out := string(data)
	for _, leaked := range []string{"abc.def", "s3cr3t", "hunter2", "q-secret", "eyJhbGciOiJSUzI1NiJ9", `"pin":"1234"`, "err-secret"} {
		if strings.Contains(out, leaked) {
			t.Errorf("log file leaked %q:\n%s", leaked, out)
		}
	}
	for _, kept := range []string{`"user":"bob"`, `"token_count":3`, `"profile":"kid"`, "&w=1", "proxy dump", "failed", "plugin.zip", ": 404"} {
		if !strings.Contains(out, kept) {
			t.Errorf("log file lost %q:\n%s", kept, out)
		}
	}
}
