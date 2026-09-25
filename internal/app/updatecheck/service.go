// Package updatecheck tells the admin when the image they run has a newer
// build. It never updates anything: a container cannot replace its own image,
// and the pull, the recreate and the migrations that follow are the admin's
// to run. It only compares what this binary is with what the registry
// publishes, every few hours, and says so on the dashboard.
package updatecheck

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/evey-project/evey/internal/domain/shared"
)

// Config keys the admin sets; both optional.
const (
	// KeyEnabled turns the check off ("false"): an offline server, or an admin
	// who does not want the registry to hear from it.
	KeyEnabled = "update_check.enabled"
	// KeyImage overrides the repository to watch (a mirror, a fork).
	KeyImage = "update_check.image"
)

const (
	checkInterval = 6 * time.Hour
	firstCheck    = 2 * time.Minute
	// A forced check may not hammer the registry: the anonymous pull quota is
	// shared by every Evey behind the same address.
	minForcedGap = time.Minute
)

// State is the answer the dashboard shows.
type State string

const (
	StateUpToDate        State = "up_to_date"
	StateUpdateAvailable State = "update_available"
	// StateUnknown: the check ran and could not conclude (registry down, a
	// development build, a tag the registry does not have).
	StateUnknown  State = "unknown"
	StateDisabled State = "disabled"
	// StatePending: no check has run yet since the server started.
	StatePending State = "pending"
)

// Build is what this binary is.
type Build struct {
	Version   string
	Commit    string
	BuildDate string
}

// Remote is what the registry publishes under one tag.
type Remote struct {
	Revision string
	Created  string
	Source   string
}

// Registry is the port to the image registry.
type Registry interface {
	// Describe reads the labels of the image published as image:tag, for the
	// platform this server runs on.
	Describe(ctx context.Context, image, tag string) (Remote, error)
	// Tags lists the tags published for image.
	Tags(ctx context.Context, image string) ([]string, error)
}

// Status is one check's outcome.
type Status struct {
	State          State     `json:"state"`
	Image          string    `json:"image,omitempty"`
	CurrentVersion string    `json:"currentVersion"`
	CurrentCommit  string    `json:"currentCommit"`
	LatestVersion  string    `json:"latestVersion,omitempty"`
	LatestCommit   string    `json:"latestCommit,omitempty"`
	LatestCreated  string    `json:"latestCreated,omitempty"`
	Source         string    `json:"source,omitempty"`
	Reason         string    `json:"reason,omitempty"`
	CheckedAt      time.Time `json:"checkedAt,omitzero"`
}

// Service runs the periodic check and keeps the last answer.
type Service struct {
	registry     Registry
	cfg          shared.ServerConfigRepository
	build        Build
	defaultImage string

	mu     sync.Mutex
	status Status
	// checkMu serialises checks: the ticker and an admin's button.
	checkMu sync.Mutex
}

// New builds the service. defaultImage is the repository the image was
// published to (EVEY_IMAGE_REPO, stamped at build); empty leaves the check
// unconfigured until the admin names one.
func New(registry Registry, cfg shared.ServerConfigRepository, build Build, defaultImage string) *Service {
	return &Service{
		registry:     registry,
		cfg:          cfg,
		build:        build,
		defaultImage: strings.TrimSpace(defaultImage),
		status:       Status{State: StatePending, CurrentVersion: build.Version, CurrentCommit: build.Commit},
	}
}

// Start checks shortly after boot, then every checkInterval, until ctx ends.
func (s *Service) Start(ctx context.Context) {
	go func() {
		t := time.NewTimer(firstCheck)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
			}
			s.Check(ctx)
			t.Reset(checkInterval)
		}
	}()
}

// Status is the last answer.
func (s *Service) Status() Status {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.status
}

// CheckNow is the admin's button: a check unless one ran under a minute ago.
func (s *Service) CheckNow(ctx context.Context) Status {
	if last := s.Status(); !last.CheckedAt.IsZero() && time.Since(last.CheckedAt) < minForcedGap {
		return last
	}
	return s.Check(ctx)
}

// Check asks the registry and records the answer.
func (s *Service) Check(ctx context.Context) Status {
	s.checkMu.Lock()
	defer s.checkMu.Unlock()
	st := s.evaluate(ctx)
	st.CurrentVersion, st.CurrentCommit = s.build.Version, s.build.Commit
	st.CheckedAt = time.Now()
	if st.State == StateUnknown && st.Reason != "" {
		slog.InfoContext(ctx, "update check: no answer", slog.String("reason", st.Reason))
	}
	s.mu.Lock()
	s.status = st
	s.mu.Unlock()
	return st
}

func (s *Service) configValue(ctx context.Context, key string) string {
	if s.cfg == nil {
		return ""
	}
	e, err := s.cfg.Get(ctx, key)
	if err != nil || e == nil {
		return ""
	}
	return strings.TrimSpace(e.Value)
}

func (s *Service) evaluate(ctx context.Context) Status {
	if strings.EqualFold(s.configValue(ctx, KeyEnabled), "false") {
		return Status{State: StateDisabled}
	}
	image := s.configValue(ctx, KeyImage)
	if image == "" {
		image = s.defaultImage
	}
	if image == "" {
		return Status{State: StateUnknown, Reason: "no image repository configured"}
	}
	if s.build.Version == "" || s.build.Version == "dev" {
		return Status{State: StateUnknown, Image: image, Reason: "development build"}
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	base, variant := splitVariant(s.build.Version)
	if cur, ok := parseSemver(base); ok {
		return s.evaluateRelease(ctx, image, cur, variant)
	}
	return s.evaluateTrack(ctx, image)
}

// evaluateRelease: a release build (1.2.3-full, what `latest` points at)
// looks for a higher release of the same variant. A pre-release build also
// hears about later pre-releases; a stable one only about stable ones.
func (s *Service) evaluateRelease(ctx context.Context, image string, cur semver, variant string) Status {
	tags, err := s.registry.Tags(ctx, image)
	if err != nil {
		return Status{State: StateUnknown, Image: image, Reason: err.Error()}
	}
	best, bestTag := cur, ""
	for _, tag := range tags {
		base, v := splitVariant(tag)
		if v != variant {
			continue
		}
		sv, ok := parseSemver(base)
		if !ok || (sv.pre != "" && cur.pre == "") {
			continue
		}
		if sv.less(best) || sv == best {
			continue
		}
		best, bestTag = sv, tag
	}
	if bestTag == "" {
		return Status{State: StateUpToDate, Image: image}
	}
	st := Status{State: StateUpdateAvailable, Image: image, LatestVersion: bestTag}
	if remote, err := s.registry.Describe(ctx, image, bestTag); err == nil {
		st.LatestCommit, st.LatestCreated, st.Source = remote.Revision, remote.Created, remote.Source
	}
	return st
}

// evaluateTrack: a branch build (beta-0.0.1-full) follows its own tag, which
// moves with every push; a different revision published after this build is
// a newer one.
func (s *Service) evaluateTrack(ctx context.Context, image string) Status {
	remote, err := s.registry.Describe(ctx, image, s.build.Version)
	if err != nil {
		return Status{State: StateUnknown, Image: image, Reason: err.Error()}
	}
	st := Status{Image: image, LatestVersion: s.build.Version, LatestCommit: remote.Revision, LatestCreated: remote.Created, Source: remote.Source}
	switch {
	case remote.Revision == "" || s.build.Commit == "" || s.build.Commit == "unknown":
		st.State, st.Reason = StateUnknown, "revision not published"
	case sameCommit(remote.Revision, s.build.Commit):
		st.State = StateUpToDate
	case publishedBefore(remote.Created, s.build.BuildDate):
		// A local build newer than what the registry holds is not behind it.
		st.State = StateUpToDate
	default:
		st.State = StateUpdateAvailable
	}
	return st
}

// ErrNotFound is what a Registry returns for a tag it does not publish.
var ErrNotFound = errors.New("updatecheck: not found")

func sameCommit(a, b string) bool {
	a, b = strings.ToLower(strings.TrimSpace(a)), strings.ToLower(strings.TrimSpace(b))
	if a == "" || b == "" {
		return false
	}
	n := min(len(a), len(b))
	return n >= 7 && a[:n] == b[:n]
}

func publishedBefore(remoteCreated, localBuild string) bool {
	r, err1 := time.Parse(time.RFC3339, remoteCreated)
	l, err2 := time.Parse(time.RFC3339, localBuild)
	return err1 == nil && err2 == nil && r.Before(l)
}

// variants are the image flavours the CI publishes as a tag suffix.
var variants = []string{"-full", "-nodejs"}

func splitVariant(tag string) (base, variant string) {
	for _, v := range variants {
		if strings.HasSuffix(tag, v) {
			return strings.TrimSuffix(tag, v), v
		}
	}
	return tag, ""
}
