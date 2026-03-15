package user

import "errors"

var (
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrUserDisabled       = errors.New("user is disabled")
	ErrNotAdmin           = errors.New("admin required")
	ErrLastAdmin          = errors.New("cannot remove last admin")
	ErrSetupDone          = errors.New("setup already completed")
	ErrInviteInvalid      = errors.New("invalid or expired invite")
	ErrPasswordTooShort   = errors.New("password must be at least 8 characters")
)
