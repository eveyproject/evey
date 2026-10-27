package plugin

import (
	"context"
	"errors"
	"fmt"

	"github.com/eveyproject/evey/internal/domain/shared"
)

// Bounds of a plugin's own key/value store, per scope. Its write RPCs need no
// permission, so its growth cannot rest on the plugin's restraint.
const (
	MaxPluginDataKeyLen   = 256
	MaxPluginDataValueLen = 64 * 1024
	MaxPluginDataKeys     = 1024
)

var (
	ErrPluginDataTooLarge = errors.New("plugin data key or value too large")
	ErrPluginDataFull     = errors.New("plugin data store full")
)

// CheckPluginDataWrite refuses an oversized key or value, and a NEW key once
// the scope holds MaxPluginDataKeys; an existing key stays writable.
func CheckPluginDataWrite(ctx context.Context, repo PluginDataRepository, pluginID, userID, profileID, key, value string) error {
	if len(key) > MaxPluginDataKeyLen || len(value) > MaxPluginDataValueLen {
		return fmt.Errorf("%w: key must be <= %d bytes and value <= %d bytes",
			ErrPluginDataTooLarge, MaxPluginDataKeyLen, MaxPluginDataValueLen)
	}
	_, err := repo.Get(ctx, pluginID, userID, profileID, key)
	if err == nil {
		return nil
	}
	if !errors.Is(err, shared.ErrNotFound) {
		return err
	}
	n, err := repo.Count(ctx, pluginID, userID, profileID)
	if err != nil {
		return err
	}
	if n >= MaxPluginDataKeys {
		return fmt.Errorf("%w: limited to %d keys", ErrPluginDataFull, MaxPluginDataKeys)
	}
	return nil
}
