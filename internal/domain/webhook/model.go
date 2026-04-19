package webhook

import (
	"errors"
	"maps"
	"slices"
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
	if c.ID == "" {
		return errors.New("webhook ID is required")
	}
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

type ConfigOption func(*Config)

func WithConfigSecret(secret string) ConfigOption {
	return func(c *Config) { c.Secret = secret }
}

func WithConfigHeaders(headers map[string]string) ConfigOption {
	return func(c *Config) { c.Headers = maps.Clone(headers) }
}

func WithConfigEnabled(enabled bool) ConfigOption {
	return func(c *Config) { c.Enabled = enabled }
}

// NewConfig is the canonical constructor. Stamps CreatedAt and
// validates. Required identity/intent fields (id, name, url, events)
// are surfaced as params so construction never succeeds with them
// missing. URL reachability is an adapter concern — Service.Create
// layers urlvalidator on top.
func NewConfig(id, name, url string, events []string, opts ...ConfigOption) (*Config, error) {
	c := &Config{
		ID:        id,
		Name:      name,
		URL:       url,
		Events:    slices.Clone(events),
		CreatedAt: time.Now(),
	}
	for _, opt := range opts {
		opt(c)
	}
	if err := c.Validate(); err != nil {
		return nil, err
	}
	return c, nil
}
