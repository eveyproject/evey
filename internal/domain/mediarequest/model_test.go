package mediarequest_test

import (
	"testing"

	"github.com/eveyproject/evey/internal/domain/mediarequest"
	"github.com/eveyproject/evey/internal/domain/shared"
)

func TestNewRequest(t *testing.T) {
	t.Run("valid movie request stamps defaults", func(t *testing.T) {
		r, err := mediarequest.NewRequest("req-1", "prf-1", "usr-1", mediarequest.MediaTypeMovie, "Some Movie",
			mediarequest.WithYear(2021), mediarequest.WithProvider("tmdb", "438631"))
		if err != nil {
			t.Fatalf("NewRequest: %v", err)
		}
		if r.Status != mediarequest.StatusPending {
			t.Errorf("status = %q, want pending", r.Status)
		}
		if r.CreatedAt.IsZero() || r.UpdatedAt.IsZero() {
			t.Error("timestamps not stamped")
		}
		if r.Year == nil || *r.Year != 2021 {
			t.Error("year option not applied")
		}
		if r.Provider != "tmdb" || r.ProviderID != "438631" {
			t.Error("provider option not applied")
		}
	})

	t.Run("missing required fields fail", func(t *testing.T) {
		cases := map[string]struct {
			id, profile, user, title string
			mt                       mediarequest.MediaType
		}{
			"no id":      {"", "prf-1", "usr-1", "T", mediarequest.MediaTypeMovie},
			"no profile": {"req-1", "", "usr-1", "T", mediarequest.MediaTypeMovie},
			"no user":    {"req-1", "prf-1", "", "T", mediarequest.MediaTypeMovie},
			"no title":   {"req-1", "prf-1", "usr-1", "", mediarequest.MediaTypeMovie},
			"bad type":   {"req-1", "prf-1", "usr-1", "T", mediarequest.MediaType("book")},
		}
		for name, c := range cases {
			if _, err := mediarequest.NewRequest(c.id, shared.ProfileID(c.profile), c.user, c.mt, c.title); err == nil {
				t.Errorf("%s: expected error, got nil", name)
			}
		}
	})
}

func TestAnswer(t *testing.T) {
	t.Run("valid decision transitions and stamps", func(t *testing.T) {
		r, _ := mediarequest.NewRequest("req-1", "prf-1", "usr-1", mediarequest.MediaTypeMovie, "Some Movie")
		if err := r.Answer(mediarequest.StatusApproved, "on it", "prf-admin"); err != nil {
			t.Fatalf("Answer: %v", err)
		}
		if r.Status != mediarequest.StatusApproved || r.AdminNote != "on it" {
			t.Error("status/note not set")
		}
		if r.AnsweredBy != "prf-admin" || r.AnsweredAt == nil {
			t.Error("answeredBy/answeredAt not set")
		}
	})

	t.Run("pending is not a valid answer state", func(t *testing.T) {
		r, _ := mediarequest.NewRequest("req-1", "prf-1", "usr-1", mediarequest.MediaTypeMovie, "Some Movie")
		if err := r.Answer(mediarequest.StatusPending, "", "prf-admin"); err == nil {
			t.Error("expected error answering with pending")
		}
	})
}

func TestStatusAndMediaTypeValidity(t *testing.T) {
	for _, s := range []mediarequest.Status{mediarequest.StatusPending, mediarequest.StatusApproved, mediarequest.StatusRejected, mediarequest.StatusLater} {
		if !s.IsValid() {
			t.Errorf("status %q should be valid", s)
		}
	}
	if mediarequest.Status("nope").IsValid() {
		t.Error("unknown status should be invalid")
	}
	if mediarequest.StatusPending.IsAnswer() {
		t.Error("pending is not an answer")
	}
	for _, m := range []mediarequest.MediaType{mediarequest.MediaTypeMovie, mediarequest.MediaTypeTV, mediarequest.MediaTypeMusic} {
		if !m.IsValid() {
			t.Errorf("media type %q should be valid", m)
		}
	}
	if mediarequest.MediaType("book").IsValid() {
		t.Error("unknown media type should be invalid")
	}
}
