package email

import (
	"crypto/tls"
	"fmt"
	"net"
	"net/smtp"
	"strings"
)

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
}

func NewSMTPSender(host string, port int, username, password, from, tlsMode string) *SMTPSender {
	return &SMTPSender{
		host:     host,
		port:     port,
		username: username,
		password: password,
		from:     from,
		tlsMode:  tlsMode,
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
		return smtp.SendMail(addr, auth, s.from, to, msg)
	default: // "none"
		return s.sendPlain(addr, auth, to, msg)
	}
}

func (s *SMTPSender) sendSSL(addr string, auth smtp.Auth, to []string, msg []byte) error {
	tlsCfg := &tls.Config{ServerName: s.host}
	conn, err := tls.Dial("tcp", addr, tlsCfg)
	if err != nil {
		return fmt.Errorf("smtp ssl dial: %w", err)
	}
	c, err := smtp.NewClient(conn, s.host)
	if err != nil {
		return fmt.Errorf("smtp ssl client: %w", err)
	}
	defer c.Close()
	if auth != nil {
		if err := c.Auth(auth); err != nil {
			return fmt.Errorf("smtp ssl auth: %w", err)
		}
	}
	return sendViaClient(c, s.from, to, msg)
}

func (s *SMTPSender) sendPlain(addr string, auth smtp.Auth, to []string, msg []byte) error {
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		return fmt.Errorf("smtp plain dial: %w", err)
	}
	c, err := smtp.NewClient(conn, s.host)
	if err != nil {
		return fmt.Errorf("smtp plain client: %w", err)
	}
	defer c.Close()
	if auth != nil {
		if err := c.Auth(auth); err != nil {
			return fmt.Errorf("smtp plain auth: %w", err)
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
