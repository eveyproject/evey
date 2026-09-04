package httpapi

import "testing"

// The two pure decisions in the upload path, kept honest here because both are
// driven by client-supplied data and both fail open if written carelessly.

func TestArtworkTypeFromForm(t *testing.T) {
	for _, tc := range []struct {
		in     string
		want   string
		wantOK bool
	}{
		{"poster", "ARTWORK_TYPE_PRIMARY", true},
		{"primary", "ARTWORK_TYPE_PRIMARY", true},
		{"backdrop", "ARTWORK_TYPE_BACKDROP", true},
		{"BACKDROP", "ARTWORK_TYPE_BACKDROP", true},
		{"  Poster  ", "ARTWORK_TYPE_PRIMARY", true},
		// Absent means poster: it is the field an admin edits by default, and
		// the alternative is failing an upload over a missing optional field.
		{"", "ARTWORK_TYPE_PRIMARY", true},
		// Anything else is refused rather than stored: a record with a type no
		// screen reads is invisible, so the admin would see the upload
		// "succeed" and nothing change.
		{"logo", "", false},
		{"ARTWORK_TYPE_PRIMARY; DROP", "", false},
	} {
		got, ok := artworkTypeFromForm(tc.in)
		if got != tc.want || ok != tc.wantOK {
			t.Errorf("artworkTypeFromForm(%q) = (%q,%v), want (%q,%v)", tc.in, got, ok, tc.want, tc.wantOK)
		}
	}
}

func TestImageExtFromContentType(t *testing.T) {
	for _, tc := range []struct {
		ct     string
		want   string
		wantOK bool
	}{
		{"image/jpeg", ".jpg", true},
		{"image/png", ".png", true},
		{"image/webp", ".webp", true},
		// DetectContentType appends a charset on some types; a prefix match
		// must still work.
		{"image/png; charset=utf-8", ".png", true},

		// The refusals are the point of the function.
		//
		// SVG is an image by MIME type and a script container in practice: it
		// would be stored under our own origin and served back to browsers.
		{"image/svg+xml", "", false},
		{"image/gif", "", false},
		{"text/html; charset=utf-8", "", false},
		{"application/octet-stream", "", false},
		{"application/x-msdownload", "", false},
		{"", "", false},
	} {
		got, ok := imageExtFromContentType(tc.ct)
		if got != tc.want || ok != tc.wantOK {
			t.Errorf("imageExtFromContentType(%q) = (%q,%v), want (%q,%v)", tc.ct, got, ok, tc.want, tc.wantOK)
		}
	}
}
