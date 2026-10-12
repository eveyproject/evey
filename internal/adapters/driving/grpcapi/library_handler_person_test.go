package grpcapi

import (
	"context"
	"testing"

	eveyv1 "github.com/eveyproject/evey/gen/evey/v1"
	applibrary "github.com/eveyproject/evey/internal/app/library"
	"github.com/eveyproject/evey/internal/domain/library"
	libfakes "github.com/eveyproject/evey/internal/domain/library/fakes"
	eveyi18n "github.com/eveyproject/evey/internal/pkg/i18n"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func newPersonHandler(people *libfakes.FakePersonRepository) *libraryHandler {
	querySvc := applibrary.NewQueryService(
		libfakes.NewFakeLibraryQueryRepo(),
		libfakes.NewFakeMediaItemQueryRepo(),
		libfakes.NewFakeMediaStreamRepository(),
		libfakes.NewFakeSearchIndex(),
		applibrary.WithPeople(people),
	)
	return &libraryHandler{
		query:      querySvc,
		translator: eveyi18n.NewEmpty(),
	}
}

// GetPerson: empty id returns InvalidArgument.
func TestGrpcLibraryGetPerson_EmptyID(t *testing.T) {
	t.Parallel()
	h := newPersonHandler(libfakes.NewFakePersonRepository())
	_, err := h.GetPerson(context.Background(), &eveyv1.GetPersonRequest{Id: ""})
	if err == nil {
		t.Fatal("want error")
	}
	if st, _ := status.FromError(err); st.Code() != codes.InvalidArgument {
		t.Fatalf("code: want InvalidArgument, got %v", st.Code())
	}
}

// GetPerson: not found → error.
func TestGrpcLibraryGetPerson_NotFound(t *testing.T) {
	t.Parallel()
	h := newPersonHandler(libfakes.NewFakePersonRepository())
	_, err := h.GetPerson(context.Background(), &eveyv1.GetPersonRequest{Id: "missing"})
	if err == nil {
		t.Fatal("want error")
	}
}

// GetPerson: happy path returns person fields.
func TestGrpcLibraryGetPerson_Found(t *testing.T) {
	t.Parallel()
	people := libfakes.NewFakePersonRepository()
	thumb := "/thumb.jpg"
	_ = people.Create(context.Background(), &library.Person{
		ID:           "p1",
		Name:         "Alice",
		Biography:    "bio",
		Birthday:     "1990-01-01",
		PlaceOfBirth: "Paris",
		KnownFor:     "Acting",
		ThumbPath:    &thumb,
		ProviderIDs:  map[string]string{"tmdb": "42"},
	})
	h := newPersonHandler(people)

	resp, err := h.GetPerson(context.Background(), &eveyv1.GetPersonRequest{Id: "p1"})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if resp.Name != "Alice" || resp.Biography != "bio" {
		t.Fatalf("fields: %+v", resp)
	}
	if resp.ThumbUrl != thumb {
		t.Fatalf("thumb: want %q, got %q", thumb, resp.ThumbUrl)
	}
	if resp.ProviderIds["tmdb"] != "42" {
		t.Fatalf("provider ids: %+v", resp.ProviderIds)
	}
}
