// Package pathsafe validates user-supplied paths before they touch the
// filesystem. Every HTTP handler that takes a path goes through here.
package pathsafe

import (
	"errors"
	"fmt"
	"path"
	"path/filepath"
	"strings"
)

var (
	ErrInvalidPath = errors.New("invalid path")
	ErrOutsideRoot = errors.New("path resolves outside root")
)

// Clean validates a slash-separated path relative to the root and returns it
// unchanged, with "" meaning the root itself. It does not normalise: any
// non-canonical form ("a//b", "a/./b", "a/../b", trailing slash) is rejected,
// so one file has exactly one accepted spelling and the index can be keyed by
// it.
func Clean(userPath string) (string, error) {
	if userPath == "" {
		return "", nil
	}
	// Backslashes are rejected even on Unix, where they are legal in names,
	// because filepath.FromSlash would turn them into separators on Windows.
	if strings.ContainsAny(userPath, "\x00\\") {
		return "", fmt.Errorf("%w: %q contains a NUL byte or backslash", ErrInvalidPath, userPath)
	}
	if strings.HasPrefix(userPath, "/") {
		return "", fmt.Errorf("%w: %q is absolute", ErrInvalidPath, userPath)
	}
	for _, segment := range strings.Split(userPath, "/") {
		if segment == ".." {
			return "", fmt.Errorf("%w: %q contains '..'", ErrInvalidPath, userPath)
		}
	}
	if path.Clean(userPath) != userPath || userPath == "." {
		return "", fmt.Errorf("%w: %q is not in canonical form", ErrInvalidPath, userPath)
	}
	return userPath, nil
}

// Resolve validates relPath and returns the real filesystem path it refers to
// under root, following symlinks. A symlink is acceptable only if its target
// is still inside root. root must be absolute with its own symlinks already
// resolved, otherwise the containment check compares mismatched prefixes.
//
// A missing file yields an error wrapping fs.ErrNotExist.
func Resolve(root, relPath string) (string, error) {
	cleanPath, err := Clean(relPath)
	if err != nil {
		return "", err
	}
	joinedPath := filepath.Join(root, filepath.FromSlash(cleanPath))
	realPath, err := filepath.EvalSymlinks(joinedPath)
	if err != nil {
		return "", fmt.Errorf("resolve %q: %w", cleanPath, err)
	}
	pathFromRoot, err := filepath.Rel(root, realPath)
	if err != nil {
		return "", fmt.Errorf("%w: %q", ErrOutsideRoot, cleanPath)
	}
	if pathFromRoot == ".." || strings.HasPrefix(pathFromRoot, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("%w: %q", ErrOutsideRoot, cleanPath)
	}
	return realPath, nil
}
