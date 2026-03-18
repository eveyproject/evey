package fcm

import (
	"context"
	"testing"
)

func TestNewSender_MissingCredentials(t *testing.T) {
	_, err := NewSender(context.Background(), "/nonexistent/credentials.json")
	if err == nil {
		t.Fatal("expected error for missing credentials file")
	}
}

func TestSendToTokens_EmptyTokens(t *testing.T) {
	s := &Sender{} // nil client, should not be called
	err := s.SendToTokens(context.Background(), nil, "title", "body", nil)
	if err != nil {
		t.Fatalf("expected nil error for empty tokens, got %v", err)
	}
}

func TestSendToTokens_EmptySlice(t *testing.T) {
	s := &Sender{}
	err := s.SendToTokens(context.Background(), []string{}, "title", "body", map[string]string{"k": "v"})
	if err != nil {
		t.Fatalf("expected nil error for empty slice, got %v", err)
	}
}
