package render

import (
	"regexp"

	"github.com/microcosm-cc/bluemonday"
)

// newSanitizer allows common README HTML plus everything this package's own
// renderer emits, and nothing that can run script. UGCPolicy already covers
// standard formatting, tables, images, ids and safe URL schemes (http, https,
// mailto, relative).
func newSanitizer() *bluemonday.Policy {
	policy := bluemonday.UGCPolicy()

	// UGCPolicy forces rel="nofollow" onto links; for a local reader there is
	// no SEO to protect, and it would clobber the rel the link rewriter sets.
	policy.RequireNoFollowOnLinks(false)
	policy.AllowAttrs("target").Matching(regexp.MustCompile(`^_blank$`)).OnElements("a")
	policy.AllowAttrs("rel").Matching(regexp.MustCompile(`^(noopener|noreferrer)( (noopener|noreferrer))*$`)).OnElements("a")

	// Chroma token classes, class="mermaid", and footnote classes. Class names
	// only select our own CSS, so a permissive pattern is harmless.
	policy.AllowAttrs("class").Matching(regexp.MustCompile(`^[\w\- ]+$`)).OnElements("span", "pre", "code", "div", "a")
	policy.AllowAttrs("role").Matching(regexp.MustCompile(`^doc-(noteref|endnotes|backlink)$`)).OnElements("a", "div")

	// GFM task-list checkboxes.
	policy.AllowElements("input")
	policy.AllowAttrs("type").Matching(regexp.MustCompile(`^checkbox$`)).OnElements("input")
	policy.AllowAttrs("checked", "disabled").OnElements("input")

	// HTML that READMEs commonly use.
	policy.AllowElements("details", "summary", "kbd", "sup", "sub", "picture")
	policy.AllowAttrs("open").OnElements("details")
	policy.AllowAttrs("align").Matching(regexp.MustCompile(`^(?i)(left|right|center|justify)$`)).
		OnElements("p", "div", "img", "td", "th", "h1", "h2", "h3", "h4", "h5", "h6")
	policy.AllowAttrs("width", "height").Matching(bluemonday.NumberOrPercent).OnElements("img")
	policy.AllowAttrs("srcset", "media").OnElements("source")
	policy.AllowElements("source")
	return policy
}
