package httpapi

import (
	"context"
	"errors"

	domainlibrary "github.com/gravity-zero/evey/internal/domain/library"
	"github.com/gravity-zero/evey/internal/domain/shared"
	domainuser "github.com/gravity-zero/evey/internal/domain/user"
)

// fakeTokenValidator implements tokenValidator for tests.
type fakeTokenValidator struct {
	claims *domainuser.Claims
	err    error
}

func (f *fakeTokenValidator) ValidateAccessToken(_ string) (*domainuser.Claims, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.claims, nil
}

func validUserValidator(isAdmin bool) *fakeTokenValidator {
	return &fakeTokenValidator{
		claims: &domainuser.Claims{
			UserID:    "user-1",
			Username:  "test",
			IsAdmin:   isAdmin,
			ProfileID: "prof-1",
		},
	}
}

func invalidValidator() *fakeTokenValidator {
	return &fakeTokenValidator{err: errors.New("invalid token")}
}

// fakeLibraryAccessChecker implements LibraryAccessChecker for tests.
type fakeLibraryAccessChecker struct {
	allowed bool
	err     error
}

func (f *fakeLibraryAccessChecker) HasAccess(_ context.Context, _ shared.UserID, _ shared.LibraryID) (bool, error) {
	return f.allowed, f.err
}

func allowAll() *fakeLibraryAccessChecker  { return &fakeLibraryAccessChecker{allowed: true} }
func denyAll() *fakeLibraryAccessChecker   { return &fakeLibraryAccessChecker{allowed: false} }

// fakeMediaGetter implements mediaLibraryGetter for tests.
type fakeMediaGetter struct {
	items map[string]*domainlibrary.MediaItem
}

func newFakeMediaGetter() *fakeMediaGetter {
	return &fakeMediaGetter{items: make(map[string]*domainlibrary.MediaItem)}
}

func (f *fakeMediaGetter) GetMediaItem(_ context.Context, id string) (*domainlibrary.MediaItem, error) {
	if item, ok := f.items[id]; ok {
		return item, nil
	}
	return nil, errors.New("not found")
}
