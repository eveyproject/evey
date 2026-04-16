package i18n

import (
	"context"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// TranslatedError with a nil translator falls back to the key as message.
func TestTranslatedError_NilTranslator(t *testing.T) {
	t.Parallel()
	var tr *Translator
	err := tr.TranslatedError(context.Background(), codes.NotFound, "some.key")
	if err == nil {
		t.Fatal("want error")
	}
	st, ok := status.FromError(err)
	if !ok {
		t.Fatal("want grpc status error")
	}
	if st.Code() != codes.NotFound {
		t.Fatalf("code: want NotFound, got %v", st.Code())
	}
	if st.Message() != "some.key" {
		t.Fatalf("message: want raw key, got %q", st.Message())
	}
}

// TranslatedError with messages wired returns the localized text.
func TestTranslatedError_WithMessages(t *testing.T) {
	t.Parallel()
	tr := NewEmpty()
	tr.messages["fr"] = map[string]string{"errors.not_found": "Introuvable"}
	ctx := ContextWithLocale(context.Background(), "fr")

	err := tr.TranslatedError(ctx, codes.NotFound, "errors.not_found")
	st, _ := status.FromError(err)
	if st.Message() != "Introuvable" {
		t.Fatalf("message: want Introuvable, got %q", st.Message())
	}
}
