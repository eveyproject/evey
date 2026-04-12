package webhook

import "time"

type WebhookCreated struct {
	WebhookID  string
	Name       string
	URL        string
	OccurredAt time.Time
}

func (e WebhookCreated) EventName() string { return "webhook.created" }

type WebhookUpdated struct {
	WebhookID  string
	OccurredAt time.Time
}

func (e WebhookUpdated) EventName() string { return "webhook.updated" }

type WebhookDeleted struct {
	WebhookID  string
	OccurredAt time.Time
}

func (e WebhookDeleted) EventName() string { return "webhook.deleted" }

type DeliveryFailed struct {
	WebhookID  string
	Event      string
	StatusCode int
	Error      string
	OccurredAt time.Time
}

func (e DeliveryFailed) EventName() string { return "webhook.delivery.failed" }
