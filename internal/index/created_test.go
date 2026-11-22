package index

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"
)

func TestFilesCreatedSince(t *testing.T) {
	root := buildTree(t, []string{"old.md", "docs/new.md", "docs/newer.md", "image.png"})
	// Backdating mtime also backdates the birth time on macOS, where a birth
	// time later than the mtime is pulled back to it.
	longAgo := time.Now().Add(-time.Hour)
	if err := os.Chtimes(filepath.Join(root, "old.md"), longAgo, longAgo); err != nil {
		t.Fatal(err)
	}
	idx := scanForTest(t, root)

	recent := idx.FilesCreatedSince(time.Now().Add(-5 * time.Minute))
	var recentPaths []string
	for _, file := range recent {
		recentPaths = append(recentPaths, file.Path)
	}
	slices.Sort(recentPaths)
	if !slices.Equal(recentPaths, []string{"docs/new.md", "docs/newer.md"}) {
		t.Fatalf("recent paths = %v", recentPaths)
	}
	if recent[0].CreatedAt.Before(recent[1].CreatedAt) {
		t.Errorf("not newest first: %+v", recent)
	}

	if got := idx.FilesCreatedSince(time.Now().Add(time.Minute)); len(got) != 0 {
		t.Errorf("future cutoff returned %+v", got)
	}
	if got := idx.FilesCreatedSince(time.Time{}); len(got) != 3 {
		t.Errorf("zero cutoff returned %d files, want 3", len(got))
	}
}
