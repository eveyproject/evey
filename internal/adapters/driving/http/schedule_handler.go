package httpapi

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"go.opentelemetry.io/otel/codes"

	appuser "github.com/eveyproject/evey/internal/app/user"
	"github.com/eveyproject/evey/internal/domain/shared"
	domainuser "github.com/eveyproject/evey/internal/domain/user"
	"github.com/eveyproject/evey/internal/pkg/tracing"
)

// scheduleHandler serves a kids profile's opening hours: read them (the kids
// profile itself may, to show when it opens), change them (an adult profile of
// the account or an admin), and lift them until the next opening with a
// parent's PIN. It shares the parental handler's PIN rate limit.
type scheduleHandler struct {
	parental *parentalHandler
	hours    *appuser.ProfileSchedules
}

type scheduleRangeJSON struct {
	Start int `json:"start"`
	End   int `json:"end"`
}

type scheduleRuleJSON struct {
	Days   []int               `json:"days"`
	Ranges []scheduleRangeJSON `json:"ranges"`
}

type scheduleJSON struct {
	Enabled bool               `json:"enabled"`
	Mode    string             `json:"mode"`
	Rules   []scheduleRuleJSON `json:"rules"`
}

type scheduleAccessJSON struct {
	Open       bool    `json:"open"`
	Overridden bool    `json:"overridden"`
	ClosesAt   *string `json:"closesAt,omitempty"`
	NextOpen   *string `json:"nextOpen,omitempty"`
}

type scheduleResponse struct {
	IsKids bool `json:"isKids"`
	// Schedule is nil when the profile has no opening hours.
	Schedule *scheduleJSON `json:"schedule"`
	// Proposal is what the editor offers when there are none yet.
	Proposal scheduleJSON       `json:"proposal"`
	Access   scheduleAccessJSON `json:"access"`
	Timezone string             `json:"timezone"`
}

func scheduleToJSON(s domainuser.AccessSchedule) scheduleJSON {
	out := scheduleJSON{Enabled: s.Enabled, Mode: string(s.Mode), Rules: make([]scheduleRuleJSON, 0, len(s.Rules))}
	for _, r := range s.Rules {
		rj := scheduleRuleJSON{Days: make([]int, 0, len(r.Days)), Ranges: make([]scheduleRangeJSON, 0, len(r.Ranges))}
		for _, d := range r.Days {
			rj.Days = append(rj.Days, int(d))
		}
		for _, tr := range r.Ranges {
			rj.Ranges = append(rj.Ranges, scheduleRangeJSON(tr))
		}
		out.Rules = append(out.Rules, rj)
	}
	return out
}

func scheduleFromJSON(in scheduleJSON) domainuser.AccessSchedule {
	s := domainuser.AccessSchedule{Enabled: in.Enabled, Mode: domainuser.ScheduleMode(in.Mode), Rules: make([]domainuser.ScheduleRule, 0, len(in.Rules))}
	for _, rj := range in.Rules {
		r := domainuser.ScheduleRule{}
		for _, d := range rj.Days {
			r.Days = append(r.Days, time.Weekday(d))
		}
		for _, tr := range rj.Ranges {
			r.Ranges = append(r.Ranges, domainuser.TimeRange(tr))
		}
		s.Rules = append(s.Rules, r)
	}
	return s
}

func rfc3339OrNil(t time.Time) *string {
	if t.IsZero() {
		return nil
	}
	v := t.UTC().Format(time.RFC3339)
	return &v
}

func (h *scheduleHandler) respond(w http.ResponseWriter, p *domainuser.Profile, acc domainuser.ProfileAccess) {
	out := scheduleResponse{
		IsKids:   p.IsKids,
		Proposal: scheduleToJSON(domainuser.DefaultKidsSchedule()),
		Access: scheduleAccessJSON{
			Open: acc.Open, Overridden: acc.Overridden,
			ClosesAt: rfc3339OrNil(acc.ClosesAt), NextOpen: rfc3339OrNil(acc.NextOpen),
		},
		Timezone: h.hours.Location().String(),
	}
	if p.Schedule != nil {
		sj := scheduleToJSON(*p.Schedule)
		out.Schedule = &sj
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	if err := json.NewEncoder(w).Encode(out); err != nil {
		slog.Warn("json encode failed", slog.Any("err", err))
	}
}

// load reads the target profile and checks the caller owns it (or is admin).
func (h *scheduleHandler) load(w http.ResponseWriter, r *http.Request) (*domainuser.Profile, *domainuser.Claims, bool) {
	claims := HTTPClaimsFromContext(r.Context())
	p, err := h.parental.profileRepo.GetByID(r.Context(), shared.ProfileID(r.PathValue("id")))
	if err != nil || p == nil {
		http.Error(w, `{"message":"profile not found"}`, http.StatusNotFound)
		return nil, nil, false
	}
	if claims == nil || (!claims.IsAdmin && string(p.UserID) != claims.UserID) {
		http.Error(w, `{"message":"forbidden"}`, http.StatusForbidden)
		return nil, nil, false
	}
	return p, claims, true
}

func (h *scheduleHandler) get(w http.ResponseWriter, r *http.Request) {
	ctx, span := tracing.StartSpan(r.Context(), "http.schedule.Get")
	defer span.End()
	r = r.WithContext(ctx)
	p, _, ok := h.load(w, r)
	if !ok {
		return
	}
	h.respond(w, p, p.AccessAt(time.Now(), h.hours.Location()))
}

func (h *scheduleHandler) update(w http.ResponseWriter, r *http.Request) {
	ctx, span := tracing.StartSpan(r.Context(), "http.schedule.Update")
	defer span.End()
	r = r.WithContext(ctx)
	p, claims, ok := h.load(w, r)
	if !ok {
		return
	}
	// Same stance as the parental settings: a kids profile never edits its
	// own hours, nor a sibling's.
	if !h.parental.refuseFromKidsProfile(w, r, span, claims, p) {
		return
	}
	var body struct {
		Schedule *scheduleJSON `json:"schedule"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, `{"message":"bad request"}`, http.StatusBadRequest)
		return
	}
	var sched *domainuser.AccessSchedule
	if body.Schedule != nil {
		s := scheduleFromJSON(*body.Schedule)
		sched = &s
	}
	if err := h.hours.SetSchedule(r.Context(), p, sched); err != nil {
		span.RecordError(err)
		switch {
		case errors.Is(err, appuser.ErrScheduleNotKids):
			http.Error(w, `{"message":"errors.profile.schedule_kids_only"}`, http.StatusBadRequest)
		case errors.Is(err, domainuser.ErrInvalidSchedule):
			writeJSONError(w, http.StatusBadRequest, "errors.profile.schedule_invalid", err.Error())
		default:
			span.SetStatus(codes.Error, "save schedule")
			logInternal(r.Context(), "schedule: save", err)
			http.Error(w, `{"message":"internal error"}`, http.StatusInternalServerError)
		}
		return
	}
	updated, acc, err := h.hours.Schedule(r.Context(), p.ID)
	if err != nil {
		logInternal(r.Context(), "schedule: read back", err)
		http.Error(w, `{"message":"internal error"}`, http.StatusInternalServerError)
		return
	}
	h.respond(w, updated, acc)
}

func (h *scheduleHandler) unlock(w http.ResponseWriter, r *http.Request) {
	ctx, span := tracing.StartSpan(r.Context(), "http.schedule.Unlock")
	defer span.End()
	r = r.WithContext(ctx)
	p, _, ok := h.load(w, r)
	if !ok {
		return
	}
	var body struct {
		PIN string `json:"pin"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<10)
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, `{"message":"bad request"}`, http.StatusBadRequest)
		return
	}
	if !h.parental.pinRL.Allow(string(p.ID)) {
		recordParentalPINThrottle(r, "schedule_unlock_pin", string(p.ID))
		http.Error(w, `{"message":"errors.auth.too_many_attempts"}`, http.StatusTooManyRequests)
		return
	}
	if _, err := h.hours.Unlock(r.Context(), p, body.PIN); err != nil {
		span.RecordError(err)
		switch {
		case errors.Is(err, appuser.ErrWrongParentPIN):
			http.Error(w, `{"message":"errors.profile.wrong_parent_pin"}`, http.StatusForbidden)
		case errors.Is(err, appuser.ErrNoParentPIN):
			http.Error(w, `{"message":"errors.profile.no_parent_pin"}`, http.StatusConflict)
		default:
			logInternal(r.Context(), "schedule: unlock", err)
			http.Error(w, `{"message":"internal error"}`, http.StatusInternalServerError)
		}
		return
	}
	slog.InfoContext(r.Context(), "kids profile opened by a parent PIN", slog.String("profile_id", string(p.ID)))
	updated, acc, err := h.hours.Schedule(r.Context(), p.ID)
	if err != nil {
		logInternal(r.Context(), "schedule: read back", err)
		http.Error(w, `{"message":"internal error"}`, http.StatusInternalServerError)
		return
	}
	h.respond(w, updated, acc)
}

func writeJSONError(w http.ResponseWriter, status int, key, detail string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(map[string]string{"message": key, "detail": detail}); err != nil {
		slog.Warn("json encode failed", slog.Any("err", err))
	}
}

func registerScheduleRoutes(mux *http.ServeMux, parental *parentalHandler, hours *appuser.ProfileSchedules) {
	if parental == nil || hours == nil {
		return
	}
	authMw := httpAuthMiddleware(parental.tokenGen)
	h := &scheduleHandler{parental: parental, hours: hours}
	mux.HandleFunc("GET /api/v1/profiles/{id}/schedule", authMw(h.get))
	mux.HandleFunc("PUT /api/v1/profiles/{id}/schedule", authMw(h.update))
	mux.HandleFunc("POST /api/v1/profiles/{id}/schedule/unlock", authMw(h.unlock))
}
