package library

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/evey-project/evey/internal/adapters/driven/filesystem"
	"github.com/evey-project/evey/internal/domain/shared"
)

// ParserConfigKey is the server_config row under which the JSON blob of
// admin-configured extra parser tokens persists. Admins can edit it
// directly via the generic UpdateServerConfig endpoint, or via the
// admin/server-config UI which knows about this key specifically.
const ParserConfigKey = "parser.config"

// parserConfig holds the admin-controlled token extensions. Empty slices
// mean "no extras" — the hardcoded knownQualities map + reEditionSuffix
// regex in the filesystem package stay authoritative. Admins can add new
// tokens here without redeploying when they hit an unusual release naming
// pattern (a distributor-specific edition tag, a quality variant the
// baseline list doesn't cover, …).
type parserConfig struct {
	ExtraQualities       []string `json:"extra_qualities"`
	ExtraEditionSuffixes []string `json:"extra_edition_suffixes"`
}

// loadParserConfig reads the JSON blob (if any) from the server_config
// store and returns the resulting struct. Missing key / parse error /
// nil repo all fall back to a zero-value parserConfig (no extras), which
// is safe — the parser keeps using its baseline maps in that case.
func loadParserConfig(ctx context.Context, repo shared.ServerConfigRepository) parserConfig {
	cfg := parserConfig{}
	if repo == nil {
		return cfg
	}
	entry, err := repo.Get(ctx, ParserConfigKey)
	if err != nil || entry == nil || entry.Value == "" {
		return cfg
	}
	if err := json.Unmarshal([]byte(entry.Value), &cfg); err != nil {
		slog.Warn("parser_config: invalid JSON, using defaults", slog.Any("err", err))
		return parserConfig{}
	}
	return cfg
}

// ApplyParserConfig loads the persisted parser_config and pushes the
// tokens into the filesystem package's runtime extension slots. Called
// at boot (so the first scan uses whatever the admin previously saved)
// and after every UpdateServerConfig that touches ParserConfigKey (so
// edits take effect without a restart).
//
// Errors are swallowed at INFO level: a misconfigured parser.config row
// must never block a scan — the baseline tokens still match every
// canonical release.
func ApplyParserConfig(ctx context.Context, repo shared.ServerConfigRepository) {
	cfg := loadParserConfig(ctx, repo)
	filesystem.SetExtraParserTokens(cfg.ExtraQualities, cfg.ExtraEditionSuffixes)
	slog.Info("parser config applied",
		slog.Int("extra_qualities", len(cfg.ExtraQualities)),
		slog.Int("extra_edition_suffixes", len(cfg.ExtraEditionSuffixes)),
	)
}
