package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRefresh(t *testing.T) {
	srv, root := newTestServerWithRoot(t, map[string]string{"a.md": "# a\n"})
	writeFile(t, root, "docs/new.md", "# new\n")

	// The tree doesn't change until a refresh.
	doGet(t, srv, "/api/tree?path=docs", http.StatusNotFound, nil)
	doGet(t, srv, "/api/doc?path=docs/new.md", http.StatusNotFound, nil)

	request := newRequest("/api/refresh")
	request.Method = http.MethodPost
	recorder := httptest.NewRecorder()
	srv.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("refresh status %d: %s", recorder.Code, recorder.Body)
	}
	if recorder.Header().Get("Cache-Control") != "no-store" {
		t.Error("refresh response is cacheable")
	}
	var refreshed refreshResponse
	decodeJSON(t, recorder, &refreshed)
	if refreshed.Generation != 2 || refreshed.Files != 2 || refreshed.Dirs != 1 || refreshed.IndexedAt.IsZero() {
		t.Errorf("refresh response = %+v", refreshed)
	}

	var tree treeResponse
	doGet(t, srv, "/api/tree?path=docs", http.StatusOK, &tree)
	if tree.Generation != 2 || len(tree.Children) != 1 {
		t.Errorf("tree after refresh = %+v", tree)
	}
	doGet(t, srv, "/api/doc?path=docs/new.md", http.StatusOK, nil)

	// GET must not trigger a rescan. (It falls through to the static handler,
	// hence 404 rather than 405.)
	doGet(t, srv, "/api/refresh", http.StatusNotFound, nil)
	if generation := srv.indexHolder.Current().Generation; generation != 2 {
		t.Errorf("generation after GET = %d, want 2", generation)
	}
}
