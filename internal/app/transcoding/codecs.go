package transcoding

import "github.com/eveyproject/evey/internal/domain/transcoding"

// IsImageSubtitle delegates to the domain's single source of truth (see
// domain/transcoding.IsImageSubtitle). Kept as a re-export so the many
// app-layer call sites don't churn; new code may call the domain directly.
func IsImageSubtitle(codec string) bool {
	return transcoding.IsImageSubtitle(codec)
}
