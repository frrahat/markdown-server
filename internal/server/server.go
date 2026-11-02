// Package server exposes the index and renderer over HTTP.
package server

import (
	"encoding/json"
	"log/slog"
	"net"
	"net/http"
	"strings"

	"github.com/frrahat/markdown-server/internal/index"
	"github.com/frrahat/markdown-server/internal/render"
)

type Config struct {
	// RootDir must be absolute with symlinks resolved (see pathsafe.Resolve).
	RootDir      string
	IndexHolder  *index.Holder
	Renderer     *render.Renderer
	MaxFileBytes int64
	Logger       *slog.Logger
}

type Server struct {
	rootDir      string
	indexHolder  *index.Holder
	renderer     *render.Renderer
	maxFileBytes int64
	logger       *slog.Logger
	mux          *http.ServeMux
}

func New(cfg Config) *Server {
	s := &Server{
		rootDir:      cfg.RootDir,
		indexHolder:  cfg.IndexHolder,
		renderer:     cfg.Renderer,
		maxFileBytes: cfg.MaxFileBytes,
		logger:       cfg.Logger,
		mux:          http.NewServeMux(),
	}
	if s.logger == nil {
		s.logger = slog.Default()
	}
	s.mux.HandleFunc("GET /api/tree", s.handleTree)
	s.mux.HandleFunc("GET /api/doc", s.handleDoc)
	s.mux.HandleFunc("GET /raw/{path...}", s.handleRaw)
	return s
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !isLoopbackHost(r.Host) {
		writeError(w, http.StatusForbidden, "host not allowed")
		return
	}
	if strings.HasPrefix(r.URL.Path, "/api/") || strings.HasPrefix(r.URL.Path, "/raw/") {
		w.Header().Set("Cache-Control", "no-store")
	}
	s.mux.ServeHTTP(w, r)
}

// isLoopbackHost guards against DNS rebinding: a page on evil.com that
// re-points its DNS at 127.0.0.1 still sends "Host: evil.com", so rejecting
// non-loopback names keeps other sites from reading local files through us.
func isLoopbackHost(hostHeader string) bool {
	hostName, _, err := net.SplitHostPort(hostHeader)
	if err != nil {
		// No port in the header.
		hostName = strings.TrimSuffix(strings.TrimPrefix(hostHeader, "["), "]")
	}
	switch strings.ToLower(hostName) {
	case "127.0.0.1", "localhost", "::1":
		return true
	}
	return false
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	// The status line is already sent, so an encoding failure can't be
	// reported to the client.
	_ = json.NewEncoder(w).Encode(body)
}

type errorResponse struct {
	Error string `json:"error"`
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, errorResponse{Error: message})
}
