package rating_test

import (
	"testing"

	"github.com/evey-project/evey/internal/domain/rating"
)

func TestRating_Validate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		r       rating.Rating
		wantErr string
	}{
		{
			name:    "valid",
			r:       rating.Rating{MediaItemID: "m1", ProfileID: "p1"},
			wantErr: "",
		},
		{
			name:    "missing media item ID",
			r:       rating.Rating{ProfileID: "p1"},
			wantErr: "rating requires a media item ID",
		},
		{
			name:    "missing profile ID",
			r:       rating.Rating{MediaItemID: "m1"},
			wantErr: "rating requires a profile ID",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := tt.r.Validate()
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
