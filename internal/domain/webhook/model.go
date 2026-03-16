package webhook

import (
	"errors"
	"time"
)

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

func (c *Config) Validate() error {
	if c.Name == "" {
		return errors.New("webhook name is required")
	}
	if c.URL == "" {
		return errors.New("webhook URL is required")
	}
	if len(c.Events) == 0 {
		return errors.New("webhook must subscribe to at least one event")
	}
	return nil
}

func (c *Config) MatchesEvent(eventName string) bool {
	for _, p := range c.Events {
		if p == "*" || p == eventName {
			return true
		}
	}
	return false
}
