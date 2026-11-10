package mkvgo

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gravity-zero/mkvgo/mkv/reader"

	"github.com/eveyproject/evey/internal/domain/mkv"
)

// The last of two metadata edits is the one a read sees. A first edit too large
// for the padding moves Info to the end of the Segment; an earlier mkvgo then
// appended the larger second edit's Info behind that copy instead of replacing
// it, and a full read kept returning the first edit.
func TestEditInPlace_TwiceKeepsTheLastEdit(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "twice.mkv")
	src, err := os.ReadFile("testdata/two_editions.mkv")
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(path, src, 0o644); err != nil {
		t.Fatal(err)
	}

	a := New()
	first, second := "A"+strings.Repeat("a", 9000), "B"+strings.Repeat("b", 18000)
	if err = a.EditInPlace(ctx, path, mkv.EditMetadataRequest{Title: &first}); err != nil {
		t.Fatal(err)
	}
	if err = a.EditInPlace(ctx, path, mkv.EditMetadataRequest{Title: &second}); err != nil {
		t.Fatal(err)
	}

	full, err := reader.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	meta, err := reader.OpenMeta(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	if full.Info.Title != second || meta.Info.Title != second {
		t.Fatalf("title after two edits: full read %.1q..., head read %.1q..., want B...", full.Info.Title, meta.Info.Title)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if n := bytes.Count(data, []byte(first)); n != 0 {
		t.Fatalf("the first edit's title is still in the file %d time(s)", n)
	}
}
