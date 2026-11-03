package index

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

var (
	markdownExtensions = []string{".md", ".markdown"}
	scanForTestLogger  = slog.New(slog.NewTextHandler(io.Discard, nil))
)

func TestScanTreeShape(t *testing.T) {
	root := buildTree(t, []string{
		"README.md",
		"zeta.md",
		"Alpha.MD",
		"notes.markdown",
		"image.png",
		".hidden.md",
		"docs/guide.md",
		"docs/deep/nested/leaf.md",
		"docs/images/logo.png",
		"empty/",
		"no-markdown/script.sh",
		"no-markdown/sub/data.json",
		".github/CONTRIBUTING.md",
		".config/settings.json",
		".git/HEAD.md",
		"node_modules/pkg/README.md",
		"sub/node_modules/pkg/README.md",
		"sub/keep.md",
		"Beta/b.md",
	})
	idx := scanForTest(t, root)

	assertChildren(t, idx, "", []string{
		"dir:.github", "dir:Beta", "dir:docs", "dir:sub",
		"file:Alpha.MD", "file:notes.markdown", "file:README.md", "file:zeta.md",
	})
	assertChildren(t, idx, "docs", []string{"dir:deep", "file:guide.md"})
	assertChildren(t, idx, "docs/deep", []string{"dir:nested"})
	assertChildren(t, idx, "docs/deep/nested", []string{"file:leaf.md"})
	assertChildren(t, idx, "sub", []string{"file:keep.md"})

	for _, prunedDir := range []string{"empty", "no-markdown", "docs/images", ".config", ".git", "node_modules", "sub/node_modules"} {
		if _, found := idx.Children(prunedDir); found {
			t.Errorf("Children(%q) found, want pruned", prunedDir)
		}
	}
	if idx.FileCount != 9 {
		t.Errorf("FileCount = %d, want 9", idx.FileCount)
	}
	if idx.DirCount != 6 {
		t.Errorf("DirCount = %d, want 6", idx.DirCount)
	}
	wantFiles := []string{".github/CONTRIBUTING.md", "Alpha.MD", "Beta/b.md", "README.md", "docs/deep/nested/leaf.md",
		"docs/guide.md", "notes.markdown", "sub/keep.md", "zeta.md"}
	if !slices.Equal(idx.FilePaths(), wantFiles) {
		t.Errorf("FilePaths = %v\nwant %v", idx.FilePaths(), wantFiles)
	}
	if !idx.HasFile("docs/deep/nested/leaf.md") || !idx.HasFile("README.md") {
		t.Error("HasFile missing an indexed file")
	}
	if idx.HasFile(".hidden.md") || idx.HasFile("docs") || idx.HasFile("image.png") {
		t.Error("HasFile reported a non-indexed path")
	}
}

func TestScanEntryPaths(t *testing.T) {
	idx := scanForTest(t, buildTree(t, []string{"a/b/c.md"}))
	entries, _ := idx.Children("a/b")
	if len(entries) != 1 || entries[0] != (Entry{Name: "c.md", Path: "a/b/c.md", Type: EntryFile}) {
		t.Fatalf("entries = %+v", entries)
	}
}

func TestScanEmptyRoot(t *testing.T) {
	idx := scanForTest(t, buildTree(t, []string{"code.go"}))
	entries, found := idx.Children("")
	if !found || len(entries) != 0 {
		t.Fatalf("Children(\"\") = %v, %v; want empty and found", entries, found)
	}
}

func TestScanDoesNotFollowSymlinks(t *testing.T) {
	outside := buildTree(t, []string{"secret.md"})
	root := buildTree(t, []string{"real.md"})
	if err := os.Symlink(outside, filepath.Join(root, "linked-dir")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "real.md"), filepath.Join(root, "linked.md")); err != nil {
		t.Fatal(err)
	}
	// A self-referencing link would loop forever if followed.
	if err := os.Symlink(root, filepath.Join(root, "loop")); err != nil {
		t.Fatal(err)
	}
	assertChildren(t, scanForTest(t, root), "", []string{"file:real.md"})
}

func TestScanSkipsUnreadableDir(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root can read any directory")
	}
	root := buildTree(t, []string{"ok/a.md", "locked/b.md"})
	lockedDir := filepath.Join(root, "locked")
	if err := os.Chmod(lockedDir, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(lockedDir, 0o755) })

	assertChildren(t, scanForTest(t, root), "", []string{"dir:ok"})
}

func TestScanMissingRoot(t *testing.T) {
	if _, err := Scan(filepath.Join(t.TempDir(), "nope"), Options{Extensions: markdownExtensions}); err == nil {
		t.Fatal("Scan of missing root succeeded")
	}
}

// buildTree creates the given paths under a new temp dir. A trailing slash
// creates an empty directory; anything else creates a small file.
func buildTree(t testing.TB, paths []string) string {
	t.Helper()
	root := t.TempDir()
	for _, relPath := range paths {
		absPath := filepath.Join(root, filepath.FromSlash(relPath))
		if relPath[len(relPath)-1] == '/' {
			mustMkdirAll(t, absPath)
			continue
		}
		mustMkdirAll(t, filepath.Dir(absPath))
		if err := os.WriteFile(absPath, []byte("# x\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func writeTestFile(t testing.TB, root, relPath, content string) {
	t.Helper()
	absPath := filepath.Join(root, filepath.FromSlash(relPath))
	mustMkdirAll(t, filepath.Dir(absPath))
	if err := os.WriteFile(absPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func mustMkdirAll(t testing.TB, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
}

func scanForTest(t testing.TB, root string) *Index {
	t.Helper()
	idx, err := Scan(root, Options{
		Extensions: markdownExtensions,
		Logger:     scanForTestLogger,
	})
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	return idx
}

func assertChildren(t *testing.T, idx *Index, dirPath string, want []string) {
	t.Helper()
	entries, found := idx.Children(dirPath)
	if !found {
		t.Errorf("Children(%q) not found", dirPath)
		return
	}
	var got []string
	for _, entry := range entries {
		got = append(got, string(entry.Type)+":"+entry.Name)
	}
	if !slices.Equal(got, want) {
		t.Errorf("Children(%q) = %v\nwant %v", dirPath, got, want)
	}
}
