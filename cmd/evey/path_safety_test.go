package main

import (
	"testing"
)

func TestSafeJoin(t *testing.T) {
	cases := []struct {
		name    string
		base    string
		rel     string
		wantErr bool
	}{
		{
			name:    "normal file",
			base:    "/cache/hls/session1",
			rel:     "segment001.ts",
			wantErr: false,
		},
		{
			name:    "normal image",
			base:    "/cache/trickplay/m1",
			rel:     "trickplay_001.jpg",
			wantErr: false,
		},
		{
			name:    "single dot-dot traversal",
			base:    "/cache/hls/session1",
			rel:     "../other/evil.ts",
			wantErr: true,
		},
		{
			name:    "deep traversal to etc passwd",
			base:    "/cache/hls/session1",
			rel:     "../../etc/passwd",
			wantErr: true,
		},
		{
			// filepath.Join strips leading slash; result is inside base — safe
			name:    "absolute path treated as relative by filepath.Join",
			base:    "/cache/trickplay/m1",
			rel:     "/etc/passwd",
			wantErr: false,
		},
		{
			name:    "sibling directory traversal",
			base:    "/cache/trickplay/m1",
			rel:     "../m2/evil.jpg",
			wantErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := safeJoin(tc.base, tc.rel)
			if tc.wantErr {
				if err == nil {
					t.Errorf("safeJoin(%q, %q) = %q, want error", tc.base, tc.rel, got)
				}
			} else {
				if err != nil {
					t.Errorf("safeJoin(%q, %q) unexpected error: %v", tc.base, tc.rel, err)
				}
			}
		})
	}
}
