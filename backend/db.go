package main

import (
	"database/sql"
	"errors"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

type DB struct{ *sql.DB }

type User struct {
	ID            int64  `json:"id"`
	Email         string `json:"email,omitempty"`
	Username      string `json:"username"`
	PasswordHash  string `json:"-"`
	GoogleID      string `json:"-"`
	RatingBullet  int    `json:"ratingBullet"`
	RatingBlitz   int    `json:"ratingBlitz"`
	RatingRapid   int    `json:"ratingRapid"`
	EmailVerified bool   `json:"emailVerified"`
	IsAdmin       bool   `json:"isAdmin,omitempty"`
}

func (u *User) Rating(cat string) int {
	switch cat {
	case "bullet":
		return u.RatingBullet
	case "blitz":
		return u.RatingBlitz
	default:
		return u.RatingRapid
	}
}

// Public returns a copy without the email for showing to other players.
func (u *User) Public() *User {
	c := *u
	c.Email = ""
	c.IsAdmin = false
	return &c
}

const schema = `
CREATE TABLE IF NOT EXISTS users (
	id            INTEGER PRIMARY KEY AUTOINCREMENT,
	email         TEXT UNIQUE COLLATE NOCASE,
	username      TEXT NOT NULL UNIQUE COLLATE NOCASE,
	password_hash TEXT NOT NULL DEFAULT '',
	google_id     TEXT UNIQUE,
	rating_bullet INTEGER NOT NULL DEFAULT 1200,
	rating_blitz  INTEGER NOT NULL DEFAULT 1200,
	rating_rapid  INTEGER NOT NULL DEFAULT 1200,
	created_at    INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS sessions (
	token_hash TEXT PRIMARY KEY,
	user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	expires_at INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS games (
	id            TEXT PRIMARY KEY,
	white_id      INTEGER NOT NULL REFERENCES users(id),
	black_id      INTEGER NOT NULL REFERENCES users(id),
	tc            TEXT NOT NULL,
	moves         TEXT NOT NULL,
	result        TEXT NOT NULL,
	reason        TEXT NOT NULL,
	white_rating  INTEGER NOT NULL,
	black_rating  INTEGER NOT NULL,
	white_diff    INTEGER NOT NULL,
	black_diff    INTEGER NOT NULL,
	white_clock   INTEGER NOT NULL,
	black_clock   INTEGER NOT NULL,
	created_at    INTEGER NOT NULL,
	ended_at      INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS games_white ON games(white_id, ended_at);
CREATE INDEX IF NOT EXISTS games_black ON games(black_id, ended_at);
`

func openDB(path string) (*DB, error) {
	db, err := sql.Open("sqlite", path+"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)&_pragma=synchronous(NORMAL)")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1) // SQLite: single writer, keeps things simple and safe
	if _, err := db.Exec(schema); err != nil {
		return nil, err
	}
	// migrations (errors = column already exists)
	db.Exec(`ALTER TABLE users ADD COLUMN email_verified INTEGER NOT NULL DEFAULT 0`)
	go func() { // purge expired sessions
		for {
			db.Exec(`DELETE FROM sessions WHERE expires_at < ?`, time.Now().Unix())
			time.Sleep(time.Hour)
		}
	}()
	return &DB{db}, nil
}

var ErrNotFound = errors.New("not found")

const userCols = `id, COALESCE(email,''), username, password_hash, COALESCE(google_id,''), rating_bullet, rating_blitz, rating_rapid, email_verified`

func scanUser(row interface{ Scan(...any) error }) (*User, error) {
	u := &User{}
	err := row.Scan(&u.ID, &u.Email, &u.Username, &u.PasswordHash, &u.GoogleID, &u.RatingBullet, &u.RatingBlitz, &u.RatingRapid, &u.EmailVerified)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return u, err
}

func (db *DB) UserByID(id int64) (*User, error) {
	return scanUser(db.QueryRow(`SELECT `+userCols+` FROM users WHERE id = ?`, id))
}

func (db *DB) UserByLogin(login string) (*User, error) {
	return scanUser(db.QueryRow(`SELECT `+userCols+` FROM users WHERE email = ? OR username = ?`, login, login))
}

func (db *DB) UserByEmail(email string) (*User, error) {
	return scanUser(db.QueryRow(`SELECT `+userCols+` FROM users WHERE email = ?`, email))
}

func (db *DB) UserByGoogleID(gid string) (*User, error) {
	return scanUser(db.QueryRow(`SELECT `+userCols+` FROM users WHERE google_id = ?`, gid))
}

func (db *DB) UsernameTaken(name string) bool {
	var n int
	db.QueryRow(`SELECT COUNT(*) FROM users WHERE username = ?`, name).Scan(&n)
	return n > 0
}

func nullable(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func (db *DB) CreateUser(email, username, passwordHash, googleID string, verified bool) (*User, error) {
	res, err := db.Exec(`INSERT INTO users (email, username, password_hash, google_id, email_verified, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
		nullable(email), username, passwordHash, nullable(googleID), verified, time.Now().Unix())
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			if strings.Contains(err.Error(), "email") {
				return nil, errors.New("email already registered")
			}
			if strings.Contains(err.Error(), "username") {
				return nil, errors.New("username already taken")
			}
		}
		return nil, err
	}
	id, _ := res.LastInsertId()
	return db.UserByID(id)
}

// LinkGoogle attaches a Google identity (with a Google-verified email) to an existing account.
// If the account's email was never verified, whoever registered it may not own the address,
// so the password and all existing sessions are revoked.
func (db *DB) LinkGoogle(u *User, gid string) error {
	if !u.EmailVerified {
		if _, err := db.Exec(`UPDATE users SET password_hash = '' WHERE id = ?`, u.ID); err != nil {
			return err
		}
		db.Exec(`DELETE FROM sessions WHERE user_id = ?`, u.ID)
	}
	_, err := db.Exec(`UPDATE users SET google_id = ?, email_verified = 1 WHERE id = ?`, gid, u.ID)
	return err
}

func (db *DB) SetEmailVerified(email string) (int64, error) {
	res, err := db.Exec(`UPDATE users SET email_verified = 1 WHERE email = ?`, email)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (db *DB) CreateSession(tokenHash string, userID int64, ttl time.Duration) error {
	_, err := db.Exec(`INSERT INTO sessions (token_hash, user_id, expires_at) VALUES (?, ?, ?)`, tokenHash, userID, time.Now().Add(ttl).Unix())
	return err
}

func (db *DB) SessionUser(tokenHash string) (*User, error) {
	return scanUser(db.QueryRow(`SELECT `+userCols+` FROM sessions s JOIN users u ON u.id = s.user_id WHERE s.token_hash = ? AND s.expires_at > ?`,
		tokenHash, time.Now().Unix()))
}

func (db *DB) DeleteSession(tokenHash string) {
	db.Exec(`DELETE FROM sessions WHERE token_hash = ?`, tokenHash)
}

func ratingColumn(cat string) string {
	switch cat {
	case "bullet":
		return "rating_bullet"
	case "blitz":
		return "rating_blitz"
	default:
		return "rating_rapid"
	}
}

// SaveGame stores a finished game and applies rating changes atomically.
func (db *DB) SaveGame(s *GameState, category string) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	_, err = tx.Exec(`INSERT INTO games (id, white_id, black_id, tc, moves, result, reason, white_rating, black_rating, white_diff, black_diff, white_clock, black_clock, created_at, ended_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		s.ID, s.White.ID, s.Black.ID, s.TC, strings.Join(s.Moves, " "), s.Result, s.Reason,
		s.White.Rating, s.Black.Rating, s.RatingDiff.W, s.RatingDiff.B, s.Clock.W, s.Clock.B, s.CreatedAt, time.Now().UnixMilli())
	if err != nil {
		return err
	}
	if s.RatingDiff.W != 0 || s.RatingDiff.B != 0 {
		col := ratingColumn(category)
		if _, err := tx.Exec(`UPDATE users SET `+col+` = `+col+` + ? WHERE id = ?`, s.RatingDiff.W, s.White.ID); err != nil {
			return err
		}
		if _, err := tx.Exec(`UPDATE users SET `+col+` = `+col+` + ? WHERE id = ?`, s.RatingDiff.B, s.Black.ID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

const gameSelect = `SELECT g.id, g.tc, g.moves, g.result, g.reason, g.white_rating, g.black_rating, g.white_diff, g.black_diff,
	g.white_clock, g.black_clock, g.created_at, g.white_id, wu.username, g.black_id, bu.username
	FROM games g JOIN users wu ON wu.id = g.white_id JOIN users bu ON bu.id = g.black_id`

func scanGame(row interface{ Scan(...any) error }) (*GameState, error) {
	s := &GameState{Status: "ended", White: &PlayerInfo{}, Black: &PlayerInfo{}}
	var moves string
	err := row.Scan(&s.ID, &s.TC, &moves, &s.Result, &s.Reason, &s.White.Rating, &s.Black.Rating, &s.RatingDiff.W, &s.RatingDiff.B,
		&s.Clock.W, &s.Clock.B, &s.CreatedAt, &s.White.ID, &s.White.Username, &s.Black.ID, &s.Black.Username)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	s.Moves = []string{}
	if moves != "" {
		s.Moves = strings.Split(moves, " ")
	}
	return s, err
}

func (db *DB) GameByID(id string) (*GameState, error) {
	return scanGame(db.QueryRow(gameSelect+` WHERE g.id = ?`, id))
}

func (db *DB) RecentGames(userID int64, limit int) ([]*GameState, error) {
	rows, err := db.Query(gameSelect+` WHERE g.white_id = ? OR g.black_id = ? ORDER BY g.ended_at DESC LIMIT ?`, userID, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*GameState{}
	for rows.Next() {
		g, err := scanGame(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}
