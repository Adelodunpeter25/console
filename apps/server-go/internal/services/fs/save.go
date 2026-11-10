// Atomic, conflict-checked file saves for the desktop editor.
package fs

import (
	"fmt"
	"os"
	"path/filepath"
)

// FileChangedError reports that a save was refused because the file on disk
// no longer matches the ETag the caller's buffer was loaded at.
type FileChangedError struct {
	CurrentEtag string
}

func (e *FileChangedError) Error() string {
	return "File changed on disk since it was loaded."
}

// SaveFileResult is the post-write state of the saved file.
type SaveFileResult struct {
	Path      string
	Etag      string
	SizeBytes int64
	MtimeMs   int64
}

// SaveFileContent writes content to path atomically, refusing the write when
// baseEtag is set and no longer matches the file's current ETag.
//
// The bytes are written verbatim: the caller (the editor) has already applied
// the file's own line endings, so CRLF/LF is never rewritten here. A symlinked
// target is resolved so the link is rewritten in place rather than replaced by
// a regular file, and an existing file keeps its permissions.
func (s *FsService) SaveFileContent(path, content, baseEtag string) (SaveFileResult, error) {
	if path == "" {
		return SaveFileResult{}, fmt.Errorf("Path is required.")
	}
	target := path
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		target = resolved
	} else if !os.IsNotExist(err) {
		return SaveFileResult{}, err
	}

	dir := filepath.Dir(target)
	info, err := os.Lstat(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return SaveFileResult{}, fmt.Errorf("Directory %q does not exist.", dir)
		}
		return SaveFileResult{}, err
	}
	if !info.IsDir() {
		return SaveFileResult{}, fmt.Errorf("%q is not a directory.", dir)
	}

	existing, statErr := os.Stat(target)
	if statErr != nil && !os.IsNotExist(statErr) {
		return SaveFileResult{}, statErr
	}
	perm := os.FileMode(0o644)
	if statErr == nil {
		if baseEtag != "" {
			current := BuildFileETag(existing.Size(), existing.ModTime().UnixMilli())
			if current != baseEtag {
				return SaveFileResult{}, &FileChangedError{CurrentEtag: current}
			}
		}
		perm = existing.Mode().Perm()
	} else if baseEtag != "" {
		// The buffer was loaded from a file that has since been deleted; a
		// blind write would resurrect it over an intentional removal.
		return SaveFileResult{}, &FileChangedError{}
	}

	if err := writeFileAtomic(target, []byte(content), perm); err != nil {
		return SaveFileResult{}, err
	}

	written, err := os.Stat(target)
	if err != nil {
		return SaveFileResult{}, err
	}
	return SaveFileResult{
		Path:      path,
		Etag:      BuildFileETag(written.Size(), written.ModTime().UnixMilli()),
		SizeBytes: written.Size(),
		MtimeMs:   written.ModTime().UnixMilli(),
	}, nil
}

// writeFileAtomic writes through a sibling temp file, fsyncs it, then renames
// it over target. Any failure removes the temp file and leaves target intact.
func writeFileAtomic(target string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(target)
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(target)+".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	cleanup := func() {
		tmp.Close()
		os.Remove(tmpName)
	}
	if err := tmp.Chmod(perm); err != nil {
		cleanup()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		cleanup()
		return err
	}
	if err := tmp.Sync(); err != nil {
		cleanup()
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return err
	}
	if err := os.Rename(tmpName, target); err != nil {
		os.Remove(tmpName)
		return err
	}
	return nil
}
