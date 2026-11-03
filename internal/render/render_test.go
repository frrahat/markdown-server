package render

import (
	"bytes"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var updateGolden = flag.Bool("update", false, "rewrite golden files in testdata/")

// TestRenderGolden compares each testdata/*.md against its .golden.html.
// Run `go test ./internal/render/ -update` after an intended output change
// and review the diff.
func TestRenderGolden(t *testing.T) {
	sourcePaths, err := filepath.Glob("testdata/*.md")
	if err != nil {
		t.Fatal(err)
	}
	if len(sourcePaths) == 0 {
		t.Fatal("no golden sources in testdata/")
	}
	renderer := New(Options{})
	for _, sourcePath := range sourcePaths {
		t.Run(filepath.Base(sourcePath), func(t *testing.T) {
			source, err := os.ReadFile(sourcePath)
			if err != nil {
				t.Fatal(err)
			}
			docPath := strings.TrimPrefix(filepath.ToSlash(sourcePath), "testdata/")
			got, err := renderer.Render(source, docPath)
			if err != nil {
				t.Fatal(err)
			}
			goldenPath := strings.TrimSuffix(sourcePath, ".md") + ".golden.html"
			if *updateGolden {
				if err := os.WriteFile(goldenPath, got, 0o644); err != nil {
					t.Fatal(err)
				}
				return
			}
			want, err := os.ReadFile(goldenPath)
			if err != nil {
				t.Fatalf("%v (run with -update to create it)", err)
			}
			if !bytes.Equal(got, want) {
				t.Errorf("output differs from %s\n--- got ---\n%s", goldenPath, got)
			}
		})
	}
}

func TestHighlightingUsesClassesNotStyles(t *testing.T) {
	html := renderString(t, New(Options{UnsafeHTML: true}), "```go\nvar x = 1\n```\n", "a.md")
	if !strings.Contains(html, `<span class="kd">var</span>`) {
		t.Errorf("missing chroma classes:\n%s", html)
	}
	if strings.Contains(html, "style=") {
		t.Errorf("highlighted code contains inline styles:\n%s", html)
	}
}

func TestMermaidPassthrough(t *testing.T) {
	html := renderString(t, New(Options{}), "```Mermaid\ngraph TD\n  A-->B<script>\n```\n", "a.md")
	want := "<pre class=\"mermaid\">graph TD\n  A--&gt;B&lt;script&gt;\n</pre>\n"
	if html != want {
		t.Errorf("got %q\nwant %q", html, want)
	}
}

func TestChromaCSS(t *testing.T) {
	light, dark, err := ChromaCSS()
	if err != nil {
		t.Fatal(err)
	}
	for name, css := range map[string][]byte{"light": light, "dark": dark} {
		if !bytes.Contains(css, []byte(".chroma .kd")) {
			t.Errorf("%s chroma CSS lacks keyword rule:\n%.300s", name, css)
		}
	}
	if bytes.Equal(light, dark) {
		t.Error("light and dark chroma CSS are identical")
	}
}

func renderString(t *testing.T, renderer *Renderer, source, docPath string) string {
	t.Helper()
	html, err := renderer.Render([]byte(source), docPath)
	if err != nil {
		t.Fatal(err)
	}
	return string(html)
}
