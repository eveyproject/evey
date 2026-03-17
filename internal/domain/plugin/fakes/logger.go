package fakes

import "sync"

type LogEntry struct {
	PluginID string
	Level    string
	Msg      string
	Fields   map[string]string
}

type FakePluginLogger struct {
	mu      sync.Mutex
	Entries []LogEntry
}

func NewFakePluginLogger() *FakePluginLogger {
	return &FakePluginLogger{}
}

func (l *FakePluginLogger) WriteLog(pluginID, level, msg string, fields map[string]string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.Entries = append(l.Entries, LogEntry{
		PluginID: pluginID,
		Level:    level,
		Msg:      msg,
		Fields:   fields,
	})
}
