package mkvgo

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	extmkv "github.com/gravity-zero/mkvgo/mkv"

	"github.com/eveyproject/evey/internal/domain/mkv"
)

// TestApplyMetadataEdit_Title verifies that a Title mutation updates
// SegmentInfo.Title and leaves tracks untouched.
func TestApplyMetadataEdit_Title(t *testing.T) {
	c := &extmkv.Container{
		Info:   extmkv.SegmentInfo{Title: "Old Title"},
		Tracks: []extmkv.Track{{ID: 1, Language: "fre"}},
	}
	title := "New Title"
	applyMetadataEdit(mkv.EditMetadataRequest{Title: &title})(c)

	if c.Info.Title != "New Title" {
		t.Errorf("title = %q, want %q", c.Info.Title, "New Title")
	}
	if c.Tracks[0].Language != "fre" {
		t.Errorf("track language unexpectedly changed to %q", c.Tracks[0].Language)
	}
}

// TestApplyMetadataEdit_NilTitle verifies that a nil Title leaves SegmentInfo
// untouched.
func TestApplyMetadataEdit_NilTitle(t *testing.T) {
	c := &extmkv.Container{Info: extmkv.SegmentInfo{Title: "Keep"}}
	applyMetadataEdit(mkv.EditMetadataRequest{})(c)
	if c.Info.Title != "Keep" {
		t.Errorf("title = %q, want %q", c.Info.Title, "Keep")
	}
}

// TestApplyMetadataEdit_TrackFields verifies that language, name, and flag
// mutations are applied only to the targeted track.
func TestApplyMetadataEdit_TrackFields(t *testing.T) {
	c := &extmkv.Container{
		Tracks: []extmkv.Track{
			{ID: 1, Language: "und", Name: "", IsDefault: false, IsForced: false},
			{ID: 2, Language: "eng", Name: "English", IsDefault: true, IsForced: false},
		},
	}

	lang := "fre"
	name := "French Audio"
	isDefault := true
	isForced := false

	req := mkv.EditMetadataRequest{
		TrackEdits: []mkv.TrackEdit{
			{TrackID: 1, Language: &lang, Name: &name, IsDefault: &isDefault, IsForced: &isForced},
		},
	}
	applyMetadataEdit(req)(c)

	t1 := c.Tracks[0]
	if t1.Language != "fre" {
		t.Errorf("track 1 language = %q, want fre", t1.Language)
	}
	if t1.Name != "French Audio" {
		t.Errorf("track 1 name = %q, want French Audio", t1.Name)
	}
	if !t1.IsDefault {
		t.Errorf("track 1 IsDefault should be true")
	}
	if t1.IsForced {
		t.Errorf("track 1 IsForced should be false")
	}

	// Track 2 must be untouched.
	t2 := c.Tracks[1]
	if t2.Language != "eng" || t2.Name != "English" || !t2.IsDefault {
		t.Errorf("track 2 unexpectedly modified: %+v", t2)
	}
}

// TestApplyMetadataEdit_PartialTrackEdit ensures nil pointer fields in a
// TrackEdit leave the corresponding attribute unchanged.
func TestApplyMetadataEdit_PartialTrackEdit(t *testing.T) {
	c := &extmkv.Container{
		Tracks: []extmkv.Track{
			{ID: 3, Language: "jpn", Name: "Japanese", IsDefault: false, IsForced: true},
		},
	}

	name := "Japanese Dub"
	req := mkv.EditMetadataRequest{
		TrackEdits: []mkv.TrackEdit{{TrackID: 3, Name: &name}},
	}
	applyMetadataEdit(req)(c)

	t3 := c.Tracks[0]
	if t3.Name != "Japanese Dub" {
		t.Errorf("name = %q, want Japanese Dub", t3.Name)
	}
	// Language, IsDefault, IsForced must be untouched.
	if t3.Language != "jpn" {
		t.Errorf("language unexpectedly changed to %q", t3.Language)
	}
	if t3.IsDefault {
		t.Errorf("IsDefault unexpectedly changed to true")
	}
	if !t3.IsForced {
		t.Errorf("IsForced unexpectedly changed to false")
	}
}

// TestEditMetadata_NonEBML verifies that EditMetadata wraps mkv.ErrNotEBML for
// non-EBML files without touching the filesystem.
func TestEditMetadata_NonEBML(t *testing.T) {
	a := New()
	dir := t.TempDir()
	src := filepath.Join(dir, "video.mp4")
	dst := filepath.Join(dir, "out.mp4")
	if err := os.WriteFile(src, []byte("not an mkv"), 0o644); err != nil {
		t.Fatal(err)
	}

	err := a.EditMetadata(context.Background(), src, dst, mkv.EditMetadataRequest{})
	if !errors.Is(err, mkv.ErrNotEBML) {
		t.Errorf("EditMetadata on mp4: want ErrNotEBML, got %v", err)
	}
	if _, statErr := os.Stat(dst); !os.IsNotExist(statErr) {
		t.Errorf("dst file must not be created for non-EBML input")
	}
}

// TestEditInPlace_NonEBML verifies that EditInPlace wraps mkv.ErrNotEBML for
// non-EBML files.
func TestEditInPlace_NonEBML(t *testing.T) {
	a := New()
	dir := t.TempDir()
	path := filepath.Join(dir, "audio.mp3")
	if err := os.WriteFile(path, []byte("not ebml"), 0o644); err != nil {
		t.Fatal(err)
	}

	err := a.EditInPlace(context.Background(), path, mkv.EditMetadataRequest{})
	if !errors.Is(err, mkv.ErrNotEBML) {
		t.Errorf("EditInPlace on mp3: want ErrNotEBML, got %v", err)
	}
}
