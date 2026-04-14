package httpapi

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/evey-project/evey/internal/pkg/i18n"
)

func i18nHandler(translator *i18n.Translator, devMode ...bool) http.HandlerFunc {
	isDev := len(devMode) > 0 && devMode[0]
	return func(w http.ResponseWriter, r *http.Request) {
		locale := i18n.ParseLocale(r.PathValue("locale"))

		messages := translator.AllMessages(locale)

		data, err := json.Marshal(messages)
		if err != nil {
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
