package pluginrunner

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewPluginLoggerDefaults(t *testing.T) {
	l := NewPluginLogger("", 0)
	assert.Equal(t, defaultLogDir, l.logDir)
	assert.Equal(t, 10, l.maxAgeDays)
}

func TestNewPluginLoggerCustom(t *testing.T) {
	l := NewPluginLogger("/tmp/test-logs", 7)
	assert.Equal(t, "/tmp/test-logs", l.logDir)
	assert.Equal(t, 7, l.maxAgeDays)
}

func TestWriteLogCreatesDateFile(t *testing.T) {
	dir := t.TempDir()
	l := NewPluginLogger(dir, 10)
	defer l.Close()

	l.WriteLog("plg-test", "hello world")

	today := time.Now().Format("2006-01-02")
	logPath := filepath.Join(dir, "plg-test-"+today+".log")
	data, err := os.ReadFile(logPath)
	require.NoError(t, err)
	assert.Contains(t, string(data), "hello world")
}

func TestWriterCreatesDateFile(t *testing.T) {
	dir := t.TempDir()
	l := NewPluginLogger(dir, 10)
	defer l.Close()

	w := l.Writer("plg-1")
	_, err := w.Write([]byte("test output\n"))
	require.NoError(t, err)

	today := time.Now().Format("2006-01-02")
	logPath := filepath.Join(dir, "plg-1-"+today+".log")
	data, readErr := os.ReadFile(logPath)
	require.NoError(t, readErr)
	assert.Contains(t, string(data), "test output")
}

func TestClosePluginRemovesWriter(t *testing.T) {
	dir := t.TempDir()
	l := NewPluginLogger(dir, 10)

	l.WriteLog("plg-close", "before")
	l.ClosePlugin("plg-close")

	l.mu.Lock()
	_, exists := l.writers["plg-close"]
	l.mu.Unlock()
	assert.False(t, exists)
}

func TestSetRetentionDays(t *testing.T) {
	l := NewPluginLogger(t.TempDir(), 10)
	l.SetRetentionDays(7)
	assert.Equal(t, 7, l.maxAgeDays)
}

func TestSetRetentionDaysZeroDefaultsTo10(t *testing.T) {
	l := NewPluginLogger(t.TempDir(), 5)
	l.SetRetentionDays(0)
	assert.Equal(t, 10, l.maxAgeDays)
}

func TestExtractDateFromFilename(t *testing.T) {
	tests := []struct {
		name string
		want string
	}{
		{"plg-abc123-2026-03-18.log", "2026-03-18"},
		{"plg-abc123-2026-03-18-14.log", "2026-03-18"},
		{"invalid.log", ""},
	}
	for _, tt := range tests {
		d, ok := extractDateFromFilename(tt.name)
		if tt.want == "" {
			assert.False(t, ok, "expected ok=false for %s", tt.name)
		} else {
			assert.True(t, ok, "expected ok=true for %s", tt.name)
			assert.Equal(t, tt.want, d.Format("2006-01-02"))
		}
	}
}

func TestCleanOldLogs(t *testing.T) {
	dir := t.TempDir()
	l := NewPluginLogger(dir, 5)
	defer l.Close()

	// Create an "old" log file (20 days ago)
	old := time.Now().AddDate(0, 0, -20).Format("2006-01-02")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "plg-old-"+old+".log"), []byte("old"), 0o644))

	// Create a "recent" log file (today)
	today := time.Now().Format("2006-01-02")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "plg-new-"+today+".log"), []byte("new"), 0o644))

	l.cleanOldLogs()

	_, err := os.Stat(filepath.Join(dir, "plg-old-"+old+".log"))
	assert.True(t, os.IsNotExist(err), "old log should be deleted")

	_, err = os.Stat(filepath.Join(dir, "plg-new-"+today+".log"))
	assert.NoError(t, err, "recent log should still exist")
}

type fakeLister struct {
	ids []string
	err error
}

func (f *fakeLister) ListIDs(_ context.Context) ([]string, error) {
	return f.ids, f.err
}

func TestCleanOrphanLogs_ListerError(t *testing.T) {
	dir := t.TempDir()
	l := NewPluginLogger(dir, 10)
	defer l.Close()

	today := time.Now().Format("2006-01-02")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "plg-x-"+today+".log"), []byte("data"), 0o644))

	lister := &fakeLister{err: assert.AnError}
	l.cleanOrphanLogs(context.Background(), lister)

	// file should NOT be removed when lister errors
	_, err := os.Stat(filepath.Join(dir, "plg-x-"+today+".log"))
	assert.NoError(t, err)
}

func TestCloseAll(t *testing.T) {
	dir := t.TempDir()
	l := NewPluginLogger(dir, 10)
	l.WriteLog("plg-a", "hello")
	l.WriteLog("plg-b", "world")

	l.Close()

	l.mu.Lock()
	count := len(l.writers)
	l.mu.Unlock()
	assert.Equal(t, 0, count)
}

func TestRotateOnSize(t *testing.T) {
	dir := t.TempDir()
	l := NewPluginLogger(dir, 10)
	defer l.Close()

	w := l.Writer("plg-rotate")
	plw := w.(*pluginLogWriter)
	plw.maxSize = 100 // very small max size

	data := make([]byte, 60)
	for i := range data {
		data[i] = 'a'
	}
	data[59] = '\n'

	_, err := w.Write(data)
	require.NoError(t, err)
	_, err = w.Write(data)
	require.NoError(t, err)

	entries, _ := os.ReadDir(dir)
	logCount := 0
	for _, e := range entries {
		if !e.IsDir() {
			logCount++
		}
	}
	assert.GreaterOrEqual(t, logCount, 1)
}

func TestCleanOldLogs_NoLogDir(t *testing.T) {
	l := NewPluginLogger("/nonexistent/path/that/does/not/exist", 5)
	l.cleanOldLogs() // should not panic
}

func TestCleanOldLogs_SkipsNonLogFiles(t *testing.T) {
	dir := t.TempDir()
	l := NewPluginLogger(dir, 5)
	defer l.Close()

	require.NoError(t, os.WriteFile(filepath.Join(dir, "readme.txt"), []byte("x"), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "subdir"), 0o755))

	l.cleanOldLogs()

	_, err := os.Stat(filepath.Join(dir, "readme.txt"))
	assert.NoError(t, err, "non-log files should not be removed")
}

func TestCleanOrphanLogs(t *testing.T) {
	dir := t.TempDir()
	l := NewPluginLogger(dir, 10)
	defer l.Close()

	today := time.Now().Format("2006-01-02")
	for _, id := range []string{"plg-a", "plg-b", "plg-orphan"} {
		require.NoError(t, os.WriteFile(filepath.Join(dir, id+"-"+today+".log"), []byte("data"), 0o644))
	}

	lister := &fakeLister{ids: []string{"plg-a", "plg-b"}}
	l.cleanOrphanLogs(context.Background(), lister)

	_, err := os.Stat(filepath.Join(dir, "plg-a-"+today+".log"))
	require.NoError(t, err)
	_, err = os.Stat(filepath.Join(dir, "plg-b-"+today+".log"))
	require.NoError(t, err)
	_, err = os.Stat(filepath.Join(dir, "plg-orphan-"+today+".log"))
	assert.True(t, os.IsNotExist(err))
}

// TestWriterDailyByteCap: a plugin spamming stdout fills at most its daily
// budget on disk, across rotations and a server restart (a fresh writer counts
// the day's existing files), and the pipe keeps draining past the cap.
func TestWriterDailyByteCap(t *testing.T) {
	dir := t.TempDir()
	newWriter := func() *pluginLogWriter {
		l := NewPluginLogger(dir, 10)
		t.Cleanup(l.Close)
		plw := l.Writer("plg-spam").(*pluginLogWriter)
		plw.maxSize = 100
		plw.maxDayBytes = 1000
		return plw
	}
	line := []byte(strings.Repeat("a", 59) + "\n")
	w := newWriter()
	for range 100 {
		n, err := w.Write(line)
		require.NoError(t, err, "a capped write must not break the plugin's stdout pipe")
		require.Equal(t, len(line), n)
	}
	w2 := newWriter()
	for range 100 {
		_, err := w2.Write(line)
		require.NoError(t, err)
	}

	var total int64
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	for _, e := range entries {
		info, err := e.Info()
		require.NoError(t, err)
		total += info.Size()
	}
	assert.LessOrEqual(t, total, int64(1000+len(logCapNotice)), "disk use must stay within the daily budget")
	assert.Greater(t, total, int64(900))
}
