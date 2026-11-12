package plugin

import (
	"context"
	"errors"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// An installed plugin can follow the repository it came from: the server looks
// at the repository's latest release every hour and applies it on its own when
// nothing in it is new and sensitive. Everything else waits for an admin.

// UpdateOutcome is what the last look at a plugin's repository concluded.
type UpdateOutcome string

const (
	// UpdateUpToDate: the latest release is the installed version (or older).
	UpdateUpToDate UpdateOutcome = "up_to_date"
	// UpdateApplied: a newer release was installed without anyone's action.
	UpdateApplied UpdateOutcome = "applied"
	// UpdateHeld: a newer release exists and waits for an admin, because it
	// asks for something the installed version does not have (a permission, a
	// capability, a slot, a host, a new kind of analysis warning, another
	// signing key), or because automatic updates are off for this plugin.
	UpdateHeld UpdateOutcome = "held"
	// UpdateRefused: the release carries code the analysis refuses. It is not
	// installed, automatically or otherwise.
	UpdateRefused UpdateOutcome = "refused"
	// UpdateFailed: the check could not conclude (repository unreachable, no
	// zip in the release, a download that did not match its hash).
	UpdateFailed UpdateOutcome = "failed"
)

// UpdateState is the last look at a plugin's repository.
type UpdateState struct {
	CheckedAt     time.Time     `json:"checkedAt,omitzero"`
	LatestVersion string        `json:"latestVersion,omitempty"`
	Outcome       UpdateOutcome `json:"outcome,omitempty"`
	Reason        string        `json:"reason,omitempty"`
	// Notified is the "<digest>/<outcome>" the admins were last told about,
	// so a held or refused release pages them once, not every hour.
	Notified string `json:"notified,omitempty"`
	// Digest is the sha256 of the release that was judged, and JudgedFor what
	// it was judged against (installed version, pinned key, switch, server
	// build). The same release judged against the same plugin gets the same
	// verdict without being downloaded and analysed again every hour.
	Digest    string `json:"digest,omitempty"`
	JudgedFor string `json:"judgedFor,omitempty"`
}

// Release is the latest release of a plugin repository.
type Release struct {
	// Version is the tag without its leading "v".
	Version string
	Tag     string
	// ZipURL is the release's plugin zip; its signature is ZipURL + ".sig".
	ZipURL string
	// SHA256 is the zip's published digest, hex; empty when the release
	// publishes none.
	SHA256 string
}

// ReleaseSource reads a followed repository's releases. A repository is
// followed by the permanent number GitHub gives it, not by its name: a name
// can be renamed away and registered again by someone else, the number
// cannot, so whoever takes an abandoned name over gets nothing followed.
type ReleaseSource interface {
	// Identify returns the repository's permanent id and its canonical link.
	Identify(ctx context.Context, repo string) (id int64, canonical string, err error)
	// Latest reads the latest release of the repository with that id.
	Latest(ctx context.Context, repoID int64) (*Release, error)
}

// ErrReleaseRateLimited: GitHub refuses more anonymous requests from this
// address for now. The check stops until its next run.
var ErrReleaseRateLimited = errors.New("GitHub's API rate limit for this address is reached")

// ErrSignatureRefused: under the enforce policy, the package is not validly
// signed by a trusted publisher. A matter of trust for an admin to settle, not
// a verdict on the code.
var ErrSignatureRefused = errors.New("plugin signature verification failed")

// ErrUnsupportedRepository: the link is not one the server can follow.
var ErrUnsupportedRepository = errors.New("plugin repository: only https://github.com/<owner>/<repo> links are supported")

// NormalizeRepoURL turns what an admin pastes (the repository page, its .git
// clone URL, or one of its release download links) into the canonical
// https://github.com/<owner>/<repo>.
func NormalizeRepoURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || !strings.EqualFold(u.Host, "github.com") || u.User != nil ||
		u.RawPath != "" || u.RawQuery != "" {
		return "", ErrUnsupportedRepository
	}
	// Every segment must be a plain name: a "." or ".." would let the link
	// name one repository while the request reaches another, and an empty
	// segment is a path GitHub would read differently.
	parts := strings.Split(strings.TrimSuffix(strings.TrimPrefix(u.Path, "/"), "/"), "/")
	if len(parts) < 2 {
		return "", ErrUnsupportedRepository
	}
	for _, seg := range parts {
		if seg == "" || seg == "." || seg == ".." {
			return "", ErrUnsupportedRepository
		}
	}
	owner, repo := parts[0], strings.TrimSuffix(parts[1], ".git")
	if !validRepoSegment(owner) || !validRepoSegment(repo) {
		return "", ErrUnsupportedRepository
	}
	return "https://github.com/" + owner + "/" + repo, nil
}

// RepoOwnerAndName splits a normalized repository URL.
func RepoOwnerAndName(repo string) (owner, name string, err error) {
	n, err := NormalizeRepoURL(repo)
	if err != nil || n == "" {
		return "", "", ErrUnsupportedRepository
	}
	parts := strings.Split(strings.TrimPrefix(n, "https://github.com/"), "/")
	return parts[0], parts[1], nil
}

// releaseVersionRe is the version a release tag may carry once its leading
// "v" is dropped: the manifest's own semver grammar, nothing else.
var releaseVersionRe = regexp.MustCompile(`^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)(-[0-9A-Za-z.-]{1,40})?$`)

// ValidReleaseVersion reports whether a release's version is one the server
// will compare and display; anything else is not read as a version at all.
func ValidReleaseVersion(v string) bool {
	return len(v) <= 64 && releaseVersionRe.MatchString(v)
}

// IsPreRelease: a version with a pre-release label ("1.2.0-beta.1"). Such a
// release never installs on its own.
func IsPreRelease(v string) bool {
	return strings.Contains(v, "-")
}

func validRepoSegment(s string) bool {
	if s == "" || len(s) > 100 || s == "." || s == ".." {
		return false
	}
	for _, r := range s {
		ok := r == '-' || r == '_' || r == '.' ||
			(r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')
		if !ok {
			return false
		}
	}
	return true
}

// PluginUpdateAttention asks the admins to look at a release that was not
// applied on its own: held for their approval, or refused.
type PluginUpdateAttention struct {
	PluginID   string
	Name       string
	Version    string
	Outcome    UpdateOutcome
	Reason     string
	OccurredAt time.Time
}

func (e PluginUpdateAttention) EventName() string { return "plugin.update.attention" }
