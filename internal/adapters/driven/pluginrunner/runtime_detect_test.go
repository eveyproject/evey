package pluginrunner

import (
	"testing"
)

func TestDetectRuntimes_FindsGo(t *testing.T) {
	runtimes := DetectRuntimes()
	found := false
	for _, rt := range runtimes {
		if rt.Language == "go" {
			found = true
			if rt.Version == "" {
				t.Error("expected non-empty version for go")
			}
		}
	}
	if !found {
		t.Error("expected go runtime to be detected")
	}
}

func TestIsRuntimeAvailable_Go(t *testing.T) {
	if !IsRuntimeAvailable("go") {
		t.Error("expected go to be available")
	}
}

func TestIsRuntimeAvailable_CompiledLanguages(t *testing.T) {
	for _, lang := range []string{"rust", "c", "c++"} {
		if !IsRuntimeAvailable(lang) {
			t.Errorf("expected %s to be available (compiled, no runtime needed)", lang)
		}
	}
}

func TestIsRuntimeAvailable_Unknown(t *testing.T) {
	if IsRuntimeAvailable("brainfuck") {
		t.Error("expected brainfuck to be unavailable")
	}
}
