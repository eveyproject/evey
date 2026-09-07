package user

import "errors"

var (
	ErrInvalidCredentials       = errors.New("invalid credentials")
	ErrUserDisabled             = errors.New("user is disabled")
	ErrUserNotFound             = errors.New("user not found")
	ErrProfileNotFound          = errors.New("profile not found")
	ErrNotAdmin                 = errors.New("admin required")
	ErrLastAdmin                = errors.New("cannot remove last admin")
	ErrSetupDone                = errors.New("setup already completed")
	ErrInviteInvalid            = errors.New("invalid or expired invite")
	ErrPasswordTooShort         = errors.New("password must be at least 8 characters")
	ErrUsernameRequired         = errors.New("username is required")
	ErrAccountLocked            = errors.New("too many login attempts, try again later")
	ErrPasswordRecoveryDisabled = errors.New("password recovery is not enabled")
	ErrResetTokenInvalid        = errors.New("invalid or expired reset token")
	ErrMaxProfilesReached       = errors.New("user has reached the maximum number of profiles")
	ErrKidsRequiresPIN          = errors.New("non-kids profile must have a PIN when a kids profile exists")
	ErrCannotDeleteDefault      = errors.New("cannot delete the default profile")
	ErrInvalidPINFormat         = errors.New("PIN must be exactly 4 digits")
	ErrAdminCannotBeKids        = errors.New("an admin user's profile cannot be a kids profile")
	ErrPINRequired              = errors.New("parental PIN required to switch out of a kids profile")
)
