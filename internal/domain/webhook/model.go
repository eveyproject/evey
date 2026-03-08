package webhook

import "time"

type Config struct {
	ID        string
	Name      string
	URL       string
	Events    []string
	Secret    string
	Headers   map[string]string
	Enabled   bool
	CreatedAt time.Time
}

type Delivery struct {
	ID         string
	WebhookID  string
	Event      string
	StatusCode int
	Error      string
	SentAt     time.Time
}
