package config

import (
	"context"
	"errors"
	"testing"

	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/evey-project/evey/internal/domain/shared/fakes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var smtpEnv = SMTPSettings{Host: "env.example", Port: 587, Username: "envuser", Password: "envpass", From: "env@example", TLS: "starttls"}

func TestResolveSMTPSettings_PanelHostTakesEveryFieldFromThePanel(t *testing.T) {
	t.Parallel()
	got := ResolveSMTPSettings(map[string]string{
		KeySMTPHost: "panel.example",
		KeySMTPPort: "465",
		KeySMTPTLS:  "SSL",
		KeySMTPFrom: "  ",
	}, smtpEnv)
	// The env credentials must never reach a host typed in the panel.
	assert.Equal(t, SMTPSettings{Host: "panel.example", Port: 465, TLS: "ssl"}, got)
}

func TestResolveSMTPSettings_NoPanelHostUsesEnvWhole(t *testing.T) {
	t.Parallel()
	got := ResolveSMTPSettings(map[string]string{KeySMTPUsername: "panel-user", KeySMTPPort: "2525"}, smtpEnv)
	assert.Equal(t, smtpEnv.Host, got.Host)
	assert.Equal(t, smtpEnv.Username, got.Username)
	assert.Equal(t, smtpEnv.Port, got.Port)
}

func TestResolveSMTPSettings_PanelDefaults(t *testing.T) {
	t.Parallel()
	got := ResolveSMTPSettings(map[string]string{KeySMTPHost: "panel.example", KeySMTPPort: "abc"}, smtpEnv)
	assert.Equal(t, 587, got.Port)
	assert.Equal(t, "starttls", got.TLS)
	assert.Equal(t, 587, ResolveSMTPSettings(map[string]string{KeySMTPHost: "h", KeySMTPPort: "70000"}, smtpEnv).Port)
}

func TestResolveSMTPSettings_EmptyEverywhereIsNotConfigured(t *testing.T) {
	t.Parallel()
	assert.False(t, ResolveSMTPSettings(nil, SMTPSettings{Port: 587}).Configured())
}

func TestLiveSMTP_FollowsPanelChangesLive(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := fakes.NewFakeServerConfigRepository()
	bus := fakes.NewFakeEventBus()
	live := NewLiveSMTP(ctx, repo, SMTPSettings{Port: 587, TLS: "starttls"}, false)
	live.Watch(bus)
	assert.False(t, live.Configured(), "no host anywhere")

	svc := NewService(repo, WithEventBus(bus))
	require.NoError(t, svc.Set(ctx, KeySMTPHost, "first.example"))
	assert.True(t, live.Configured(), "a host saved in the panel applies without a restart")
	assert.Equal(t, "first.example", live.Current().Host)

	require.NoError(t, svc.Set(ctx, KeySMTPHost, "second.example"))
	assert.Equal(t, "second.example", live.Current().Host)

	require.NoError(t, svc.Delete(ctx, KeySMTPHost))
	assert.False(t, live.Configured(), "clearing the panel host falls back to the (empty) env")
}

func TestLiveSMTP_StoreAtBootWinsOverEnv(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := fakes.NewFakeServerConfigRepository()
	require.NoError(t, repo.Set(ctx, KeySMTPHost, "panel.example"))
	live := NewLiveSMTP(ctx, repo, smtpEnv, false)
	assert.Equal(t, "panel.example", live.Current().Host)
	assert.Empty(t, live.Current().Username, "a panel host never borrows env credentials")
}

func TestLiveSMTP_DisabledNeverConfigured(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := fakes.NewFakeServerConfigRepository()
	require.NoError(t, repo.Set(ctx, KeySMTPHost, "panel.example"))
	live := NewLiveSMTP(ctx, repo, smtpEnv, true)
	assert.False(t, live.Configured(), "the demo sends no mail whatever the panel or env say")
	assert.Equal(t, SMTPSettings{}, live.Reload(ctx))
}

type failingListRepo struct{ shared.ServerConfigRepository }

func (failingListRepo) List(context.Context) ([]*shared.ServerConfigEntry, error) {
	return nil, errors.New("db down")
}

func TestLiveSMTP_UnreadableStoreKeepsCurrent(t *testing.T) {
	t.Parallel()
	live := NewLiveSMTP(context.Background(), failingListRepo{}, smtpEnv, false)
	assert.Equal(t, "env.example", live.Current().Host, "boot with an unreadable store keeps env")
	assert.True(t, IsSMTPKey(KeySMTPPassword))
	assert.False(t, IsSMTPKey("server.name"))
}
