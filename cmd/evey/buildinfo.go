package main

import (
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"time"
)

// unknownStamp is what every build-identity accessor returns when it cannot
// prove what it is running. It is a value the tooling REFUSES (see
// scripts/verify-deploy.sh), never a value it silently accepts: a server that
// cannot name its commit must say so, because the alternative - inventing a
// plausible answer - is what made "which image is in production?" answerable
// only by grepping a served HTML asset.
const unknownStamp = "unknown"

// Stamped at link time:
//
//	-ldflags "-X main.version=... -X main.commit=<sha> -X main.buildDate=<RFC3339>"
//
// The Docker build passes them from the CI checkout (.git is dockerignored, so
// the toolchain's own VCS stamp is unavailable inside the image); a plain
// `go build` on a dev box gets them from buildVCS below instead.
var (
	version   = "dev"
	commit    = unknownStamp
	buildDate = unknownStamp
)

// The TV app build released with this server, stamped from
// frontend/apps/tv/app.json (expo.version, expo.android.versionCode):
//
//	-ldflags "-X main.tvVersion=0.1.11 -X main.tvVersionCode=56"
var (
	tvVersion     = ""
	tvVersionCode = ""
)

// tvRelease reads those stamps. An unstamped or malformed versionCode yields
// 0, which /healthz treats as "no TV release known" rather than a version.
func tvRelease() (string, int) {
	code, err := strconv.Atoi(strings.TrimSpace(tvVersionCode))
	if err != nil || code <= 0 {
		return "", 0
	}
	return strings.TrimSpace(tvVersion), code
}

// buildIdentity is the single answer to "what is this binary?", resolved once
// so /healthz, the admin API and the boot banner cannot disagree.
type buildIdentity struct {
	Version   string
	Commit    string
	BuildDate string
}

// buildID is that single answer. Every surface reads it from here; nothing
// re-derives a version, a commit or a date on its own.
var buildID = sync.OnceValue(resolveBuildIdentity)

// resolveBuildIdentity prefers the link-time stamps and falls back to the VCS
// metadata the Go toolchain embeds when the build happened inside a git
// checkout. Anything blank or unusable collapses to unknownStamp - an empty
// string in a JSON field reads as "field not implemented" to a client, which
// is exactly the ambiguity this whole mechanism exists to remove.
func resolveBuildIdentity() buildIdentity {
	id := buildIdentity{
		Version:   firstNonEmpty(version, "dev"),
		Commit:    normalizeStamp(commit),
		BuildDate: normalizeStamp(buildDate),
	}
	if id.Commit != unknownStamp && id.BuildDate != unknownStamp {
		return id
	}
	vcsRev, vcsTime := buildVCS()
	if id.Commit == unknownStamp {
		id.Commit = normalizeStamp(vcsRev)
	}
	if id.BuildDate == unknownStamp {
		id.BuildDate = normalizeStamp(vcsTime)
	}
	return id
}

// buildVCS reads the revision/time the toolchain stamps into the binary when
// it is built from a git working tree with buildvcs enabled.
func buildVCS() (rev, buildTime string) {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "", ""
	}
	var dirty bool
	for _, s := range info.Settings {
		switch s.Key {
		case "vcs.revision":
			rev = s.Value
		case "vcs.time":
			buildTime = s.Value
		case "vcs.modified":
			dirty = s.Value == "true"
		}
	}
	// A dirty tree's revision names a commit whose code is NOT what is running.
	// Reporting it would be a lie of exactly the kind this file removes.
	if dirty && rev != "" {
		rev += "-dirty"
	}
	return rev, buildTime
}

func normalizeStamp(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return unknownStamp
	}
	return s
}

func firstNonEmpty(s, fallback string) string {
	if strings.TrimSpace(s) == "" {
		return fallback
	}
	return s
}

// shortCommit is the 7-char form used in log lines and the boot banner; the
// APIs always report the full stamp.
func shortCommit(c string) string {
	if c == unknownStamp || len(c) < 7 {
		return c
	}
	return c[:7]
}

// buildTimeAge reports how old the build is, for the boot log. Zero when the
// date is unknown or unparseable.
func buildTimeAge(buildDate string, now time.Time) time.Duration {
	t, err := time.Parse(time.RFC3339, buildDate)
	if err != nil {
		return 0
	}
	return now.Sub(t)
}
