package grpcapi

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"

	eveyv1 "github.com/eveyproject/evey/gen/evey/v1"
	"github.com/eveyproject/evey/internal/adapters/driving/grpcapi/interceptors"
	appuser "github.com/eveyproject/evey/internal/app/user"
	"github.com/eveyproject/evey/internal/domain/shared"
	"github.com/eveyproject/evey/internal/domain/user"
	"github.com/eveyproject/evey/internal/pkg/auth"
	"github.com/eveyproject/evey/internal/pkg/id"
	"github.com/eveyproject/evey/internal/pkg/tracing"
	"go.opentelemetry.io/otel/attribute"
	otelcodes "go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// user_handler_profile.go holds the profile RPCs and their shared helpers: the
// parental-update gate, the restricted-profile refusal and the field/persist
// steps behind create / update / set-default / delete / list.

func (h *userHandler) CreateProfile(ctx context.Context, req *eveyv1.CreateProfileRequest) (*eveyv1.Profile, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.user.CreateProfile")
	defer span.End()
	claims, err := requireClaims(ctx)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, fmt.Errorf("userHandler.CreateProfile: %w", err)
	}
	// Another account's profiles are admin work an API key never gets
	// (interceptors.AdminReach), same as below in update and delete.
	if !interceptors.AdminReach(ctx, auth.ScopeAdminWrite) && req.UserId != claims.UserID {
		span.SetStatus(otelcodes.Error, "cannot create profile for other")
		return nil, h.translator.TranslatedError(ctx, codes.PermissionDenied, "errors.user.cannot_create_profile_other")
	}
	// Profile management is an adult action. Without this, a kids profile could
	// create a fresh non-kids profile WITH A PIN OF ITS OWN CHOOSING and then
	// switch onto it: RefreshToken's switch gate accepts the target's own PIN as
	// proof, on the stated assumption that "a kid cannot know the adult
	// profile's PIN" - which does not hold for a profile the kid just created.
	// That defeated parental control account-wide without the parent's PIN.
	// Same signal as enforceParentalUpdateGate: the active PROFILE, not admin.
	if gateErr := h.refuseFromRestrictedProfile(ctx, span, claims); gateErr != nil {
		return nil, gateErr
	}
	opts := []user.ProfileOption{
		user.WithProfilePreferences(req.Preferences),
		user.WithProfileIsKids(req.IsKids),
	}
	// req.AvatarPath is ignored: only the upload handler sets the avatar, at a
	// path it derives from the profile ID. A client path could name another
	// profile's file, which the next upload deletes as "the old avatar".
	if req.MaxRating != "" {
		opts = append(opts, user.WithProfileMaxRating(req.MaxRating))
	}
	if req.Locale != nil {
		opts = append(opts, user.WithProfileLocale(*req.Locale))
	}
	if req.Region != nil && *req.Region != "" {
		opts = append(opts, user.WithProfileRegion(strings.ToUpper(*req.Region)))
	}
	parental := &user.ParentalSettings{}
	hasParental := false
	if req.Pin != nil && *req.Pin != "" {
		hash, hErr := h.userSvc.HashPIN(*req.Pin)
		if hErr != nil {
			span.RecordError(hErr)
			span.SetStatus(otelcodes.Error, hErr.Error())
			if errors.Is(hErr, appuser.ErrInvalidPINFormat) {
				return nil, h.translator.TranslatedError(ctx, codes.InvalidArgument, "errors.auth.pin_must_be_4_digits")
			}
			return nil, status.Errorf(codes.InvalidArgument, "%v", hErr)
		}
		parental.PINHash = hash
		hasParental = true
	}
	if req.MaxAge != nil && *req.MaxAge >= 0 {
		age := int(*req.MaxAge)
		parental.MaxAge = &age
		hasParental = true
	}
	if hasParental {
		opts = append(opts, user.WithProfileParental(parental))
	}
	p, err := user.NewProfile(
		shared.ProfileID(id.New("prf-")),
		shared.UserID(req.UserId),
		req.Name,
		opts...,
	)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		switch {
		case req.UserId == "":
			return nil, h.translator.TranslatedError(ctx, codes.InvalidArgument, "errors.user.id_required")
		case req.Name == "":
			return nil, h.translator.TranslatedError(ctx, codes.InvalidArgument, "errors.user.profile_name_required")
		}
		return nil, status.Errorf(codes.InvalidArgument, "%v", err)
	}
	if err := h.userSvc.CreateProfile(ctx, p); err != nil {
		span.RecordError(err)
		switch {
		case errors.Is(err, appuser.ErrMaxProfilesReached):
			span.SetStatus(otelcodes.Error, err.Error())
			return nil, h.translator.TranslatedError(ctx, codes.FailedPrecondition, "errors.user.max_profiles_reached")
		case errors.Is(err, appuser.ErrKidsRequiresPIN):
			span.SetStatus(otelcodes.Error, err.Error())
			return nil, h.translator.TranslatedError(ctx, codes.FailedPrecondition, "errors.auth.pin_required_for_non_kids")
		case errors.Is(err, appuser.ErrAdminCannotBeKids):
			span.SetStatus(otelcodes.Error, err.Error())
			return nil, h.translator.TranslatedError(ctx, codes.FailedPrecondition, "errors.user.admin_cannot_be_kids")
		}
		slog.ErrorContext(ctx, "create profile", slog.Any("err", err))
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, status.Errorf(codes.Internal, "internal error")
	}
	return profileToProto(p), nil
}

// loadProfileForUpdate fetches the profile to update and enforces the
// owner/admin gate: a non-admin caller may only touch their own profile. A
// missing profile and a foreign profile both map to the same
// permission-denied response for a non-admin caller, so a probe can't tell
// them apart; an admin gets NotFound instead since only they may legitimately
// look up an arbitrary profile ID.
func (h *userHandler) loadProfileForUpdate(ctx context.Context, span trace.Span, id string, claims *user.Claims) (*user.Profile, error) {
	adminReach := interceptors.AdminReach(ctx, auth.ScopeAdminWrite)
	p, gerr := h.query.GetProfileByID(ctx, id)
	if gerr != nil || p == nil {
		if gerr != nil {
			span.RecordError(gerr)
		}
		if !adminReach {
			span.SetStatus(otelcodes.Error, "cannot update profile other")
			return nil, h.translator.TranslatedError(ctx, codes.PermissionDenied, "errors.user.cannot_update_profile_other")
		}
		span.SetStatus(otelcodes.Error, "profile not found")
		return nil, h.translator.TranslatedError(ctx, codes.NotFound, "errors.auth.profile_not_found")
	}
	if !adminReach && string(p.UserID) != claims.UserID {
		span.SetStatus(otelcodes.Error, "cannot update profile other")
		return nil, h.translator.TranslatedError(ctx, codes.PermissionDenied, "errors.user.cannot_update_profile_other")
	}
	return p, nil
}

// refuseFromRestrictedProfile blocks an action when the caller's ACTIVE profile
// is parental-restricted (kids, or non-kids with parental control and a PIN),
// whatever account it belongs to and even for an admin. Fails closed: if the
// caller's profile cannot be loaded we cannot confirm it is unrestricted.
func (h *userHandler) refuseFromRestrictedProfile(ctx context.Context, span trace.Span, claims *user.Claims) error {
	if claims.ProfileID == "" {
		return nil
	}
	cp, err := h.query.GetProfileByID(ctx, claims.ProfileID)
	if err != nil || cp == nil {
		return internalError(ctx, "parental gate: load caller profile", err)
	}
	if cp.IsParentalRestricted() {
		span.SetStatus(otelcodes.Error, "profile management from restricted profile")
		return h.translator.TranslatedError(ctx, codes.PermissionDenied, "errors.auth.profiles_need_non_kids")
	}
	return nil
}

// enforceParentalUpdateGate blocks parental changes (kids flag, age cap, PIN)
// made FROM a kids profile, and makes any loosening of a PIN-guarded or
// restricted profile prove the current PIN, like the HTTP parental route.
// Not gated on admin: the active PROFILE is the signal.
func (h *userHandler) enforceParentalUpdateGate(ctx context.Context, span trace.Span, req *eveyv1.UpdateProfileRequest, claims *user.Claims, p *user.Profile) error {
	if req.IsKids == nil && req.MaxAge == nil && req.Pin == nil {
		return nil
	}
	var caller *user.Profile
	switch {
	case claims.ProfileID == req.Id:
		caller = p // editing the profile we're on
	case claims.ProfileID != "":
		cp, cerr := h.query.GetProfileByID(ctx, claims.ProfileID)
		if cerr != nil || cp == nil {
			// Fail closed: can't confirm the caller isn't on a kids profile.
			return internalError(ctx, "parental gate: load caller profile", cerr)
		}
		caller = cp
	}
	if caller != nil && caller.IsKids {
		span.SetStatus(otelcodes.Error, "parental change from kids profile")
		return h.translator.TranslatedError(ctx, codes.PermissionDenied, "errors.auth.parental_needs_non_kids")
	}
	if !loosensProfileParental(p, req) {
		return nil
	}
	guard := p
	if !guard.HasPIN() && caller.IsParentalRestricted() {
		guard = caller
	}
	if !guard.HasPIN() {
		return nil
	}
	return h.verifyCurrentPIN(ctx, span, guard, req.GetCurrentPin())
}

// loosensProfileParental reports whether req relaxes what p enforces: the kids
// flag dropped, or the effective age cap cleared or raised.
func loosensProfileParental(p *user.Profile, req *eveyv1.UpdateProfileRequest) bool {
	if req.IsKids != nil && !*req.IsKids && p.IsKids {
		return true
	}
	eff := p.EffectiveParental()
	if req.MaxAge == nil || eff == nil || eff.MaxAge == nil {
		return false
	}
	return *req.MaxAge < 0 || int(*req.MaxAge) > *eff.MaxAge
}

// verifyCurrentPIN checks pin against guard's PIN under the wrong-PIN lockout
// shared with the HTTP parental routes (a 4-digit PIN is 10 000 guesses).
func (h *userHandler) verifyCurrentPIN(ctx context.Context, span trace.Span, guard *user.Profile, pin string) error {
	if strings.TrimSpace(pin) == "" {
		span.SetStatus(otelcodes.Error, "current PIN required")
		return h.translator.TranslatedError(ctx, codes.InvalidArgument, "errors.auth.current_pin_required")
	}
	ok, err := h.userSvc.VerifyProfilePIN(ctx, string(guard.ID), pin, guard.Parental.PINHash)
	if errors.Is(err, appuser.ErrPINLocked) {
		span.SetStatus(otelcodes.Error, "PIN attempts locked out")
		return h.translator.TranslatedError(ctx, codes.ResourceExhausted, "errors.auth.too_many_attempts")
	}
	if err != nil {
		span.RecordError(err)
		return internalError(ctx, "verify current PIN", err)
	}
	if !ok {
		span.SetStatus(otelcodes.Error, "invalid current PIN")
		// PermissionDenied, not Unauthenticated: clients sign out on a 401.
		return h.translator.TranslatedError(ctx, codes.PermissionDenied, "errors.auth.invalid_pin")
	}
	return nil
}

// enforceProfilePINProof is the gRPC twin of the parental route's
// authorizeParentalChange for the PIN itself: a request that sets, rotates or
// clears the PIN proves something first. A profile that already has a PIN
// needs its current PIN (rate-limited per profile, a 4-digit PIN is 10 000
// guesses); a first PIN needs the caller's account password, so a stolen
// access token cannot lock the owner out behind a PIN of the thief's choosing.
// An admin on another user's profile proves their own password in both cases:
// they do not know the user's PIN, and their token alone must not be enough.
// Until now the gRPC route accepted the new PIN on the token alone.
func (h *userHandler) enforceProfilePINProof(ctx context.Context, span trace.Span, req *eveyv1.UpdateProfileRequest, claims *user.Claims, p *user.Profile) error {
	if req.Pin == nil {
		return nil
	}
	hasPIN := p.Parental != nil && p.Parental.PINHash != ""
	if !hasPIN && *req.Pin == "" {
		return nil // nothing to clear, nothing to prove
	}
	if !hasPIN || string(p.UserID) != claims.UserID {
		return h.reauthenticate(ctx, span, claims.UserID, req.GetCurrentPassword())
	}
	return h.verifyCurrentPIN(ctx, span, p, req.GetCurrentPin())
}

// applyProfileUpdateFields patches every non-nil field of req onto p. Fields
// left unset in the request leave the existing column untouched, so p must
// be freshly loaded from storage (see loadProfileForUpdate) before this call.
func (h *userHandler) applyProfileUpdateFields(ctx context.Context, span trace.Span, req *eveyv1.UpdateProfileRequest, p *user.Profile) error {
	if req.Preferences != nil {
		p.Preferences = req.Preferences
	}
	if req.Name != nil {
		p.Name = *req.Name
	}
	// req.AvatarPath is ignored, as in CreateProfile.
	if req.MaxRating != nil {
		p.MaxRating = req.MaxRating
	}
	if req.Locale != nil {
		p.Locale = *req.Locale
	}
	if req.Region != nil {
		if *req.Region == "" {
			p.Region = nil
		} else {
			r := strings.ToUpper(*req.Region)
			p.Region = &r
		}
	}
	if v, ok := p.Preferences["max_bitrate_kbps"]; ok {
		delete(p.Preferences, "max_bitrate_kbps")
		if v == "" || v == "0" {
			p.MaxBitrateKbps = nil
		} else if n, err := strconv.Atoi(v); err == nil && n > 0 {
			p.MaxBitrateKbps = &n
		}
	}
	if req.IsKids != nil {
		p.IsKids = *req.IsKids
	}
	if req.Pin != nil {
		hash, hErr := h.userSvc.HashPIN(*req.Pin)
		if hErr != nil {
			span.RecordError(hErr)
			span.SetStatus(otelcodes.Error, hErr.Error())
			if errors.Is(hErr, appuser.ErrInvalidPINFormat) {
				return h.translator.TranslatedError(ctx, codes.InvalidArgument, "errors.auth.pin_must_be_4_digits")
			}
			return status.Errorf(codes.InvalidArgument, "%v", hErr)
		}
		if p.Parental == nil {
			p.Parental = &user.ParentalSettings{}
		}
		p.Parental.PINHash = hash
	}
	if req.MaxAge != nil {
		if p.Parental == nil {
			p.Parental = &user.ParentalSettings{}
		}
		if *req.MaxAge < 0 {
			p.Parental.MaxAge = nil
		} else {
			age := int(*req.MaxAge)
			p.Parental.MaxAge = &age
		}
	}
	return nil
}

// persistProfileUpdate saves the patched profile and maps storage errors to
// gRPC status codes.
func (h *userHandler) persistProfileUpdate(ctx context.Context, span trace.Span, p *user.Profile) (*eveyv1.Profile, error) {
	if err := h.userSvc.UpdateProfile(ctx, p); err != nil {
		span.RecordError(err)
		if errors.Is(err, appuser.ErrKidsRequiresPIN) {
			span.SetStatus(otelcodes.Error, err.Error())
			return nil, h.translator.TranslatedError(ctx, codes.FailedPrecondition, "errors.auth.pin_required_for_non_kids")
		}
		slog.ErrorContext(ctx, "update profile", slog.Any("err", err))
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, status.Errorf(codes.Internal, "internal error")
	}
	return profileToProto(p), nil
}

func (h *userHandler) UpdateProfile(ctx context.Context, req *eveyv1.UpdateProfileRequest) (*eveyv1.Profile, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.user.UpdateProfile")
	defer span.End()
	claims, err := requireClaims(ctx)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, fmt.Errorf("userHandler.UpdateProfile: %w", err)
	}
	// Always load the existing profile - we patch non-nil fields into it so
	// unset columns (audio/subtitle langs, parental MaxAge/BlockedTags/Enabled,
	// is_default, …) survive. This also populates UserID for the invariant
	// check in UpdateProfile.
	p, err := h.loadProfileForUpdate(ctx, span, req.Id, claims)
	if err != nil {
		return nil, err
	}
	if err := h.enforceParentalUpdateGate(ctx, span, req, claims, p); err != nil {
		return nil, err
	}
	if err := h.enforceProfilePINProof(ctx, span, req, claims, p); err != nil {
		return nil, err
	}
	if err := h.applyProfileUpdateFields(ctx, span, req, p); err != nil {
		return nil, err
	}
	return h.persistProfileUpdate(ctx, span, p)
}

func (h *userHandler) SetDefaultProfile(ctx context.Context, req *eveyv1.SetDefaultProfileRequest) (*eveyv1.Profile, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.user.SetDefaultProfile")
	defer span.End()
	span.SetAttributes(attribute.String("profile.id", req.Id))
	if _, err := requireClaims(ctx); err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, fmt.Errorf("userHandler.SetDefaultProfile: %w", err)
	}
	// RBAC interceptor already gates this to admin; double-check is unnecessary.
	if req.Id == "" {
		span.SetStatus(otelcodes.Error, "missing profile id")
		return nil, h.translator.TranslatedError(ctx, codes.InvalidArgument, "errors.user.profile_id_required")
	}
	updated, err := h.userSvc.SetDefaultProfile(ctx, shared.ProfileID(req.Id))
	if err != nil {
		span.RecordError(err)
		if errors.Is(err, shared.ErrNotFound) {
			span.SetStatus(otelcodes.Error, "profile not found")
			return nil, h.translator.TranslatedError(ctx, codes.NotFound, "errors.auth.profile_not_found")
		}
		slog.ErrorContext(ctx, "set default profile", slog.Any("err", err))
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, status.Errorf(codes.Internal, "internal error")
	}
	return profileToProto(updated), nil
}

func (h *userHandler) DeleteProfile(ctx context.Context, req *eveyv1.DeleteProfileRequest) (*eveyv1.DeleteProfileResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.user.DeleteProfile")
	defer span.End()
	claims, err := requireClaims(ctx)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, fmt.Errorf("userHandler.DeleteProfile: %w", err)
	}
	// Same gate as CreateProfile: managing profiles is an adult action. Owning
	// the account is not enough - a kids profile could otherwise delete its
	// siblings, including the gated adult profile.
	if gateErr := h.refuseFromRestrictedProfile(ctx, span, claims); gateErr != nil {
		return nil, gateErr
	}
	if !interceptors.AdminReach(ctx, auth.ScopeAdminWrite) {
		existing, gerr := h.query.GetProfileByID(ctx, req.Id)
		if gerr != nil || existing == nil || string(existing.UserID) != claims.UserID {
			if gerr != nil {
				span.RecordError(gerr)
			}
			span.SetStatus(otelcodes.Error, "cannot delete profile other")
			return nil, h.translator.TranslatedError(ctx, codes.PermissionDenied, "errors.user.cannot_delete_profile_other")
		}
	}
	if err := h.userSvc.DeleteProfile(ctx, req.Id); err != nil {
		span.RecordError(err)
		if errors.Is(err, appuser.ErrCannotDeleteDefault) {
			span.SetStatus(otelcodes.Error, err.Error())
			return nil, h.translator.TranslatedError(ctx, codes.FailedPrecondition, "errors.user.cannot_delete_default_profile")
		}
		slog.ErrorContext(ctx, "delete profile", slog.Any("err", err))
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, status.Errorf(codes.Internal, "internal error")
	}
	return &eveyv1.DeleteProfileResponse{}, nil
}

func (h *userHandler) ListProfiles(ctx context.Context, req *eveyv1.ListProfilesRequest) (*eveyv1.ListProfilesResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.user.ListProfiles")
	defer span.End()
	claims, err := requireClaims(ctx)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, fmt.Errorf("userHandler.ListProfiles: %w", err)
	}
	userID := req.UserId
	if !interceptors.AdminReach(ctx, auth.ScopeAdminRead) && userID != claims.UserID {
		userID = claims.UserID
	}
	profiles, err := h.query.ListProfiles(ctx, userID)
	if err != nil {
		slog.ErrorContext(ctx, "list profiles", slog.Any("err", err))
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, status.Errorf(codes.Internal, "internal error")
	}
	resp := &eveyv1.ListProfilesResponse{}
	for _, p := range profiles {
		resp.Profiles = append(resp.Profiles, profileToProto(p))
	}
	return resp, nil
}

// ListAllProfiles answers "who lives on this server?" in one call. Admin-only
// via the RBAC interceptor; the redundant claims check here keeps the handler
// safe if it is ever mounted on a chain without it.
func (h *userHandler) ListAllProfiles(ctx context.Context, _ *eveyv1.ListAllProfilesRequest) (*eveyv1.ListAllProfilesResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.user.ListAllProfiles")
	defer span.End()
	claims, err := requireClaims(ctx)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, fmt.Errorf("userHandler.ListAllProfiles: %w", err)
	}
	if !claims.IsAdmin {
		return nil, h.translator.TranslatedError(ctx, codes.PermissionDenied, "errors.auth.admin_access_required")
	}
	profiles, err := h.query.ListAllProfiles(ctx)
	if err != nil {
		slog.ErrorContext(ctx, "list all profiles", slog.Any("err", err))
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, err.Error())
		return nil, status.Errorf(codes.Internal, "internal error")
	}
	resp := &eveyv1.ListAllProfilesResponse{}
	for _, p := range profiles {
		resp.Profiles = append(resp.Profiles, profileToProto(p))
	}
	return resp, nil
}
