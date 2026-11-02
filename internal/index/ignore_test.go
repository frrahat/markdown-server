package index

import "testing"

func TestScanHonoursGitignore(t *testing.T) {
	root := buildTree(t, []string{
		"README.md",
		"draft.md",
		"keep.md",
		"build/out.md",
		"src/build/also-ignored.md",
		"src/notes.md",
		"src/tmp.md",
		"docs/a.md",
		"docs/private/secret.md",
		"docs/generated/api.md",
		"docs/sub/tmp.md",
		"other/private/visible.md",
		"other/tmp.md",
		"deep/x/y/z.md",
		"logs/app.log.md",
	})
	writeTestFile(t, root, ".gitignore", "# comment\n\n*.md\n!README.md\n!keep.md\n!*/\n!src/**\n!docs/**\n!other/**\n!deep/**\nbuild/\nlogs/\n")
	// Nested rules apply only inside their own directory.
	writeTestFile(t, root, "docs/.gitignore", "private/\n/generated\ntmp.md\r\n")
	writeTestFile(t, root, "src/.gitignore", "tmp.md\n")
	writeTestFile(t, root, "deep/x/.gitignore", "**/z.md\n")

	idx := scanForTest(t, root)

	assertChildren(t, idx, "", []string{"dir:docs", "dir:other", "dir:src", "file:keep.md", "file:README.md"})
	assertChildren(t, idx, "docs", []string{"file:a.md"})
	assertChildren(t, idx, "src", []string{"file:notes.md"})
	assertChildren(t, idx, "other", []string{"dir:private", "file:tmp.md"})
	for _, ignoredDir := range []string{"build", "src/build", "logs", "docs/private", "docs/generated", "docs/sub", "deep"} {
		if _, found := idx.Children(ignoredDir); found {
			t.Errorf("Children(%q) found, want ignored", ignoredDir)
		}
	}

	servable := map[string]bool{
		"README.md":                true,
		"docs/a.md":                true,
		"docs/img.png":             true,
		"other/private/visible.md": true,
		"other/tmp.md":             true,
		"draft.md":                 false,
		"build/out.md":             false,
		"build/img.png":            false,
		"src/build/x.png":          false,
		"docs/private/secret.md":   false,
		"docs/generated/api.md":    false,
		"docs/sub/tmp.md":          false,
		"src/tmp.md":               false,
		"deep/x/y/z.md":            false,
		".env":                     false,
		"docs/.env":                false,
		".git/config":              false,
		"sub/node_modules/a.png":   false,
		"":                         false,
	}
	for relPath, want := range servable {
		if got := idx.IsServable(relPath); got != want {
			t.Errorf("IsServable(%q) = %v, want %v", relPath, got, want)
		}
	}
}

func TestIsServableWithoutGitignore(t *testing.T) {
	idx := scanForTest(t, buildTree(t, []string{"a.md"}))
	if !idx.IsServable("img/logo.png") {
		t.Error("IsServable rejected a plain path with no ignore rules")
	}
	if idx.IsServable(".github/.secret") {
		t.Error("IsServable accepted a dotfile")
	}
}
