package render

import (
	"bytes"

	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

// mermaidBlockKind is a separate node kind so mermaid fences never reach the
// syntax highlighter, which owns the FencedCodeBlock renderer.
var mermaidBlockKind = ast.NewNodeKind("MermaidBlock")

type mermaidBlock struct {
	ast.BaseBlock
}

func (n *mermaidBlock) Kind() ast.NodeKind { return mermaidBlockKind }

func (n *mermaidBlock) IsRaw() bool { return true }

func (n *mermaidBlock) Dump(source []byte, level int) {
	ast.DumpHelper(n, source, level, nil, nil)
}

type mermaidTransformer struct{}

// Transform swaps each ```mermaid fenced block for a mermaidBlock carrying the
// same source lines.
func (mermaidTransformer) Transform(document *ast.Document, reader text.Reader, _ parser.Context) {
	source := reader.Source()
	var fences []*ast.FencedCodeBlock
	_ = ast.Walk(document, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if fence, ok := node.(*ast.FencedCodeBlock); ok && entering {
			if bytes.EqualFold(fence.Language(source), []byte("mermaid")) {
				fences = append(fences, fence)
			}
			return ast.WalkSkipChildren, nil
		}
		return ast.WalkContinue, nil
	})
	// Replaced after the walk, since replacing nodes mid-walk breaks the
	// sibling traversal.
	for _, fence := range fences {
		block := &mermaidBlock{}
		block.SetLines(fence.Lines())
		fence.Parent().ReplaceChild(fence.Parent(), fence, block)
	}
}

type mermaidRenderer struct{}

func (mermaidRenderer) RegisterFuncs(registerer renderer.NodeRendererFuncRegisterer) {
	registerer.Register(mermaidBlockKind, renderMermaid)
}

// renderMermaid emits the diagram source HTML-escaped inside
// <pre class="mermaid">; the client-side mermaid library reads it from there.
func renderMermaid(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkContinue, nil
	}
	_, _ = w.WriteString(`<pre class="mermaid">`)
	lines := node.Lines()
	for i := range lines.Len() {
		segment := lines.At(i)
		_, _ = w.Write(util.EscapeHTML(segment.Value(source)))
	}
	_, _ = w.WriteString("</pre>\n")
	return ast.WalkSkipChildren, nil
}
