package localui

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/subtle"
	"embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/RandyVentures/greenlight/internal/history"
)

//go:embed web/static/*
var assets embed.FS

type Server struct {
	root    string
	token   string
	host    string
	mu      sync.Mutex
	jobs    map[string]*Job
	active  string
	ctx     context.Context
	history *history.Store
}

func New(ctx context.Context, root string) (*Server, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	abs, err = filepath.EvalSymlinks(abs)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(abs)
	if err != nil || !info.IsDir() {
		return nil, fmt.Errorf("workspace must be an existing directory")
	}
	token, err := randomID()
	if err != nil {
		return nil, err
	}
	store, err := history.New("")
	if err != nil {
		return nil, err
	}
	return &Server{root: abs, token: token, jobs: map[string]*Job{}, ctx: ctx, history: store}, nil
}

// SetHistoryDir is a CLI-only override; browsers cannot select the state directory.
func (s *Server) SetHistoryDir(dir string) error {
	store, err := history.New(dir)
	if err != nil {
		return err
	}
	s.history = store
	return nil
}

func randomID() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// Serve binds an ephemeral IPv4 loopback port by default; it cannot listen on a
// public interface. The session token stays in the browser URL fragment.
func (s *Server) Serve(port int, ready func(string)) error {
	ln, err := net.Listen("tcp4", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return err
	}
	s.host = ln.Addr().String()
	h := &http.Server{Handler: s.Handler(), ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 60 * time.Second}
	go func() {
		<-s.ctx.Done()
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		h.Shutdown(ctx)
	}()
	ready("http://" + s.host + "/#" + s.token)
	err = h.Serve(ln)
	if err == http.ErrServerClosed {
		return nil
	}
	return err
}

func (s *Server) Handler() http.Handler {
	static, _ := fs.Sub(assets, "web/static")
	files := http.FileServer(http.FS(static))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' blob: data:; connect-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'none'")
		if s.host != "" && r.Host != s.host {
			http.Error(w, "invalid host", http.StatusForbidden)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/api/") {
			if !s.authorized(r) {
				http.Error(w, "local session required; reopen the URL printed by greenlight ui", http.StatusForbidden)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			switch {
			case r.URL.Path == "/api/info" && r.Method == "GET":
				_, err := os.Stat(filepath.Join(s.root, "examples", "ui-demo"))
				json.NewEncoder(w).Encode(map[string]any{"workspace": s.root, "name": filepath.Base(s.root), "example_available": err == nil})
			case r.URL.Path == "/api/browse" && r.Method == "GET":
				s.browse(w, r)
			case r.URL.Path == "/api/plan" && r.Method == "POST":
				var selection Selection
				if !decode(w, r, &selection) {
					return
				}
				plan, err := Plan(selection)
				if err != nil {
					apiError(w, err, 400)
					return
				}
				json.NewEncoder(w).Encode(plan)
			case r.URL.Path == "/api/run" && r.Method == "POST":
				s.start(w, r)
			case r.URL.Path == "/api/history" && (r.Method == "GET" || r.Method == "POST"):
				s.historyAPI(w, r)
			case strings.HasPrefix(r.URL.Path, "/api/jobs/") && r.Method == "GET":
				s.job(w, r)
			default:
				apiError(w, fmt.Errorf("unknown API route or method"), 404)
			}
			return
		}
		if r.Method != "GET" && r.Method != "HEAD" {
			http.Error(w, "method not allowed", 405)
			return
		}
		if (r.URL.Path == "/app.js" || r.URL.Path == "/app.css") && strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
			data, err := fs.ReadFile(static, strings.TrimPrefix(r.URL.Path, "/")+".gz")
			if err == nil {
				w.Header().Set("Content-Encoding", "gzip")
				w.Header().Set("Vary", "Accept-Encoding")
				mime := "text/javascript; charset=utf-8"
				if r.URL.Path == "/app.css" {
					mime = "text/css; charset=utf-8"
				}
				w.Header().Set("Content-Type", mime)
				http.ServeContent(w, r, r.URL.Path, time.Time{}, bytes.NewReader(data))
				return
			}
		}
		files.ServeHTTP(w, r)
	})
}

func (s *Server) authorized(r *http.Request) bool {
	if origin := r.Header.Get("Origin"); origin != "" && origin != "http://"+r.Host {
		return false
	}
	if site := r.Header.Get("Sec-Fetch-Site"); site != "" && site != "same-origin" && site != "none" {
		return false
	}
	token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	return len(token) == len(s.token) && subtle.ConstantTimeCompare([]byte(token), []byte(s.token)) == 1
}

func decode(w http.ResponseWriter, r *http.Request, dest any) bool {
	if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		apiError(w, fmt.Errorf("JSON required"), 415)
		return false
	}
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 256<<10))
	d.DisallowUnknownFields()
	if err := d.Decode(dest); err != nil {
		apiError(w, fmt.Errorf("invalid request: %w", err), 400)
		return false
	}
	var extra any
	if err := d.Decode(&extra); err != io.EOF {
		apiError(w, fmt.Errorf("expected one JSON document"), 400)
		return false
	}
	return true
}

func apiError(w http.ResponseWriter, err error, code int) {
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
}

// Resolve includes symlink resolution before enforcing the workspace boundary.
func (s *Server) resolve(path string) (string, error) {
	if path == "" {
		path = "."
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(s.root, path)
	}
	abs, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(s.root, abs)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("choose a path inside the opened workspace")
	}
	return abs, nil
}

type Entry struct {
	Name      string `json:"name"`
	Path      string `json:"path"`
	Directory bool   `json:"directory"`
}

func (s *Server) browse(w http.ResponseWriter, r *http.Request) {
	path, err := s.resolve(r.URL.Query().Get("path"))
	if err != nil {
		apiError(w, err, 400)
		return
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		apiError(w, err, 400)
		return
	}
	out := []Entry{}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".") || entry.Type()&os.ModeSymlink != 0 || entry.Name() == "node_modules" {
			continue
		}
		if len(out) >= 2000 {
			apiError(w, fmt.Errorf("folder has too many entries; choose a smaller workspace"), 400)
			return
		}
		p, _ := filepath.Rel(s.root, filepath.Join(path, entry.Name()))
		out = append(out, Entry{Name: entry.Name(), Path: filepath.ToSlash(p), Directory: entry.IsDir()})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Directory != out[j].Directory {
			return out[i].Directory
		}
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	rel, _ := filepath.Rel(s.root, path)
	json.NewEncoder(w).Encode(map[string]any{"path": filepath.ToSlash(rel), "entries": out})
}
