package grpcapi

import (
	"context"
	crypto_rand "crypto/rand"
	"encoding/hex"
	"errors"
	"github.com/eveyproject/evey/internal/domain/shared"
	"log/slog"
	"strings"
	"sync/atomic"

	"github.com/eveyproject/evey/internal/adapters/driving/grpcapi/interceptors"
	appactivity "github.com/eveyproject/evey/internal/app/activity"
	applibrary "github.com/eveyproject/evey/internal/app/library"
	appuser "github.com/eveyproject/evey/internal/app/user"
	"github.com/eveyproject/evey/internal/domain/activity"
	"github.com/eveyproject/evey/internal/domain/user"
	eveyi18n "github.com/eveyproject/evey/internal/pkg/i18n"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// sanitizeLogValue escapes CRLF and other control characters in a string
// before it is passed to slog as a value, so an attacker-controlled input
// (most commonly the `username` on a failed login) cannot inject forged log
// lines into the human-readable dev console. The durable JSON log already
// escapes \n/\r, but the tinted dev handler used in `make dev` does not,
// which is enough of a forensic concern to scrub centrally.
//
// Returns the empty string unchanged so empty-value logs still look clean.
// Truncates pathologically long inputs to 256 bytes to avoid flooding the log
// line if an attacker submits a giant payload.
func sanitizeLogValue(s string) string {
	if s == "" {
		return s
	}
	if len(s) > 256 {
		s = s[:256] + "…"
	}
	// Replace CR/LF first so we don't end up with literal `\n` followed by a
	// real newline still surviving in the input.
	r := strings.NewReplacer(
		"\r", `\r`,
		"\n", `\n`,
		"\t", `\t`,
		"\x1b", `\x1b`, // ANSI escape - prevents fake colour/control sequences
		"\x00", `\x00`,
	)
	return r.Replace(s)
}

func secureToken(prefix string, n int) string {
	b := make([]byte, n)
	// Fail closed: a token built from a failed entropy read would be all-zeros
	// (fully predictable). crypto/rand.Read does not return an error on Linux,
	// but never silently emit a guessable security token if it ever did.
	if _, err := crypto_rand.Read(b); err != nil {
		panic("secureToken: crypto/rand read failed: " + err.Error())
	}
	return prefix + hex.EncodeToString(b)
}

// requireClaims extracts claims from context, returns error if not authenticated.
// helperTranslator localises the answers of the shared helpers below, which
// take no translator of their own (requireClaims alone has ~150 callers). Set
// once by NewServer; nil (unit tests, a server built without one) keeps the
// English message, so nothing depends on it being there.
var helperTranslator atomic.Pointer[eveyi18n.Translator]

// helperError answers code with the localised key, or the English fallback.
func helperError(ctx context.Context, code codes.Code, key, fallback string) error {
	if t := helperTranslator.Load(); t != nil {
		return t.TranslatedError(ctx, code, key)
	}
	return status.Error(code, fallback)
}

func requireClaims(ctx context.Context) (*user.Claims, error) {
	claims := interceptors.ClaimsFromContext(ctx)
	if claims == nil {
		return nil, helperError(ctx, codes.Unauthenticated, "errors.auth.unauthenticated", "not authenticated")
	}
	return claims, nil
}

// ctxIsAdmin reports whether the request is from an authenticated admin. Used to
// gate admin-only fields (e.g. absolute file paths) in proto converters; nil
// claims → false (fail closed).
func ctxIsAdmin(ctx context.Context) bool {
	c := interceptors.ClaimsFromContext(ctx)
	return c != nil && c.IsAdmin
}

const maxPageSize = 500

// clampLimitOffset returns bounded (limit, offset) from raw limit/offset params.
// A negative offset is normalised to 0 so Postgres never receives OFFSET < 0
// (which panics as a slice index error in fakes and returns 500 from the DB).
// A limit that is zero, negative, or above maxPageSize is replaced with
// defaultLimit to prevent unbounded fetches.
func clampLimitOffset(limit, offset int32, defaultLimit int) (int, int) {
	l := int(limit)
	if l <= 0 || l > maxPageSize {
		l = defaultLimit
	}
	o := max(int(offset), 0)
	return l, o
}

// clampPagination returns bounded (limit, offset) from pagination params.
func clampPagination(pageSize, page int32, defaultLimit int) (int, int) {
	limit := defaultLimit
	if pageSize > 0 {
		limit = int(pageSize)
	}
	if limit > maxPageSize {
		limit = maxPageSize
	}
	offset := 0
	if page > 1 {
		offset = int(page-1) * limit
	}
	return limit, offset
}

// internalError logs the real error server-side and returns a generic message to
// the client.
//
// ctx is required, not optional: this helper is the tail of ~200 gRPC failure
// paths, and logging without it stripped the request_id and trace_id that
// contextHandler injects. An operator holding a user's request id could match
// the interceptor's one-line summary and nothing else - every ERROR raised
// during that request was unreachable from the id that identified it.
func internalError(ctx context.Context, op string, err error) error {
	if isClientCanceled(err) {
		// The client disconnected mid-request, which cancels the context; the DB
		// driver then surfaces context.Canceled or pg's 57014 "canceling statement
		// due to user request". That is the caller giving up, not a server fault -
		// log it at DEBUG so it doesn't pollute the ERROR stream. The code says
		// what happened too: Internal made the sanitizer log every navigation
		// away from the home page as "sanitized internal error" (176 lines in
		// 25 days of prod, none a fault) and count it as a 5xx in the metrics.
		slog.DebugContext(ctx, op+": client canceled", slog.Any("err", err))
		if errors.Is(err, context.DeadlineExceeded) {
			return status.Error(codes.DeadlineExceeded, "deadline exceeded")
		}
		return status.Error(codes.Canceled, "request canceled")
	}
	slog.ErrorContext(ctx, op, slog.Any("err", err))
	return status.Error(codes.Internal, "internal error")
}

// errUserAccessNotConfigured is returned by a handler's access guard when the
// userAccess port is nil. wire.go always wires it in production, so this is
// never a live prod condition - it is a safety net for a future wiring
// regression. The guard used to treat a nil port as "no restrictions" (see
// git history), which meant a wiring mistake would silently expose every
// library to every non-admin caller instead of breaking loudly. Logging here
// (rather than only returning the status) is what lets an operator diagnose
// the regression from server logs even when the RPC caller ignores the error.
// libraryAccessDecision answers "may this caller touch this library?" - ONE
// implementation of a security rule that four handlers (library, music, photo,
// share) each carried their own byte-identical copy of.
//
// They agreed at the time this was extracted, which is exactly when to merge
// them: this codebase's history is full of near-identical logic that diverged
// silently, and a rule about who may read a library is the worst possible place
// for that. The handlers still differ in how they REPORT the answer (some
// return (bool, error), some a gRPC status), so only the decision is shared.
//
// Two invariants live here, both learned the hard way:
//   - the admin bypass is a claims check and is resolved FIRST, independent of
//     whether the port is wired: an unwired port must never turn an admin's
//     request into a denial.
//   - a missing port DENIES. Returning "allowed" would let a wiring regression
//     silently expose every library.
func libraryAccessDecision(ctx context.Context, userAccess *appuser.AccessService, libraryID, op string) (bool, error) {
	claims := interceptors.ClaimsFromContext(ctx)
	if claims == nil || claims.IsAdmin {
		return true, nil
	}
	if userAccess == nil {
		return false, errUserAccessNotConfigured(ctx, op)
	}
	return userAccess.HasLibraryAccess(ctx, shared.UserID(claims.UserID), shared.LibraryID(libraryID))
}

// libraryAccessOrStatus is libraryAccessDecision for the handlers that want a
// gRPC status rather than a boolean: nil means allowed.
func libraryAccessOrStatus(ctx context.Context, tr *eveyi18n.Translator, userAccess *appuser.AccessService, libraryID, op string) error {
	ok, err := libraryAccessDecision(ctx, userAccess, libraryID, op)
	if err != nil {
		// A missing port already produced its own status; anything else is a
		// lookup failure.
		if status.Code(err) == codes.FailedPrecondition {
			return err
		}
		return internalError(ctx, "check library access", err)
	}
	if !ok {
		return tr.TranslatedError(ctx, codes.PermissionDenied, "errors.library.no_access")
	}
	return nil
}

func errUserAccessNotConfigured(ctx context.Context, op string) error {
	slog.ErrorContext(ctx, op+": user access authorization service not configured")
	return status.Error(codes.FailedPrecondition, "user access authorization service not configured")
}

// isClientCanceled reports whether err is the result of the caller going away
// (a canceled context or its DB-driver projection) rather than a genuine
// server-side failure.
func isClientCanceled(err error) bool {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	// lib/pq surfaces a cancellation as SQLSTATE 57014 without wrapping
	// context.Canceled; match on the code without importing the driver here.
	return strings.Contains(err.Error(), "57014")
}

// resolveProfileID returns the profile a profile-scoped RPC must act on. The
// caller may only target the profile their token was minted for
// (claims.ProfileID); an empty request defaults to it, anything else is denied.
//
// Admins are deliberately NOT special-cased here. Cross-profile admin access
// goes through the explicit, audited ImpersonateUser RPC, which mints a token
// whose claims.ProfileID IS the target - so by the time a request reaches here,
// an impersonating admin legitimately owns the profile. This keeps every
// profile-scoped write attributable and forbids a silent profile_id override on
// a normal admin request.
// auditProfileID returns the profile to stamp on an audit entry: the caller's
// own, and only when the entry is attributed to that same account. An entry
// written on behalf of somebody else (admin acting on a target user) or by a
// path with no claims gets no profile rather than a misleading one - "which
// profile did this" must never answer with a profile that did not do it.
// recordAudit writes one admin audit entry with the request's client details.
// Best-effort: a failed write is logged, never surfaced to the caller.
func recordAudit(ctx context.Context, svc *appactivity.CommandService, userID, action, resourceType string, resourceID *string, details map[string]any) {
	if svc == nil {
		return
	}
	ua := interceptors.UserAgentFromCtx(ctx)
	dn := interceptors.DeviceNameFromCtx(ctx)
	ip := interceptors.ClientIPFromCtx(ctx)
	e := &activity.AuditLogEntry{
		UserID:       shared.UserID(userID),
		ProfileID:    auditProfileID(ctx, userID),
		Action:       action,
		ResourceType: resourceType,
		ResourceID:   resourceID,
		Details:      details,
	}
	if ua != "" {
		e.UserAgent = &ua
	}
	if dn != "" {
		e.DeviceName = &dn
	}
	if ip != "" {
		e.ClientIP = &ip
	}
	if err := svc.CreateAuditEntry(ctx, e); err != nil {
		slog.WarnContext(ctx, "audit log failed", slog.String("action", action), slog.Any("err", err))
	}
}

func auditProfileID(ctx context.Context, userID string) shared.ProfileID {
	c := interceptors.ClaimsFromContext(ctx)
	if c == nil || c.UserID != userID {
		return ""
	}
	return shared.ProfileID(c.ProfileID)
}

func resolveProfileID(ctx context.Context, claims *user.Claims, reqProfileID string) (string, error) {
	if reqProfileID == "" || reqProfileID == claims.ProfileID {
		return claims.ProfileID, nil
	}
	return "", helperError(ctx, codes.PermissionDenied, "errors.profile.cannot_access_other", "cannot access another profile")
}

// requireMediaLibraryAccess refuses a media item the caller's user has no
// library grant for, answering NotFound so an inaccessible id is
// indistinguishable from a missing one (no existence oracle).
//
// One implementation, two callers: favoriteHandler.requireMediaAccess and
// playlistHandler both ask the exact same question - "may this caller act on
// this media_item_id?" - and AddPlaylistItem simply never asked it, while its
// sibling AddFavorite did. Copying the body a third time is how the two
// existing copies already drifted (favorite bypasses admin BEFORE resolving
// the item, rating resolves first).
//
// A missing port REFUSES rather than panics or fails open: this path started
// dereferencing libQuery only once the fail-open guard was removed.
func requireMediaLibraryAccess(
	ctx context.Context,
	mediaItemID string,
	libQuery *applibrary.QueryService,
	userAccess *appuser.AccessService,
	who string,
) error {
	claims := interceptors.ClaimsFromContext(ctx)
	if claims == nil {
		return helperError(ctx, codes.Unauthenticated, "errors.auth.unauthenticated", "unauthenticated")
	}
	if libQuery == nil {
		return errUserAccessNotConfigured(ctx, who)
	}
	// The existence check runs for EVERY caller, admin included - the ordering
	// ratingHandler arrived at and documented. Letting an admin straight
	// through sends a non-existent id down to the write, where the FK on
	// playlist_items/ratings surfaces as something unrelated (rating's comment:
	// a misleading "rating must be between 1 and 10"). Only the library GRANT
	// is waived for an admin, not the question of whether the thing exists.
	item, err := libQuery.GetMediaItem(ctx, mediaItemID)
	if err != nil || item == nil {
		return helperError(ctx, codes.NotFound, "errors.general.not_found", "not found")
	}
	if claims.IsAdmin {
		return nil
	}
	if userAccess == nil {
		return errUserAccessNotConfigured(ctx, who)
	}
	ok, accessErr := userAccess.HasLibraryAccess(ctx, shared.UserID(claims.UserID), item.LibraryID)
	if accessErr != nil || !ok {
		return helperError(ctx, codes.NotFound, "errors.general.not_found", "not found")
	}
	return nil
}
