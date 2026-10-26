package user

import (
	"context"
	"errors"

	"github.com/eveyproject/evey/internal/domain/shared"
	"github.com/eveyproject/evey/internal/domain/user"
	"github.com/eveyproject/evey/internal/pkg/tracing"
)

// ErrQuickConnectConsumed: the authorized pairing already handed out its session.
var ErrQuickConnectConsumed = errors.New("quick connect already redeemed")

// ClaimQuickConnect returns the request behind code when it is authorized and
// not yet redeemed, marking it consumed in the store; nil when it is still
// pending. The store's conditional write decides: two collectors racing on one
// code (gRPC poll and event stream alike), or a replay after a restart, mint
// one session, not two.
func (s *CommandService) ClaimQuickConnect(ctx context.Context, code string) (*user.QuickConnectRequest, error) {
	ctx, span := tracing.StartSpan(ctx, "app.user.ClaimQuickConnect")
	defer span.End()
	qc, err := s.quickConnect.GetByCode(ctx, code)
	if err != nil {
		return nil, err
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
