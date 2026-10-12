package playback

import (
	"github.com/eveyproject/evey/internal/domain/playback"
	"github.com/eveyproject/evey/internal/domain/shared"
	"github.com/eveyproject/evey/internal/domain/user"
)

// Service bundles the playback CommandService and QueryService so wiring stays
// terse. The bundle has no behaviour of its own - callers reach the inner
// services via CommandService()/QueryService(). Earlier delegating methods
// were dropped after deadcode confirmed they had no callers.
type Service struct {
	cmd   *CommandService
	query *QueryService
}

func NewService(
	progressCmd playback.ProgressCommandRepo,
	progressQuery playback.ProgressQueryRepo,
	sessionCmd playback.SessionCommandRepo,
	sessionQuery playback.SessionQueryRepo,
	users user.UserRepository,
	profiles user.ProfileRepository,
	events shared.EventBus,
	opts ...CommandServiceOption,
) *Service {
	cmd := NewCommandService(progressCmd, sessionCmd, sessionQuery, users, profiles, events, opts...)
	q := NewQueryService(progressQuery, sessionQuery)
	return &Service{cmd: cmd, query: q}
}

func (s *Service) CommandService() *CommandService { return s.cmd }
func (s *Service) QueryService() *QueryService     { return s.query }
