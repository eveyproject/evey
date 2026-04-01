package subtitle_test

import (
	"testing"

	"github.com/gravity-zero/evey/internal/domain/subtitle"
)

func TestSearchResult_Validate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		sr      subtitle.SearchResult
		wantErr string
	}{
		{
			name:    "valid",
			sr:      subtitle.SearchResult{FileName: "sub.srt", Language: "en"},
			wantErr: "",
		},
		{
			name:    "missing file name",
			sr:      subtitle.SearchResult{Language: "en"},
			wantErr: "search result requires a file name",
		},
		{
			name:    "missing language",
			sr:      subtitle.SearchResult{FileName: "sub.srt"},
			wantErr: "search result requires a language",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := tt.sr.Validate()
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
