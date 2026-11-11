package i18n

import (
	"context"
	"errors"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// KeyedError is a gRPC status whose message was translated from Key. Clients
// see the status (code and translated message); Key stays on the server, for
// an audit entry that must read the same whatever the caller's language.
type KeyedError struct {
	Key string
	st  *status.Status
}

func (e *KeyedError) Error() string { return e.st.Err().Error() }

// GRPCStatus lets status.FromError, status.Code and the gateway see the status.
func (e *KeyedError) GRPCStatus() *status.Status { return e.st }

// ErrorKey returns the message key behind err, or "" when err did not come
// from TranslatedError.
func ErrorKey(err error) string {
	var ke *KeyedError
	if errors.As(err, &ke) {
		return ke.Key
	}
	return ""
}

func (t *Translator) TranslatedError(ctx context.Context, code codes.Code, key string, params ...map[string]string) error {
	if t == nil {
		return &KeyedError{Key: key, st: status.New(code, key)}
	}
	locale := LocaleFromCtx(ctx)
	msg := t.T(locale, key, params...)
	return &KeyedError{Key: key, st: status.New(code, msg)}
}
