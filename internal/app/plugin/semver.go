package plugin

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"strconv"
	"strings"
)

// semver.go holds the installer's pure helpers: file hashing, semver
// comparison and small set utilities, shared by the install / preview /
// update paths.

func hashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// CompareSemver compares two semver strings (X.Y.Z). Returns -1, 0, or 1.
// Pre-release labels are ignored for numeric comparison.
func CompareSemver(a, b string) int {
	pa := parseSemverParts(a)
	pb := parseSemverParts(b)
	for i := range 3 {
		if pa[i] < pb[i] {
			return -1
		}
		if pa[i] > pb[i] {
			return 1
		}
	}
	return 0
}

// sliceToSet converts a string slice to a set map for O(1) membership tests.
func sliceToSet(ss []string) map[string]struct{} {
	m := make(map[string]struct{}, len(ss))
	for _, s := range ss {
		m[s] = struct{}{}
	}
	return m
}

func parseSemverParts(v string) [3]int {
	// strip pre-release label
	if idx := strings.IndexByte(v, '-'); idx >= 0 {
		v = v[:idx]
	}
	parts := strings.SplitN(v, ".", 3)
	var result [3]int
	for i := range 3 {
		if i < len(parts) {
			n, _ := strconv.Atoi(parts[i])
			result[i] = n
		}
	}
	return result
}
