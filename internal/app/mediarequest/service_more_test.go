package mediarequest_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	appmediarequest "github.com/eveyproject/evey/internal/app/mediarequest"
	"github.com/eveyproject/evey/internal/domain/mediarequest"
	mrfakes "github.com/eveyproject/evey/internal/domain/mediarequest/fakes"
	"github.com/eveyproject/evey/internal/domain/shared"
)

// failingLookup stands in for a library whose provider-id index is unreachable.
type failingLookup struct{}

func (failingLookup) FindByProviderIDs(context.Context, string, []string) (map[string]string, error) {
	return nil, errors.New("db down")
}

// A request for a title the server already holds is refused server-side: the
// UI hides the button, a crafted call must not get past it. Music is never
// checked (no MusicBrainz id in music_tracks), and a lookup failure fails OPEN.
func TestCreateRequest_AlreadyHeldTitle(t *testing.T) {
	ctx := context.Background()
	held := &countingLookup{held: map[string]string{appmediarequest.ProviderKey("tmdb", "42"): "mi-42"}}
	in := appmediarequest.CreateInput{MediaType: mediarequest.MediaTypeMovie, Title: "Held", Provider: "tmdb", ProviderID: "42"}

	svc := newService(mrfakes.NewFakeRepository(), mrfakes.NewFakeAccess(true), &fakeBus{}).WithLibraryLookup(held)
	if _, err := svc.CreateRequest(ctx, "prf-1", "usr-1", in); !errors.Is(err, mediarequest.ErrAlreadyHeld) {
		t.Fatalf("err = %v, want ErrAlreadyHeld", err)
	}

	music := in
	music.MediaType = mediarequest.MediaTypeMusic
	svc = newService(mrfakes.NewFakeRepository(), mrfakes.NewFakeAccess(true), &fakeBus{}).WithLibraryLookup(held)
	if _, err := svc.CreateRequest(ctx, "prf-1", "usr-1", music); err != nil {
		t.Fatalf("music is never checked against the library: %v", err)
	}
	if held.calls != 1 {
		t.Errorf("library consulted %d times, want 1 (the movie only)", held.calls)
	}

	svc = newService(mrfakes.NewFakeRepository(), mrfakes.NewFakeAccess(true), &fakeBus{}).WithLibraryLookup(failingLookup{})
	if _, err := svc.CreateRequest(ctx, "prf-1", "usr-1", in); err != nil {
		t.Fatalf("a lookup failure must fail open: %v", err)
	}
}

func TestListRequests_ClampLimitsAndScope(t *testing.T) {
	ctx := context.Background()
	repo := mrfakes.NewFakeRepository()
	svc := newService(repo, mrfakes.NewFakeAccess(true), &fakeBus{})
	for i := range 60 {
		profile := "prf-a"
		if i%2 == 1 {
			profile = "prf-b"
		}
		if _, err := svc.CreateRequest(ctx, shared.ProfileID(profile), "usr-1", appmediarequest.CreateInput{
			MediaType: mediarequest.MediaTypeMovie, Title: fmt.Sprintf("Title %d", i),
		}); err != nil {
			t.Fatal(err)
		}
	}

	mine, err := svc.ListMyRequests(ctx, "prf-a", 0, 0)
	if err != nil || len(mine) != 30 {
		t.Fatalf("own requests with default limit: got %d err=%v, want 30", len(mine), err)
	}
	for _, r := range mine {
		if r.ProfileID != "prf-a" {
			t.Fatalf("another profile's request leaked: %+v", r)
		}
	}
	if got, _ := svc.ListMyRequests(ctx, "prf-a", 5, 0); len(got) != 5 {
		t.Errorf("limit 5: got %d", len(got))
	}
	// Out-of-range limits fall back to the default page, never unbounded.
	if got, _ := svc.ListMyRequests(ctx, "prf-a", 10_000, 0); len(got) != 30 {
		t.Errorf("limit 10000 must clamp to the default 50 (30 exist): got %d", len(got))
	}

	all, err := svc.ListAll(ctx, nil, 0, 0)
	if err != nil || len(all) != 60 {
		t.Fatalf("moderation view with default limit: got %d err=%v, want 60", len(all), err)
	}
	if got, _ := svc.ListAll(ctx, []mediarequest.Status{mediarequest.StatusApproved}, 0, 0); len(got) != 0 {
		t.Errorf("status filter: got %d approved, want 0", len(got))
	}
	if got, _ := svc.ListAll(ctx, nil, 7, 0); len(got) != 7 {
		t.Errorf("limit 7: got %d", len(got))
	}
}

func TestAccessSettings_RoundTrip(t *testing.T) {
	ctx := context.Background()
	access := mrfakes.NewFakeAccess(false)
	svc := newService(mrfakes.NewFakeRepository(), access, &fakeBus{})

	if err := svc.SetDefault(ctx, true); err != nil {
		t.Fatal(err)
	}
	if err := svc.SetUserAccess(ctx, "usr-1", appmediarequest.AccessDisabled); err != nil {
		t.Fatal(err)
	}
	if err := svc.SetUserAccess(ctx, "usr-2", appmediarequest.AccessEnabled); err != nil {
		t.Fatal(err)
	}
	got, err := svc.GetAccessSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !got.DefaultEnabled || got.Overrides["usr-1"] != false || got.Overrides["usr-2"] != true || len(got.Overrides) != 2 {
		t.Errorf("settings = %+v", got)
	}

	// Clearing goes back to the default.
	if err := svc.SetUserAccess(ctx, "usr-1", appmediarequest.AccessDefault); err != nil {
		t.Fatal(err)
	}
	got, _ = svc.GetAccessSettings(ctx)
	if _, still := got.Overrides["usr-1"]; still {
		t.Errorf("override not cleared: %+v", got.Overrides)
	}
	if ok, _ := svc.CanRequest(ctx, "usr-1"); !ok {
		t.Error("usr-1 should follow the (enabled) default after the clear")
	}

	if err := svc.SetUserAccess(ctx, "", appmediarequest.AccessEnabled); err == nil {
		t.Error("an empty user id must be refused")
	}
	if err := svc.SetUserAccess(ctx, "usr-3", appmediarequest.AccessMode(99)); err == nil {
		t.Error("an unknown mode must be refused")
	}
}
