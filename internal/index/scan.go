package index

import (
	"bufio"
	"bytes"
	"cmp"
	"fmt"
	"log/slog"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/go-git/go-git/v5/plumbing/format/gitignore"
)

// maxConcurrentDirReads bounds how many directories are read at once. Scan
// time is almost entirely ReadDir syscalls; benchmarked on an SSD with a warm
// file cache, the speedup levels off around 8 concurrent reads.
const maxConcurrentDirReads = 8

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
	scanStart := time.Now()
	if _, err := os.ReadDir(root); err != nil {
		return nil, fmt.Errorf("scan root: %w", err)
	}
	scanner := &scanner{
		root:                root,
		extensions:          opts.Extensions,
		logger:              cmp.Or(opts.Logger, slog.Default()),
		walkSlots:           make(chan struct{}, maxConcurrentDirReads-1),
		childrenByDir:       make(map[string][]Entry),
		createdAtByPath:     make(map[string]time.Time),
		ignorePatternsByDir: make(map[string][]gitignore.Pattern),
	}
	scanner.walkDir("", nil)
	// The root is always listed, even when it has no markdown, so the UI gets
	// an empty tree rather than a 404.
	if _, found := scanner.childrenByDir[""]; !found {
		scanner.childrenByDir[""] = []Entry{}
	}
	return &Index{
		IndexedAt:       time.Now(),
		ScanDuration:    time.Since(scanStart),
		FileCount:       len(scanner.filePaths),
		DirCount:        len(scanner.childrenByDir) - 1,
		childrenByDir:   scanner.childrenByDir,
		createdAtByPath: scanner.createdAtByPath,
		filePaths:       scanner.sortedFilePaths(),
		ignoreMatcher:   gitignore.NewMatcher(scanner.allIgnorePatterns()),
	}, nil
}

type scanner struct {
	root       string
	extensions []string
	logger     *slog.Logger
	// walkSlots has one slot per goroutine allowed to walk alongside the one
	// running Scan, so at most maxConcurrentDirReads directories are read at
	// once.
	walkSlots chan struct{}

	mu              sync.Mutex // guards the fields below
	childrenByDir   map[string][]Entry
	createdAtByPath map[string]time.Time
	filePaths       []string
	// ignorePatternsByDir holds each directory's .gitignore patterns, for
	// Index.IsServable.
	ignorePatternsByDir map[string][]gitignore.Pattern
}

// walkDir records relDir's markdown children and reports whether relDir has
// any markdown beneath it. Directories that don't are left out of
// childrenByDir and of their parent's entries, which is the bottom-up prune.
//
// ancestorPatterns holds the .gitignore patterns of relDir's ancestors only.
// Matching against those, rather than every pattern in the repo, keeps the
// per-entry cost proportional to depth instead of repo size.
func (s *scanner) walkDir(relDir string, ancestorPatterns []gitignore.Pattern) bool {
	absDir := filepath.Join(s.root, filepath.FromSlash(relDir))
	dirEntries, err := os.ReadDir(absDir)
	if err != nil {
		s.logger.Warn("skipping unreadable directory", "dir", relDir, "err", err)
		return false
	}

	dirSegments := splitSegments(relDir)
	patterns := ancestorPatterns
	localPatterns := s.readIgnoreFile(absDir, relDir, dirSegments, dirEntries)
	if len(localPatterns) > 0 {
		// Clip before appending so sibling directories never share (and
		// overwrite) the same backing array.
		patterns = append(slices.Clip(ancestorPatterns), localPatterns...)
	}
	ignoreMatcher := gitignore.NewMatcher(patterns)

	var children []Entry
	var localFilePaths []string
	localCreatedAt := make(map[string]time.Time)
	var subdirNames []string
	for _, dirEntry := range dirEntries {
		name := dirEntry.Name()
		if dirEntry.Type()&os.ModeSymlink != 0 {
			continue
		}
		if len(patterns) > 0 && ignoreMatcher.Match(append(dirSegments, name), dirEntry.IsDir()) {
			continue
		}
		switch {
		case dirEntry.IsDir():
			if !alwaysSkippedDirs[name] {
				subdirNames = append(subdirNames, name)
			}
		case dirEntry.Type().IsRegular():
			if strings.HasPrefix(name, ".") || !s.isMarkdown(name) {
				continue
			}
			childPath := joinRel(relDir, name)
			children = append(children, Entry{Name: name, Path: childPath, Type: EntryFile})
			localFilePaths = append(localFilePaths, childPath)
			// dirEntry.Info stats the file again, so it fails for a file
			// deleted since ReadDir. That file stays listed but never shows
			// as recently created.
			if info, err := dirEntry.Info(); err == nil {
				localCreatedAt[childPath] = fileCreatedAt(info)
			}
		}
	}

	// When no slot is free, walk the subdirectory on this goroutine. Otherwise,
	// with every slot held by a parent waiting for its children, blocking for
	// a slot would deadlock.
	subdirHasMarkdown := make([]bool, len(subdirNames))
	var subdirWalks sync.WaitGroup
	for position, name := range subdirNames {
		childPath := joinRel(relDir, name)
		select {
		case s.walkSlots <- struct{}{}:
			subdirWalks.Go(func() {
				defer func() { <-s.walkSlots }()
				subdirHasMarkdown[position] = s.walkDir(childPath, patterns)
			})
		default:
			subdirHasMarkdown[position] = s.walkDir(childPath, patterns)
		}
	}
	subdirWalks.Wait()
	for position, name := range subdirNames {
		if subdirHasMarkdown[position] {
			children = append(children, Entry{Name: name, Path: joinRel(relDir, name), Type: EntryDir})
		}
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if len(localPatterns) > 0 {
		s.ignorePatternsByDir[relDir] = localPatterns
	}
	if len(children) == 0 {
		return false
	}
	slices.SortFunc(children, compareEntries)
	s.childrenByDir[relDir] = children
	s.filePaths = append(s.filePaths, localFilePaths...)
	maps.Copy(s.createdAtByPath, localCreatedAt)
	return true
}

// allIgnorePatterns returns every .gitignore pattern with each directory's
// patterns after its ancestors'. The matcher lets later patterns win, which
// is how a nested !rule overrides a parent's rule; directories are walked
// concurrently, so that order has to be restored here. Sorting directory
// paths lexically is enough because a directory's path sorts before every
// path inside it.
func (s *scanner) allIgnorePatterns() []gitignore.Pattern {
	var patterns []gitignore.Pattern
	for _, dirPath := range slices.Sorted(maps.Keys(s.ignorePatternsByDir)) {
		patterns = append(patterns, s.ignorePatternsByDir[dirPath]...)
	}
	return patterns
}

// readIgnoreFile parses relDir's .gitignore, if it has one. A .gitignore that
// is a symlink is ignored, since reading it could pull rules from outside the
// root.
func (s *scanner) readIgnoreFile(absDir, relDir string, dirSegments []string, dirEntries []os.DirEntry) []gitignore.Pattern {
	hasIgnoreFile := slices.ContainsFunc(dirEntries, func(dirEntry os.DirEntry) bool {
		return dirEntry.Name() == ".gitignore" && dirEntry.Type().IsRegular()
	})
	if !hasIgnoreFile {
		return nil
	}
	content, err := os.ReadFile(filepath.Join(absDir, ".gitignore"))
	if err != nil {
		s.logger.Warn("skipping unreadable .gitignore", "dir", relDir, "err", err)
		return nil
	}
	var patterns []gitignore.Pattern
	lineScanner := bufio.NewScanner(bytes.NewReader(content))
	for lineScanner.Scan() {
		line := strings.TrimSuffix(lineScanner.Text(), "\r")
		if strings.HasPrefix(line, "#") || strings.TrimSpace(line) == "" {
			continue
		}
		patterns = append(patterns, gitignore.ParsePattern(line, dirSegments))
	}
	return patterns
}

func (s *scanner) sortedFilePaths() []string {
	slices.Sort(s.filePaths)
	if s.filePaths == nil {
		return []string{}
	}
	return s.filePaths
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

func splitSegments(relPath string) []string {
	if relPath == "" {
		return nil
	}
	return strings.Split(relPath, "/")
}

func splitParent(relPath string) (parentDir, name string) {
	lastSlash := strings.LastIndexByte(relPath, '/')
	if lastSlash < 0 {
		return "", relPath
	}
	return relPath[:lastSlash], relPath[lastSlash+1:]
}
