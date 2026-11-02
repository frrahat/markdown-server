package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestStaticAssets(t *testing.T) {
	srv := newTestServer(t, map[string]string{"a.md": "# a\n"})
	tests := []struct {
		path            string
		wantContentType string
		wantBody        string
	}{
		{"/", "text/html; charset=utf-8", `<ul id="tree"`},
		{"/index.html", "text/html; charset=utf-8", `<ul id="tree"`},
		{"/app.js", "text/javascript; charset=utf-8", "loadChildren"},
		{"/style.css", "text/css; charset=utf-8", ".tree-row"},
		{"/chroma.css", "text/css; charset=utf-8", ".chroma"},
		{"/vendor/mermaid.min.js", "text/javascript; charset=utf-8", "mermaid"},
	}
	for _, tc := range tests {
		recorder := httptest.NewRecorder()
		srv.ServeHTTP(recorder, newRequest(tc.path))
		if recorder.Code != http.StatusOK {
			t.Errorf("GET %s: status %d", tc.path, recorder.Code)
			continue
		}
		if got := recorder.Header().Get("Content-Type"); got != tc.wantContentType {
			t.Errorf("GET %s: Content-Type %q, want %q", tc.path, got, tc.wantContentType)
		}
		if !strings.Contains(recorder.Body.String(), tc.wantBody) {
			t.Errorf("GET %s: body lacks %q", tc.path, tc.wantBody)
		}
		if recorder.Header().Get("ETag") == "" || recorder.Header().Get("Cache-Control") != "no-cache" {
			t.Errorf("GET %s: ETag %q, Cache-Control %q", tc.path, recorder.Header().Get("ETag"), recorder.Header().Get("Cache-Control"))
		}
	}

	doGet(t, srv, "/missing.js", http.StatusNotFound, nil)
	doGet(t, srv, "/embed.go", http.StatusNotFound, nil)
}

func TestStaticETagRevalidation(t *testing.T) {
	srv := newTestServer(t, map[string]string{"a.md": "# a\n"})
	first := httptest.NewRecorder()
	srv.ServeHTTP(first, newRequest("/vendor/mermaid.min.js"))
	etag := first.Header().Get("ETag")

	request := newRequest("/vendor/mermaid.min.js")
	request.Header.Set("If-None-Match", etag)
	second := httptest.NewRecorder()
	srv.ServeHTTP(second, request)
	if second.Code != http.StatusNotModified || second.Body.Len() != 0 {
		t.Errorf("revalidation: status %d, %d body bytes; want 304 and empty", second.Code, second.Body.Len())
	}
}

func TestIndexPageCSP(t *testing.T) {
	srv := newTestServer(t, map[string]string{"a.md": "# a\n"})
	recorder := httptest.NewRecorder()
	srv.ServeHTTP(recorder, newRequest("/"))
	if csp := recorder.Header().Get("Content-Security-Policy"); !strings.Contains(csp, "script-src 'self'") {
		t.Errorf("index CSP = %q", csp)
	}
}
