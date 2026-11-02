// Package render turns markdown into the HTML shown in the viewer.
package render

import (
	"bytes"
	"fmt"

	chromahtml "github.com/alecthomas/chroma/v2/formatters/html"
	"github.com/alecthomas/chroma/v2/styles"
	"github.com/yuin/goldmark"
	highlighting "github.com/yuin/goldmark-highlighting/v2"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/renderer/html"
	"github.com/yuin/goldmark/util"
)

const chromaStyleName = "github"

type Options struct {
	// UnsafeHTML skips sanitizing, so raw HTML in documents (scripts
	// included) reaches the browser unchanged.
	UnsafeHTML bool
}

// Renderer is safe for concurrent use.
type Renderer struct {
	markdown goldmark.Markdown
}

func New(opts Options) *Renderer {
	markdown := goldmark.New(
		goldmark.WithExtensions(
			extension.GFM,
			extension.Footnote,
			highlighting.NewHighlighting(
				highlighting.WithStyle(chromaStyleName),
				// Classes rather than inline styles, so the sanitizer can strip
				// every style attribute without losing highlighting.
				highlighting.WithFormatOptions(chromahtml.WithClasses(true)),
			),
		),
		goldmark.WithParserOptions(
			parser.WithAutoHeadingID(),
			parser.WithASTTransformers(
				util.Prioritized(mermaidTransformer{}, 100),
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
	return &Renderer{markdown: markdown}
}

// Render converts markdown source to HTML. docPath is the document's
// root-relative path.
func (r *Renderer) Render(source []byte, docPath string) ([]byte, error) {
	var output bytes.Buffer
	if err := r.markdown.Convert(source, &output); err != nil {
		return nil, fmt.Errorf("render markdown: %w", err)
	}
	return output.Bytes(), nil
}

// ChromaCSS returns the stylesheet matching the class names that highlighted
// code blocks use.
func ChromaCSS() ([]byte, error) {
	var css bytes.Buffer
	formatter := chromahtml.New(chromahtml.WithClasses(true))
	if err := formatter.WriteCSS(&css, styles.Get(chromaStyleName)); err != nil {
		return nil, fmt.Errorf("write chroma css: %w", err)
	}
	return css.Bytes(), nil
}
