package library

import "testing"

// The watcher's liveness probe files must be recognised by name wherever they
// land, and nothing else may be mistaken for one.
func TestIsProbeFile(t *testing.T) {
	if !IsProbeFile("/media/films/" + ProbeFilePrefix + "abc123") {
		t.Error("a probe file in a library root")
	}
	if !IsProbeFile(ProbeFilePrefix) {
		t.Error("the bare prefix")
	}
	if IsProbeFile("/media/films/film.mkv") || IsProbeFile("/media/"+ProbeFilePrefix+"dir/film.mkv") {
		t.Error("only the base name decides")
	}
}
