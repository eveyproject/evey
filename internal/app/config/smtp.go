package config

import (
	"context"
	"errors"
	"log/slog"
	"strconv"
	"strings"
	"sync/atomic"

	"github.com/evey-project/evey/internal/domain/shared"
)

// Keys the Admin -> Server Config "Email / SMTP" card writes. Each one wins
// over its EVEY_SMTP_* variable when set; the environment stays the fallback.
const (
	KeySMTPHost     = "smtp.host"
	KeySMTPPort     = "smtp.port"
	KeySMTPUsername = "smtp.username"
	KeySMTPPassword = "smtp.password" //nolint:gosec // G101: a config key name, not a credential
	KeySMTPFrom     = "smtp.from"
	KeySMTPTLS      = "smtp.tls"
)

// ErrSMTPNotConfigured is returned when a mail must go out and no SMTP host is
// in force (none in the panel nor the environment, or the demo).
var ErrSMTPNotConfigured = errors.New("smtp not configured")

// SMTPSettings is one complete SMTP configuration.
type SMTPSettings struct {
	Host     string
	Port     int
	Username string
	Password string
	From     string
	TLS      string // none, starttls, ssl
}

// Configured reports whether these settings can send at all.
func (s SMTPSettings) Configured() bool { return strings.TrimSpace(s.Host) != "" }

// IsSMTPKey reports whether a config key feeds the SMTP settings.
func IsSMTPKey(key string) bool { return strings.HasPrefix(key, "smtp.") }

// ResolveSMTPSettings picks one source as a whole: the panel when it names a
// host, env otherwise. Mixing them would hand env credentials to a host typed
// in the panel. An empty or unparseable stored port or TLS mode gets the
// usual defaults (587, starttls).
func ResolveSMTPSettings(values map[string]string, env SMTPSettings) SMTPSettings {
	host := strings.TrimSpace(values[KeySMTPHost])
	if host == "" {
		out := env
		out.TLS = strings.ToLower(out.TLS)
		return out
	}
	out := SMTPSettings{
		Host:     host,
		Port:     defaultSMTPPort,
		Username: strings.TrimSpace(values[KeySMTPUsername]),
		// Not trimmed: a password may legitimately carry spaces.
		Password: values[KeySMTPPassword],
		From:     strings.TrimSpace(values[KeySMTPFrom]),
		TLS:      strings.ToLower(strings.TrimSpace(values[KeySMTPTLS])),
	}
	if out.TLS == "" {
		out.TLS = defaultSMTPTLS
	}
	if v := strings.TrimSpace(values[KeySMTPPort]); v != "" {
		if p, err := strconv.Atoi(v); err == nil && p > 0 && p < 65536 {
			out.Port = p
		} else {
			slog.Warn("smtp: ignoring stored port", slog.String("value", v))
		}
	}
	return out
}

const (
	defaultSMTPPort = 587
	defaultSMTPTLS  = "starttls"
)

// LiveSMTP holds the SMTP settings in force now. It is loaded at boot and
// re-read from the store on every smtp.* change, so the panel applies without
// a restart. A disabled LiveSMTP (the demo) never reports a configuration.
type LiveSMTP struct {
	repo     shared.ServerConfigRepository
	env      SMTPSettings
	disabled bool
	cur      atomic.Pointer[SMTPSettings]
}

// NewLiveSMTP loads the effective settings now. repo may be nil (env only).
func NewLiveSMTP(ctx context.Context, repo shared.ServerConfigRepository, env SMTPSettings, disabled bool) *LiveSMTP {
	l := &LiveSMTP{repo: repo, env: env, disabled: disabled}
	initial := ResolveSMTPSettings(nil, env)
	l.cur.Store(&initial)
	l.Reload(ctx)
	return l
}

// Watch re-reads the store whenever an smtp.* key changes.
func (l *LiveSMTP) Watch(bus shared.EventBus) {
	if bus == nil {
		return
	}
	bus.Subscribe(shared.ServerConfigChanged{}.EventName(), func(evtCtx context.Context, ev shared.Event) {
		if changed, ok := ev.(shared.ServerConfigChanged); ok && IsSMTPKey(changed.Key) {
			s := l.Reload(evtCtx)
			slog.InfoContext(evtCtx, "smtp: settings updated", slog.String("key", changed.Key), slog.String("host", s.Host))
		}
	})
}

// Reload re-reads the store and returns the settings now in force. A store
// read error keeps the previous settings rather than dropping to env.
func (l *LiveSMTP) Reload(ctx context.Context) SMTPSettings {
	if l.repo != nil {
		entries, err := l.repo.List(ctx)
		if err != nil {
			slog.WarnContext(ctx, "smtp: config unreadable, keeping the current settings", slog.Any("err", err))
			return l.Current()
		}
		values := make(map[string]string)
		for _, e := range entries {
			if e != nil && IsSMTPKey(e.Key) {
				values[e.Key] = e.Value
			}
		}
		s := ResolveSMTPSettings(values, l.env)
		l.cur.Store(&s)
	}
	return l.Current()
}

// Current returns the settings in force; zero when disabled.
func (l *LiveSMTP) Current() SMTPSettings {
	if l == nil || l.disabled {
		return SMTPSettings{}
	}
	return *l.cur.Load()
}

// Configured reports whether mail can go out now.
func (l *LiveSMTP) Configured() bool { return l.Current().Configured() }
