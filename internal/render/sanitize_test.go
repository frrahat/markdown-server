package render

import (
	"strings"
	"testing"
)

func TestSanitizerKeepsReadmeHTML(t *testing.T) {
	markdown := `<p align="center"><img src="docs/logo.png" width="120" alt="logo"></p>

<details open><summary>More</summary>

Press <kbd>Ctrl</kbd>+<kbd>C</kbd>. E = mc<sup>2</sup>, H<sub>2</sub>O.

</details>
`
	html := renderString(t, New(Options{}), markdown, "README.md")
	for _, want := range []string{
		`<p align="center">`,
		`<img src="docs/logo.png" width="120" alt="logo">`,
		`<details open="">`,
		`<summary>More</summary>`,
		`<kbd>Ctrl</kbd>`,
		`<sup>2</sup>`,
		`<sub>2</sub>`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("sanitized output lost %q:\n%s", want, html)
		}
	}
}

func TestSanitizerStripsScript(t *testing.T) {
	markdown := `<script>alert(1)</script>
<img src="x.png" onerror="alert(2)">
<a href="javascript:alert(3)">click</a>
<p style="position:fixed">styled</p>
<iframe src="https://evil.example"></iframe>
<svg onload="alert(4)"></svg>

[md link](javascript:alert(5))
`
	html := renderString(t, New(Options{}), markdown, "a.md")
	for _, forbidden := range []string{"<script", "alert(", "onerror", "javascript:", "style=", "<iframe", "onload"} {
		if strings.Contains(html, forbidden) {
			t.Errorf("sanitized output contains %q:\n%s", forbidden, html)
		}
	}
	if !strings.Contains(html, "styled") || !strings.Contains(html, "click") {
		t.Errorf("sanitizer dropped text content:\n%s", html)
	}
}

// TestSanitizerKeepsRendererOutput guards against the policy silently
// breaking features: everything the renderer itself emits must survive.
func TestSanitizerKeepsRendererOutput(t *testing.T) {
	markdown := "# Heading One\n\n- [x] done\n- [ ] todo\n\nNote[^n] and [ext](https://example.com).\n\n[^n]: Text.\n\n" +
		"```go\nvar x = 1\n```\n\n```mermaid\ngraph TD\n  A-->B\n```\n"
	html := renderString(t, New(Options{}), markdown, "a.md")
	for _, want := range []string{
		`<h1 id="heading-one">`,
		`<input checked="" disabled="" type="checkbox">`,
		`<input disabled="" type="checkbox">`,
		`<sup id="fnref:1"><a href="#fn:1" class="footnote-ref" role="doc-noteref">`,
		`<div class="footnotes" role="doc-endnotes">`,
		`<li id="fn:1">`,
		`class="footnote-backref" role="doc-backlink"`,
		`<a href="https://example.com" target="_blank" rel="noopener noreferrer">`,
		`<pre class="chroma">`,
		`<span class="kd">var</span>`,
		`<pre class="mermaid">graph TD`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("sanitizer removed %q:\n%s", want, html)
		}
	}
}

func TestUnsafeHTMLBypassesSanitizer(t *testing.T) {
	markdown := "<div style=\"color:red\" onclick=\"go()\">x</div>\n"
	html := renderString(t, New(Options{UnsafeHTML: true}), markdown, "a.md")
	if html != markdown {
		t.Errorf("unsafe mode changed raw HTML: got %q, want %q", html, markdown)
	}
}
