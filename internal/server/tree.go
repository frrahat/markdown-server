package server

import (
	"fmt"
	"net/http"
	"time"

	"github.com/frrahat/markdown-server/internal/index"
	"github.com/frrahat/markdown-server/internal/pathsafe"
)

type treeResponse struct {
	Path       string        `json:"path"`
	Children   []index.Entry `json:"children"`
	Generation uint64        `json:"generation"`
	IndexedAt  time.Time     `json:"indexedAt"`
}

func (s *Server) handleTree(w http.ResponseWriter, r *http.Request) {
	dirPath, err := pathsafe.Clean(r.URL.Query().Get("path"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	idx := s.indexHolder.Current()
	entries, found := idx.Children(dirPath)
	if !found {
		writeError(w, http.StatusNotFound, fmt.Sprintf("directory %q not indexed", dirPath))
		return
	}
	writeJSON(w, http.StatusOK, treeResponse{
		Path:       dirPath,
		Children:   entries,
		Generation: idx.Generation,
		IndexedAt:  idx.IndexedAt,
	})
}
