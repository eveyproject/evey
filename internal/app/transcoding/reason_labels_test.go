package transcoding

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// Every transcode reason the server can emit has a label in Stream info.
//
// The four players print player.streamInfo.reason.<code> and fall back to the
// raw code with spaces, in English whatever the locale: six reasons shipped
// that way before this guard. The reasons are string literals scattered
// across the resolver, so the guard reads this package's sources rather than
// a list someone would have to remember to update. English is enough here:
// internal/pkg/i18n TestLocaleCatalogues_HaveIdenticalKeySets carries every
// key to the other locales.
func TestEveryTranscodeReasonHasAStreamInfoLabel(t *testing.T) {
	t.Parallel()
	_, thisFile, _, _ := runtime.Caller(0)
	dir := filepath.Dir(thisFile)

	emitted := map[string]string{}
	pattern := regexp.MustCompile(`TranscodeReasons, "([a-z0-9_]+)"|addReasonOnce\(d, "([a-z0-9_]+)"|reason[A-Z]\w* += "([a-z0-9_]+)"`)
	files, err := filepath.Glob(filepath.Join(dir, "*.go"))
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		src, readErr := os.ReadFile(f)
		if readErr != nil {
			t.Fatal(readErr)
		}
		for _, m := range pattern.FindAllStringSubmatch(string(src), -1) {
			emitted[m[1]+m[2]+m[3]] = filepath.Base(f)
		}
	}
	// A pattern that stops matching would pass vacuously.
	if len(emitted) < 20 {
		t.Fatalf("found only %d reasons in the sources, the scan pattern no longer matches how reasons are written", len(emitted))
	}

	raw, err := os.ReadFile(filepath.Join(dir, "..", "..", "..", "i18n", "en", "player.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var catalogue struct {
		StreamInfo struct {
			Reason map[string]string `yaml:"reason"`
		} `yaml:"streamInfo"`
	}
	if err := yaml.Unmarshal(raw, &catalogue); err != nil {
		t.Fatal(err)
	}

	var missing []string
	for reason, file := range emitted {
		if strings.TrimSpace(catalogue.StreamInfo.Reason[reason]) == "" {
			missing = append(missing, reason+" ("+file+")")
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		t.Errorf("reasons with no player.streamInfo.reason label in i18n/en/player.yaml: %s", strings.Join(missing, ", "))
	}
}
