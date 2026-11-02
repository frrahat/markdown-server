package index

import (
	"cmp"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// alwaysSkippedDirs are skipped regardless of .gitignore: .git holds no docs
// and node_modules is often huge and frequently not gitignored in subprojects.
var alwaysSkippedDirs = map[string]bool{
	".git":         true,
	"node_modules": true,
}

type Options struct {
	// Extensions are lowercase and include the leading dot, e.g. ".md".
	Extensions []string
	Logger     *slog.Logger
}

// Scan walks root once and builds an Index. root must be an absolute path.
// Symlinks are never followed, which rules out cycles and escaping the root.
// Unreadable subdirectories are logged and skipped; only an unreadable root
// fails the scan.
func Scan(root string, opts Options) (*Index, error) {
	if _, err := os.ReadDir(root); err != nil {
		return nil, fmt.Errorf("scan root: %w", err)
	}
	scanner := &scanner{
		root:          root,
		extensions:    opts.Extensions,
		logger:        cmp.Or(opts.Logger, slog.Default()),
		childrenByDir: make(map[string][]Entry),
	}
	scanner.walkDir("")
	// The root is always listed, even when it has no markdown, so the UI gets
	// an empty tree rather than a 404.
	if _, found := scanner.childrenByDir[""]; !found {
		scanner.childrenByDir[""] = []Entry{}
	}
	return &Index{
		IndexedAt:     time.Now(),
		FileCount:     scanner.fileCount,
		DirCount:      len(scanner.childrenByDir) - 1,
		childrenByDir: scanner.childrenByDir,
	}, nil
}

type scanner struct {
	root          string
	extensions    []string
	logger        *slog.Logger
	childrenByDir map[string][]Entry
	fileCount     int
}

// walkDir records relDir's markdown children and reports whether relDir has
// any markdown beneath it. Directories that don't are left out of
// childrenByDir and of their parent's entries, which is the bottom-up prune.
func (s *scanner) walkDir(relDir string) bool {
	dirEntries, err := os.ReadDir(filepath.Join(s.root, filepath.FromSlash(relDir)))
	if err != nil {
		s.logger.Warn("skipping unreadable directory", "dir", relDir, "err", err)
		return false
	}

	var children []Entry
	for _, dirEntry := range dirEntries {
		name := dirEntry.Name()
		childPath := joinRel(relDir, name)
		switch {
		case dirEntry.Type()&os.ModeSymlink != 0:
			continue
		case dirEntry.IsDir():
			if alwaysSkippedDirs[name] {
				continue
			}
			if s.walkDir(childPath) {
				children = append(children, Entry{Name: name, Path: childPath, Type: EntryDir})
			}
		case dirEntry.Type().IsRegular():
			if strings.HasPrefix(name, ".") || !s.isMarkdown(name) {
				continue
			}
			children = append(children, Entry{Name: name, Path: childPath, Type: EntryFile})
			s.fileCount++
		}
	}
	if len(children) == 0 {
		return false
	}
	slices.SortFunc(children, compareEntries)
	s.childrenByDir[relDir] = children
	return true
}

func (s *scanner) isMarkdown(name string) bool {
	ext := strings.ToLower(filepath.Ext(name))
	return slices.Contains(s.extensions, ext)
}

// compareEntries orders dirs before files, then names case-insensitively,
// falling back to a case-sensitive compare so the order is deterministic.
func compareEntries(a, b Entry) int {
	if a.Type != b.Type {
		if a.Type == EntryDir {
			return -1
		}
		return 1
	}
	return cmp.Or(
		strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name)),
		strings.Compare(a.Name, b.Name),
	)
}

func joinRel(relDir, name string) string {
	if relDir == "" {
		return name
	}
	return relDir + "/" + name
}

func splitParent(relPath string) (parentDir, name string) {
	lastSlash := strings.LastIndexByte(relPath, '/')
	if lastSlash < 0 {
		return "", relPath
	}
	return relPath[:lastSlash], relPath[lastSlash+1:]
}
