package media

import (
	"testing"
	"time"

	"github.com/eveyproject/evey/internal/domain/shared"
)

// A cast token must be distinguishable from every other long-TTL token (the
// Safari/iOS "native" one shares the 12h TTL): the server-side cast progress
// tracker derives watch progress from the segments fetched on CAST tokens
// only - deriving it for a local native player would fight its own real
// progress reports.
func TestCreateCast_MarksTheTokenAsCast(t *testing.T) {
	s := NewStreamTokenStore(DefaultStreamTokenTTL)
	defer s.Stop()

	castTok := s.CreateCast("u1", "p1", "m1", CastStreamTokenTTL)
	st, ok := s.Validate(castTok, "m1")
	if !ok {
		t.Fatal("cast token must validate")
	}
	if !st.Cast {
		t.Fatal("cast token must carry Cast=true")
	}
	if st.TTL != CastStreamTokenTTL {
		t.Fatalf("cast token TTL = %v, want %v", st.TTL, CastStreamTokenTTL)
	}
	if st.UserID != shared.UserID("u1") || st.ProfileID != shared.ProfileID("p1") {
		t.Fatalf("cast token must carry user+profile, got %q/%q", st.UserID, st.ProfileID)
	}

	nativeTok := s.CreateWithTTL("u1", "p1", "m1", 12*time.Hour)
	stn, ok := s.Validate(nativeTok, "m1")
	if !ok {
		t.Fatal("native token must validate")
	}
	if stn.Cast {
		t.Fatal("a non-cast long-TTL token must NOT be marked Cast")
	}
}
