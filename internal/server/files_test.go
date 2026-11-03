package server

import (
	"fmt"
	"net/http"
	"slices"
	"testing"
)

func TestFiles(t *testing.T) {
	srv := newTestServer(t, map[string]string{
		"README.md":                "# r\n",
		"docs/guide.md":            "# g\n",
		"docs/setup-guide.md":      "# sg\n",
		"go/ui/de/notes.md":        "# n\n",
		"notes/2026/jan.md":        "# j\n",
		"notes/2026/feb.markdown":  "# f\n",
		"docs/img/not-markdown.md": "# x\n",
		".gitignore":               "build/\n",
		"build/guide.md":           "# ignored\n",
	})

	var response filesResponse
	doGet(t, srv, "/api/files?q=guide", http.StatusOK, &response)
	var paths []string
	for _, file := range response.Files {
		paths = append(paths, file.Path)
	}
	if !slices.Equal(paths, []string{"docs/guide.md", "docs/setup-guide.md", "go/ui/de/notes.md"}) {
		t.Errorf("guide results = %v", paths)
	}
	if response.Total != 3 || response.Query != "guide" || response.Generation != 1 {
		t.Errorf("response = %+v", response)
	}
	if !slices.Equal(response.Files[0].Positions, []int{5, 6, 7, 8, 9}) {
		t.Errorf("positions = %v", response.Files[0].Positions)
	}

	doGet(t, srv, "/api/files?q=notes+jan", http.StatusOK, &response)
	if len(response.Files) != 1 || response.Files[0].Path != "notes/2026/jan.md" {
		t.Errorf("multi-term results = %+v", response.Files)
	}

	doGet(t, srv, "/api/files?q=", http.StatusOK, &response)
	if response.Total != 0 || response.Files == nil || len(response.Files) != 0 {
		t.Errorf("empty query = %+v", response)
	}

	doGet(t, srv, "/api/files?q=md&limit=2", http.StatusOK, &response)
	if len(response.Files) != 2 || response.Total != 7 {
		t.Errorf("limited results: %d files, total %d", len(response.Files), response.Total)
	}

	for _, badLimit := range []string{"0", "-1", "lots"} {
		doGet(t, srv, fmt.Sprintf("/api/files?q=a&limit=%s", badLimit), http.StatusBadRequest, nil)
	}
}
