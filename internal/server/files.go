package server

import (
	"cmp"
	"net/http"
	"slices"
	"strconv"

	"github.com/frrahat/markdown-server/internal/fuzzy"
)

const (
	defaultFileResults = 50
	maxFileResults     = 200
)

type fileResult struct {
	Path      string `json:"path"`
	Positions []int  `json:"positions"`
}

type filesResponse struct {
	Query      string       `json:"query"`
	Total      int          `json:"total"`
	Files      []fileResult `json:"files"`
	Generation uint64       `json:"generation"`
}

type scoredFile struct {
	path  string
	match fuzzy.Match
}

// handleFiles fuzzy-filters every indexed file path. Filtering runs here,
// against the in-memory index, so clients never need the full file list.
func (s *Server) handleFiles(w http.ResponseWriter, r *http.Request) {
	rawQuery := r.URL.Query().Get("q")
	limit := defaultFileResults
	if rawLimit := r.URL.Query().Get("limit"); rawLimit != "" {
		parsedLimit, err := strconv.Atoi(rawLimit)
		if err != nil || parsedLimit < 1 {
			writeError(w, http.StatusBadRequest, "limit must be a positive integer")
			return
		}
		limit = min(parsedLimit, maxFileResults)
	}

	idx := s.indexHolder.Current()
	response := filesResponse{Query: rawQuery, Files: []fileResult{}, Generation: idx.Generation}
	query := fuzzy.ParseQuery(rawQuery)
	if query.IsEmpty() {
		writeJSON(w, http.StatusOK, response)
		return
	}

	var matches []scoredFile
	for _, filePath := range idx.FilePaths() {
		if match, ok := query.Match(filePath); ok {
			matches = append(matches, scoredFile{path: filePath, match: match})
		}
	}
	// Best score first. Ties go to the shorter path, then lexical order, so
	// equal scores keep the same order across requests.
	slices.SortFunc(matches, func(a, b scoredFile) int {
		return cmp.Or(
			cmp.Compare(b.match.Score, a.match.Score),
			cmp.Compare(len(a.path), len(b.path)),
			cmp.Compare(a.path, b.path),
		)
	})
	response.Total = len(matches)
	for _, matched := range matches[:min(limit, len(matches))] {
		response.Files = append(response.Files, fileResult{Path: matched.path, Positions: matched.match.Positions})
	}
	writeJSON(w, http.StatusOK, response)
}
