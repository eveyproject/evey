package plugin

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/eveyproject/evey/internal/domain/plugin"
	"github.com/eveyproject/evey/internal/domain/shared"
)

// OperatorKeyName is the name of the key seeded from EVEY_PLUGINS_COSIGN_PUBLIC_KEY.
const OperatorKeyName = "Server operator key"

// maxPublisherNameLen bounds the display name an admin gives a key.
const maxPublisherNameLen = 100

var (
	ErrPublisherNameRequired = errors.New("publisher name is required")
	ErrPublisherNameTooLong  = errors.New("publisher name is too long")
	// ErrPublisherKeyRequired: exactly one of PEM or fingerprint must be given.
	ErrPublisherKeyRequired = errors.New("give either a public key or a fingerprint")
	// ErrPublisherKeyUnknown: no installed plugin carries a key with that fingerprint.
	ErrPublisherKeyUnknown = errors.New("no installed plugin carries a key with this fingerprint")
	// ErrPublisherKeyInvalid wraps a rejected PEM.
	ErrPublisherKeyInvalid = errors.New("invalid public key")
)

// TrustedPublisherService manages the keys the instance accepts as plugin
// publishers. It is also the verifier's KeyLookup.
type TrustedPublisherService struct {
	repo    plugin.TrustedPublisherRepository
	plugins plugin.PluginRepository
}

func NewTrustedPublisherService(repo plugin.TrustedPublisherRepository, plugins plugin.PluginRepository) *TrustedPublisherService {
	return &TrustedPublisherService{repo: repo, plugins: plugins}
}

func (s *TrustedPublisherService) List(ctx context.Context) ([]plugin.TrustedPublisher, error) {
	return s.repo.List(ctx)
}

// Lookup is the plugin.KeyLookup the signature verifier uses. A repository
// failure reads as "not trusted": failing closed beats trusting by accident.
func (s *TrustedPublisherService) Lookup(ctx context.Context, fingerprint string) (string, bool) {
	p, err := s.repo.Get(ctx, fingerprint)
	if err != nil {
		if !errors.Is(err, shared.ErrNotFound) {
			slog.WarnContext(ctx, "trusted publisher lookup failed, treating the key as untrusted",
				slog.String("fingerprint", fingerprint), slog.Any("err", err))
		}
		return "", false
	}
	return p.Name, true
}

// AddByPEM trusts a pasted public key.
func (s *TrustedPublisherService) AddByPEM(ctx context.Context, name, keyPEM string) (*plugin.TrustedPublisher, error) {
	name, err := cleanPublisherName(name)
	if err != nil {
		return nil, err
	}
	fp, canonical, err := plugin.ParsePublisherKey([]byte(keyPEM))
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrPublisherKeyInvalid, err)
	}
	return s.upsert(ctx, plugin.TrustedPublisher{Fingerprint: fp, Name: name, PublicKeyPEM: canonical})
}

// AddByFingerprint trusts the key an installed plugin already carries: the
// server holds its PEM from the package, so nothing is pasted.
func (s *TrustedPublisherService) AddByFingerprint(ctx context.Context, name, fingerprint string) (*plugin.TrustedPublisher, error) {
	name, err := cleanPublisherName(name)
	if err != nil {
		return nil, err
	}
	fingerprint = strings.TrimSpace(fingerprint)
	installed, err := s.plugins.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("list plugins: %w", err)
	}
	for _, p := range installed {
		if p.PublisherKeyFingerprint == fingerprint && p.PublisherKeyPEM != "" {
			return s.upsert(ctx, plugin.TrustedPublisher{Fingerprint: fingerprint, Name: name, PublicKeyPEM: p.PublisherKeyPEM})
		}
	}
	return nil, ErrPublisherKeyUnknown
}

func (s *TrustedPublisherService) upsert(ctx context.Context, p plugin.TrustedPublisher) (*plugin.TrustedPublisher, error) {
	if err := s.repo.Upsert(ctx, p); err != nil {
		return nil, fmt.Errorf("store trusted publisher: %w", err)
	}
	return s.repo.Get(ctx, p.Fingerprint)
}

// Remove un-trusts a key. Plugins already installed under it keep running; only
// their badge falls back to "unknown key". A seeded key is refused.
func (s *TrustedPublisherService) Remove(ctx context.Context, fingerprint string) error {
	return s.repo.Delete(ctx, fingerprint)
}

// SeedOperatorKey makes the key at keyPath (EVEY_PLUGINS_COSIGN_PUBLIC_KEY) a
// seeded trusted publisher, and drops seeded keys the configuration no longer
// names. An empty keyPath only prunes.
func (s *TrustedPublisherService) SeedOperatorKey(ctx context.Context, readFile func(string) ([]byte, error), keyPath string) error {
	keep := []string{}
	if keyPath != "" {
		raw, err := readFile(keyPath)
		if err != nil {
			return fmt.Errorf("read operator public key: %w", err)
		}
		fp, canonical, err := plugin.ParsePublisherKey(raw)
		if err != nil {
			return fmt.Errorf("operator public key: %w", err)
		}
		if err := s.repo.Upsert(ctx, plugin.TrustedPublisher{
			Fingerprint: fp, Name: OperatorKeyName, PublicKeyPEM: canonical, Seeded: true,
		}); err != nil {
			return fmt.Errorf("seed operator key: %w", err)
		}
		keep = append(keep, fp)
	}
	return s.repo.PruneSeeded(ctx, keep)
}

func cleanPublisherName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", ErrPublisherNameRequired
	}
	if len([]rune(name)) > maxPublisherNameLen {
		return "", ErrPublisherNameTooLong
	}
	return name, nil
}

// RefreshTrust returns a copy of an install-time signature verdict with Publisher
// and Trusted read from the keyring NOW, so trusting a key from a plugin card
// shows at once instead of at the next install. Only a verified signature can
// be trusted. nil in, nil out; the stored verdict is never mutated.
func RefreshTrust(ctx context.Context, sig *plugin.SignatureStatus, lookup plugin.KeyLookup) *plugin.SignatureStatus {
	if sig == nil || lookup == nil {
		return sig
	}
	cp := *sig
	cp.Publisher, cp.Trusted = "", false
	if cp.Verified && cp.KeyFingerprint != "" {
		cp.Publisher, cp.Trusted = lookup(ctx, cp.KeyFingerprint)
	}
	return &cp
}
