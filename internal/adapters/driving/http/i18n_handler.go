package httpapi

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/gravity-zero/evey/internal/pkg/i18n"
)

func i18nHandler(translator *i18n.Translator) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		locale := r.PathValue("locale")
		if locale == "" {
			locale = "en"
		}

		messages := translator.AllMessages(locale)

		data, err := json.Marshal(messages)
		if err != nil {
			http.Error(w, "encode error", http.StatusInternalServerError)
			return
		}

		hash := sha256.Sum256(data)
		etag := fmt.Sprintf(`"%x"`, hash[:8])

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "public, max-age=86400")
		w.Header().Set("ETag", etag)

		if r.Header.Get("If-None-Match") == etag {
			w.WriteHeader(http.StatusNotModified)
			return
		}

		if _, wErr := w.Write(data); wErr != nil {
			return
		}
	}
}
