package mkvgo

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	extmatroska "github.com/gravity-zero/mkvgo/matroska"

	"github.com/eveyproject/evey/internal/domain/mkv"
)

// The operator reads a repair's refusal verbatim in the media-health panel, so
// it must say the thing once. mkvgo's own message already ends with "the source
// is structurally corrupt"; carrying our domain sentinel with a second %w - or
// with errors.Join, which renders both - put that sentence on screen twice
// (reported from production, 2026-09-07). Internal rather than in the external
// probe file so the wrapping stays unexported: a test-only export is API.
func TestCorruptSourceWrap_SaysTheSameThingOnce(t *testing.T) {
	src := fmt.Errorf("reindex: top-level element: invalid VINT: leading zero byte (a corrupted region can be skipped with Options.Resync / --resync): %w",
		extmatroska.ErrCorruptSource)
	got := fmt.Errorf("reindex: rebuild: %w", corruptSource{src})

	if n := strings.Count(got.Error(), "the source is structurally corrupt"); n != 1 {
		t.Errorf("the operator reads the sentence %d times:\n%v", n, got)
	}
	if !errors.Is(got, mkv.ErrCorruptSource) {
		t.Error("domain sentinel lost - the batch would stop routing this file to a resync")
	}
	if !errors.Is(got, extmatroska.ErrCorruptSource) {
		t.Error("library sentinel lost")
	}
}
