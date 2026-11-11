package main

import (
	"errors"
	"math/rand/v2"
	"sort"
	"strings"
	"time"
)

const challengeTTL = 60 * time.Second

// Challenge is a direct game invitation from one online player to another.
type Challenge struct {
	ID      string      `json:"id"`
	From    *PlayerInfo `json:"from"`
	To      *PlayerInfo `json:"to"`
	TC      string      `json:"tc"`
	Expires int64       `json:"expires"` // unix ms
	timer   *time.Timer
}

type onlinePlayer struct {
	ID           int64  `json:"id"`
	Username     string `json:"username"`
	RatingBullet int    `json:"ratingBullet"`
	RatingBlitz  int    `json:"ratingBlitz"`
	RatingRapid  int    `json:"ratingRapid"`
	Status       string `json:"status"` // idle | seeking | playing
}

func (h *Hub) profileLocked(uid int64) *User {
	if u := h.profiles[uid]; u != nil {
		return u
	}
	for c := range h.clients[uid] {
		return c.user
	}
	return nil
}

func (h *Hub) statusLocked(uid int64) string {
	if h.active[uid] != "" {
		return "playing"
	}
	if _, ok := h.seekOf[uid]; ok {
		return "seeking"
	}
	return "idle"
}

// challenge: c invites user `to` to a game with time control tcStr.
func (h *Hub) challenge(c *Client, to int64, tcStr string) {
	tc, err := parseTC(tcStr)
	if err != nil {
		c.errMsg(err.Error())
		return
	}
	from := c.user.ID
	if to == from {
		c.errMsg("you can't challenge yourself")
		return
	}
	cat := tc.Category()

	h.mu.Lock()
	if err := func() error {
		if len(h.clients[to]) == 0 {
			return errors.New("that player is no longer online")
		}
		if h.active[from] != "" {
			return errors.New("finish your current game first")
		}
		if h.active[to] != "" {
			return errors.New("that player is in a game right now")
		}
		return nil
	}(); err != nil {
		h.mu.Unlock()
		c.errMsg(err.Error())
		return
	}
	// one outgoing challenge per player: replace the previous one
	for id, ch := range h.challenges {
		if ch.From.ID == from {
			h.dropChallengeLocked(id, "canceled")
		}
	}
	me, opp := h.profileLocked(from), h.profileLocked(to)
	ch := &Challenge{
		ID:      randomID(10),
		From:    &PlayerInfo{ID: me.ID, Username: me.Username, Rating: me.Rating(cat)},
		To:      &PlayerInfo{ID: opp.ID, Username: opp.Username, Rating: opp.Rating(cat)},
		TC:      tcStr,
		Expires: time.Now().Add(challengeTTL).UnixMilli(),
	}
	id := ch.ID
	ch.timer = time.AfterFunc(challengeTTL, func() {
		h.mu.Lock()
		h.dropChallengeLocked(id, "expired")
		h.mu.Unlock()
	})
	h.challenges[id] = ch
	h.sendUserLocked(to, mustJSON(map[string]any{"t": "challenge", "challenge": ch}))
	h.sendUserLocked(from, mustJSON(map[string]any{"t": "challengeSent", "challenge": ch}))
	h.mu.Unlock()
}

// dropChallengeLocked removes a challenge and tells both sides why ("declined", "canceled", "expired", "accepted").
func (h *Hub) dropChallengeLocked(id, reason string) {
	ch := h.challenges[id]
	if ch == nil {
		return
	}
	ch.timer.Stop()
	delete(h.challenges, id)
	msg := mustJSON(map[string]any{"t": "challengeGone", "id": id, "reason": reason, "by": ch.To.Username})
	h.sendUserLocked(ch.From.ID, msg)
	h.sendUserLocked(ch.To.ID, msg)
}

func (h *Hub) answerChallenge(c *Client, id string, accept bool) {
	h.mu.Lock()
	ch := h.challenges[id]
	if ch == nil {
		h.mu.Unlock()
		c.errMsg("this challenge is no longer available")
		return
	}
	switch c.user.ID {
	case ch.To.ID:
		if !accept {
			h.dropChallengeLocked(id, "declined")
			h.mu.Unlock()
			return
		}
	case ch.From.ID: // challenger cancels
		h.dropChallengeLocked(id, "canceled")
		h.mu.Unlock()
		return
	default:
		h.mu.Unlock()
		return
	}
	h.dropChallengeLocked(id, "accepted")
	h.mu.Unlock()

	white, black := ch.From.ID, ch.To.ID
	if rand.IntN(2) == 0 {
		white, black = black, white
	}
	if err := h.startGame(white, black, ch.TC); err != nil {
		c.errMsg(err.Error())
	}
}

// dropChallengesOfLocked removes every challenge involving uid (used on disconnect / game start).
func (h *Hub) dropChallengesOfLocked(uid int64, reason string) {
	for id, ch := range h.challenges {
		if ch.From.ID == uid || ch.To.ID == uid {
			h.dropChallengeLocked(id, reason)
		}
	}
}

// pendingChallengesLocked: challenges to send to a freshly connected client.
func (h *Hub) pendingChallengesLocked(uid int64) (incoming []*Challenge, outgoing *Challenge) {
	incoming = []*Challenge{}
	for _, ch := range h.challenges {
		if ch.To.ID == uid {
			incoming = append(incoming, ch)
		}
		if ch.From.ID == uid {
			outgoing = ch
		}
	}
	return
}

func (h *Hub) onlineListLocked() []onlinePlayer {
	list := make([]onlinePlayer, 0, len(h.clients))
	for uid := range h.clients {
		u := h.profileLocked(uid)
		if u == nil {
			continue
		}
		list = append(list, onlinePlayer{
			ID: u.ID, Username: u.Username, RatingBullet: u.RatingBullet, RatingBlitz: u.RatingBlitz,
			RatingRapid: u.RatingRapid, Status: h.statusLocked(uid),
		})
	}
	sort.Slice(list, func(i, j int) bool { return strings.ToLower(list[i].Username) < strings.ToLower(list[j].Username) })
	if len(list) > 300 {
		list = list[:300]
	}
	return list
}
