package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math/rand/v2"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const (
	writeWait  = 10 * time.Second
	pongWait   = 60 * time.Second
	pingPeriod = 25 * time.Second
)

type Client struct {
	hub      *Hub
	conn     *websocket.Conn
	user     *User
	send     chan []byte
	watching string

	mu      sync.Mutex // guards closed
	closed  bool
	hintKey string // game+ply of the last hint request
	hintN   int    // how many times that position was requested
}

type Hub struct {
	app      *App
	mu       sync.Mutex
	clients  map[int64]map[*Client]bool
	seeks    map[string][]int64 // tc -> queue of user IDs
	seekOf   map[int64]string   // user -> tc
	games    map[string]*Game   // live + recently finished games
	active   map[int64]string   // user -> active game id
	watchers map[string]map[*Client]bool
	// online players + direct challenges
	profiles   map[int64]*User // freshest known profile (ratings) per online user
	challenges map[string]*Challenge
	upgrader   websocket.Upgrader
}

func newHub(app *App) *Hub {
	h := &Hub{
		app: app, clients: map[int64]map[*Client]bool{}, seeks: map[string][]int64{}, seekOf: map[int64]string{},
		games: map[string]*Game{}, active: map[int64]string{}, watchers: map[string]map[*Client]bool{},
		profiles: map[int64]*User{}, challenges: map[string]*Challenge{},
	}
	h.upgrader = websocket.Upgrader{
		ReadBufferSize: 1024, WriteBufferSize: 4096,
		CheckOrigin: func(r *http.Request) bool {
			origin := r.Header.Get("Origin")
			if origin == "" {
				return true
			}
			u, err := url.Parse(origin)
			if err != nil {
				return false
			}
			if strings.EqualFold(u.Host, r.Host) {
				return true
			}
			for _, o := range app.cfg.AllowedOrigins {
				if strings.EqualFold(strings.TrimRight(o, "/"), origin) {
					return true
				}
			}
			return false
		},
	}
	go func() { // periodic online count
		for range time.Tick(10 * time.Second) {
			h.broadcastOnline()
		}
	}()
	return h
}

func mustJSON(v any) []byte {
	b, _ := json.Marshal(v)
	return b
}

func (h *Hub) serveWS(w http.ResponseWriter, r *http.Request) {
	u := h.app.currentUser(r)
	if u == nil {
		http.Error(w, "unauthorized", 401)
		return
	}
	conn, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	c := &Client{hub: h, conn: conn, user: u, send: make(chan []byte, 64)}
	h.mu.Lock()
	if h.clients[u.ID] == nil {
		h.clients[u.ID] = map[*Client]bool{}
	}
	h.clients[u.ID][c] = true
	h.profiles[u.ID] = u.Public()
	activeID := h.active[u.ID]
	seeking := h.seekOf[u.ID]
	incoming, outgoing := h.pendingChallengesLocked(u.ID)
	h.mu.Unlock()

	c.queue(mustJSON(map[string]any{"t": "hello", "user": h.app.withAdmin(u), "activeGame": activeID, "seeking": seeking}))
	c.queue(mustJSON(map[string]any{"t": "challenges", "incoming": incoming, "outgoing": outgoing}))
	go c.writePump()
	go h.broadcastOnline()
	c.readPump()
}

func (c *Client) queue(b []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return
	}
	select {
	case c.send <- b:
	default: // slow client: drop connection, it will reconnect and resync
		c.conn.Close()
	}
}

func (c *Client) writePump() {
	t := time.NewTicker(pingPeriod)
	defer func() { t.Stop(); c.conn.Close() }()
	for {
		select {
		case msg, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				c.conn.WriteMessage(websocket.CloseMessage, nil)
				return
			}
			if err := c.conn.WriteMessage(websocket.TextMessage, msg); err != nil {
				return
			}
		case <-t.C:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

type inMsg struct {
	T      string `json:"t"`
	TC     string `json:"tc"`
	Game   string `json:"game"`
	Move   string `json:"move"`
	Accept bool   `json:"accept"`
	To     int64  `json:"to"`
	ID     string `json:"id"`
}

func (c *Client) readPump() {
	defer c.hub.disconnect(c)
	c.conn.SetReadLimit(4096)
	c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error { c.conn.SetReadDeadline(time.Now().Add(pongWait)); return nil })
	for {
		_, data, err := c.conn.ReadMessage()
		if err != nil {
			return
		}
		var m inMsg
		if json.Unmarshal(data, &m) != nil {
			continue
		}
		c.hub.handle(c, m)
	}
}

func (c *Client) errMsg(msg string) {
	c.queue(mustJSON(map[string]string{"t": "error", "msg": msg}))
}

func (h *Hub) disconnect(c *Client) {
	h.mu.Lock()
	delete(h.clients[c.user.ID], c)
	if len(h.clients[c.user.ID]) == 0 {
		delete(h.clients, c.user.ID)
		delete(h.profiles, c.user.ID)
		h.removeSeekLocked(c.user.ID)
		h.dropChallengesOfLocked(c.user.ID, "offline")
	}
	if c.watching != "" {
		delete(h.watchers[c.watching], c)
		if len(h.watchers[c.watching]) == 0 {
			delete(h.watchers, c.watching)
		}
	}
	h.mu.Unlock()
	c.mu.Lock()
	c.closed = true
	close(c.send)
	c.mu.Unlock()
	go h.broadcastOnline()
}

func (h *Hub) handle(c *Client, m inMsg) {
	uid := c.user.ID
	switch m.T {
	case "seek":
		h.seek(c, m.TC)
	case "cancelSeek":
		h.mu.Lock()
		h.removeSeekLocked(uid)
		h.mu.Unlock()
		h.sendUser(uid, mustJSON(map[string]string{"t": "seekCanceled"}))
	case "watch":
		h.watch(c, m.Game)
	case "move", "resign", "abort", "draw", "drawAnswer":
		g := h.game(m.Game)
		if g == nil {
			c.errMsg("game not found")
			return
		}
		var err error
		switch m.T {
		case "move":
			err = g.Move(uid, m.Move)
		case "resign":
			err = g.Resign(uid)
		case "abort":
			err = g.Abort(uid)
		case "draw":
			err = g.OfferDraw(uid)
		case "drawAnswer":
			err = g.AnswerDraw(uid, m.Accept)
		}
		if err != nil {
			c.errMsg(err.Error())
			// resync the client so it can undo an optimistic move
			c.queue(mustJSON(map[string]any{"t": "game", "game": g.State()}))
		}
	case "hint":
		h.hint(c, m.Game)
	case "hintStop":
		if h.app.isAdmin(c.user) {
			h.app.engine.Interrupt()
		}
	case "rematch", "rematchDecline":
		h.rematch(c, m.Game, m.T == "rematch")
	case "challenge":
		h.challenge(c, m.To, m.TC)
	case "challengeAnswer":
		h.answerChallenge(c, m.ID, m.Accept)
	case "ping":
		c.queue([]byte(`{"t":"pong"}`))
	}
}

func (h *Hub) game(id string) *Game {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.games[id]
}

func (h *Hub) watch(c *Client, id string) {
	h.mu.Lock()
	if c.watching != "" {
		delete(h.watchers[c.watching], c)
	}
	c.watching = id
	if h.watchers[id] == nil {
		h.watchers[id] = map[*Client]bool{}
	}
	h.watchers[id][c] = true
	g := h.games[id]
	h.mu.Unlock()

	if g != nil {
		c.queue(mustJSON(map[string]any{"t": "game", "game": g.State()}))
		return
	}
	st, err := h.app.db.GameByID(id)
	if err != nil {
		c.errMsg("game not found")
		return
	}
	c.queue(mustJSON(map[string]any{"t": "game", "game": st}))
}

func (h *Hub) removeSeekLocked(uid int64) {
	tc, ok := h.seekOf[uid]
	if !ok {
		return
	}
	delete(h.seekOf, uid)
	q := h.seeks[tc]
	for i, id := range q {
		if id == uid {
			h.seeks[tc] = append(q[:i:i], q[i+1:]...)
			break
		}
	}
}

func (h *Hub) seek(c *Client, tcStr string) {
	_, err := parseTC(tcStr)
	if err != nil {
		c.errMsg(err.Error())
		return
	}
	uid := c.user.ID
	h.mu.Lock()
	if gid := h.active[uid]; gid != "" {
		h.mu.Unlock()
		c.queue(mustJSON(map[string]any{"t": "redirect", "game": gid}))
		return
	}
	h.removeSeekLocked(uid)
	var oppID int64
	if q := h.seeks[tcStr]; len(q) > 0 {
		oppID = q[0]
		h.seeks[tcStr] = q[1:]
		delete(h.seekOf, oppID)
	} else {
		h.seeks[tcStr] = append(h.seeks[tcStr], uid)
		h.seekOf[uid] = tcStr
	}
	h.mu.Unlock()

	if oppID == 0 {
		h.sendUser(uid, mustJSON(map[string]string{"t": "seeking", "tc": tcStr}))
		return
	}

	white, black := uid, oppID
	if rand.IntN(2) == 0 {
		white, black = black, white
	}
	if err := h.startGame(white, black, tcStr); err != nil {
		c.errMsg(err.Error())
	}
}

// startGame creates a game between two users (fresh ratings are read from the DB).
func (h *Hub) startGame(whiteID, blackID int64, tcStr string) error {
	tc, err := parseTC(tcStr)
	if err != nil {
		return err
	}
	wu, err1 := h.app.db.UserByID(whiteID)
	bu, err2 := h.app.db.UserByID(blackID)
	if err1 != nil || err2 != nil {
		return errors.New("could not start game")
	}
	cat := tc.Category()
	a := &PlayerInfo{ID: wu.ID, Username: wu.Username, Rating: wu.Rating(cat)}
	b := &PlayerInfo{ID: bu.ID, Username: bu.Username, Rating: bu.Rating(cat)}
	id := randomID(8)

	h.mu.Lock()
	if h.active[a.ID] != "" || h.active[b.ID] != "" {
		h.mu.Unlock()
		return errors.New("a player is already in another game")
	}
	h.removeSeekLocked(a.ID)
	h.removeSeekLocked(b.ID)
	h.dropChallengesOfLocked(a.ID, "busy")
	h.dropChallengesOfLocked(b.ID, "busy")
	g := newGame(h, id, a, b, tcStr, tc)
	h.games[id] = g
	h.active[a.ID] = id
	h.active[b.ID] = id
	h.mu.Unlock()

	log.Printf("game %s started: %s vs %s (%s)", id, a.Username, b.Username, tcStr)
	msg := mustJSON(map[string]any{"t": "start", "game": g.State()})
	h.sendUser(a.ID, msg)
	h.sendUser(b.ID, msg)
	go h.broadcastOnline()
	return nil
}

// rematch: offer (or accept, if the opponent already offered) a rematch with colors swapped.
func (h *Hub) rematch(c *Client, gameID string, offer bool) {
	g := h.game(gameID)
	if g == nil {
		c.errMsg("game expired, start a new one")
		return
	}
	agreed, err := g.Rematch(c.user.ID, offer)
	if err != nil {
		c.errMsg(err.Error())
		return
	}
	if agreed {
		if err := h.startGame(g.black.ID, g.white.ID, g.tcStr); err != nil {
			c.errMsg(err.Error())
			g.ResetRematch()
		}
	}
}

// hint runs Stockfish on the current position of a game (admin only).
// Asking again for the same position searches longer each time.
func (h *Hub) hint(c *Client, gameID string) {
	if !h.app.isAdmin(c.user) {
		c.errMsg("unknown command")
		return
	}
	g := h.game(gameID)
	if g == nil {
		c.errMsg("game not found")
		return
	}
	fen, ply, rem, ok := g.FENFor(c.user.ID)
	if !ok {
		c.errMsg("you are not playing in this game")
		return
	}
	key := fmt.Sprintf("%s:%d", gameID, ply)
	c.mu.Lock()
	if c.hintKey == key {
		c.hintN++
	} else {
		c.hintKey, c.hintN = key, 1
	}
	n := c.hintN
	c.mu.Unlock()

	// 1st click: quick, then deeper on every re-click
	times := []time.Duration{1200 * time.Millisecond, 3 * time.Second, 6 * time.Second, 10 * time.Second}
	mt := times[min(n, len(times))-1]
	if rem > 0 { // don't burn the clock: first search ~1/25 of remaining time, re-searches ~1/6
		limit := rem / 25
		if n > 1 {
			limit = rem / 6
		}
		mt = min(mt, max(limit, 300*time.Millisecond))
	}
	send := func(status string, inf EngineInfo) {
		c.queue(mustJSON(map[string]any{"t": "hint", "game": gameID, "ply": ply, "status": status, "attempt": n, "ms": mt.Milliseconds(), "info": inf}))
	}
	send("thinking", EngineInfo{})
	go func() {
		res, err := h.app.engine.Analyze(fen, mt, false, func(inf EngineInfo) { send("thinking", inf) })
		if err != nil {
			log.Printf("engine: %v", err)
			c.errMsg("engine error: " + err.Error())
			send("error", res)
			return
		}
		send("done", res)
	}()
}

const idChars = "abcdefghijkmnopqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ23456789"

func randomID(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = idChars[rand.IntN(len(idChars))]
	}
	return string(b)
}

// gameEnded is called with the game lock held; must only touch hub state.
func (h *Hub) gameEnded(g *Game) {
	h.mu.Lock()
	if h.active[g.white.ID] == g.id {
		delete(h.active, g.white.ID)
	}
	if h.active[g.black.ID] == g.id {
		delete(h.active, g.black.ID)
	}
	h.mu.Unlock()
	go h.broadcastOnline()
	// keep finished game in memory for a bit; afterwards it is served from the DB
	time.AfterFunc(15*time.Minute, func() {
		h.mu.Lock()
		delete(h.games, g.id)
		h.mu.Unlock()
	})
}

func (h *Hub) broadcastGame(id string, white, black int64, st *GameState) {
	msg := mustJSON(map[string]any{"t": "game", "game": st})
	h.mu.Lock()
	defer h.mu.Unlock()
	sent := map[*Client]bool{}
	for _, uid := range []int64{white, black} {
		for c := range h.clients[uid] {
			c.queue(msg)
			sent[c] = true
		}
	}
	for c := range h.watchers[id] {
		if !sent[c] {
			c.queue(msg)
		}
	}
}

func (h *Hub) sendUser(uid int64, msg []byte) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.sendUserLocked(uid, msg)
}

func (h *Hub) sendUserLocked(uid int64, msg []byte) {
	for c := range h.clients[uid] {
		c.queue(msg)
	}
}

func (h *Hub) notifyUserUpdated(ids ...int64) {
	for _, id := range ids {
		if u, err := h.app.db.UserByID(id); err == nil {
			h.mu.Lock()
			if len(h.clients[id]) > 0 {
				h.profiles[id] = u.Public()
			}
			h.mu.Unlock()
			h.sendUser(id, mustJSON(map[string]any{"t": "user", "user": h.app.withAdmin(u)}))
		}
	}
	h.broadcastOnline()
}

func (h *Hub) broadcastOnline() {
	h.mu.Lock()
	defer h.mu.Unlock()
	msg := mustJSON(map[string]any{"t": "online", "players": len(h.clients), "games": len(h.active) / 2, "list": h.onlineListLocked()})
	for _, cs := range h.clients {
		for c := range cs {
			c.queue(msg)
		}
	}
}

// ---------- REST: games ----------

func (a *App) handleGetGame(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if g := a.hub.game(id); g != nil {
		writeJSON(w, 200, map[string]any{"game": g.State()})
		return
	}
	st, err := a.db.GameByID(id)
	if err != nil {
		writeErr(w, 404, "game not found")
		return
	}
	writeJSON(w, 200, map[string]any{"game": st})
}

func (a *App) handleMyGames(w http.ResponseWriter, r *http.Request) {
	u := a.currentUser(r)
	if u == nil {
		writeErr(w, 401, "not logged in")
		return
	}
	games, err := a.db.RecentGames(u.ID, 20)
	if err != nil {
		writeErr(w, 500, "internal error")
		return
	}
	writeJSON(w, 200, map[string]any{"games": games})
}
