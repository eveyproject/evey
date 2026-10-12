package email

import (
	"context"
	"errors"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"

	appconfig "github.com/eveyproject/evey/internal/app/config"
	"github.com/eveyproject/evey/internal/domain/shared/fakes"
)

func (s *scriptedSMTP) hostPort() (string, string) {
	host, port, _ := net.SplitHostPort(s.ln.Addr().String())
	return host, port
}

// A host saved in the panel after boot is used by the very next mail, and a
// second change moves the next mail again - no restart, no re-registration.
func TestLiveSMTPSender_NextSendUsesTheNewConfig(t *testing.T) {
	ctx := context.Background()
	repo := fakes.NewFakeServerConfigRepository()
	bus := fakes.NewFakeEventBus()
	live := appconfig.NewLiveSMTP(ctx, repo, appconfig.SMTPSettings{Port: 587, TLS: "none"}, false)
	live.Watch(bus)
	snd := NewLiveSMTPSender(live, nil)
	snd.timeout = 2 * time.Second

	if err := snd.Send([]string{"a@test.local"}, "x", "y"); err != nil {
		t.Fatalf("unconfigured Send must be a silent no-op, got %v", err)
	}

	svc := appconfig.NewService(repo, appconfig.WithEventBus(bus))
	first, second := startScriptedSMTP(t), startScriptedSMTP(t)
	for _, srv := range []*scriptedSMTP{first, second} {
		host, port := srv.hostPort()
		if err := svc.Set(ctx, appconfig.KeySMTPHost, host); err != nil {
			t.Fatal(err)
		}
		if err := svc.Set(ctx, appconfig.KeySMTPPort, port); err != nil {
			t.Fatal(err)
		}
		if err := snd.Send([]string{"a@test.local"}, "Hello", "body"); err != nil {
			t.Fatalf("Send: %v", err)
		}
		srv.mu.Lock()
		got := srv.data
		srv.mu.Unlock()
		if !strings.Contains(got, "Subject: Hello") {
			t.Fatalf("server on port %s did not receive the mail: %q", port, got)
		}
	}
}

func TestLiveSMTPSender_SendTest(t *testing.T) {
	ctx := context.Background()
	repo := fakes.NewFakeServerConfigRepository()
	live := appconfig.NewLiveSMTP(ctx, repo, appconfig.SMTPSettings{TLS: "none"}, false)
	snd := NewLiveSMTPSender(live, func() string { return "Home" })
	snd.timeout = 2 * time.Second

	if err := snd.SendTest(ctx, "admin@test.local"); !errors.Is(err, appconfig.ErrSMTPNotConfigured) {
		t.Fatalf("no host: want ErrSMTPNotConfigured, got %v", err)
	}

	srv := startScriptedSMTP(t)
	host, port := srv.hostPort()
	// Written straight to the store with no event: SendTest must re-read it.
	_ = repo.Set(ctx, appconfig.KeySMTPHost, host)
	_ = repo.Set(ctx, appconfig.KeySMTPPort, port)
	if err := snd.SendTest(ctx, "admin@test.local"); err != nil {
		t.Fatalf("SendTest: %v", err)
	}
	srv.mu.Lock()
	data := srv.data
	srv.mu.Unlock()
	if !strings.Contains(data, "Subject: Home - Test e-mail") {
		t.Errorf("DATA = %q", data)
	}

	srv.rejectRcpt = "nobody@"
	err := snd.SendTest(ctx, "nobody@test.local")
	if err == nil || !strings.Contains(err.Error(), "550") {
		t.Fatalf("the SMTP refusal must surface verbatim, got %v", err)
	}
}

func TestLiveSMTPSender_DemoSendsNothing(t *testing.T) {
	ctx := context.Background()
	srv := startScriptedSMTP(t)
	host, portStr := srv.hostPort()
	port, _ := strconv.Atoi(portStr)
	live := appconfig.NewLiveSMTP(ctx, nil, appconfig.SMTPSettings{Host: host, Port: port, TLS: "none"}, true)
	snd := NewLiveSMTPSender(live, nil)
	if err := snd.Send([]string{"a@test.local"}, "x", "y"); err != nil {
		t.Fatal(err)
	}
	if err := snd.SendTest(ctx, "a@test.local"); !errors.Is(err, appconfig.ErrSMTPNotConfigured) {
		t.Fatalf("demo: want ErrSMTPNotConfigured, got %v", err)
	}
	srv.mu.Lock()
	defer srv.mu.Unlock()
	if srv.data != "" {
		t.Error("the demo must never reach the SMTP server")
	}
}
