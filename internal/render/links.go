package render

import (
	"net/url"
	"path"
	"slices"
	"strings"

	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
)

var docPathContextKey = parser.NewContextKey()

// linkTransformer rewrites link and image destinations so they work inside
// the single-page viewer: markdown links become hash routes, other relative
// files go through /raw/, and external links open in a new tab.
type linkTransformer struct {
	markdownExtensions []string
}

func (t linkTransformer) Transform(document *ast.Document, reader text.Reader, pc parser.Context) {
	docPath, _ := pc.Get(docPathContextKey).(string)
	docDir := path.Dir(docPath)
	source := reader.Source()
	_ = ast.Walk(document, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch n := node.(type) {
		case *ast.Link:
			n.Destination = t.rewrite(n, n.Destination, docDir, true)
		case *ast.Image:
			n.Destination = t.rewrite(n, n.Destination, docDir, false)
		case *ast.AutoLink:
			if n.AutoLinkType == ast.AutoLinkURL && isWebURL(string(n.URL(source))) {
				markExternal(n)
			}
		}
		return ast.WalkContinue, nil
	})
}

// rewrite returns the new destination for a link (isLink) or image.
// Destinations it doesn't understand are returned unchanged.
func (t linkTransformer) rewrite(node ast.Node, destination []byte, docDir string, isLink bool) []byte {
	rawDest := string(destination)
	if rawDest == "" || strings.HasPrefix(rawDest, "#") {
		return destination
	}
	if strings.HasPrefix(rawDest, "//") || hasScheme(rawDest) {
		if isLink && isWebURL(rawDest) {
			markExternal(node)
		}
		return destination
	}

	pathPart, fragment, _ := strings.Cut(rawDest, "#")
	pathPart, _, _ = strings.Cut(pathPart, "?")
	unescapedPath, err := url.PathUnescape(pathPart)
	if err != nil || unescapedPath == "" {
		return destination
	}
	var resolved string
	if strings.HasPrefix(unescapedPath, "/") {
		// Leading slash means "from the root", as on GitHub.
		resolved = path.Clean(strings.TrimPrefix(unescapedPath, "/"))
	} else {
		resolved = path.Join(docDir, unescapedPath)
	}
	// Escaping the root, or pointing at the root itself: nothing to serve.
	if resolved == "." || resolved == ".." || strings.HasPrefix(resolved, "../") {
		return destination
	}

	escapedPath := escapePath(resolved)
	if isLink && t.isMarkdown(resolved) {
		// The viewer routes on the hash; a second '#' carries the heading
		// anchor, which the frontend splits off.
		route := "#/" + escapedPath
		if fragment != "" {
			route += "#" + fragment
		}
		return []byte(route)
	}
	rawURL := "/raw/" + escapedPath
	if fragment != "" {
		rawURL += "#" + fragment
	}
	return []byte(rawURL)
}

func (t linkTransformer) isMarkdown(relPath string) bool {
	return slices.Contains(t.markdownExtensions, strings.ToLower(path.Ext(relPath)))
}

func markExternal(node ast.Node) {
	node.SetAttributeString("target", []byte("_blank"))
	node.SetAttributeString("rel", []byte("noopener noreferrer"))
}

func hasScheme(rawURL string) bool {
	parsed, err := url.Parse(rawURL)
	return err == nil && parsed.Scheme != ""
}

func isWebURL(rawURL string) bool {
	lower := strings.ToLower(rawURL)
	return strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") || strings.HasPrefix(lower, "//")
}

func escapePath(relPath string) string {
	segments := strings.Split(relPath, "/")
	for i, segment := range segments {
		segments[i] = url.PathEscape(segment)
	}
	return strings.Join(segments, "/")
}
