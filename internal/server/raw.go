package server

import (
	"net/http"
	"os"
	"path/filepath"

	"github.com/frrahat/markdown-server/internal/pathsafe"
)

// handleRaw serves files referenced by documents, typically images. The
// security headers matter for anything opened directly: an SVG or HTML file
// from an untrusted repo would otherwise run script on our origin.
func (s *Server) handleRaw(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", "sandbox")

	requestedPath, err := pathsafe.Clean(r.PathValue("path"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	idx := s.indexHolder.Current()
	if !idx.IsServable(requestedPath) {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	realPath, err := pathsafe.Resolve(s.rootDir, requestedPath)
	if err != nil {
		s.writePathError(w, err)
		return
	}
	// An in-root symlink can point at a file the rules exclude (say,
	// "logo.png -> .env"), so the target is checked as well as the link.
	realRelPath, err := filepath.Rel(s.rootDir, realPath)
	if err != nil || !idx.IsServable(filepath.ToSlash(realRelPath)) {
		writeError(w, http.StatusNotFound, "not found")
		return
	}

	file, err := os.Open(realPath)
	if err != nil {
		s.writePathError(w, err)
		return
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		s.writePathError(w, err)
		return
	}
	if !info.Mode().IsRegular() {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	http.ServeContent(w, r, info.Name(), info.ModTime(), file)
}
