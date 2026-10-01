//go:build !unix

package safefs

import (
	"errors"
	"os"
)

// Non-unix builds cannot refuse links race-free: every call fails closed.

func OpenRegular(string, string) (*os.File, error) { return nil, errors.ErrUnsupported }

func StatRegular(string, string) (os.FileInfo, error) { return nil, errors.ErrUnsupported }

func ReadFile(string, string, int64) ([]byte, os.FileInfo, error) {
	return nil, nil, errors.ErrUnsupported
}

func WriteFileReplace(string, string, []byte, os.FileMode) error { return errors.ErrUnsupported }
