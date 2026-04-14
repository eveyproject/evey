package activity_test

import (
	"testing"

	"github.com/evey-project/evey/internal/domain/activity"
)

func TestEventNames(t *testing.T) {
	t.Parallel()

	cases := []struct {
		event interface{ EventName() string }
		want  string
	}{
		{activity.LoginRecorded{}, "activity.login.recorded"},
		{activity.MetricRecorded{}, "activity.metric.recorded"},
		{activity.AuditLogCreated{}, "activity.audit.created"},
	}

	for _, c := range cases {
		got := c.event.EventName()
		if got != c.want {
			t.Errorf("EventName() = %q, want %q", got, c.want)
		}
	}
}
