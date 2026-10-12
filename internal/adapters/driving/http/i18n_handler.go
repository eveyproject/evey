package httpapi

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"

	"go.opentelemetry.io/otel/attribute"
	otelcodes "go.opentelemetry.io/otel/codes"

	"github.com/eveyproject/evey/internal/pkg/i18n"
	"github.com/eveyproject/evey/internal/pkg/tracing"
)

func i18nHandler(translator *i18n.Translator, devMode ...bool) http.HandlerFunc {
	isDev := len(devMode) > 0 && devMode[0]
	return func(w http.ResponseWriter, r *http.Request) {
		_, span := tracing.StartSpan(r.Context(), "http.i18n.GetMessages")
		defer span.End()
		locale := i18n.ParseLocale(r.PathValue("locale"))
		span.SetAttributes(attribute.String("locale", locale))

		messages := translator.AllMessages(locale)

		data, err := json.Marshal(messages)
		if err != nil {
			span.RecordError(err)
			span.SetStatus(otelcodes.Error, "encode error")
			logInternal(r.Context(), "i18n: encode messages", err)
			http.Error(w, "encode error", http.StatusInternalServerError)
			return
		}

		hash := sha256.Sum256(data)
		etag := fmt.Sprintf(`"%x"`, hash[:8])

		w.Header().Set("Content-Type", "application/json")
		if isDev {
			w.Header().Set("Cache-Control", "no-store")
		} else {
			w.Header().Set("Cache-Control", "public, max-age=3600, must-revalidate")
			w.Header().Set("ETag", etag)
			if r.Header.Get("If-None-Match") == etag {
				w.WriteHeader(http.StatusNotModified)
				return
			}
		}

		if _, wErr := w.Write(data); wErr != nil {
			return
		}
	}
}
