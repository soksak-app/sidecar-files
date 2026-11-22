package files

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"
)

// folderMode is the mode that a created folder requests; the process umask masks it, as os.Mkdir does.
const folderMode = 0o777

// CreateFolder creates root/path and each missing parent folder. An existing folder is not an error. It fails with
// `not a folder` when path or one of its existing components is not a folder, and it resolves path like List.
func CreateFolder(root, path string) error {
	target, err := resolveFolder(root, path)
	if err != nil {
		return err
	}
	return os.MkdirAll(target, folderMode)
}

// resolveFolder returns the absolute path that CreateFolder creates for the relative path inside root. The deepest existing
// ancestor of path is resolved through its symbolic links; the components below it do not exist, so they are new names
// inside that ancestor and cannot be symbolic links.
func resolveFolder(root, path string) (string, error) {
	if filepath.IsAbs(path) {
		return "", fmt.Errorf("path %q must be relative to the root", path)
	}
	base, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", err
	}
	joined := filepath.Join(base, path)
	// A path whose name leaves root is refused before any outside folder is looked up.
	if !within(base, joined) {
		return "", fmt.Errorf("path %q leaves the root", path)
	}
	existing := joined
	for {
		_, err := os.Lstat(existing)
		if err == nil {
			break
		}
		// ENOTDIR means that a component of the path is a file, so the path cannot name a folder.
		if errors.Is(err, syscall.ENOTDIR) {
			return "", fmt.Errorf("not a folder: %s", path)
		}
		if !errors.Is(err, fs.ErrNotExist) {
			return "", err
		}
		existing = filepath.Dir(existing)
	}
	resolved, err := filepath.EvalSymlinks(existing)
	if err != nil {
		return "", err
	}
	if !within(base, resolved) {
		return "", fmt.Errorf("path %q leaves the root", path)
	}
	relative, err := filepath.Rel(existing, joined)
	if err != nil {
		return "", err
	}
	target := filepath.Join(resolved, relative)
	info, err := os.Stat(target)
	if err == nil && !info.IsDir() {
		return "", fmt.Errorf("not a folder: %s", path)
	}
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return "", err
	}
	return target, nil
}
