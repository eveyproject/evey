package activity

import "context"

type impersonatorKey struct{}

// WithImpersonator marks a request made by an admin acting as another user
// ("connect as"): every audit entry it writes then names that admin too.
func WithImpersonator(ctx context.Context, adminUserID string) context.Context {
	if adminUserID == "" {
		return ctx
	}
	return context.WithValue(ctx, impersonatorKey{}, adminUserID)
}

// ImpersonatorFrom is the admin acting through this request, "" when nobody is.
func ImpersonatorFrom(ctx context.Context) string {
	v, _ := ctx.Value(impersonatorKey{}).(string)
	return v
}
