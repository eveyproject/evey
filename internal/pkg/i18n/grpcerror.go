package i18n

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (t *Translator) TranslatedError(ctx context.Context, code codes.Code, key string, params ...map[string]string) error {
	if t == nil {
		return status.Error(code, key)
	}
	locale := LocaleFromCtx(ctx)
	msg := t.T(locale, key, params...)
	return status.Error(code, msg)
}
