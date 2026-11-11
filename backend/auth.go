package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/mail"
	"regexp"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

const sessionCookie = "sid"
const sessionTTL = 30 * 24 * time.Hour

var usernameRe = regexp.MustCompile(`^[A-Za-z0-9_-]{3,20}$`)

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func randomToken(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

func hashToken(t string) string {
	h := sha256.Sum256([]byte(t))
	return hex.EncodeToString(h[:])
}

func (a *App) clientIP(r *http.Request) string {
	if a.cfg.TrustProxy {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			return strings.TrimSpace(strings.Split(xff, ",")[0])
		}
	}
	host, _, _ := net.SplitHostPort(r.RemoteAddr)
	return host
}

func (a *App) startSession(w http.ResponseWriter, userID int64) error {
	tok := randomToken(32)
	if err := a.db.CreateSession(hashToken(tok), userID, sessionTTL); err != nil {
		return err
	}
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookie, Value: tok, Path: "/", HttpOnly: true, Secure: a.cfg.CookieSecure,
		SameSite: http.SameSiteLaxMode, MaxAge: int(sessionTTL.Seconds()),
	})
	return nil
}

// currentUser returns the logged in user or nil.
func (a *App) currentUser(r *http.Request) *User {
	c, err := r.Cookie(sessionCookie)
	if err != nil || c.Value == "" {
		return nil
	}
	u, err := a.db.SessionUser(hashToken(c.Value))
	if err != nil {
		return nil
	}
	return u
}

func decodeJSON(r *http.Request, v any) error {
	if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		return errors.New("expected JSON")
	}
	return json.NewDecoder(http.MaxBytesReader(nil, r.Body, 1<<16)).Decode(v)
}

func (a *App) handleRegister(w http.ResponseWriter, r *http.Request) {
	if !a.limiter.Allow(a.clientIP(r)) {
		writeErr(w, 429, "too many attempts, try again later")
		return
	}
	var req struct{ Email, Username, Password string }
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, 400, "bad request")
		return
	}
	req.Email = strings.TrimSpace(strings.ToLower(req.Email))
	req.Username = strings.TrimSpace(req.Username)
	if addr, err := mail.ParseAddress(req.Email); err != nil || addr.Address != req.Email || len(req.Email) > 254 {
		writeErr(w, 400, "invalid email address")
		return
	}
	if !usernameRe.MatchString(req.Username) {
		writeErr(w, 400, "username must be 3-20 characters: letters, digits, _ or -")
		return
	}
	if len(req.Password) < 6 || len(req.Password) > 72 {
		writeErr(w, 400, "password must be 6-72 characters")
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		writeErr(w, 500, "internal error")
		return
	}
	u, err := a.db.CreateUser(req.Email, req.Username, string(hash), "", false)
	if err != nil {
		writeErr(w, 409, err.Error())
		return
	}
	if err := a.startSession(w, u.ID); err != nil {
		writeErr(w, 500, "internal error")
		return
	}
	writeJSON(w, 200, map[string]any{"user": a.withAdmin(u)})
}

func (a *App) handleLogin(w http.ResponseWriter, r *http.Request) {
	if !a.limiter.Allow(a.clientIP(r)) {
		writeErr(w, 429, "too many attempts, try again later")
		return
	}
	var req struct{ Login, Password string }
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, 400, "bad request")
		return
	}
	u, err := a.db.UserByLogin(strings.TrimSpace(req.Login))
	if err != nil || u.PasswordHash == "" || bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(req.Password)) != nil {
		writeErr(w, 401, "wrong email/username or password")
		return
	}
	if err := a.startSession(w, u.ID); err != nil {
		writeErr(w, 500, "internal error")
		return
	}
	writeJSON(w, 200, map[string]any{"user": a.withAdmin(u)})
}

func (a *App) handleLogout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(sessionCookie); err == nil {
		a.db.DeleteSession(hashToken(c.Value))
	}
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: "", Path: "/", MaxAge: -1, HttpOnly: true})
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (a *App) handleMe(w http.ResponseWriter, r *http.Request) {
	u := a.currentUser(r)
	if u == nil {
		writeErr(w, 401, "not logged in")
		return
	}
	writeJSON(w, 200, map[string]any{"user": a.withAdmin(u)})
}

// ---------- Google OAuth ----------

func (a *App) googleConfig() *oauth2.Config {
	return &oauth2.Config{
		ClientID:     a.cfg.GoogleID,
		ClientSecret: a.cfg.GoogleSecret,
		RedirectURL:  a.cfg.BaseURL + "/api/auth/google/callback",
		Scopes:       []string{"openid", "email", "profile"},
		Endpoint:     google.Endpoint,
	}
}

func (a *App) handleGoogleStart(w http.ResponseWriter, r *http.Request) {
	if a.cfg.GoogleID == "" {
		http.Error(w, "Google login not configured", 404)
		return
	}
	state := randomToken(16)
	http.SetCookie(w, &http.Cookie{Name: "oauth_state", Value: state, Path: "/api/auth/google", HttpOnly: true,
		Secure: a.cfg.CookieSecure, SameSite: http.SameSiteLaxMode, MaxAge: 600})
	http.Redirect(w, r, a.googleConfig().AuthCodeURL(state, oauth2.SetAuthURLParam("prompt", "select_account")), http.StatusFound)
}

func (a *App) handleGoogleCallback(w http.ResponseWriter, r *http.Request) {
	fail := func(msg string) { http.Redirect(w, r, "/login?error="+msg, http.StatusFound) }
	c, err := r.Cookie("oauth_state")
	if err != nil || c.Value == "" || c.Value != r.URL.Query().Get("state") {
		fail("state")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	tok, err := a.googleConfig().Exchange(ctx, r.URL.Query().Get("code"))
	if err != nil {
		fail("exchange")
		return
	}
	resp, err := a.googleConfig().Client(ctx, tok).Get("https://openidconnect.googleapis.com/v1/userinfo")
	if err != nil {
		fail("userinfo")
		return
	}
	defer resp.Body.Close()
	var info struct {
		Sub           string `json:"sub"`
		Email         string `json:"email"`
		EmailVerified bool   `json:"email_verified"`
		Name          string `json:"name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil || info.Sub == "" {
		fail("userinfo")
		return
	}
	info.Email = strings.ToLower(info.Email)

	u, err := a.db.UserByGoogleID(info.Sub)
	if errors.Is(err, ErrNotFound) && info.Email != "" && info.EmailVerified {
		// Link to an existing email/password account with the same verified email.
		if u, err = a.db.UserByEmail(info.Email); err == nil {
			if err = a.db.LinkGoogle(u, info.Sub); err == nil {
				u, err = a.db.UserByID(u.ID)
			}
		}
	}
	if errors.Is(err, ErrNotFound) {
		email := ""
		if info.EmailVerified {
			email = info.Email
		}
		u, err = a.db.CreateUser(email, a.pickUsername(info.Name, info.Email), "", info.Sub, email != "")
	}
	if err != nil {
		fail("account")
		return
	}
	if err := a.startSession(w, u.ID); err != nil {
		fail("session")
		return
	}
	http.Redirect(w, r, "/", http.StatusFound)
}

var nonUsernameChars = regexp.MustCompile(`[^A-Za-z0-9_-]+`)

func (a *App) pickUsername(name, email string) string {
	base := nonUsernameChars.ReplaceAllString(name, "")
	if len(base) < 3 {
		base = nonUsernameChars.ReplaceAllString(strings.Split(email, "@")[0], "")
	}
	if len(base) < 3 {
		base = "player"
	}
	if len(base) > 14 {
		base = base[:14]
	}
	if !a.db.UsernameTaken(base) {
		return base
	}
	for i := 0; i < 50; i++ {
		b := make([]byte, 2)
		rand.Read(b)
		cand := fmt.Sprintf("%s%d", base, int(b[0])<<8|int(b[1]))
		if !a.db.UsernameTaken(cand) {
			return cand
		}
	}
	return base + randomToken(4)
}

// ---------- tiny rate limiter ----------

type RateLimiter struct {
	mu     sync.Mutex
	max    int
	window time.Duration
	hits   map[string]*rlEntry
}

type rlEntry struct {
	n     int
	start time.Time
}

func newRateLimiter(max int, window time.Duration) *RateLimiter {
	rl := &RateLimiter{max: max, window: window, hits: map[string]*rlEntry{}}
	go func() {
		for range time.Tick(window) {
			rl.mu.Lock()
			for k, e := range rl.hits {
				if time.Since(e.start) > rl.window {
					delete(rl.hits, k)
				}
			}
			rl.mu.Unlock()
		}
	}()
	return rl
}

func (rl *RateLimiter) Allow(key string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	e := rl.hits[key]
	if e == nil || time.Since(e.start) > rl.window {
		rl.hits[key] = &rlEntry{n: 1, start: time.Now()}
		return true
	}
	e.n++
	return e.n <= rl.max
}

// isAdmin: email listed in ADMIN_EMAILS *and* proven to belong to the user.
func (a *App) isAdmin(u *User) bool {
	if u == nil || !u.EmailVerified || u.Email == "" {
		return false
	}
	for _, e := range a.cfg.AdminEmails {
		if strings.EqualFold(e, u.Email) {
			return true
		}
	}
	return false
}

func (a *App) withAdmin(u *User) *User {
	u.IsAdmin = a.isAdmin(u)
	return u
}
