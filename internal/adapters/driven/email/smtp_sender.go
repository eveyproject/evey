package email

import (
	"crypto/tls"
	"fmt"
	"log/slog"
	"net"
	"net/smtp"
	"strings"
	"time"
)

// smtpDefaultTimeout bounds the ENTIRE SMTP conversation (connect through DATA).
// Email bodies here are tiny (notification/reset text), so a single absolute
// deadline is ample - and it stops a hung or slow SMTP server from blocking an
// event-bus worker (NotificationEmailer runs on the shared pool) or the
// password-reset HTTP handler forever.
const smtpDefaultTimeout = 30 * time.Second

// Sender is the port for sending emails.
type Sender interface {
	Send(to []string, subject, body string) error
}

// SMTPSender sends emails via SMTP.
type SMTPSender struct {
	host     string
	port     int
	username string
	password string
	from     string
	tlsMode  string // "none", "starttls", "ssl"
	timeout  time.Duration
}

func NewSMTPSender(host string, port int, username, password, from, tlsMode string) *SMTPSender {
	return &SMTPSender{
		host:     host,
		port:     port,
		username: username,
		password: password,
		from:     from,
		tlsMode:  tlsMode,
		timeout:  smtpDefaultTimeout,
	}
}

func (s *SMTPSender) Send(to []string, subject, body string) error {
	if s.host == "" {
		return nil
	}

	msg := buildMessage(s.from, to, subject, body)
	addr := fmt.Sprintf("%s:%d", s.host, s.port)

	var auth smtp.Auth
	if s.username != "" {
		auth = smtp.PlainAuth("", s.username, s.password, s.host)
	}

	switch strings.ToLower(s.tlsMode) {
	case "ssl":
		return s.sendSSL(addr, auth, to, msg)
	case "starttls":
		return s.sendStartTLS(addr, auth, to, msg)
	default: // "none"
		return s.sendPlain(addr, auth, to, msg)
	}
}

func (s *SMTPSender) sendSSL(addr string, auth smtp.Auth, to []string, msg []byte) error {
	tlsCfg := &tls.Config{ServerName: s.host, MinVersion: tls.VersionTLS12}
	conn, err := tls.DialWithDialer(&net.Dialer{Timeout: s.timeout}, "tcp", addr, tlsCfg)
	if err != nil {
		return fmt.Errorf("smtp ssl dial: %w", err)
	}
	if err = armDeadline(conn, s.timeout); err != nil {
		return fmt.Errorf("smtp ssl deadline: %w", err)
	}
	c, err := smtp.NewClient(conn, s.host)
	if err != nil {
		return fmt.Errorf("smtp ssl client: %w", err)
	}
	defer func() {
		if err := c.Close(); err != nil {
			slog.Warn("smtp ssl close", slog.Any("err", err))
		}
	}()
	if auth != nil {
		if err := c.Auth(auth); err != nil {
			return fmt.Errorf("smtp ssl auth: %w", err)
		}
	}
	return sendViaClient(c, s.from, to, msg)
}

func (s *SMTPSender) sendPlain(addr string, auth smtp.Auth, to []string, msg []byte) error {
	conn, err := net.DialTimeout("tcp", addr, s.timeout)
	if err != nil {
		return fmt.Errorf("smtp plain dial: %w", err)
	}
	if err = armDeadline(conn, s.timeout); err != nil {
		return fmt.Errorf("smtp plain deadline: %w", err)
	}
	c, err := smtp.NewClient(conn, s.host)
	if err != nil {
		return fmt.Errorf("smtp plain client: %w", err)
	}
	defer func() {
		if err := c.Close(); err != nil {
			slog.Warn("smtp plain close", slog.Any("err", err))
		}
	}()
	if auth != nil {
		if err := c.Auth(auth); err != nil {
			return fmt.Errorf("smtp plain auth: %w", err)
		}
	}
	return sendViaClient(c, s.from, to, msg)
}

// sendStartTLS replaces smtp.SendMail (which gives no control over dial/deadline)
// with a manual dial+deadline path: connect with a timeout, bound the whole
// conversation with an absolute deadline, then STARTTLS (if advertised), auth,
// and send. Mirrors smtp.SendMail's STARTTLS-when-supported behaviour.
func (s *SMTPSender) sendStartTLS(addr string, auth smtp.Auth, to []string, msg []byte) error {
	conn, err := net.DialTimeout("tcp", addr, s.timeout)
	if err != nil {
		return fmt.Errorf("smtp starttls dial: %w", err)
	}
	if err = armDeadline(conn, s.timeout); err != nil {
		return fmt.Errorf("smtp starttls deadline: %w", err)
	}
	c, err := smtp.NewClient(conn, s.host)
	if err != nil {
		return fmt.Errorf("smtp starttls client: %w", err)
	}
	defer func() {
		if err := c.Close(); err != nil {
			slog.Warn("smtp starttls close", slog.Any("err", err))
		}
	}()
	if ok, _ := c.Extension("STARTTLS"); ok {
		if err := c.StartTLS(&tls.Config{ServerName: s.host, MinVersion: tls.VersionTLS12}); err != nil {
			return fmt.Errorf("smtp starttls: %w", err)
		}
	}
	if auth != nil {
		if err := c.Auth(auth); err != nil {
			return fmt.Errorf("smtp starttls auth: %w", err)
		}
	}
	return sendViaClient(c, s.from, to, msg)
}

func sendViaClient(c *smtp.Client, from string, to []string, msg []byte) error {
	if err := c.Mail(from); err != nil {
		return fmt.Errorf("smtp MAIL FROM: %w", err)
	}
	for _, addr := range to {
		if err := c.Rcpt(addr); err != nil {
			return fmt.Errorf("smtp RCPT TO %s: %w", addr, err)
		}
	}
	w, err := c.Data()
	if err != nil {
		return fmt.Errorf("smtp DATA: %w", err)
	}
	if _, err := w.Write(msg); err != nil {
		return fmt.Errorf("smtp write: %w", err)
	}
	return w.Close()
}

// armDeadline sets an absolute deadline covering the whole SMTP conversation on
// a freshly dialed conn. If that fails (a dead conn) it closes the conn so we
// don't leak it. Set BEFORE smtp.NewClient, whose greeting read is the first
// thing that would otherwise block forever on a silent server.
func armDeadline(conn net.Conn, timeout time.Duration) error {
	if err := conn.SetDeadline(time.Now().Add(timeout)); err != nil {
		if cerr := conn.Close(); cerr != nil {
			slog.Warn("smtp close after deadline error", slog.Any("err", cerr))
		}
		return err
	}
	return nil
}

func buildMessage(from string, to []string, subject, body string) []byte {
	var sb strings.Builder
	sb.WriteString("From: " + from + "\r\n")
	sb.WriteString("To: " + strings.Join(to, ", ") + "\r\n")
	sb.WriteString("Subject: " + subject + "\r\n")
	sb.WriteString("MIME-Version: 1.0\r\n")
	sb.WriteString("Content-Type: text/plain; charset=UTF-8\r\n")
	sb.WriteString("\r\n")
	sb.WriteString(body)
	return []byte(sb.String())
}
