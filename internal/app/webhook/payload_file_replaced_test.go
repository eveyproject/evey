package webhook_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/eveyproject/evey/internal/domain/library"
	sharedfakes "github.com/eveyproject/evey/internal/domain/shared/fakes"
	webhookdomain "github.com/eveyproject/evey/internal/domain/webhook"
)

// The event struct goes out as is (no json tags), so its Go field names ARE
// the receivers' contract: renaming a field breaks every webhook consumer
// without a compile error anywhere. This freezes the bytes of a file_replaced
// delivery as a receiver reads them.
func TestHandleAny_FileReplacedPayloadKeysAreFrozen(t *testing.T) {
	t.Parallel()
	repo := &fakeWebhookRepo{}
	bus := sharedfakes.NewFakeEventBus()
	_, sender := newTestService(repo, &fakeDeliveryRepo{}, bus)
	repo.mu.Lock()
	repo.data = append(repo.data, &webhookdomain.Config{
		ID: "file-replaced", URL: "http://hook.example", Events: []string{"media.item.updated"}, Enabled: true,
	})
	repo.mu.Unlock()

	_ = bus.Publish(context.Background(), library.MediaItemUpdated{
		MediaItemID:        "item-1",
		LibraryID:          "lib-1",
		Reason:             library.UpdateReasonFileReplaced,
		PreviousDurationMs: 5_400_000,
		DurationMs:         5_460_500,
		OccurredAt:         time.Date(2026, 10, 14, 9, 30, 0, 0, time.UTC),
	})
	waitForCalls(t, sender, 1, 2*time.Second)

	var body map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(sender.Calls()[0].Body, &body))
	assert.ElementsMatch(t, []string{"event", "payload"}, keysOf(body))
	assert.JSONEq(t, `"media.item.updated"`, string(body["event"]))
	assert.JSONEq(t, `{
		"MediaItemID": "item-1",
		"LibraryID": "lib-1",
		"Reason": "file_replaced",
		"PreviousDurationMs": 5400000,
		"DurationMs": 5460500,
		"OccurredAt": "2026-10-14T09:30:00Z"
	}`, string(body["payload"]))
}

func keysOf(m map[string]json.RawMessage) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
