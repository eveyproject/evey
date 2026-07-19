package grpcapi

import (
	"context"
	"errors"

	eveyv1 "github.com/evey-project/evey/gen/evey/v1"
	appmediarequest "github.com/evey-project/evey/internal/app/mediarequest"
	appuser "github.com/evey-project/evey/internal/app/user"
	"github.com/evey-project/evey/internal/domain/mediarequest"
	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/evey-project/evey/internal/pkg/tracing"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// mediaRequestHandler is the requester-facing surface. Every operation is
// scoped to the caller's own profile/user from the JWT claims - request bodies
// never carry a profile id, so there is no IDOR surface.
type mediaRequestHandler struct {
	eveyv1.UnimplementedMediaRequestServiceServer
	svc *appmediarequest.Service
}

func (h *mediaRequestHandler) SearchTitles(ctx context.Context, req *eveyv1.SearchTitlesRequest) (*eveyv1.SearchTitlesResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.mediarequest.SearchTitles")
	defer span.End()
	if _, err := requireClaims(ctx); err != nil {
		return nil, err
	}
	mediaType := mediarequest.MediaType(req.MediaType)
	if !mediaType.IsValid() {
		return nil, status.Error(codes.InvalidArgument, "invalid media type")
	}
	var year *int
	if req.Year > 0 {
		y := int(req.Year)
		year = &y
	}
	candidates, err := h.svc.SearchTitles(ctx, mediaType, req.Query, year)
	if err != nil {
		return nil, internalError("search titles", err)
	}
	resp := &eveyv1.SearchTitlesResponse{}
	for _, c := range candidates {
		tc := &eveyv1.TitleCandidate{
			Provider:    c.Provider,
			ProviderId:  c.ProviderID,
			Title:       c.Title,
			PosterUrl:   c.PosterURL,
			Overview:    c.Overview,
			InLibrary:   c.InLibrary,
			MediaItemId: c.MediaItemID,
		}
		if c.Year != nil {
			tc.Year = int32(*c.Year)
		}
		resp.Results = append(resp.Results, tc)
	}
	return resp, nil
}

func (h *mediaRequestHandler) CreateRequest(ctx context.Context, req *eveyv1.CreateRequestRequest) (*eveyv1.MediaRequestProto, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.mediarequest.CreateRequest")
	defer span.End()
	claims, err := requireClaims(ctx)
	if err != nil {
		return nil, err
	}
	in := appmediarequest.CreateInput{
		MediaType:  mediarequest.MediaType(req.MediaType),
		Provider:   req.Provider,
		ProviderID: req.ProviderId,
		Title:      req.Title,
		PosterURL:  req.PosterUrl,
	}
	if req.Year > 0 {
		y := int(req.Year)
		in.Year = &y
	}
	created, err := h.svc.CreateRequest(ctx, shared.ProfileID(claims.ProfileID), claims.UserID, in)
	if err != nil {
		return nil, mapMediaRequestError("create request", err)
	}
	return toMediaRequestProto(created, ""), nil
}

func (h *mediaRequestHandler) ListMyRequests(ctx context.Context, req *eveyv1.ListMyRequestsRequest) (*eveyv1.ListMyRequestsResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.mediarequest.ListMyRequests")
	defer span.End()
	claims, err := requireClaims(ctx)
	if err != nil {
		return nil, err
	}
	limit, offset := clampLimitOffset(req.Limit, req.Offset, 50)
	reqs, err := h.svc.ListMyRequests(ctx, claims.ProfileID, limit, offset)
	if err != nil {
		return nil, internalError("list my requests", err)
	}
	resp := &eveyv1.ListMyRequestsResponse{}
	for _, r := range reqs {
		resp.Items = append(resp.Items, toMediaRequestProto(r, ""))
	}
	return resp, nil
}

func (h *mediaRequestHandler) GetRequestPolicy(ctx context.Context, _ *eveyv1.GetRequestPolicyRequest) (*eveyv1.GetRequestPolicyResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.mediarequest.GetRequestPolicy")
	defer span.End()
	claims, err := requireClaims(ctx)
	if err != nil {
		return nil, err
	}
	can, err := h.svc.CanRequest(ctx, claims.UserID)
	if err != nil {
		return nil, internalError("get request policy", err)
	}
	return &eveyv1.GetRequestPolicyResponse{CanRequest: can}, nil
}

// adminMediaRequestHandler is the operator moderation surface. Admin gating is
// enforced by the RBAC interceptor (MethodRoles) ahead of these handlers.
type adminMediaRequestHandler struct {
	eveyv1.UnimplementedAdminMediaRequestServiceServer
	svc       *appmediarequest.Service
	userQuery *appuser.QueryService
}

func (h *adminMediaRequestHandler) ListRequests(ctx context.Context, req *eveyv1.AdminListRequestsRequest) (*eveyv1.AdminListRequestsResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.mediarequest.admin.ListRequests")
	defer span.End()
	var statuses []mediarequest.Status
	for _, s := range req.Statuses {
		st := mediarequest.Status(s)
		if st.IsValid() {
			statuses = append(statuses, st)
		}
	}
	limit, offset := clampLimitOffset(req.Limit, req.Offset, 100)
	reqs, err := h.svc.ListAll(ctx, statuses, limit, offset)
	if err != nil {
		return nil, internalError("list requests", err)
	}
	names := h.requesterNames(ctx, reqs)
	resp := &eveyv1.AdminListRequestsResponse{}
	for _, r := range reqs {
		resp.Items = append(resp.Items, toMediaRequestProto(r, names[r.UserID]))
	}
	return resp, nil
}

// requesterNames resolves userID -> display name for the moderation view,
// best-effort (a missing name never fails the listing).
func (h *adminMediaRequestHandler) requesterNames(ctx context.Context, reqs []*mediarequest.Request) map[string]string {
	names := make(map[string]string)
	if h.userQuery == nil {
		return names
	}
	for _, r := range reqs {
		if _, seen := names[r.UserID]; seen {
			continue
		}
		if u, err := h.userQuery.GetUserByID(ctx, r.UserID); err == nil && u != nil {
			names[r.UserID] = u.Username
		}
	}
	return names
}

func (h *adminMediaRequestHandler) AnswerRequest(ctx context.Context, req *eveyv1.AnswerRequestRequest) (*eveyv1.MediaRequestProto, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.mediarequest.admin.AnswerRequest")
	defer span.End()
	claims, err := requireClaims(ctx)
	if err != nil {
		return nil, err
	}
	st := mediarequest.Status(req.Status)
	if !st.IsAnswer() {
		return nil, status.Error(codes.InvalidArgument, "status must be approved, rejected or later")
	}
	answered, err := h.svc.Answer(ctx, req.Id, st, req.Note, shared.ProfileID(claims.ProfileID))
	if err != nil {
		return nil, mapMediaRequestError("answer request", err)
	}
	return toMediaRequestProto(answered, ""), nil
}

func (h *adminMediaRequestHandler) GetAccessSettings(ctx context.Context, _ *eveyv1.GetAccessSettingsRequest) (*eveyv1.GetAccessSettingsResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.mediarequest.admin.GetAccessSettings")
	defer span.End()
	settings, err := h.svc.GetAccessSettings(ctx)
	if err != nil {
		return nil, internalError("get access settings", err)
	}
	resp := &eveyv1.GetAccessSettingsResponse{DefaultEnabled: settings.DefaultEnabled}
	for userID, enabled := range settings.Overrides {
		ov := &eveyv1.UserAccessOverride{UserId: userID, Enabled: enabled}
		if h.userQuery != nil {
			if u, err := h.userQuery.GetUserByID(ctx, userID); err == nil && u != nil {
				ov.Username = u.Username
			}
		}
		resp.Overrides = append(resp.Overrides, ov)
	}
	return resp, nil
}

func (h *adminMediaRequestHandler) SetAccessDefault(ctx context.Context, req *eveyv1.SetAccessDefaultRequest) (*eveyv1.SetAccessDefaultResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.mediarequest.admin.SetAccessDefault")
	defer span.End()
	if err := h.svc.SetDefault(ctx, req.Enabled); err != nil {
		return nil, internalError("set access default", err)
	}
	return &eveyv1.SetAccessDefaultResponse{}, nil
}

func (h *adminMediaRequestHandler) SetUserAccess(ctx context.Context, req *eveyv1.SetUserAccessRequest) (*eveyv1.SetUserAccessResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.mediarequest.admin.SetUserAccess")
	defer span.End()
	if req.UserId == "" {
		return nil, status.Error(codes.InvalidArgument, "user id required")
	}
	var mode appmediarequest.AccessMode
	switch req.Mode {
	case eveyv1.AccessMode_ACCESS_MODE_ENABLED:
		mode = appmediarequest.AccessEnabled
	case eveyv1.AccessMode_ACCESS_MODE_DISABLED:
		mode = appmediarequest.AccessDisabled
	case eveyv1.AccessMode_ACCESS_MODE_DEFAULT:
		mode = appmediarequest.AccessDefault
	default:
		return nil, status.Error(codes.InvalidArgument, "invalid access mode")
	}
	if err := h.svc.SetUserAccess(ctx, req.UserId, mode); err != nil {
		return nil, internalError("set user access", err)
	}
	return &eveyv1.SetUserAccessResponse{}, nil
}

// toMediaRequestProto maps a domain request to its proto form. requesterName is
// set only on the admin moderation view (empty otherwise).
func toMediaRequestProto(r *mediarequest.Request, requesterName string) *eveyv1.MediaRequestProto {
	p := &eveyv1.MediaRequestProto{
		Id:            r.ID,
		ProfileId:     string(r.ProfileID),
		UserId:        r.UserID,
		MediaType:     string(r.MediaType),
		Provider:      r.Provider,
		ProviderId:    r.ProviderID,
		Title:         r.Title,
		PosterUrl:     r.PosterURL,
		Status:        string(r.Status),
		AdminNote:     r.AdminNote,
		AnsweredBy:    string(r.AnsweredBy),
		CreatedAt:     toTimestamppb(r.CreatedAt),
		UpdatedAt:     toTimestamppb(r.UpdatedAt),
		AnsweredAt:    toTimestamppbPtr(r.AnsweredAt),
		RequesterName: requesterName,
	}
	if r.Year != nil {
		p.Year = int32(*r.Year)
	}
	return p
}

// mapMediaRequestError translates domain errors to gRPC status codes.
func mapMediaRequestError(op string, err error) error {
	switch {
	case errors.Is(err, mediarequest.ErrDisabled):
		return status.Error(codes.PermissionDenied, "media requests are disabled for your account")
	case errors.Is(err, mediarequest.ErrDuplicate):
		return status.Error(codes.AlreadyExists, "you already have an open request for this title")
	case errors.Is(err, mediarequest.ErrAlreadyHeld):
		return status.Error(codes.AlreadyExists, "this title is already in the library")
	case errors.Is(err, mediarequest.ErrNotFound):
		return status.Error(codes.NotFound, "request not found")
	default:
		return internalError(op, err)
	}
}
