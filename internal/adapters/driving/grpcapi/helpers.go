package grpcapi

import (
	"context"
	crypto_rand "crypto/rand"
	"encoding/hex"
	"log/slog"
	"strings"

	"github.com/evey-project/evey/internal/adapters/driving/grpcapi/interceptors"
	"github.com/evey-project/evey/internal/domain/user"
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
		"\x1b", `\x1b`, // ANSI escape — prevents fake colour/control sequences
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
func requireClaims(ctx context.Context) (*user.Claims, error) {
	claims := interceptors.ClaimsFromContext(ctx)
	if claims == nil {
		return nil, status.Error(codes.Unauthenticated, "not authenticated")
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

// internalError logs the real error server-side and returns a generic message to the client.
func internalError(op string, err error) error {
	slog.Error(op, slog.Any("err", err))
	return status.Error(codes.Internal, "internal error")
}

// resolveProfileID returns the profile a profile-scoped RPC must act on. The
// caller may only target the profile their token was minted for
// (claims.ProfileID); an empty request defaults to it, anything else is denied.
//
// Admins are deliberately NOT special-cased here. Cross-profile admin access
// goes through the explicit, audited ImpersonateUser RPC, which mints a token
// whose claims.ProfileID IS the target — so by the time a request reaches here,
// an impersonating admin legitimately owns the profile. This keeps every
// profile-scoped write attributable and forbids a silent profile_id override on
// a normal admin request.
func resolveProfileID(claims *user.Claims, reqProfileID string) (string, error) {
	if reqProfileID == "" || reqProfileID == claims.ProfileID {
		return claims.ProfileID, nil
	}
	return "", status.Error(codes.PermissionDenied, "cannot access another profile")
}
