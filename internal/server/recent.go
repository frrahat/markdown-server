package server

import (
	"net/http"
	"time"

	"github.com/frrahat/markdown-server/internal/index"
)

const (
	recentlyUpdatedWindow = 5 * time.Minute
	// A checkout or bulk copy can touch hundreds of files at once; the
	// sidebar only has room for a handful.
	maxRecentlyUpdated = 20
)

type recentFilesResponse struct {
	Total      int                  `json:"total"`
	Files      []index.ModifiedFile `json:"files"`
	Generation uint64               `json:"generation"`
}

// handleRecentFiles lists files modified within recentlyUpdatedWindow of now,
// newest first. Modification times come from the last scan, so a file
// changed after it appears only once the tree is refreshed.
func (s *Server) handleRecentFiles(w http.ResponseWriter, r *http.Request) {
	idx := s.indexHolder.Current()
	files := idx.FilesModifiedSince(time.Now().Add(-recentlyUpdatedWindow))
	writeJSON(w, http.StatusOK, recentFilesResponse{
		Total:      len(files),
		Files:      files[:min(len(files), maxRecentlyUpdated)],
		Generation: idx.Generation,
	})
}
