// Package mediarequest is the application service for user media requests:
// title search, submission (policy-gated), the requester's own list, and the
// operator moderation + per-user access surface. Notifications are decoupled -
// the service only publishes domain events; the notification service reacts.
package mediarequest

import (
	"context"
	"errors"
	"log/slog"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/eveyproject/evey/internal/domain/mediarequest"
	"github.com/eveyproject/evey/internal/domain/metadata"
	"github.com/eveyproject/evey/internal/domain/shared"
	"github.com/eveyproject/evey/internal/pkg/id"
	"github.com/eveyproject/evey/internal/pkg/tracing"
)

// Candidate is a title the requester can pick, resolved from a metadata provider.
type Candidate struct {
	Provider   string
	ProviderID string
	Title      string
	Year       *int
	PosterURL  string
	Overview   string
	// ReleaseDate is the provider's premiere date; Rating its community score
	// on a 0-10 scale. Either is nil when the provider had none.
	ReleaseDate *time.Time
	Rating      *float64
	// InLibrary is set when this exact title is already on the server, matched
	// by provider id. MediaItemID then points at it so the client can send the
	// user to the media instead of offering a pointless request.
	InLibrary   bool
	MediaItemID string
}

// LibraryLookup answers "which of these provider ids do we already have?".
// Optional: a nil lookup leaves every candidate reporting InLibrary=false,
// which is the pre-feature behaviour rather than a wrong answer.
type LibraryLookup interface {
	// FindByProviderIDs maps "<provider>:<id>" to the media item id holding it,
	// restricted to mediaType ("movie" or "tv"). Absent keys mean "not held".
	FindByProviderIDs(ctx context.Context, mediaType string, keys []string) (map[string]string, error)
}

// ProviderKey is the lookup key shared by the service and its adapters.
func ProviderKey(provider, id string) string { return provider + ":" + id }

// CreateInput carries the picked title into CreateRequest.
type CreateInput struct {
	MediaType  mediarequest.MediaType
	Provider   string
	ProviderID string
	Title      string
	Year       *int
	PosterURL  string
	// Copied by the client from the Candidate it picked; sanitised on create
	// (the client is not trusted to keep them short or in range).
	Overview    string
	ReleaseDate *time.Time
	Rating      *float64
	// HeldVisible says whether the caller may see the library item a held
	// title resolves to; nil means every held title counts. Without it a
	// profile that cannot see a title (library not granted, parental rating)
	// learned it exists from the "already held" refusal.
	HeldVisible func(ctx context.Context, mediaItemID string) bool
}

// maxOverviewRunes bounds the synopsis a client may attach: TMDB's longest
// overviews sit around 1000 characters, so this is generous without letting a
// crafted request park a novel in the operator's queue.
const maxOverviewRunes = 2000

// Service orchestrates media requests.
type Service struct {
	repo      mediarequest.Repository
	access    mediarequest.Access
	providers []metadata.MetadataProvider // movie/tv (TMDB/OMDb/IMDb) + music (MusicBrainz)
	events    shared.EventBus
	library   LibraryLookup // optional
}

// WithLibraryLookup enables the "already on the server" resolution.
func (s *Service) WithLibraryLookup(l LibraryLookup) *Service {
	s.library = l
	return s
}

// NewService builds the media request service.
func NewService(repo mediarequest.Repository, access mediarequest.Access, providers []metadata.MetadataProvider, events shared.EventBus) *Service {
	return &Service{repo: repo, access: access, providers: providers, events: events}
}

// CanRequest resolves the effective policy for a user: an explicit per-user
// override wins, otherwise the global default.
func (s *Service) CanRequest(ctx context.Context, userID string) (bool, error) {
	ctx, span := tracing.StartSpan(ctx, "app.mediarequest.CanRequest")
	defer span.End()
	if v, ok, err := s.access.Override(ctx, userID); err != nil {
		return false, err
	} else if ok {
		return v, nil
	}
	return s.access.GlobalDefault(ctx)
}

// SearchTitles resolves candidate titles for a media type across the providers.
// Provider errors (e.g. TMDB rejecting a music query) are ignored so one bad
// provider never sinks the search.
func (s *Service) SearchTitles(ctx context.Context, userID string, mediaType mediarequest.MediaType, query string, year *int) ([]Candidate, error) {
	ctx, span := tracing.StartSpan(ctx, "app.mediarequest.SearchTitles")
	defer span.End()
	// Same policy gate as CreateRequest: search only exists to feed a request,
	// and every call spends the operator's own provider quota. Authentication
	// alone is not enough - a user explicitly denied request access could
	// otherwise still drive unbounded upstream searches through their API key.
	allowed, err := s.CanRequest(ctx, userID)
	if err != nil {
		return nil, err
	}
	if !allowed {
		return nil, mediarequest.ErrDisabled
	}
	if !mediaType.IsValid() {
		return nil, errors.New("invalid media type")
	}
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, nil
	}
	var out []Candidate
	for _, p := range s.providers {
		results, err := p.Search(ctx, query, year, string(mediaType))
		if err != nil {
			continue
		}
		for _, r := range results {
			out = append(out, toCandidate(p.Name(), r))
		}
	}
	s.markInLibrary(ctx, mediaType, out)
	return out, nil
}

// markInLibrary flags the candidates already held by the server. Best-effort by
// design: a lookup failure leaves every candidate unflagged (the request stays
// possible) rather than sinking a search that is otherwise perfectly usable.
// Music is skipped outright - music_tracks carries no MusicBrainz id, so the
// only join available would be on the album title, and a wrong "you already
// have this" is worse than no badge at all.
func (s *Service) markInLibrary(ctx context.Context, mediaType mediarequest.MediaType, cands []Candidate) {
	if s.library == nil || len(cands) == 0 || mediaType == mediarequest.MediaTypeMusic {
		return
	}
	keys := make([]string, 0, len(cands))
	for _, c := range cands {
		if c.ProviderID != "" {
			keys = append(keys, ProviderKey(c.Provider, c.ProviderID))
		}
	}
	if len(keys) == 0 {
		return
	}
	held, err := s.library.FindByProviderIDs(ctx, string(mediaType), keys)
	if err != nil {
		slog.WarnContext(ctx, "media request: library lookup failed, candidates left unflagged",
			slog.Any("err", err))
		return
	}
	for i := range cands {
		if id, ok := held[ProviderKey(cands[i].Provider, cands[i].ProviderID)]; ok {
			cands[i].InLibrary = true
			cands[i].MediaItemID = id
		}
	}
}

// toCandidate maps a provider result to a UI-facing candidate, picking the
// provider id that matches the source and the first artwork as the poster.
func toCandidate(providerName string, r metadata.MetadataResult) Candidate {
	c := Candidate{
		Provider:    providerName,
		Title:       r.Title,
		Year:        r.Year,
		Overview:    r.Overview,
		ReleaseDate: r.PremiereDate,
		Rating:      r.CommunityRating,
	}
	// Prefer the id keyed by the provider's own name, else any known id.
	if r.ProviderIDs != nil {
		if v, ok := r.ProviderIDs[providerName]; ok {
			c.ProviderID = v
		} else {
			for _, k := range []string{"tmdb", "imdb", "musicbrainz"} {
				if v, ok := r.ProviderIDs[k]; ok {
					c.Provider = k
					c.ProviderID = v
					break
				}
			}
		}
	}
	if len(r.Artwork) > 0 {
		c.PosterURL = r.Artwork[0].URL
	}
	return c
}

// heldInLibrary reports whether the server already holds this provider id.
// Fails OPEN on a lookup error: refusing a legitimate request because a
// database hiccup is worse than accepting a duplicate the admin can reject.
func (s *Service) heldInLibrary(ctx context.Context, mt mediarequest.MediaType, provider, providerID string) (string, bool) {
	if s.library == nil || mt == mediarequest.MediaTypeMusic {
		return "", false
	}
	held, err := s.library.FindByProviderIDs(ctx, string(mt), []string{ProviderKey(provider, providerID)})
	if err != nil {
		slog.WarnContext(ctx, "media request: library lookup failed on create, allowing", slog.Any("err", err))
		return "", false
	}
	id, ok := held[ProviderKey(provider, providerID)]
	return id, ok
}

// CreateRequest submits a request for the given profile/user. Fails closed with
// ErrDisabled when the policy denies it, and ErrDuplicate when an open request
// for the same title already exists.
func (s *Service) CreateRequest(ctx context.Context, profileID shared.ProfileID, userID string, in CreateInput) (*mediarequest.Request, error) {
	ctx, span := tracing.StartSpan(ctx, "app.mediarequest.CreateRequest")
	defer span.End()

	allowed, err := s.CanRequest(ctx, userID)
	if err != nil {
		return nil, err
	}
	if !allowed {
		return nil, mediarequest.ErrDisabled
	}

	if in.ProviderID != "" {
		dupe, dupeErr := s.repo.HasOpenRequest(ctx, string(profileID), in.MediaType, in.ProviderID)
		if dupeErr != nil {
			return nil, dupeErr
		}
		if dupe {
			return nil, mediarequest.ErrDuplicate
		}
		// Re-resolved server-side: the UI hides the button for a held title, but
		// a crafted call must not slip a request for something already on the
		// server past it.
		// A title the caller may not see is, to them, not on the server: the
		// request goes through and the admin sees the duplicate, instead of
		// the refusal telling the caller what the library holds.
		if id, ok := s.heldInLibrary(ctx, in.MediaType, in.Provider, in.ProviderID); ok && (in.HeldVisible == nil || in.HeldVisible(ctx, id)) {
			return nil, mediarequest.ErrAlreadyHeld
		}
	}

	opts := []mediarequest.Option{}
	if in.Year != nil {
		opts = append(opts, mediarequest.WithYear(*in.Year))
	}
	if p := sanitizePosterURL(in.PosterURL); p != "" {
		opts = append(opts, mediarequest.WithPosterURL(p))
	}
	if in.Provider != "" || in.ProviderID != "" {
		opts = append(opts, mediarequest.WithProvider(in.Provider, in.ProviderID))
	}
	if o := truncateRunes(strings.TrimSpace(in.Overview), maxOverviewRunes); o != "" {
		opts = append(opts, mediarequest.WithOverview(o))
	}
	if in.ReleaseDate != nil && !in.ReleaseDate.IsZero() {
		opts = append(opts, mediarequest.WithReleaseDate(*in.ReleaseDate))
	}
	// A rating outside the 0-10 scale is a client bug, not data: dropped, not
	// clamped, so the operator never sees a confident number that was never
	// given. Zero means "unrated" on every provider we read, so it is not kept.
	if in.Rating != nil && *in.Rating > 0 && *in.Rating <= 10 {
		opts = append(opts, mediarequest.WithRating(*in.Rating))
	}

	req, err := mediarequest.NewRequest(id.New("req-"), profileID, userID, in.MediaType, strings.TrimSpace(in.Title), opts...)
	if err != nil {
		return nil, err
	}
	if err := s.repo.Create(ctx, req); err != nil {
		return nil, err
	}

	evt := mediarequest.RequestCreated{
		RequestID:  req.ID,
		ProfileID:  req.ProfileID,
		UserID:     req.UserID,
		MediaType:  req.MediaType,
		Title:      req.Title,
		OccurredAt: req.CreatedAt,
	}
	if err := s.events.Publish(ctx, evt); err != nil {
		slog.WarnContext(ctx, "publish event", slog.String("event", evt.EventName()), slog.Any("err", err))
	}
	return req, nil
}

// ListMyRequests returns the caller's own requests, newest first.
func (s *Service) ListMyRequests(ctx context.Context, profileID string, limit, offset int) ([]*mediarequest.Request, error) {
	ctx, span := tracing.StartSpan(ctx, "app.mediarequest.ListMyRequests")
	defer span.End()
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	return s.repo.ListByProfile(ctx, profileID, limit, offset)
}

// ListAll is the operator moderation view, optionally filtered by status.
func (s *Service) ListAll(ctx context.Context, statuses []mediarequest.Status, limit, offset int) ([]*mediarequest.Request, error) {
	ctx, span := tracing.StartSpan(ctx, "app.mediarequest.ListAll")
	defer span.End()
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	return s.repo.ListAll(ctx, statuses, limit, offset)
}

// Answer records the operator's decision + note and notifies the requester.
func (s *Service) Answer(ctx context.Context, requestID string, status mediarequest.Status, note string, answeredBy shared.ProfileID) (*mediarequest.Request, error) {
	ctx, span := tracing.StartSpan(ctx, "app.mediarequest.Answer")
	defer span.End()
	if !status.IsAnswer() {
		return nil, errors.New("answer requires approved, rejected or later")
	}
	req, err := s.repo.GetByID(ctx, requestID)
	if err != nil {
		return nil, err
	}
	if err := req.Answer(status, strings.TrimSpace(note), answeredBy); err != nil {
		return nil, err
	}
	if err := s.repo.Update(ctx, req); err != nil {
		return nil, err
	}
	evt := mediarequest.RequestAnswered{
		RequestID:  req.ID,
		ProfileID:  req.ProfileID,
		Title:      req.Title,
		Status:     req.Status,
		Note:       req.AdminNote,
		OccurredAt: *req.AnsweredAt,
	}
	if err := s.events.Publish(ctx, evt); err != nil {
		slog.WarnContext(ctx, "publish event", slog.String("event", evt.EventName()), slog.Any("err", err))
	}
	return req, nil
}

// AccessSettings is the operator view of the request policy.
type AccessSettings struct {
	DefaultEnabled bool
	Overrides      map[string]bool // userID -> enabled
}

// GetAccessSettings returns the global default and every per-user override.
func (s *Service) GetAccessSettings(ctx context.Context) (AccessSettings, error) {
	ctx, span := tracing.StartSpan(ctx, "app.mediarequest.GetAccessSettings")
	defer span.End()
	def, err := s.access.GlobalDefault(ctx)
	if err != nil {
		return AccessSettings{}, err
	}
	overrides, err := s.access.ListOverrides(ctx)
	if err != nil {
		return AccessSettings{}, err
	}
	return AccessSettings{DefaultEnabled: def, Overrides: overrides}, nil
}

// SetDefault sets the global default policy.
func (s *Service) SetDefault(ctx context.Context, enabled bool) error {
	ctx, span := tracing.StartSpan(ctx, "app.mediarequest.SetDefault")
	defer span.End()
	return s.access.SetGlobalDefault(ctx, enabled)
}

// AccessMode is the tri-state a per-user override can take.
type AccessMode int

const (
	AccessDefault AccessMode = iota // clear the override
	AccessEnabled
	AccessDisabled
)

// SetUserAccess sets or clears a per-user override.
func (s *Service) SetUserAccess(ctx context.Context, userID string, mode AccessMode) error {
	ctx, span := tracing.StartSpan(ctx, "app.mediarequest.SetUserAccess")
	defer span.End()
	if userID == "" {
		return errors.New("user id required")
	}
	switch mode {
	case AccessDefault:
		return s.access.ClearOverride(ctx, userID)
	case AccessEnabled:
		return s.access.SetOverride(ctx, userID, true)
	case AccessDisabled:
		return s.access.SetOverride(ctx, userID, false)
	}
	return errors.New("invalid access mode")
}

// trustedPosterHosts are the artwork CDNs our metadata providers actually
// emit (tmdb_mappers.go, musicbrainz_provider.go, and OMDB's IMDb poster
// passthrough). The poster URL arrives from the CLIENT at creation and is
// later rendered on the ADMIN's device: anything outside this set is dropped
// - the poster is cosmetic, and an attacker-chosen host must never see the
// admin's IP.
var trustedPosterHosts = map[string]struct{}{
	"image.tmdb.org":      {},
	"coverartarchive.org": {},
	"m.media-amazon.com":  {},
	"ia.media-imdb.com":   {},
}

// truncateRunes cuts s to at most n runes, never splitting a character.
func truncateRunes(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	i := 0
	for pos := range s {
		if i == n {
			return s[:pos]
		}
		i++
	}
	return s
}

func sanitizePosterURL(raw string) string {
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" {
		return ""
	}
	if _, ok := trustedPosterHosts[strings.ToLower(u.Hostname())]; !ok {
		return ""
	}
	return raw
}
