package server

import (
	"net/http"
	"time"

	"github.com/frrahat/markdown-server/internal/index"
)

const (
	recentlyCreatedWindow = 5 * time.Minute
	// A checkout or bulk copy can create hundreds of files at once; the
	// sidebar only has room for a handful.
	maxRecentlyCreated = 20
)

type recentFilesResponse struct {
	Total      int                 `json:"total"`
	Files      []index.CreatedFile `json:"files"`
	Generation uint64              `json:"generation"`
}

// handleRecentFiles lists files created within recentlyCreatedWindow of now,
// newest first. Creation times come from the last scan, so a file created
// after it appears only once the tree is refreshed.
func (s *Server) handleRecentFiles(w http.ResponseWriter, r *http.Request) {
	idx := s.indexHolder.Current()
	files := idx.FilesCreatedSince(time.Now().Add(-recentlyCreatedWindow))
	writeJSON(w, http.StatusOK, recentFilesResponse{
		Total:      len(files),
		Files:      files[:min(len(files), maxRecentlyCreated)],
		Generation: idx.Generation,
	})
}
