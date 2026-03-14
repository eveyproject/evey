package fcm

import (
	"context"
	"log/slog"

	firebase "firebase.google.com/go/v4"
	"firebase.google.com/go/v4/messaging"
	"google.golang.org/api/option"
)

type Sender struct {
	client *messaging.Client
}

func NewSender(ctx context.Context, serviceAccountJSON string) (*Sender, error) {
	opt := option.WithCredentialsFile(serviceAccountJSON)
	app, err := firebase.NewApp(ctx, nil, opt)
	if err != nil {
		return nil, err
	}
	client, err := app.Messaging(ctx)
	if err != nil {
		return nil, err
	}
	return &Sender{client: client}, nil
}

func (s *Sender) SendToTokens(ctx context.Context, tokens []string, title, body string, data map[string]string) error {
	if len(tokens) == 0 {
		return nil
	}
	msg := &messaging.MulticastMessage{
		Tokens: tokens,
		Notification: &messaging.Notification{
			Title: title,
			Body:  body,
		},
		Data: data,
	}
	resp, err := s.client.SendEachForMulticast(ctx, msg)
	if err != nil {
		return err
	}
	if resp.FailureCount > 0 {
		slog.Warn("fcm: some tokens failed", slog.Int("failed", resp.FailureCount), slog.Int("success", resp.SuccessCount))
	}
	return nil
}
