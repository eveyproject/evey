package email

import (
	"context"
	"time"

	appconfig "github.com/eveyproject/evey/internal/app/config"
)

// LiveSMTPSender sends with the SMTP settings in force at send time, so a
// change made in the admin panel applies to the next mail without a restart.
// With no host configured, Send is a silent no-op.
type LiveSMTPSender struct {
	live         *appconfig.LiveSMTP
	serverNameFn func() string
	timeout      time.Duration
}

func NewLiveSMTPSender(live *appconfig.LiveSMTP, serverNameFn func() string) *LiveSMTPSender {
	return &LiveSMTPSender{live: live, serverNameFn: serverNameFn, timeout: smtpDefaultTimeout}
}

func (s *LiveSMTPSender) senderFor(cfg appconfig.SMTPSettings) *SMTPSender {
	snd := NewSMTPSender(cfg.Host, cfg.Port, cfg.Username, cfg.Password, cfg.From, cfg.TLS)
	snd.timeout = s.timeout
	return snd
}

// Configured reports whether a mail would go out now.
func (s *LiveSMTPSender) Configured() bool { return s.live.Configured() }

func (s *LiveSMTPSender) Send(to []string, subject, body string) error {
	cfg := s.live.Current()
	if !cfg.Configured() {
		return nil
	}
	return s.senderFor(cfg).Send(to, subject, body)
}

// SendTest re-reads the store (a save just before the click must count) and
// sends a short test message to `to`, returning the SMTP error verbatim.
func (s *LiveSMTPSender) SendTest(ctx context.Context, to string) error {
	cfg := s.live.Reload(ctx)
	if !cfg.Configured() {
		return appconfig.ErrSMTPNotConfigured
	}
	name := serverNameOrDefault(s.serverNameFn)
	return s.senderFor(cfg).Send([]string{to}, name+" - Test e-mail",
		"This is a test message from "+name+".\n\nIf you can read it, e-mail delivery works.")
}
