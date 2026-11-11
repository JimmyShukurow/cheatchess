package main

import (
	"context"
	"embed"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path"
	"strings"
	"syscall"
	"time"
)

//go:embed all:web
var webFS embed.FS

type Config struct {
	Addr           string
	DBPath         string
	BaseURL        string // public URL, e.g. https://chess.example.com (used for Google redirect)
	GoogleID       string
	GoogleSecret   string
	CookieSecure   bool
	TrustProxy     bool
	AllowedOrigins []string
	AdminEmails    []string // accounts allowed to use the engine hint
	StockfishPath  string
}

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

// loadDotEnv reads KEY=VALUE lines from .env (if present) without overriding real env vars.
func loadDotEnv(path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		k, v = strings.TrimSpace(k), strings.Trim(strings.TrimSpace(v), `"'`)
		if _, exists := os.LookupEnv(k); !exists {
			os.Setenv(k, v)
		}
	}
}

func splitList(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, strings.ToLower(p))
		}
	}
	return out
}

func loadConfig() Config {
	loadDotEnv(".env")
	loadDotEnv("../.env")
	c := Config{
		AdminEmails:   splitList(os.Getenv("ADMIN_EMAILS")),
		StockfishPath: env("STOCKFISH_PATH", "stockfish"),
		Addr:          env("ADDR", ":8080"),
		DBPath:        env("DB_PATH", "cheatchess.db"),
		BaseURL:       strings.TrimRight(env("BASE_URL", "http://localhost:8080"), "/"),
		GoogleID:      os.Getenv("GOOGLE_CLIENT_ID"),
		GoogleSecret:  os.Getenv("GOOGLE_CLIENT_SECRET"),
		TrustProxy:    os.Getenv("TRUST_PROXY") == "1",
	}
	c.CookieSecure = strings.HasPrefix(c.BaseURL, "https://")
	c.AllowedOrigins = []string{c.BaseURL, "http://localhost:5173", "http://127.0.0.1:5173"}
	if extra := os.Getenv("ALLOWED_ORIGINS"); extra != "" {
		c.AllowedOrigins = append(c.AllowedOrigins, strings.Split(extra, ",")...)
	}
	return c
}

type App struct {
	cfg     Config
	db      *DB
	hub     *Hub
	limiter *RateLimiter
	engine  *Engine
}

func main() {
	cfg := loadConfig()
	// CLI: ./cheatchess health  (used by the Docker healthcheck)
	if len(os.Args) == 2 && os.Args[1] == "health" {
		c := http.Client{Timeout: 3 * time.Second}
		host := cfg.Addr
		if strings.HasPrefix(host, ":") {
			host = "127.0.0.1" + host
		}
		resp, err := c.Get("http://" + host + "/api/config")
		if err != nil || resp.StatusCode != 200 {
			os.Exit(1)
		}
		return
	}
	db, err := openDB(cfg.DBPath)
	if err != nil {
		log.Fatalf("db: %v", err)
	}
	// CLI: ./cheatchess verify-email someone@example.com
	if len(os.Args) == 3 && os.Args[1] == "verify-email" {
		n, err := db.SetEmailVerified(strings.ToLower(os.Args[2]))
		if err != nil || n == 0 {
			log.Fatalf("no account with that email (%v)", err)
		}
		log.Printf("email %s marked as verified", os.Args[2])
		return
	}
	// CLI: ./cheatchess backup /data/backups/x.db  (consistent copy, safe while the server runs)
	if len(os.Args) == 3 && os.Args[1] == "backup" {
		if _, err := db.Exec(`VACUUM INTO ?`, os.Args[2]); err != nil {
			log.Fatalf("backup failed: %v", err)
		}
		log.Printf("backup written to %s", os.Args[2])
		return
	}
	app := &App{cfg: cfg, db: db, limiter: newRateLimiter(20, 10*time.Minute)}
	app.engine = newEngine(cfg.StockfishPath)
	app.hub = newHub(app)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/config", app.handleConfig)
	mux.HandleFunc("POST /api/auth/register", app.handleRegister)
	mux.HandleFunc("POST /api/auth/login", app.handleLogin)
	mux.HandleFunc("POST /api/auth/logout", app.handleLogout)
	mux.HandleFunc("GET /api/auth/me", app.handleMe)
	mux.HandleFunc("GET /api/auth/google", app.handleGoogleStart)
	mux.HandleFunc("GET /api/auth/google/callback", app.handleGoogleCallback)
	mux.HandleFunc("GET /api/me/games", app.handleMyGames)
	mux.HandleFunc("GET /api/games/{id}", app.handleGetGame)
	mux.HandleFunc("GET /ws", app.hub.serveWS)
	mux.Handle("/", spaHandler())

	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	go func() {
		log.Printf("CheatChess listening on %s (public URL %s, google login: %v)", cfg.Addr, cfg.BaseURL, cfg.GoogleID != "")
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal(err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	log.Println("shutting down…")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = srv.Shutdown(ctx)
	app.engine.Close()
	db.Close()
}

func (a *App) handleConfig(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, map[string]any{"googleEnabled": a.cfg.GoogleID != "" && a.cfg.GoogleSecret != ""})
}

// spaHandler serves the built Vue app and falls back to index.html for client-side routes.
func spaHandler() http.Handler {
	dist, err := fs.Sub(webFS, "web/dist")
	if err != nil {
		panic(err)
	}
	fileServer := http.FileServer(http.FS(dist))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if p == "" {
			p = "index.html"
		}
		if _, err := fs.Stat(dist, p); err != nil {
			index, err := fs.ReadFile(dist, "index.html")
			if err != nil {
				http.Error(w, "frontend not built – run `make build` (or use the Vite dev server on :5173)", 404)
				return
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Header().Set("Cache-Control", "no-cache")
			w.Write(index)
			return
		}
		if strings.HasPrefix(p, "assets/") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		}
		fileServer.ServeHTTP(w, r)
	})
}
