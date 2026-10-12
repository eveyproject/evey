package config

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/eveyproject/evey/internal/domain/shared"
	"github.com/eveyproject/evey/internal/domain/shared/fakes"
)

// A webhook subscribed to "*" marshals every event: a secret config value must
// never reach that JSON, while in-process subscribers still read it.
func TestService_Set_secretValueNeverSerialized(t *testing.T) {
	bus := fakes.NewFakeEventBus()
	svc := NewService(fakes.NewFakeServerConfigRepository(), WithEventBus(bus))

	require.NoError(t, svc.Set(context.Background(), "smtp.password", "hunter2-smtp"))
	require.NoError(t, svc.Set(context.Background(), "artwork.auto_download", "false"))

	events := bus.PublishedEvents()
	require.Len(t, events, 2)
	secret, ok := events[0].(shared.ServerConfigChanged)
	require.True(t, ok)
	assert.Equal(t, "hunter2-smtp", secret.Value, "in-process subscribers keep the value")

	body, err := json.Marshal(events[0])
	require.NoError(t, err)
	assert.NotContains(t, string(body), "hunter2-smtp")
	assert.Contains(t, string(body), "smtp.password")

	plain, err := json.Marshal(events[1])
	require.NoError(t, err)
	assert.Contains(t, string(plain), `"false"`, "a non-secret value stays useful to webhooks")
}
