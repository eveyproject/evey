package shared

import "errors"

var (
	ErrNotFound      = errors.New("not found")
	ErrConflict      = errors.New("conflict")
	ErrForbidden     = errors.New("forbidden")
	ErrUnauthorized  = errors.New("unauthorized")
	ErrInvalidInput  = errors.New("invalid input")
	ErrRateLimited   = errors.New("rate limited")
	ErrNotConfigured = errors.New("not configured")
)
