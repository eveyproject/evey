package email

import (
	"bufio"
	"net"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// scriptedSMTP is the smallest server the sender can talk to: it greets,
// answers EHLO/HELO, accepts or refuses recipients as told, and keeps the
// DATA payload so the test can read what went on the wire.
type scriptedSMTP struct {
	ln         net.Listener
	rejectRcpt string
	mu         sync.Mutex
	from       string
	rcpts      []string
	data       string
	authSeen   bool
}

func startScriptedSMTP(t *testing.T) *scriptedSMTP {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	s := &scriptedSMTP{ln: ln}
	t.Cleanup(func() { _ = ln.Close() })
	go s.serve()
	return s
}

func (s *scriptedSMTP) serve() {
	for {
		conn, err := s.ln.Accept()
		if err != nil {
			return
		}
		go s.handle(conn)
	}
}

func (s *scriptedSMTP) handle(conn net.Conn) {
	defer func() { _ = conn.Close() }()
	r := bufio.NewReader(conn)
	w := bufio.NewWriter(conn)
	say := func(line string) { _, _ = w.WriteString(line + "\r\n"); _ = w.Flush() }
	say("220 scripted ESMTP")
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return
		}
		line = strings.TrimRight(line, "\r\n")
		cmd := strings.ToUpper(line)
		switch {
		case strings.HasPrefix(cmd, "EHLO"):
			say("250-scripted")
			say("250 AUTH PLAIN")
		case strings.HasPrefix(cmd, "HELO"):
			say("250 scripted")
		case strings.HasPrefix(cmd, "AUTH"):
			s.mu.Lock()
			s.authSeen = true
			s.mu.Unlock()
			say("235 ok")
		case strings.HasPrefix(cmd, "MAIL FROM:"):
			s.mu.Lock()
			s.from = line[len("MAIL FROM:"):]
			s.mu.Unlock()
			say("250 ok")
		case strings.HasPrefix(cmd, "RCPT TO:"):
			rcpt := line[len("RCPT TO:"):]
			if s.rejectRcpt != "" && strings.Contains(rcpt, s.rejectRcpt) {
				say("550 no such user")
				continue
			}
			s.mu.Lock()
			s.rcpts = append(s.rcpts, rcpt)
			s.mu.Unlock()
			say("250 ok")
		case cmd == "DATA":
			say("354 go ahead")
			var body strings.Builder
			for {
				l, err := r.ReadString('\n')
				if err != nil {
					return
				}
				if l == ".\r\n" {
					break
				}
				body.WriteString(l)
			}
			s.mu.Lock()
			s.data = body.String()
			s.mu.Unlock()
			say("250 queued")
		case cmd == "QUIT":
			say("221 bye")
			return
		default:
			say("250 ok")
		}
	}
}

func (s *scriptedSMTP) sender(tlsMode, username string) *SMTPSender {
	host, portStr, _ := net.SplitHostPort(s.ln.Addr().String())
	port, _ := strconv.Atoi(portStr)
	snd := NewSMTPSender(host, port, username, "secret", "evey@test.local", tlsMode)
	snd.timeout = 2 * time.Second
	return snd
}

// The plain path end to end against a scripted server: envelope from the
// configured sender, one RCPT per recipient, the built message as DATA, and
// the plain-auth handshake only when a username is configured. The server's
// refusal of one recipient surfaces as an error naming that address.
func TestSMTPSend_PlainWire(t *testing.T) {
	srv := startScriptedSMTP(t)
	if err := srv.sender("none", "").Send([]string{"a@test.local", "b@test.local"}, "Hello", "line one"); err != nil {
		t.Fatalf("Send: %v", err)
	}
	srv.mu.Lock()
	defer srv.mu.Unlock()
	if !strings.Contains(srv.from, "evey@test.local") {
		t.Errorf("MAIL FROM = %q", srv.from)
	}
	if len(srv.rcpts) != 2 || !strings.Contains(srv.rcpts[1], "b@test.local") {
		t.Errorf("RCPT TO = %v", srv.rcpts)
	}
	if !strings.Contains(srv.data, "Subject: Hello") || !strings.Contains(srv.data, "line one") {
		t.Errorf("DATA = %q", srv.data)
	}
	if srv.authSeen {
		t.Error("no username configured: no AUTH must be attempted")
	}
}

func TestSMTPSend_PlainWire_AuthAndRejectedRecipient(t *testing.T) {
	srv := startScriptedSMTP(t)
	srv.rejectRcpt = "nobody@"
	err := srv.sender("none", "user").Send([]string{"nobody@test.local"}, "Hello", "body")
	if err == nil || !strings.Contains(err.Error(), "RCPT TO nobody@test.local") {
		t.Fatalf("a refused recipient must surface with its address, got %v", err)
	}
	srv.mu.Lock()
	defer srv.mu.Unlock()
	if !srv.authSeen {
		t.Error("a configured username must authenticate before the envelope")
	}
	if srv.data != "" {
		t.Error("nothing may be sent after a refused recipient")
	}
}

// STARTTLS mode against a server that does not advertise it falls back to a
// plain conversation (smtp.SendMail's documented behaviour), and still sends.
func TestSMTPSend_StartTLSWire_NotAdvertised(t *testing.T) {
	srv := startScriptedSMTP(t)
	if err := srv.sender("starttls", "").Send([]string{"a@test.local"}, "Hi", "b"); err != nil {
		t.Fatalf("Send: %v", err)
	}
	srv.mu.Lock()
	defer srv.mu.Unlock()
	if !strings.Contains(srv.data, "Subject: Hi") {
		t.Errorf("DATA = %q", srv.data)
	}
}
