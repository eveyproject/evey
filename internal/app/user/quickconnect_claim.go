package user

import (
	"context"
	"crypto/subtle"
	"errors"

	"github.com/eveyproject/evey/internal/domain/shared"
	"github.com/eveyproject/evey/internal/domain/user"
	"github.com/eveyproject/evey/internal/pkg/tracing"
)

// ErrQuickConnectConsumed: the authorized pairing already handed out its session.
var ErrQuickConnectConsumed = errors.New("quick connect already redeemed")

// ErrQuickConnectSecret: the initiator asked for the secret and the collector
// did not present it. Handlers answer as for an unknown code.
var ErrQuickConnectSecret = errors.New("quick connect secret missing or wrong")

// ClaimQuickConnect returns the authorized request behind code and marks it
// consumed in the store, nil while it is still pending. The store's conditional
// write is what makes racing collectors or a replay after a restart mint once.
// When the initiator declared itself secret-aware, secret must match the one
// it was handed: the code is printed on the screen, the secret never is.
func (s *CommandService) ClaimQuickConnect(ctx context.Context, code, secret string) (*user.QuickConnectRequest, error) {
	ctx, span := tracing.StartSpan(ctx, "app.user.ClaimQuickConnect")
	defer span.End()
	qc, err := s.quickConnect.GetByCode(ctx, code)
	if err != nil {
		return nil, err
	}
	if qc.SecretRequired && subtle.ConstantTimeCompare([]byte(secret), []byte(qc.Secret)) != 1 {
		return nil, ErrQuickConnectSecret
	}
	if qc.Status == user.Consumed {
		return nil, ErrQuickConnectConsumed
	}
	if qc.Status != user.Authorized || qc.UserID == nil {
		return nil, nil
	}
	if err := s.quickConnect.Consume(ctx, code); err != nil {
		if errors.Is(err, shared.ErrNotFound) {
			return nil, ErrQuickConnectConsumed
		}
		return nil, err
	}
	return qc, nil
}
