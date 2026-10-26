package user

import (
	"context"
	"errors"
	"time"

	"github.com/eveyproject/evey/internal/domain/user"
	"github.com/eveyproject/evey/internal/pkg/tracing"
)

// ErrQuickConnectConsumed: the authorized pairing already handed out its session.
var ErrQuickConnectConsumed = errors.New("quick connect already redeemed")

// ClaimQuickConnect returns the request behind code when it is authorized and
// not yet redeemed, marking it redeemed; nil when it is still pending. The
// claim is atomic within the process, so two collectors racing on one code
// (gRPC poll and event stream alike) mint one session, not two. The repository
// can only move a request out of "pending", so the redeemed set lives here and
// is kept until the request expires.
func (s *CommandService) ClaimQuickConnect(ctx context.Context, code string) (*user.QuickConnectRequest, error) {
	ctx, span := tracing.StartSpan(ctx, "app.user.ClaimQuickConnect")
	defer span.End()
	s.qcMu.Lock()
	defer s.qcMu.Unlock()
	now := time.Now()
	for c, exp := range s.qcClaimed {
		if now.After(exp) {
			delete(s.qcClaimed, c)
		}
	}
	if _, done := s.qcClaimed[code]; done {
		return nil, ErrQuickConnectConsumed
	}
	qc, err := s.quickConnect.GetByCode(ctx, code)
	if err != nil {
		return nil, err
	}
	if qc.Status != user.Authorized || qc.UserID == nil {
		return nil, nil
	}
	if s.qcClaimed == nil {
		s.qcClaimed = make(map[string]time.Time)
	}
	s.qcClaimed[code] = qc.ExpiresAt
	return qc, nil
}
