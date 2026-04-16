package playlist

import (
	"testing"
)

// NewService exposes the Command and Query facades.
func TestService_FacadeGetters(t *testing.T) {
	t.Parallel()
	svc, _, _, _ := newTestService()
	if svc.CommandService() == nil {
		t.Fatal("expected non-nil CommandService()")
	}
	if svc.QueryService() == nil {
		t.Fatal("expected non-nil QueryService()")
	}
}
