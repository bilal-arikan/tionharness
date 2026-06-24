package backup

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// zipDir writes every regular file under srcDir into a zip archive at dstPath,
// preserving the relative directory layout. Any path inside `skip` (the backups
// root) is excluded so a workspace whose data dir is an ancestor of the backups
// folder never archives its own backups recursively. Returns the archive size
// in bytes.
func zipDir(srcDir, dstPath, skip string) (int64, error) {
	out, err := os.Create(dstPath)
	if err != nil {
		return 0, err
	}
	zw := zip.NewWriter(out)

	skipAbs, _ := filepath.Abs(skip)

	walkErr := filepath.Walk(srcDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		// Exclude the backups root (and anything under it) to avoid recursion.
		if skipAbs != "" {
			if abs, aerr := filepath.Abs(path); aerr == nil {
				if abs == skipAbs || strings.HasPrefix(abs, skipAbs+string(os.PathSeparator)) {
					if info.IsDir() {
						return filepath.SkipDir
					}
					return nil
				}
			}
		}
		// Skip directories themselves (zip entries are created from files); empty
		// dirs are intentionally not preserved (no data to restore).
		if info.IsDir() {
			return nil
		}
		// Only regular files; skip sockets/symlinks/etc. that can't be archived.
		if !info.Mode().IsRegular() {
			return nil
		}
		rel, rerr := filepath.Rel(srcDir, path)
		if rerr != nil {
			return rerr
		}
		// zip uses forward slashes.
		w, werr := zw.Create(filepath.ToSlash(rel))
		if werr != nil {
			return werr
		}
		f, oerr := os.Open(path)
		if oerr != nil {
			return oerr
		}
		_, cerr := io.Copy(w, f)
		f.Close()
		return cerr
	})

	closeErr := zw.Close()
	if cerr := out.Close(); closeErr == nil {
		closeErr = cerr
	}
	if walkErr != nil {
		return 0, walkErr
	}
	if closeErr != nil {
		return 0, closeErr
	}

	if st, serr := os.Stat(dstPath); serr == nil {
		return st.Size(), nil
	}
	return 0, nil
}

// Unzip extracts a zip archive into destDir, recreating the stored relative
// directory layout. Entries are written under destDir; any entry that would
// escape destDir (via "..") is rejected (zip-slip guard). destDir is created if
// missing. Existing files at a target path are overwritten.
func Unzip(archivePath, destDir string) error {
	zr, err := zip.OpenReader(archivePath)
	if err != nil {
		return err
	}
	defer zr.Close()

	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return err
	}
	destAbs, err := filepath.Abs(destDir)
	if err != nil {
		return err
	}

	for _, f := range zr.File {
		// Normalise and guard against zip-slip.
		target := filepath.Join(destDir, filepath.FromSlash(f.Name))
		targetAbs, aerr := filepath.Abs(target)
		if aerr != nil {
			return aerr
		}
		if targetAbs != destAbs && !strings.HasPrefix(targetAbs, destAbs+string(os.PathSeparator)) {
			return fmt.Errorf("unsafe archive entry %q", f.Name)
		}
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		rc, oerr := f.Open()
		if oerr != nil {
			return oerr
		}
		out, cerr := os.Create(target)
		if cerr != nil {
			rc.Close()
			return cerr
		}
		_, copyErr := io.Copy(out, rc)
		rc.Close()
		out.Close()
		if copyErr != nil {
			return copyErr
		}
	}
	return nil
}
