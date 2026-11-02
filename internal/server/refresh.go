package server

import (
	"net/http"
	"time"
)

type refreshResponse struct {
	Generation uint64    `json:"generation"`
	IndexedAt  time.Time `json:"indexedAt"`
	Files      int       `json:"files"`
	Dirs       int       `json:"dirs"`
	DurationMs int64     `json:"durationMs"`
}

func (s *Server) handleRefresh(w http.ResponseWriter, r *http.Request) {
	idx, err := s.indexHolder.Refresh()
	if err != nil {
		s.logger.Error("refresh failed", "err", err)
		writeError(w, http.StatusInternalServerError, "refresh failed")
		return
	}
	s.logger.Info("refreshed", "generation", idx.Generation, "files", idx.FileCount, "dirs", idx.DirCount,
		"duration", idx.ScanDuration.Round(time.Millisecond))
	writeJSON(w, http.StatusOK, refreshResponse{
		Generation: idx.Generation,
		IndexedAt:  idx.IndexedAt,
		Files:      idx.FileCount,
		Dirs:       idx.DirCount,
		DurationMs: idx.ScanDuration.Milliseconds(),
	})
}
