package main

import (
	"errors"
	"fmt"
	"log"
	"math"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/notnil/chess"
)

// Time allowed to make the first move (each side) before the game is aborted.
const firstMoveTimeout = 30 * time.Second

var allowedTCs = map[string]bool{
	"1+0": true, "1+1": true, "2+1": true,
	"3+0": true, "3+2": true, "5+0": true, "5+3": true,
	"10+0": true, "10+5": true, "15+10": true, "30+0": true,
}

type TimeControl struct {
	Base time.Duration
	Inc  time.Duration
}

func parseTC(s string) (TimeControl, error) {
	if !allowedTCs[s] {
		return TimeControl{}, errors.New("unsupported time control")
	}
	parts := strings.Split(s, "+")
	m, _ := strconv.Atoi(parts[0])
	i, _ := strconv.Atoi(parts[1])
	return TimeControl{Base: time.Duration(m) * time.Minute, Inc: time.Duration(i) * time.Second}, nil
}

func (tc TimeControl) Category() string {
	est := tc.Base.Seconds() + 40*tc.Inc.Seconds()
	switch {
	case est < 180:
		return "bullet"
	case est < 600:
		return "blitz"
	default:
		return "rapid"
	}
}

type WB[T any] struct {
	W T `json:"w"`
	B T `json:"b"`
}

type PlayerInfo struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
	Rating   int    `json:"rating"`
}

// GameState is what gets sent to clients and stored in the DB.
type GameState struct {
	ID         string      `json:"id"`
	White      *PlayerInfo `json:"white"`
	Black      *PlayerInfo `json:"black"`
	TC         string      `json:"tc"`
	Moves      []string    `json:"moves"` // UCI
	Clock      WB[int64]   `json:"clock"` // ms remaining
	Running    bool        `json:"running"`
	Status     string      `json:"status"` // active | ended
	Result     string      `json:"result"` // 1-0 | 0-1 | 1/2-1/2 | *
	Reason     string      `json:"reason"`
	DrawOffer  string      `json:"drawOffer"` // "", "w", "b"
	RatingDiff WB[int]     `json:"ratingDiff"`
	CreatedAt  int64       `json:"createdAt"`
	Rematch    string      `json:"rematch"` // "", "w", "b": who offered a rematch
	// ms left to make the first move (only while fewer than 2 moves are played)
	FirstMoveLeft int64 `json:"firstMoveLeft,omitempty"`
}

type Game struct {
	mu        sync.Mutex
	hub       *Hub
	id        string
	white     *PlayerInfo
	black     *PlayerInfo
	tcStr     string
	tc        TimeControl
	board     *chess.Game
	moves     []string
	clock     [2]time.Duration // 0 = white, 1 = black
	turnStart time.Time
	ended     bool
	result    string
	reason    string
	drawOffer int // -1 none, 0 white, 1 black
	diff      [2]int
	timer     *time.Timer
	createdAt time.Time
	rematchBy int // -1 none, else color offering a rematch
	rematched bool
}

func newGame(h *Hub, id string, white, black *PlayerInfo, tcStr string, tc TimeControl) *Game {
	g := &Game{
		hub: h, id: id, white: white, black: black, tcStr: tcStr, tc: tc,
		board: chess.NewGame(), moves: []string{}, clock: [2]time.Duration{tc.Base, tc.Base},
		turnStart: time.Now(), drawOffer: -1, rematchBy: -1, result: "*", createdAt: time.Now(),
	}
	g.timer = time.AfterFunc(firstMoveTimeout, g.onTimer)
	return g
}

func (g *Game) turn() int { return len(g.moves) % 2 }

// clockRunning: clocks only start after both sides made their first move.
func (g *Game) clockRunning() bool { return !g.ended && len(g.moves) >= 2 }

func (g *Game) remaining(color int) time.Duration {
	r := g.clock[color]
	if g.clockRunning() && g.turn() == color {
		r -= time.Since(g.turnStart)
	}
	if r < 0 {
		r = 0
	}
	return r
}

func (g *Game) colorOf(userID int64) int {
	switch userID {
	case g.white.ID:
		return 0
	case g.black.ID:
		return 1
	}
	return -1
}

func (g *Game) stateLocked() *GameState {
	s := &GameState{
		ID: g.id, White: g.white, Black: g.black, TC: g.tcStr,
		Moves:      append([]string{}, g.moves...),
		Clock:      WB[int64]{g.remaining(0).Milliseconds(), g.remaining(1).Milliseconds()},
		Running:    g.clockRunning(),
		Status:     "active",
		Result:     g.result,
		Reason:     g.reason,
		RatingDiff: WB[int]{g.diff[0], g.diff[1]},
		CreatedAt:  g.createdAt.UnixMilli(),
	}
	if g.ended {
		s.Status = "ended"
	}
	if g.drawOffer == 0 {
		s.DrawOffer = "w"
	} else if g.drawOffer == 1 {
		s.DrawOffer = "b"
	}
	if g.rematchBy == 0 {
		s.Rematch = "w"
	} else if g.rematchBy == 1 {
		s.Rematch = "b"
	}
	if !g.ended && len(g.moves) < 2 {
		left := firstMoveTimeout - time.Since(g.turnStart)
		if left < 0 {
			left = 0
		}
		s.FirstMoveLeft = left.Milliseconds()
	}
	return s
}

func (g *Game) State() *GameState {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.stateLocked()
}

func (g *Game) broadcastLocked() {
	g.hub.broadcastGame(g.id, g.white.ID, g.black.ID, g.stateLocked())
}

func (g *Game) resetTimerLocked() {
	if g.ended {
		g.timer.Stop()
		return
	}
	if len(g.moves) < 2 {
		g.timer.Reset(firstMoveTimeout - time.Since(g.turnStart))
		return
	}
	g.timer.Reset(g.remaining(g.turn()) + 5*time.Millisecond)
}

func (g *Game) onTimer() {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.ended {
		return
	}
	if len(g.moves) < 2 {
		if time.Since(g.turnStart) >= firstMoveTimeout {
			g.endLocked("*", "aborted")
			return
		}
	} else if g.remaining(g.turn()) <= 0 {
		g.flagLocked(g.turn())
		return
	}
	g.resetTimerLocked()
}

// flagLocked: `color` ran out of time.
func (g *Game) flagLocked(color int) {
	g.clock[color] = 0
	g.turnStart = time.Now()
	if !hasMatingMaterial(g.board.Position(), 1-color) {
		g.endLocked("1/2-1/2", "timeout vs insufficient material")
		return
	}
	if color == 0 {
		g.endLocked("0-1", "timeout")
	} else {
		g.endLocked("1-0", "timeout")
	}
}

func hasMatingMaterial(pos *chess.Position, color int) bool {
	c := chess.White
	if color == 1 {
		c = chess.Black
	}
	minors := 0
	for _, p := range pos.Board().SquareMap() {
		if p.Color() != c {
			continue
		}
		switch p.Type() {
		case chess.Pawn, chess.Rook, chess.Queen:
			return true
		case chess.Bishop, chess.Knight:
			minors++
		}
	}
	return minors >= 2
}

func (g *Game) Move(userID int64, uci string) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.ended {
		return errors.New("game is over")
	}
	color := g.colorOf(userID)
	if color < 0 {
		return errors.New("you are not playing in this game")
	}
	if color != g.turn() {
		return errors.New("not your turn")
	}
	m, err := chess.UCINotation{}.Decode(g.board.Position(), uci)
	if err != nil {
		return errors.New("bad move")
	}
	now := time.Now()
	if g.clockRunning() {
		g.clock[color] -= now.Sub(g.turnStart)
		if g.clock[color] <= 0 {
			g.flagLocked(color)
			return errors.New("time is up")
		}
	}
	if err := g.board.Move(m); err != nil {
		return errors.New("illegal move")
	}
	if len(g.moves) >= 2 {
		g.clock[color] += g.tc.Inc
	}
	g.moves = append(g.moves, uci)
	g.turnStart = now
	g.drawOffer = -1

	// Automatic draws (chess.com style): threefold & fifty-move rule.
	if g.board.Outcome() == chess.NoOutcome {
		for _, d := range g.board.EligibleDraws() {
			if d == chess.ThreefoldRepetition || d == chess.FiftyMoveRule {
				g.board.Draw(d)
				break
			}
		}
	}
	if out := g.board.Outcome(); out != chess.NoOutcome {
		g.endLocked(string(out), methodName(g.board.Method()))
		return nil
	}
	g.resetTimerLocked()
	g.broadcastLocked()
	return nil
}

func methodName(m chess.Method) string {
	switch m {
	case chess.Checkmate:
		return "checkmate"
	case chess.Stalemate:
		return "stalemate"
	case chess.ThreefoldRepetition, chess.FivefoldRepetition:
		return "repetition"
	case chess.FiftyMoveRule, chess.SeventyFiveMoveRule:
		return "50-move rule"
	case chess.InsufficientMaterial:
		return "insufficient material"
	}
	return strings.ToLower(m.String())
}

func (g *Game) Resign(userID int64) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	c := g.colorOf(userID)
	if c < 0 || g.ended {
		return errors.New("cannot resign")
	}
	g.freezeClockLocked()
	if c == 0 {
		g.endLocked("0-1", "resignation")
	} else {
		g.endLocked("1-0", "resignation")
	}
	return nil
}

func (g *Game) Abort(userID int64) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.colorOf(userID) < 0 || g.ended {
		return errors.New("cannot abort")
	}
	if len(g.moves) >= 2 {
		return errors.New("too late to abort, resign instead")
	}
	g.endLocked("*", "aborted")
	return nil
}

func (g *Game) OfferDraw(userID int64) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	c := g.colorOf(userID)
	if c < 0 || g.ended {
		return errors.New("cannot offer draw")
	}
	if g.drawOffer == 1-c { // opponent already offered: offering back = accept
		g.freezeClockLocked()
		g.endLocked("1/2-1/2", "agreement")
		return nil
	}
	g.drawOffer = c
	g.broadcastLocked()
	return nil
}

func (g *Game) AnswerDraw(userID int64, accept bool) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	c := g.colorOf(userID)
	if c < 0 || g.ended || g.drawOffer != 1-c {
		return errors.New("no draw offer")
	}
	if accept {
		g.freezeClockLocked()
		g.endLocked("1/2-1/2", "agreement")
		return nil
	}
	g.drawOffer = -1
	g.broadcastLocked()
	return nil
}

// freezeClockLocked charges the running side for the time used so far.
func (g *Game) freezeClockLocked() {
	if g.clockRunning() {
		t := g.turn()
		g.clock[t] = g.remaining(t)
		g.turnStart = time.Now()
	}
}

func eloDiff(ra, rb int, score float64) int {
	exp := 1 / (1 + math.Pow(10, float64(rb-ra)/400))
	return int(math.Round(32 * (score - exp)))
}

func (g *Game) endLocked(result, reason string) {
	if g.ended {
		return
	}
	g.ended = true
	g.result = result
	g.reason = reason
	g.drawOffer = -1
	g.timer.Stop()

	if result != "*" {
		ws := 0.5
		if result == "1-0" {
			ws = 1
		} else if result == "0-1" {
			ws = 0
		}
		g.diff[0] = eloDiff(g.white.Rating, g.black.Rating, ws)
		g.diff[1] = eloDiff(g.black.Rating, g.white.Rating, 1-ws)
	}

	st := g.stateLocked()
	if result != "*" {
		go func() {
			if err := g.hub.app.db.SaveGame(st, g.tc.Category()); err != nil {
				log.Printf("save game %s: %v", g.id, err)
			}
			g.hub.notifyUserUpdated(g.white.ID, g.black.ID)
		}()
	}
	g.hub.gameEnded(g)
	g.hub.broadcastGame(g.id, g.white.ID, g.black.ID, st)
	log.Printf("game %s ended: %s %s (%s vs %s)", g.id, result, reason, g.white.Username, g.black.Username)
}

func (g *Game) String() string { return fmt.Sprintf("Game(%s)", g.id) }

// FENFor returns the current position (and the side-to-move's remaining clock,
// 0 if the clock is not running) if userID plays in this game.
func (g *Game) FENFor(userID int64) (string, int, time.Duration, bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.colorOf(userID) < 0 {
		return "", 0, 0, false
	}
	var rem time.Duration
	if g.clockRunning() {
		rem = g.remaining(g.turn())
	}
	return g.board.FEN(), len(g.moves), rem, true
}

// Rematch registers an offer (or a decline). It returns true when both players agreed.
func (g *Game) Rematch(userID int64, offer bool) (bool, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	c := g.colorOf(userID)
	if c < 0 || !g.ended {
		return false, errors.New("rematch is only possible after the game")
	}
	if g.rematched {
		return false, errors.New("rematch already started")
	}
	if !offer {
		g.rematchBy = -1
		g.broadcastLocked()
		return false, nil
	}
	if g.rematchBy == 1-c {
		g.rematched = true
		g.rematchBy = -1
		return true, nil
	}
	g.rematchBy = c
	g.broadcastLocked()
	return false, nil
}

func (g *Game) ResetRematch() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.rematched = false
	g.rematchBy = -1
	g.broadcastLocked()
}
