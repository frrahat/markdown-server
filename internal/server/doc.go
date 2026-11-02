package server

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"time"

	"github.com/frrahat/markdown-server/internal/pathsafe"
)

type docResponse struct {
	Path    string    `json:"path"`
	HTML    string    `json:"html"`
	ModTime time.Time `json:"modTime"`
	Size    int64     `json:"size"`
}

// handleDoc reads the file from disk on every request, so Reload always shows
// the current content. Only files in the current index are served; a file
// created since the last refresh is 404 until the tree is refreshed.
func (s *Server) handleDoc(w http.ResponseWriter, r *http.Request) {
	docPath, err := pathsafe.Clean(r.URL.Query().Get("path"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if !s.indexHolder.Current().HasFile(docPath) {
		writeError(w, http.StatusNotFound, fmt.Sprintf("file %q not indexed", docPath))
		return
	}
	realPath, err := pathsafe.Resolve(s.rootDir, docPath)
	if err != nil {
		s.writePathError(w, err)
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
	if info.Size() > s.maxFileBytes {
		writeError(w, http.StatusRequestEntityTooLarge, fmt.Sprintf("file is %d bytes; the limit is %d", info.Size(), s.maxFileBytes))
		return
	}
	// The limit is re-checked while reading because the file can grow after
	// Stat.
	source, err := io.ReadAll(io.LimitReader(file, s.maxFileBytes+1))
	if err != nil {
		s.writePathError(w, err)
		return
	}
	if int64(len(source)) > s.maxFileBytes {
		writeError(w, http.StatusRequestEntityTooLarge, fmt.Sprintf("file exceeds the %d byte limit", s.maxFileBytes))
		return
	}
	html, err := s.renderer.Render(source, docPath)
	if err != nil {
		s.logger.Error("render failed", "path", docPath, "err", err)
		writeError(w, http.StatusInternalServerError, "render failed")
		return
	}
	writeJSON(w, http.StatusOK, docResponse{
		Path:    docPath,
		HTML:    string(html),
		ModTime: info.ModTime(),
		Size:    int64(len(source)),
	})
}

// writePathError maps filesystem and validation errors to a status code.
// Unexpected errors are logged in full but reported generically, so absolute
// paths don't leak into responses.
func (s *Server) writePathError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, pathsafe.ErrInvalidPath), errors.Is(err, pathsafe.ErrOutsideRoot):
		writeError(w, http.StatusBadRequest, "invalid path")
	case errors.Is(err, fs.ErrNotExist):
		writeError(w, http.StatusNotFound, "not found")
	default:
		s.logger.Error("file access failed", "err", err)
		writeError(w, http.StatusInternalServerError, "file access failed")
	}
}
