package photo_test

import (
	"testing"

	"github.com/gravity-zero/evey/internal/domain/photo"
)

func TestPhoto_Validate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		p       photo.Photo
		wantErr string
	}{
		{
			name:    "valid",
			p:       photo.Photo{LibraryID: "lib1", FilePath: "/photos/img.jpg"},
			wantErr: "",
		},
		{
			name:    "missing library ID",
			p:       photo.Photo{FilePath: "/photos/img.jpg"},
			wantErr: "photo requires a library ID",
		},
		{
			name:    "missing file path",
			p:       photo.Photo{LibraryID: "lib1"},
			wantErr: "photo requires a file path",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := tt.p.Validate()
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if err.Error() != tt.wantErr {
				t.Errorf("error = %q, want %q", err.Error(), tt.wantErr)
			}
		})
	}
}
