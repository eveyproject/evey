//go:build unix

package safefs

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"
)

// OpenRegular opens root/rel read-only. root is trusted (operator config) and
// may itself be a link; no component of rel may be one. FIFOs and devices are
// opened non-blocking and refused with ErrNotRegular.
func OpenRegular(root, rel string) (*os.File, error) {
	fd, err := openBeneath(root, rel, unix.O_RDONLY|unix.O_NONBLOCK|unix.O_NOCTTY)
	if err != nil {
		return nil, err
	}
	if err := requireRegular(fd); err != nil {
		closeFD(fd)
		return nil, err
	}
	if err := unix.SetNonblock(fd, false); err != nil {
		closeFD(fd)
		return nil, err
	}
	return os.NewFile(uintptr(fd), filepath.Join(root, rel)), nil
}

// StatRegular returns the info of root/rel under the OpenRegular rules
// without reading it (no read permission needed where O_PATH exists).
func StatRegular(root, rel string) (os.FileInfo, error) {
	fd, err := openBeneath(root, rel, statOpenFlags)
	if err != nil {
		return nil, err
	}
	f := os.NewFile(uintptr(fd), filepath.Join(root, rel))
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, ErrNotRegular
	}
	return info, nil
}

// ReadFile reads root/rel under the OpenRegular rules. maxBytes > 0 caps the
// size (ErrTooLarge); the info comes from the opened fd, not a path re-stat.
func ReadFile(root, rel string, maxBytes int64) ([]byte, os.FileInfo, error) {
	f, err := OpenRegular(root, rel)
	if err != nil {
		return nil, nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, nil, err
	}
	if maxBytes > 0 && info.Size() > maxBytes {
		return nil, info, ErrTooLarge
	}
	var r io.Reader = f
	if maxBytes > 0 {
		r = io.LimitReader(f, maxBytes+1)
	}
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, nil, err
	}
	if maxBytes > 0 && int64(len(data)) > maxBytes {
		return nil, info, ErrTooLarge
	}
	return data, info, nil
}

// WriteFileReplace writes data to dir/name through a fresh temp file renamed
// over name: a link planted at name is replaced, never followed. dir itself
// must not be a symlink and name must be a single component.
func WriteFileReplace(dir, name string, data []byte, perm os.FileMode) error {
	if name == "" || name == "." || name == ".." || strings.ContainsAny(name, "/\x00") {
		return ErrInvalidPath
	}
	dirfd, err := unix.Open(dir, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return &os.PathError{Op: "open", Path: dir, Err: err}
	}
	defer closeFD(dirfd)

	var rnd [8]byte
	if _, rerr := rand.Read(rnd[:]); rerr != nil {
		return rerr
	}
	tmp := ".safefs-" + hex.EncodeToString(rnd[:]) + ".tmp"
	fd, err := unix.Openat(dirfd, tmp, unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_NOFOLLOW|unix.O_CLOEXEC, uint32(perm.Perm()))
	if err != nil {
		return &os.PathError{Op: "create", Path: filepath.Join(dir, tmp), Err: err}
	}
	f := os.NewFile(uintptr(fd), filepath.Join(dir, tmp))
	_, werr := f.Write(data)
	if cerr := f.Close(); werr == nil {
		werr = cerr
	}
	if werr == nil {
		werr = unix.Renameat(dirfd, tmp, dirfd, name)
	}
	if werr != nil {
		if uerr := unix.Unlinkat(dirfd, tmp, 0); uerr != nil {
			slog.Debug("safefs: remove temp file", slog.String("path", filepath.Join(dir, tmp)), slog.Any("err", uerr))
		}
		return fmt.Errorf("safefs: replace %s: %w", filepath.Join(dir, name), werr)
	}
	return nil
}

// openBeneath opens root/rel with flags, refusing every link in rel: openat2
// RESOLVE_BENEATH|RESOLVE_NO_SYMLINKS when the kernel has it, else a
// component walk with O_NOFOLLOW on each openat.
func openBeneath(root, rel string, flags int) (int, error) {
	comps, err := splitRel(rel)
	if err != nil {
		return -1, err
	}
	rootfd, err := unix.Open(root, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if err != nil {
		return -1, &os.PathError{Op: "open", Path: root, Err: err}
	}
	defer closeFD(rootfd)

	if fd, ok, err := openat2Beneath(rootfd, strings.Join(comps, "/"), flags); ok {
		if err != nil {
			return -1, &os.PathError{Op: "openat2", Path: filepath.Join(root, rel), Err: err}
		}
		return fd, nil
	}
	return walkBeneath(rootfd, root, comps, flags)
}

// walkBeneath is the portable fallback: each intermediate component must be
// a real directory, the last one is opened with O_NOFOLLOW.
func walkBeneath(rootfd int, root string, comps []string, flags int) (int, error) {
	dirfd := rootfd
	closeDir := func() {
		if dirfd != rootfd {
			closeFD(dirfd)
		}
	}
	defer func() { closeDir() }()
	for i, c := range comps {
		last := i == len(comps)-1
		f := unix.O_RDONLY | unix.O_DIRECTORY | unix.O_NOFOLLOW | unix.O_CLOEXEC
		if last {
			f = flags | unix.O_NOFOLLOW | unix.O_CLOEXEC
		}
		fd, err := unix.Openat(dirfd, c, f, 0)
		if err != nil {
			return -1, &os.PathError{Op: "openat", Path: filepath.Join(append([]string{root}, comps[:i+1]...)...), Err: err}
		}
		if last {
			return fd, nil
		}
		var st unix.Stat_t
		if err := unix.Fstat(fd, &st); err != nil || st.Mode&unix.S_IFMT != unix.S_IFDIR {
			closeFD(fd)
			return -1, ErrNotRegular
		}
		closeDir()
		dirfd = fd
	}
	return -1, ErrInvalidPath
}

// closeFD closes a raw fd; nothing was written through it, so a close error
// changes nothing for the caller.
func closeFD(fd int) {
	if err := unix.Close(fd); err != nil {
		slog.Debug("safefs: close fd", slog.Any("err", err))
	}
}

func requireRegular(fd int) error {
	var st unix.Stat_t
	if err := unix.Fstat(fd, &st); err != nil {
		return err
	}
	if st.Mode&unix.S_IFMT != unix.S_IFREG {
		return ErrNotRegular
	}
	return nil
}
