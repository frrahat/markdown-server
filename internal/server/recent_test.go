package server

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRecentFiles(t *testing.T) {
	srv, root := newTestServerWithRoot(t, map[string]string{"old.md": "# old\n"})
	longAgo := time.Now().Add(-time.Hour)
	if err := os.Chtimes(filepath.Join(root, "old.md"), longAgo, longAgo); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "fresh.md"), []byte("# fresh\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Neither change is visible until a refresh rescans the root.
	var before recentFilesResponse
	doGet(t, srv, "/api/recent-files", http.StatusOK, &before)
	if before.Total != 1 || before.Files[0].Path != "old.md" {
		t.Fatalf("before refresh = %+v", before)
	}

	refresh := newRequest("/api/refresh")
	refresh.Method = http.MethodPost
	refreshRecorder := httptest.NewRecorder()
	srv.ServeHTTP(refreshRecorder, refresh)
	if refreshRecorder.Code != http.StatusOK {
		t.Fatalf("refresh status %d", refreshRecorder.Code)
	}

	var after recentFilesResponse
	doGet(t, srv, "/api/recent-files", http.StatusOK, &after)
	if after.Total != 1 || after.Files[0].Path != "fresh.md" || after.Generation != 2 {
		t.Fatalf("after refresh = %+v", after)
	}
}

func TestRecentFilesCapsResults(t *testing.T) {
	files := map[string]string{}
	for fileNumber := range maxRecentlyUpdated + 5 {
		files[fmt.Sprintf("bulk/%02d.md", fileNumber)] = "# x\n"
	}
	srv := newTestServer(t, files)
	var response recentFilesResponse
	doGet(t, srv, "/api/recent-files", http.StatusOK, &response)
	if response.Total != maxRecentlyUpdated+5 || len(response.Files) != maxRecentlyUpdated {
		t.Errorf("total %d, files %d", response.Total, len(response.Files))
	}
}
