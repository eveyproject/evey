package config

import (
	"context"

	"github.com/evey-project/evey/internal/domain/shared"
)

// LocaleDefaults is what each language-and-region setting resolves to when
// the admin leaves it empty - the same chains the server applies - so the
// panel can say "Default: French" instead of "env default". configLanguage is
// the configured language (EVEY_METADATA_LANGUAGE, "en" unless set).
//   - server.language: the configured language;
//   - metadata.language: server.language when set, else the configured language;
//   - server.timezone: TZ, else DefaultServerTimezone.
//
// The region follows the effective server language, resolved by the clients
// (defaultRegionForLocale).
func LocaleDefaults(ctx context.Context, repo shared.ServerConfigRepository, configLanguage string) map[string]string {
	meta := configLanguage
	if e, err := repo.Get(ctx, "server.language"); err == nil && e != nil && e.Value != "" {
		meta = e.Value
	}
	return map[string]string{
		"server.language":   configLanguage,
		"metadata.language": meta,
		KeyServerTimezone:   defaultLocation().String(),
	}
}
