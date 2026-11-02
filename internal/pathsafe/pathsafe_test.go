package pathsafe

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestClean(t *testing.T) {
	tests := []struct {
		name      string
		userPath  string
		wantPath  string
		wantError bool
	}{
		{name: "root", userPath: "", wantPath: ""},
		{name: "file at root", userPath: "README.md", wantPath: "README.md"},
		{name: "nested file", userPath: "docs/guide/intro.md", wantPath: "docs/guide/intro.md"},
		{name: "dot dir", userPath: ".github/CONTRIBUTING.md", wantPath: ".github/CONTRIBUTING.md"},
		// Query strings and URL paths arrive percent-decoded, so a literal
		// "%2e%2e" here is a filename, not a parent reference.
		{name: "literal percent name", userPath: "%2e%2e/a.md", wantPath: "%2e%2e/a.md"},

		{name: "parent", userPath: "..", wantError: true},
		{name: "parent prefix", userPath: "../etc/passwd", wantError: true},
		{name: "parent in middle", userPath: "docs/../../secret", wantError: true},
		{name: "parent that stays inside", userPath: "docs/../README.md", wantError: true},
		{name: "absolute", userPath: "/etc/passwd", wantError: true},
		{name: "backslash", userPath: `docs\..\secret`, wantError: true},
		{name: "nul byte", userPath: "a.md\x00.png", wantError: true},
		{name: "dot", userPath: ".", wantError: true},
		{name: "dot segment", userPath: "docs/./a.md", wantError: true},
		{name: "double slash", userPath: "docs//a.md", wantError: true},
		{name: "trailing slash", userPath: "docs/", wantError: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gotPath, err := Clean(tc.userPath)
			if tc.wantError {
				if !errors.Is(err, ErrInvalidPath) {
					t.Fatalf("Clean(%q) error = %v, want ErrInvalidPath", tc.userPath, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("Clean(%q) unexpected error: %v", tc.userPath, err)
			}
			if gotPath != tc.wantPath {
				t.Fatalf("Clean(%q) = %q, want %q", tc.userPath, gotPath, tc.wantPath)
			}
		})
	}
}

func TestResolve(t *testing.T) {
	root := newRoot(t)
	outside := t.TempDir()
	writeFile(t, filepath.Join(outside, "secret.md"))
	writeFile(t, filepath.Join(root, "docs", "a.md"))
	writeFile(t, filepath.Join(root, "real.md"))
	symlink(t, outside, filepath.Join(root, "escape-dir"))
	symlink(t, filepath.Join(outside, "secret.md"), filepath.Join(root, "escape.md"))
	symlink(t, filepath.Join(root, "real.md"), filepath.Join(root, "inside.md"))

	tests := []struct {
		name      string
		relPath   string
		wantPath  string
		wantError error
	}{
		{name: "root", relPath: "", wantPath: root},
		{name: "file", relPath: "docs/a.md", wantPath: filepath.Join(root, "docs", "a.md")},
		{name: "symlink staying inside", relPath: "inside.md", wantPath: filepath.Join(root, "real.md")},
		{name: "symlinked file outside", relPath: "escape.md", wantError: ErrOutsideRoot},
		{name: "through symlinked dir outside", relPath: "escape-dir/secret.md", wantError: ErrOutsideRoot},
		{name: "missing", relPath: "nope.md", wantError: os.ErrNotExist},
		{name: "invalid", relPath: "../x", wantError: ErrInvalidPath},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gotPath, err := Resolve(root, tc.relPath)
			if tc.wantError != nil {
				if !errors.Is(err, tc.wantError) {
					t.Fatalf("Resolve(%q) error = %v, want %v", tc.relPath, err, tc.wantError)
				}
				return
			}
			if err != nil {
				t.Fatalf("Resolve(%q) unexpected error: %v", tc.relPath, err)
			}
			if gotPath != tc.wantPath {
				t.Fatalf("Resolve(%q) = %q, want %q", tc.relPath, gotPath, tc.wantPath)
			}
		})
	}
}

// newRoot returns a temp dir with symlinks resolved, matching how main
// prepares the root (on macOS t.TempDir lives under the /var -> /private/var
// symlink).
func newRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return root
}

func writeFile(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("# x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func symlink(t *testing.T, target, link string) {
	t.Helper()
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
}
