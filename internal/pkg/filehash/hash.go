package filehash

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"os"
)

const chunkSize = 64 * 1024 // 64KB

// Compute returns a partial hash of a file for fast change detection.
// For files <= 128KB, the entire file is hashed.
// For larger files, only the first 64KB + last 64KB + file size are hashed.
func Compute(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return "", fmt.Errorf("stat %s: %w", path, err)
	}
	size := info.Size()

	h := sha256.New()

	if size <= 2*chunkSize {
		if _, err := io.Copy(h, f); err != nil {
			return "", fmt.Errorf("read %s: %w", path, err)
		}
	} else {
		head := make([]byte, chunkSize)
		if _, err := io.ReadFull(f, head); err != nil {
			return "", fmt.Errorf("read head %s: %w", path, err)
		}
		h.Write(head)

		if _, err := f.Seek(-chunkSize, io.SeekEnd); err != nil {
			return "", fmt.Errorf("seek %s: %w", path, err)
		}
		tail := make([]byte, chunkSize)
		if _, err := io.ReadFull(f, tail); err != nil {
			return "", fmt.Errorf("read tail %s: %w", path, err)
		}
		h.Write(tail)
	}

	var sizeBuf [8]byte
	binary.BigEndian.PutUint64(sizeBuf[:], uint64(size))
	h.Write(sizeBuf[:])

	return hex.EncodeToString(h.Sum(nil)), nil
}
