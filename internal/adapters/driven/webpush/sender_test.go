package webpush

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/eveyproject/evey/internal/domain/notification"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) Do(req *http.Request) (*http.Response, error) {
	return f(req)
}

func validKeys(t *testing.T) (pub, priv string) {
	t.Helper()
	priv, pub, err := GenerateVAPIDKeys()
	require.NoError(t, err)
	return pub, priv
}

func newTestSender(t *testing.T, statusCode int) (*Sender, *notification.PushSubscription) {
	t.Helper()
	pub, priv := validKeys(t)
	s := NewSender(pub, priv, "mailto:test@localhost")
	s.httpClient = roundTripFunc(func(_ *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: statusCode,
			Body:       io.NopCloser(strings.NewReader("")),
		}, nil
	})
	sub := &notification.PushSubscription{
		Endpoint: "https://push.example.com/send/abc",
		P256dh:   pub,
		Auth:     "dGVzdGF1dGhrZXkx",
	}
	return s, sub
}

func TestSend_Success(t *testing.T) {
	s, sub := newTestSender(t, http.StatusCreated)
	err := s.Send(context.Background(), *sub, "title", "body", nil)
	assert.NoError(t, err)
}

func TestSend_WithData(t *testing.T) {
	s, sub := newTestSender(t, http.StatusCreated)
	err := s.Send(context.Background(), *sub, "title", "body", map[string]string{"url": "/watch/1"})
	assert.NoError(t, err)
}

func TestSend_Gone410(t *testing.T) {
	s, sub := newTestSender(t, http.StatusGone)
	err := s.Send(context.Background(), *sub, "title", "body", nil)
	require.ErrorIs(t, err, notification.ErrSubscriptionGone)
}

func TestSend_ServerError(t *testing.T) {
	s, sub := newTestSender(t, http.StatusInternalServerError)
	err := s.Send(context.Background(), *sub, "title", "body", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "status 500")
}

func TestSend_BadRequest(t *testing.T) {
	s, sub := newTestSender(t, http.StatusBadRequest)
	err := s.Send(context.Background(), *sub, "title", "body", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "status 400")
}

func TestNewSender(t *testing.T) {
	s := NewSender("pub", "priv", "mailto:x@y")
	assert.Equal(t, "pub", s.vapidPublicKey)
	assert.Equal(t, "priv", s.vapidPrivateKey)
	assert.Equal(t, "mailto:x@y", s.vapidSubject)
}

func TestGenerateVAPIDKeys(t *testing.T) {
	priv, pub, err := GenerateVAPIDKeys()
	require.NoError(t, err)
	assert.NotEmpty(t, priv)
	assert.NotEmpty(t, pub)
}
