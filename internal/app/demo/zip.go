package demo

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
)

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
		if _, werr := io.Copy(out, rc); werr != nil {
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
