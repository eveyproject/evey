// Package safefs reads and writes inside a directory tree that a less trusted
// party (a plugin running under the server uid) can modify: no symbolic link
// below the root is ever followed, and only regular files are read.
package safefs

import (
	"errors"
	"strings"
)

var (
	// ErrInvalidPath: the relative path is empty, absolute, or has an empty,
	// "." or ".." component.
	ErrInvalidPath = errors.New("safefs: invalid relative path")
	// ErrNotRegular: the target exists but is not a regular file (directory,
	// FIFO, device, socket, symlink).
	ErrNotRegular = errors.New("safefs: not a regular file")
	// ErrTooLarge: the file exceeds the caller's size limit.
	ErrTooLarge = errors.New("safefs: file too large")
)

// splitRel validates a slash-separated relative path and returns its components.
func splitRel(rel string) ([]string, error) {
	if rel == "" || strings.HasPrefix(rel, "/") || strings.Contains(rel, "\x00") {
		return nil, ErrInvalidPath
	}
	comps := strings.Split(rel, "/")
	for _, c := range comps {
		if c == "" || c == "." || c == ".." {
			return nil, ErrInvalidPath
		}
	}
	return comps, nil
}
