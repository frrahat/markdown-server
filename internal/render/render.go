// Package render turns markdown into the HTML shown in the viewer.
package render

import (
	"bytes"
	"fmt"

	chromahtml "github.com/alecthomas/chroma/v2/formatters/html"
	"github.com/alecthomas/chroma/v2/styles"
	"github.com/microcosm-cc/bluemonday"
	"github.com/yuin/goldmark"
	highlighting "github.com/yuin/goldmark-highlighting/v2"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/renderer/html"
	"github.com/yuin/goldmark/util"
)

const (
	chromaLightStyle = "github"
	chromaDarkStyle  = "github-dark"
)

type Options struct {
	// MarkdownExtensions decide which relative links open in the viewer
	// rather than through /raw/. Lowercase with a leading dot; defaults to
	// .md and .markdown.
	MarkdownExtensions []string
	// UnsafeHTML skips sanitizing, so raw HTML in documents (scripts
	// included) reaches the browser unchanged.
	UnsafeHTML bool
}

// Renderer is safe for concurrent use.
type Renderer struct {
	markdown goldmark.Markdown
	// sanitizer is nil when Options.UnsafeHTML is set.
	sanitizer *bluemonday.Policy
}

func New(opts Options) *Renderer {
	markdownExtensions := opts.MarkdownExtensions
	if len(markdownExtensions) == 0 {
		markdownExtensions = []string{".md", ".markdown"}
	}
	markdown := goldmark.New(
		goldmark.WithExtensions(
			extension.GFM,
			extension.Footnote,
			highlighting.NewHighlighting(
				highlighting.WithStyle(chromaLightStyle),
				// Classes rather than inline styles, so the sanitizer can strip
				// every style attribute without losing highlighting.
				highlighting.WithFormatOptions(chromahtml.WithClasses(true)),
			),
		),
		goldmark.WithParserOptions(
			parser.WithAutoHeadingID(),
			parser.WithASTTransformers(
				util.Prioritized(mermaidTransformer{}, 100),
				util.Prioritized(linkTransformer{markdownExtensions: markdownExtensions}, 200),
			),
		),
		goldmark.WithRendererOptions(
			// Raw HTML is always emitted here and filtered afterwards by the
			// sanitizer; goldmark's own safe mode would drop it entirely.
			html.WithUnsafe(),
			renderer.WithNodeRenderers(
				util.Prioritized(mermaidRenderer{}, 100),
			),
		),
	)
	renderer := &Renderer{markdown: markdown}
	if !opts.UnsafeHTML {
		renderer.sanitizer = newSanitizer()
	}
	return renderer
}

// Render converts markdown source to HTML. docPath is the document's
// root-relative path, used to resolve its relative links.
func (r *Renderer) Render(source []byte, docPath string) ([]byte, error) {
	parserContext := parser.NewContext()
	parserContext.Set(docPathContextKey, docPath)
	var output bytes.Buffer
	if err := r.markdown.Convert(source, &output, parser.WithContext(parserContext)); err != nil {
		return nil, fmt.Errorf("render markdown: %w", err)
	}
	if r.sanitizer == nil {
		return output.Bytes(), nil
	}
	return r.sanitizer.SanitizeBytes(output.Bytes()), nil
}

// ChromaCSS returns the light and dark stylesheets matching the class names
// that highlighted code blocks use. Rendered HTML is the same for both themes;
// only the stylesheet differs.
func ChromaCSS() (light, dark []byte, err error) {
	light, err = chromaStyleCSS(chromaLightStyle)
	if err != nil {
		return nil, nil, err
	}
	dark, err = chromaStyleCSS(chromaDarkStyle)
	if err != nil {
		return nil, nil, err
	}
	return light, dark, nil
}

func chromaStyleCSS(styleName string) ([]byte, error) {
	var css bytes.Buffer
	formatter := chromahtml.New(chromahtml.WithClasses(true))
	if err := formatter.WriteCSS(&css, styles.Get(styleName)); err != nil {
		return nil, fmt.Errorf("write chroma css for %s: %w", styleName, err)
	}
	return css.Bytes(), nil
}
