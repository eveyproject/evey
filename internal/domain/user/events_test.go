package user_test

import (
	"testing"

	"github.com/gravity-zero/evey/internal/domain/user"
)

func TestEventNames(t *testing.T) {
	t.Parallel()

	cases := []struct {
		event interface{ EventName() string }
		want  string
	}{
		{user.UserCreated{}, "user.created"},
		{user.UserDeleted{}, "user.deleted"},
		{user.UserDisabled{}, "user.disabled"},
		{user.ProfileCreated{}, "user.profile.created"},
		{user.LoginSucceeded{}, "user.login.succeeded"},
		{user.LoginFailed{}, "user.login.failed"},
	}

	for _, c := range cases {
		got := c.event.EventName()
		if got != c.want {
			t.Errorf("EventName() = %q, want %q", got, c.want)
		}
	}
}
