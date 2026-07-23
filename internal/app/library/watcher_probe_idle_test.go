package library

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"
)

func probeWS(buf *bytes.Buffer, failed map[string]bool) *WatcherService {
	return &WatcherService{
		log:        slog.New(slog.NewTextHandler(buf, nil)),
		probes:     newProbeState(),
		pathToLib:  map[string]string{},
		failedLibs: failed,
	}
}

// Prod 2026-07-23: all five libraries had realtime monitoring disabled BY
// CHOICE, and the probe still warned every 5 minutes - 247 of the day's 254
// WARNs, burying the real signals. A config the operator chose must be said
// once (INFO), not warned about forever.
func TestProbeRound_NoLibraryWantsWatching_SaysItOnceNotAsWarn(t *testing.T) {
	var buf bytes.Buffer
	ws := probeWS(&buf, map[string]bool{})

	for range 3 {
		ws.probeRound(context.Background())
	}

	out := buf.String()
	if strings.Contains(out, "level=WARN") {
		t.Fatalf("monitoring disabled everywhere is a config choice, not a fault - no WARN expected, got:\n%s", out)
	}
	if got := strings.Count(out, "periodic scan is the only discovery path"); got != 1 {
		t.Fatalf("the notice must be logged exactly once across rounds, got %d:\n%s", got, out)
	}
}

// The opposite case stays loud: at least one library ASKED for realtime
// monitoring and every watch failed - that is a fault, not a choice, and it
// must keep warning so a dead watcher can't fade into silence.
func TestProbeRound_WatchingRequestedButAllDead_KeepsWarningEveryRound(t *testing.T) {
	var buf bytes.Buffer
	ws := probeWS(&buf, map[string]bool{"lib-1": true})

	for range 2 {
		ws.probeRound(context.Background())
	}

	if got := strings.Count(buf.String(), "level=WARN"); got != 2 {
		t.Fatalf("watching requested but dead must warn every round, got %d WARNs:\n%s", got, buf.String())
	}
}

// A successful watch re-arms the one-shot notice: if monitoring is later
// disabled again, the operator gets one fresh notice instead of silence
// inherited from the previous episode.
func TestProbeRound_NoticeReArmsAfterAWatchSucceeds(t *testing.T) {
	var buf bytes.Buffer
	ws := probeWS(&buf, map[string]bool{})

	ws.probeRound(context.Background()) // notice #1
	ws.mu.Lock()
	ws.noWatchLogged = false // what watchLibrary does on a successful watch
	ws.mu.Unlock()
	ws.probeRound(context.Background()) // notice #2

	if got := strings.Count(buf.String(), "periodic scan is the only discovery path"); got != 2 {
		t.Fatalf("notice should re-arm after a successful watch, got %d:\n%s", got, buf.String())
	}
}
