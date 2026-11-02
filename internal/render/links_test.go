package render

import (
	"strings"
	"testing"
)

func TestLinkRewriting(t *testing.T) {
	tests := []struct {
		name     string
		docPath  string
		markdown string
		wantHTML string
	}{
		{"relative md", "docs/intro.md", "[x](guide.md)", `<a href="#/docs/guide.md">`},
		{"md with anchor", "docs/intro.md", "[x](sub/page.md#setup-steps)", `<a href="#/docs/sub/page.md#setup-steps">`},
		{"uppercase extension", "a.md", "[x](NOTES.MARKDOWN)", `<a href="#/NOTES.MARKDOWN">`},
		{"parent within root", "docs/deep/a.md", "[x](../../README.md)", `<a href="#/README.md">`},
		{"root-absolute", "docs/deep/a.md", "[x](/other/b.md)", `<a href="#/other/b.md">`},
		{"escaped space", "a.md", "[x](my%20notes.md)", `<a href="#/my%20notes.md">`},
		{"query dropped", "a.md", "[x](b.md?plain=1)", `<a href="#/b.md">`},
		{"image", "docs/intro.md", "![alt](img/a.png)", `<img src="/raw/docs/img/a.png" alt="alt">`},
		{"image parent", "docs/intro.md", "![alt](../logo.svg)", `<img src="/raw/logo.svg" alt="alt">`},
		{"non-md link goes raw", "docs/intro.md", "[x](diagram.pdf)", `<a href="/raw/docs/diagram.pdf">`},
		{"md image is still raw", "a.md", "![x](b.md)", `<img src="/raw/b.md" alt="x">`},
		{"external", "a.md", "[x](https://example.com/a.md)", `<a href="https://example.com/a.md" target="_blank" rel="noopener noreferrer">`},
		{"protocol-relative", "a.md", "[x](//example.com)", `<a href="//example.com" target="_blank" rel="noopener noreferrer">`},
		{"autolink", "a.md", "see https://example.com now", `<a href="https://example.com" target="_blank" rel="noopener noreferrer">`},
		{"angle autolink", "a.md", "<https://example.com>", `<a href="https://example.com" target="_blank" rel="noopener noreferrer">`},
		{"mailto untouched", "a.md", "[x](mailto:a@b.c)", `<a href="mailto:a@b.c">`},
		{"anchor only", "docs/a.md", "[x](#section)", `<a href="#section">`},
		{"escapes root", "docs/a.md", "[x](../../etc/passwd)", `<a href="../../etc/passwd">`},
		{"root itself", "docs/a.md", "[x](..)", `<a href="..">`},
	}
	renderer := New(Options{UnsafeHTML: true})
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			html := renderString(t, renderer, tc.markdown, tc.docPath)
			if !strings.Contains(html, tc.wantHTML) {
				t.Errorf("rendered %q\nwant it to contain %q", html, tc.wantHTML)
			}
		})
	}
}
