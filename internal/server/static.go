package server

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"net/http"
	"time"

	"github.com/frrahat/markdown-server/internal/render"
	"github.com/frrahat/markdown-server/web"
)

// indexPageCSP is defence in depth behind the HTML sanitizer: even if
// something slipped through, inline and third-party scripts can't run.
// Styles allow 'unsafe-inline' because mermaid writes inline styles into its
// SVGs. Images allow any https source so README badges still load.
const indexPageCSP = "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; " +
	"img-src 'self' data: https:; object-src 'none'; base-uri 'none'; frame-ancestors 'none'"

type staticAsset struct {
	content []byte
	etag    string
}

// staticAssets is computed once, since embedded files never change while the
// process runs.
type staticAssets struct {
	byPath    map[string]staticAsset
	startTime time.Time
}

func loadStaticAssets() (*staticAssets, error) {
	assets := &staticAssets{byPath: make(map[string]staticAsset), startTime: time.Now()}
	err := fs.WalkDir(web.Files, ".", func(assetPath string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		content, err := fs.ReadFile(web.Files, assetPath)
		if err != nil {
			return err
		}
		assets.add("/"+assetPath, content)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("load embedded web files: %w", err)
	}
	chromaCSS, err := render.ChromaCSS()
	if err != nil {
		return nil, err
	}
	assets.add("/chroma.css", chromaCSS)
	assets.byPath["/"] = assets.byPath["/index.html"]
	return assets, nil
}

func (a *staticAssets) add(urlPath string, content []byte) {
	digest := sha256.Sum256(content)
	a.byPath[urlPath] = staticAsset{content: content, etag: `"` + hex.EncodeToString(digest[:8]) + `"`}
}

// handleStatic serves embedded assets with an ETag. "no-cache" makes the
// browser revalidate on each load, which costs a 304 rather than re-sending
// the multi-megabyte mermaid bundle.
func (s *Server) handleStatic(w http.ResponseWriter, r *http.Request) {
	asset, found := s.staticAssets.byPath[r.URL.Path]
	if !found {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	if r.URL.Path == "/" || r.URL.Path == "/index.html" {
		w.Header().Set("Content-Security-Policy", indexPageCSP)
	}
	w.Header().Set("ETag", asset.etag)
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	// ServeContent picks the Content-Type from the extension and answers
	// If-None-Match with 304 using the ETag set above.
	http.ServeContent(w, r, r.URL.Path, s.staticAssets.startTime, bytes.NewReader(asset.content))
}
