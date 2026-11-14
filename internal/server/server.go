// Package server is the local HTTP server that serves the plan and review
// state to a browser on the same machine.
package server

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"os/exec"
	"runtime"
	"time"

	"github.com/sakshamsaxena/blaze/internal/gitx"
	"github.com/sakshamsaxena/blaze/internal/logger"
	"github.com/sakshamsaxena/blaze/internal/plan"
	"github.com/sakshamsaxena/blaze/internal/state"
	"github.com/sakshamsaxena/blaze/web"
)

// Options configures Serve.
type Options struct {
	Plan    *plan.Plan
	State   state.IStateStore
	Git     *gitx.Runner
	Port    int
	OpenURL bool
	Version string
	Stderr  io.Writer
}

// Serve blocks until ctx is cancelled or the listener fails.
func Serve(ctx context.Context, opts Options) error {
	token, err := newToken()
	if err != nil {
		return fmt.Errorf("server: new token: %w", err)
	}

	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", opts.Port))
	if err != nil {
		return fmt.Errorf("server: listen: %w", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	url := fmt.Sprintf("http://127.0.0.1:%d/", port)

	fmt.Fprintf(opts.Stderr, "blaze: %s  (Ctrl-C to stop)\n", url)

	if opts.OpenURL {
		openBrowser(url)
	}

	handler, err := buildHandler(opts.Plan, opts.State, opts.Git, port, token)
	if err != nil {
		return fmt.Errorf("server: build handler: %w", err)
	}

	srv := &http.Server{Handler: handler, ReadHeaderTimeout: 5 * time.Second}

	serveErr := make(chan error, 1)
	go func() {
		serveErr <- srv.Serve(ln)
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
		return nil
	case err := <-serveErr:
		if err != nil && err != http.ErrServerClosed {
			return fmt.Errorf("server: serve: %w", err)
		}
		return nil
	}
}

// buildHandler assembles the full HTTP handler: the API mux wrapped by the
// security middleware. port is the bound listener's port, used by the host
// check; it needs no network connection of its own.
func buildHandler(p *plan.Plan, st state.IStateStore, git sourceReader, port int, token string) (http.Handler, error) {
	planJSON, err := json.Marshal(p)
	if err != nil {
		return nil, fmt.Errorf("encode plan: %w", err)
	}

	raw, err := fs.ReadFile(web.Static(), "index.html")
	if err != nil {
		return nil, fmt.Errorf("read index.html: %w", err)
	}
	index := bytes.Replace(raw, []byte("__BLAZE_TOKEN__"), []byte(token), 1)

	a := &api{plan: p, planJSON: planJSON, state: st, git: git}
	mux := newMux(a, index)
	return withSecurity(mux, port, token), nil
}

// newMux registers the routes of 06-server.md on a fresh ServeMux.
func newMux(a *api, index []byte) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(index)
	})
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServerFS(web.Static())))
	mux.HandleFunc("GET /api/plan", a.handlePlan)
	mux.HandleFunc("GET /api/state", a.handleState)
	mux.HandleFunc("PUT /api/items/{id}/review", a.handleReview)
	mux.HandleFunc("PUT /api/items/{id}/note", a.handleNote)
	mux.HandleFunc("GET /api/source", a.handleSource)
	return mux
}

// newToken returns 16 random bytes, hex-encoded.
func newToken() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("read random: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// openBrowser starts the OS's URL opener for url without waiting for it. A
// failure is logged at warn level only.
func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	if err := cmd.Start(); err != nil {
		log := logger.Get().With().Str("component", "server").Logger()
		log.Warn().Err(err).Msg("open browser")
	}
}
