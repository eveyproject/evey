package demo

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
)

// maxZipMemberBytes caps one extracted member: the 4K 60 fps Big Buck Bunny
// master, the largest the manifest names, is under 1 GiB.
const maxZipMemberBytes = 4 << 30

// extractZipMember writes the named member of the archive at zipPath to dest,
// through a .part file like a plain download. The member is matched on its
// base name: the Blender masters ship as a single file at the archive root.
func extractZipMember(zipPath, member, dest string) (err error) {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return fmt.Errorf("open zip: %w", err)
	}
	defer func() {
		if cerr := r.Close(); cerr != nil && err == nil {
			err = cerr
		}
	}()
	for _, f := range r.File {
		if path.Base(f.Name) != member || f.FileInfo().IsDir() {
			continue
		}
		rc, oerr := f.Open()
		if oerr != nil {
			return oerr
		}
		defer func() { _ = rc.Close() }()
		part := dest + ".unzip"
		out, cerr := os.Create(part)
		if cerr != nil {
			return cerr
		}
		// Bounded: the header declares the member's size and a member larger
		// than the largest demo master is not one of ours - a decompression
		// bomb stops at the cap instead of filling the disk.
		n, werr := io.Copy(out, io.LimitReader(rc, maxZipMemberBytes+1))
		if werr == nil && (n > maxZipMemberBytes || uint64(n) != f.UncompressedSize64) {
			werr = fmt.Errorf("zip member %s: %d bytes extracted, header declares %d (cap %d)", member, n, f.UncompressedSize64, int64(maxZipMemberBytes))
		}
		if werr != nil {
			_ = out.Close()
			return errors.Join(werr, removeTemp(part))
		}
		if cerr := out.Close(); cerr != nil {
			return errors.Join(cerr, removeTemp(part))
		}
		return os.Rename(part, dest)
	}
	return fmt.Errorf("member %q not found in %s", member, zipPath)
}

// removeTemp deletes a partial extraction; a missing file is not an error.
func removeTemp(p string) error {
	if err := os.Remove(p); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}
