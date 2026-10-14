package mkvgo

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/gravity-zero/mkvgo/matroska"
)

// testdata/two_editions.mkv (mkvmerge v92, 10 s): a first edition A1/A2/A3 at
// 0/3/6 s, then the DEFAULT edition B1/B2 at 0/5 s.
//
// Until mkvgo 0.51 every edition's chapters came in one list: a file whose
// non-default edition names an "Opening" handed the intro detection an intro
// the edition that plays does not have. Only the default edition is stored.
func TestReadChapters_DefaultEditionOnly(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "editions.mkv")
	src, err := os.ReadFile("testdata/two_editions.mkv")
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(path, src, 0o644); err != nil {
		t.Fatal(err)
	}
	// Name the non-default edition's second chapter the way chapterPass reads
	// an intro.
	if err = matroska.EditInPlace(ctx, path, func(c *matroska.Container) {
		if len(c.OtherEditions) != 1 || len(c.OtherEditions[0].Chapters) != 3 {
			t.Fatalf("fixture editions: %+v", c.OtherEditions)
		}
		c.OtherEditions[0].Chapters[1].Title = "Opening"
	}); err != nil {
		t.Fatal(err)
	}

	chapters, err := New().ReadChapters(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	var titles []string
	for _, ch := range chapters {
		titles = append(titles, ch.Title)
	}
	if len(titles) != 2 || titles[0] != "B1" || titles[1] != "B2" {
		t.Fatalf("chapters = %v, want the default edition's [B1 B2] only", titles)
	}
}
