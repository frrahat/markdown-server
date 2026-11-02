package server

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/frrahat/markdown-server/internal/index"
	"github.com/frrahat/markdown-server/internal/render"
)

var discardLogger = slog.New(slog.NewTextHandler(io.Discard, nil))

func TestTree(t *testing.T) {
	srv := newTestServer(t, map[string]string{
		"README.md":     "# hi\n",
		"docs/guide.md": "# guide\n",
		"docs/x.png":    "png",
	})

	var root treeResponse
	response := doGet(t, srv, "/api/tree?path=", http.StatusOK, &root)
	if response.Header.Get("Cache-Control") != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store", response.Header.Get("Cache-Control"))
	}
	if root.Path != "" || root.Generation != 1 || root.IndexedAt.IsZero() || len(root.Children) != 2 {
		t.Fatalf("root response = %+v", root)
	}
	if root.Children[0] != (index.Entry{Name: "docs", Path: "docs", Type: index.EntryDir}) {
		t.Errorf("first child = %+v", root.Children[0])
	}

	var docs treeResponse
	doGet(t, srv, "/api/tree?path=docs", http.StatusOK, &docs)
	if len(docs.Children) != 1 || docs.Children[0].Path != "docs/guide.md" {
		t.Errorf("docs children = %+v", docs.Children)
	}

	// Omitting the parameter is the same as asking for the root.
	doGet(t, srv, "/api/tree", http.StatusOK, nil)
	doGet(t, srv, "/api/tree?path=nope", http.StatusNotFound, nil)
	doGet(t, srv, "/api/tree?path=README.md", http.StatusNotFound, nil)
	doGet(t, srv, "/api/tree?path=..", http.StatusBadRequest, nil)
	doGet(t, srv, "/api/tree?path=%2e%2e%2fetc", http.StatusBadRequest, nil)
	doGet(t, srv, "/api/tree?path=/etc", http.StatusBadRequest, nil)
}

func TestTreeEmptyRootSerializesEmptyArray(t *testing.T) {
	srv := newTestServer(t, map[string]string{"main.go": "package main\n"})
	recorder := httptest.NewRecorder()
	srv.ServeHTTP(recorder, newRequest("/api/tree?path="))
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(recorder.Body.Bytes(), &raw); err != nil {
		t.Fatal(err)
	}
	if string(raw["children"]) != "[]" {
		t.Errorf("children = %s, want []", raw["children"])
	}
}

func TestHostGuard(t *testing.T) {
	srv := newTestServer(t, map[string]string{"a.md": "# a\n"})
	tests := map[string]int{
		"127.0.0.1:8080":       http.StatusOK,
		"localhost:8080":       http.StatusOK,
		"LOCALHOST:9999":       http.StatusOK,
		"[::1]:8080":           http.StatusOK,
		"localhost":            http.StatusOK,
		"[::1]":                http.StatusOK,
		"evil.com:8080":        http.StatusForbidden,
		"evil.com":             http.StatusForbidden,
		"127.0.0.1.evil.com":   http.StatusForbidden,
		"localhost.evil.com:1": http.StatusForbidden,
		"192.168.1.5:8080":     http.StatusForbidden,
		"":                     http.StatusForbidden,
	}
	for hostHeader, wantStatus := range tests {
		request := newRequest("/api/tree")
		request.Host = hostHeader
		recorder := httptest.NewRecorder()
		srv.ServeHTTP(recorder, request)
		if recorder.Code != wantStatus {
			t.Errorf("Host %q: status %d, want %d", hostHeader, recorder.Code, wantStatus)
		}
	}
}

func TestUnknownRouteAndMethod(t *testing.T) {
	srv := newTestServer(t, map[string]string{"a.md": "# a\n"})
	doGet(t, srv, "/api/nope", http.StatusNotFound, nil)

	request := newRequest("/api/tree")
	request.Method = http.MethodPost
	recorder := httptest.NewRecorder()
	srv.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST /api/tree status %d, want 405", recorder.Code)
	}
}

func newTestServer(t *testing.T, files map[string]string) *Server {
	t.Helper()
	srv, _ := newTestServerWithRoot(t, files)
	return srv
}

// newTestServerWithRoot also returns the root so tests can change files on
// disk after the initial scan.
func newTestServerWithRoot(t *testing.T, files map[string]string) (*Server, string) {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for relPath, content := range files {
		absPath := filepath.Join(root, filepath.FromSlash(relPath))
		if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(absPath, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	holder, err := index.NewHolder(root, index.Options{
		Extensions: []string{".md", ".markdown"},
		Logger:     discardLogger,
	})
	if err != nil {
		t.Fatal(err)
	}
	srv, err := New(Config{
		RootDir:      root,
		IndexHolder:  holder,
		Renderer:     render.New(render.Options{}),
		MaxFileBytes: 1 << 10,
		Logger:       discardLogger,
	})
	if err != nil {
		t.Fatal(err)
	}
	return srv, root
}

func newRequest(target string) *http.Request {
	request := httptest.NewRequest(http.MethodGet, target, nil)
	request.Host = "127.0.0.1:8080"
	return request
}

// doGet issues a GET, checks the status, and decodes the JSON body into
// decodeInto when it is non-nil.
func doGet(t *testing.T, srv *Server, target string, wantStatus int, decodeInto any) *http.Response {
	t.Helper()
	recorder := httptest.NewRecorder()
	srv.ServeHTTP(recorder, newRequest(target))
	if recorder.Code != wantStatus {
		t.Fatalf("GET %s: status %d, want %d; body %s", target, recorder.Code, wantStatus, recorder.Body)
	}
	if decodeInto != nil {
		decodeJSON(t, recorder, decodeInto)
	}
	return recorder.Result()
}

func decodeJSON(t *testing.T, recorder *httptest.ResponseRecorder, decodeInto any) {
	t.Helper()
	if err := json.Unmarshal(recorder.Body.Bytes(), decodeInto); err != nil {
		t.Fatalf("decode %s: %v", recorder.Body, err)
	}
}
