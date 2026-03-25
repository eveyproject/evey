package pluginrunner

import (
	"context"
	"os"
	"path/filepath"
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
		d := extractDateFromFilename(tt.name)
		if tt.want == "" {
			assert.True(t, d.IsZero(), "expected zero time for %s", tt.name)
		} else {
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
