package server

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDoc(t *testing.T) {
	srv, root := newTestServerWithRoot(t, map[string]string{
		"docs/guide.md": "# Guide\n\n[next](next.md) <script>x()</script>\n",
		"docs/next.md":  "# Next\n",
	})

	var doc docResponse
	response := doGet(t, srv, "/api/doc?path=docs/guide.md", http.StatusOK, &doc)
	if response.Header.Get("Cache-Control") != "no-store" {
		t.Errorf("Cache-Control = %q", response.Header.Get("Cache-Control"))
	}
	if doc.Path != "docs/guide.md" || doc.ModTime.IsZero() || doc.Size == 0 {
		t.Errorf("doc metadata = %+v", doc)
	}
	if !strings.Contains(doc.HTML, `<h1 id="guide">Guide</h1>`) ||
		!strings.Contains(doc.HTML, `href="#/docs/next.md"`) ||
		strings.Contains(doc.HTML, "<script") {
		t.Errorf("unexpected html: %s", doc.HTML)
	}

	// No caching anywhere: an edit shows up on the very next request.
	writeFile(t, root, "docs/guide.md", "# Edited\n")
	doGet(t, srv, "/api/doc?path=docs/guide.md", http.StatusOK, &doc)
	if !strings.Contains(doc.HTML, "Edited") {
		t.Errorf("edit not visible: %s", doc.HTML)
	}
}

func TestDocErrors(t *testing.T) {
	srv, root := newTestServerWithRoot(t, map[string]string{
		"a.md":     "# a\n",
		"big.md":   strings.Repeat("x", 2<<10),
		"gone.md":  "# gone\n",
		"notes.go": "package notes\n",
	})
	writeFile(t, root, "created-after-scan.md", "# new\n")
	if err := os.Remove(filepath.Join(root, "gone.md")); err != nil {
		t.Fatal(err)
	}

	doGet(t, srv, "/api/doc?path=big.md", http.StatusRequestEntityTooLarge, nil)
	doGet(t, srv, "/api/doc?path=created-after-scan.md", http.StatusNotFound, nil)
	doGet(t, srv, "/api/doc?path=gone.md", http.StatusNotFound, nil)
	doGet(t, srv, "/api/doc?path=notes.go", http.StatusNotFound, nil)
	doGet(t, srv, "/api/doc?path=", http.StatusNotFound, nil)
	doGet(t, srv, "/api/doc?path=..%2Fa.md", http.StatusBadRequest, nil)
	doGet(t, srv, "/api/doc?path=%2e%2e%2Fx", http.StatusBadRequest, nil)
	doGet(t, srv, "/api/doc?path=/etc/passwd", http.StatusBadRequest, nil)
}

func TestRaw(t *testing.T) {
	srv, root := newTestServerWithRoot(t, map[string]string{
		"a.md":              "# a\n",
		"img/logo.png":      "\x89PNG\r\n\x1a\nfake",
		"img/my logo.svg":   "<svg xmlns=\"http://www.w3.org/2000/svg\"/>",
		".env":              "SECRET=1",
		"img/.hidden.png":   "x",
		".gitignore":        "build/\n",
		"build/out.png":     "x",
		"node_modules/a.js": "x",
		".git/config":       "x",
	})
	outside := t.TempDir()
	writeFile(t, outside, "secret.png", "outside")
	mustSymlink(t, filepath.Join(outside, "secret.png"), filepath.Join(root, "img", "escape.png"))
	mustSymlink(t, filepath.Join(root, ".env"), filepath.Join(root, "img", "env.png"))
	mustSymlink(t, filepath.Join(root, "img", "logo.png"), filepath.Join(root, "img", "alias.png"))

	recorder := httptest.NewRecorder()
	srv.ServeHTTP(recorder, newRequest("/raw/img/logo.png"))
	if recorder.Code != http.StatusOK || recorder.Header().Get("Content-Type") != "image/png" {
		t.Fatalf("logo: status %d, type %q", recorder.Code, recorder.Header().Get("Content-Type"))
	}
	for header, want := range map[string]string{
		"X-Content-Type-Options":  "nosniff",
		"Content-Security-Policy": "sandbox",
		"Cache-Control":           "no-store",
	} {
		if got := recorder.Header().Get(header); got != want {
			t.Errorf("%s = %q, want %q", header, got, want)
		}
	}

	doGet(t, srv, "/raw/img/my%20logo.svg", http.StatusOK, nil)
	doGet(t, srv, "/raw/img/alias.png", http.StatusOK, nil)
	// Percent-encoded traversal decodes to ".." and is rejected outright.
	doGet(t, srv, "/raw/%2e%2e/etc/passwd", http.StatusBadRequest, nil)
	doGet(t, srv, "/raw/img/..%2F..%2Fetc%2Fpasswd", http.StatusBadRequest, nil)

	notServed := []string{
		"/raw/.env",
		"/raw/img/.hidden.png",
		"/raw/.gitignore",
		"/raw/build/out.png",
		"/raw/node_modules/a.js",
		"/raw/.git/config",
		"/raw/img",
		"/raw/missing.png",
		"/raw/img/escape.png",
		"/raw/img/env.png",
		"/raw/%2e%2e/etc/passwd",
		"/raw/img/..%2F..%2Fetc%2Fpasswd",
		"/raw/img%5C..%5Ca.md",
		"/raw/",
	}
	for _, target := range notServed {
		recorder := httptest.NewRecorder()
		srv.ServeHTTP(recorder, newRequest(target))
		if recorder.Code == http.StatusOK || strings.Contains(recorder.Body.String(), "SECRET") || strings.Contains(recorder.Body.String(), "outside") {
			t.Errorf("GET %s: status %d, body %q; want it refused", target, recorder.Code, recorder.Body)
		}
	}
}

func writeFile(t *testing.T, root, relPath, content string) {
	t.Helper()
	absPath := filepath.Join(root, filepath.FromSlash(relPath))
	if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(absPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func mustSymlink(t *testing.T, target, link string) {
	t.Helper()
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
}
