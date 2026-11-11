package main

import (
	"encoding/json"
	"math/rand/v2"
	"strconv"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// A squad is a 2v2 room: two people fight together on the left (slots 0 and 2 of the game) against two computers with characters
// chosen at random (slots 1 and 3). It is a room of its own beside the duels, so nothing about those changes: the first person opens
// it ("play=squad"), the second finds it in the room menu and joins, both choose a character, and the match starts. The computers
// are new at every match; a match that ends is shown and then played again with the same characters until someone chooses again.
type squad struct {
	id      string // "q" and the number it was opened as (never shown to players)
	n       int
	mode    string
	mu      sync.Mutex
	g       *Game
	c       [2]*client // the two people: game slots 0 and 2
	pick    [2]int     // what each chose; -1 until it does
	foes    [2]*Bot    // the computers: game slots 1 and 3
	running bool
	overAt  time.Time // zero while a match is in progress
	winner  int
	aloneAt time.Time // since when one person has been in it alone
	bonus   int       // a solo squad's extra HP for its person, in percent (0 to boostMax, a multiple of easeStep)
	solo    bool      // one person alone against the two computers ("1v2"): slot 2 stays empty (fallen from the start) and nobody else joins
}

// slotOf is the game slot of the squad's person h (0 or 1), and foeSlot that of its computer f.
func squadSlot(h int) int { return 2 * h }
func foeSlot(f int) int   { return 2*f + 1 }

func (s *squad) alone() bool { return !s.solo && (s.c[0] == nil) != (s.c[1] == nil) }
func (s *squad) playing() bool {
	if s.solo {
		return s.c[0] != nil && s.pick[0] >= 0
	}
	return s.c[0] != nil && s.c[1] != nil && s.pick[0] >= 0 && s.pick[1] >= 0
}

// reset starts the match over with the same characters; in a solo squad the empty slot is down from the start and the person
// stands in the middle.
func (s *squad) reset() {
	s.g.Reset()
	if s.solo {
		s.g.P[2].HP = 0
		s.g.P[0].Y = s.g.court.Width / 2
	}
}

// openSquad makes a squad room with c as its first person. Must be called with h.mu held; nil when there are too many rooms.
func (h *hub) openSquad(mode string, c *client, solo bool) (*squad, int, bool) {
	if len(h.rooms)+len(h.squads) >= maxRooms {
		return nil, 0, false
	}
	h.seq++
	s := &squad{id: "q" + strconv.Itoa(h.seq), n: h.seq, mode: mode, g: NewTeamGame(h.cfg, mode, 4), winner: -1, pick: [2]int{-1, -1}, aloneAt: time.Now(), solo: solo}
	s.c[0] = c
	h.squads[s.id] = s
	return s, 0, true
}

// joinSquad seats c as the second person of the squad the player picked from the menu, if someone is still alone in it.
func (h *hub) joinSquad(id string, c *client) (*squad, int, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	s := h.squads[id]
	if s == nil {
		return nil, 0, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.alone() {
		return nil, 0, false
	}
	idx := 0
	if s.c[0] != nil {
		idx = 1
	}
	s.c[idx], s.pick[idx] = c, -1
	return s, idx, true
}

// leaveSquad removes person idx. The other one stays and waits in the lobby for someone new.
func (h *hub) leaveSquad(s *squad, idx int) {
	h.mu.Lock()
	defer h.mu.Unlock()
	s.mu.Lock()
	defer s.mu.Unlock()
	if c := s.c[idx]; c != nil {
		close(c.send) // only closed under s.mu, which is also held while broadcasting
		s.c[idx] = nil
	}
	s.pick[idx] = -1
	if s.c[0] == nil && s.c[1] == nil {
		delete(h.squads, s.id)
		return
	}
	s.overAt, s.winner = time.Time{}, -1
	s.pick = [2]int{-1, -1}
	s.aloneAt = time.Now()
	s.broadcastLobby()
}

// choose records person idx's pick and starts the match once both people have one. Must be called with s.mu held.
func (s *squad) choose(idx, kind int) bool {
	if s.playing() || kind < 0 || kind >= kinds {
		return false
	}
	s.pick[idx] = kind
	s.startIfReady()
	return true
}

// setBonus records the extra HP percent of a solo squad's person, which counts from the next match. It is refused in any other squad,
// while a match is on, and unless it is a multiple of easeStep up to boostMax. Must be called with s.mu held.
func (s *squad) setBonus(pct int) bool {
	if !s.solo || s.playing() || pct < 0 || pct > boostMax || pct%easeStep != 0 {
		return false
	}
	s.bonus = pct
	return true
}

// repick sends both people back to the lobby once the result of a match is showing. Must be called with s.mu held.
func (s *squad) repick() bool {
	if s.overAt.IsZero() {
		return false
	}
	s.pick = [2]int{-1, -1}
	s.overAt, s.winner = time.Time{}, -1
	return true
}

// startIfReady draws the two computers' characters and starts the match once both people have chosen. Must be called with s.mu held.
func (s *squad) startIfReady() {
	if !s.playing() {
		return
	}
	for f := range s.foes {
		s.foes[f] = newBot(botKinds[rand.IntN(len(botKinds))], uint64(time.Now().UnixNano())+uint64(f))
	}
	s.g.P[0].Kind, s.g.P[2].Kind = s.pick[0], pikachu // the empty slot of a solo squad never fights, but it needs a kind that exists
	if !s.solo {
		s.g.P[2].Kind = s.pick[1]
	}
	s.g.P[1].Kind, s.g.P[3].Kind = s.foes[0].kind, s.foes[1].kind
	s.g.Boost[0] = s.bonus
	s.reset()
	s.overAt, s.winner = time.Time{}, -1
	if !s.running {
		s.running = true
		go s.loop()
	}
}

// broadcastLobby tells both people who is here and what each chose, and what the computers will play (once a match is on). Must be
// called with s.mu held.
func (s *squad) broadcastLobby() {
	foes := [2]int{-1, -1}
	if s.playing() {
		for f, b := range s.foes {
			if b != nil {
				foes[f] = b.kind
			}
		}
	}
	msg, _ := json.Marshal(map[string]any{"t": "lobby", "squad": true, "solo": s.solo, "bonus": s.bonus, "on": s.playing(), "picks": s.pick, "here": [2]bool{s.c[0] != nil, s.c[1] != nil}, "foes": foes})
	s.broadcast(msg)
}

// broadcast must be called with s.mu held.
func (s *squad) broadcast(msg []byte) {
	for _, c := range s.c {
		if c == nil {
			continue
		}
		select {
		case c.send <- msg:
		default:
		}
	}
}

func (s *squad) loop() {
	t := time.NewTicker(time.Second / tickRate)
	defer t.Stop()
	for range t.C {
		s.mu.Lock()
		if !s.playing() {
			s.running = false
			s.mu.Unlock()
			return
		}
		s.broadcast(s.step(time.Now()))
		s.mu.Unlock()
	}
}

// step advances the match by one tick and returns the snapshot to send. Must be called with s.mu held while a match is on.
func (s *squad) step(now time.Time) []byte {
	var events []Event
	if s.overAt.IsZero() {
		for f, b := range s.foes {
			if slot := foeSlot(f); b != nil && s.g.P[slot].HP > 0 {
				s.g.P[slot].In = b.Decide(s.g, slot)
			}
		}
		events = s.g.Step(1.0 / tickRate)
		if w := s.g.Winner(); w >= 0 {
			s.overAt, s.winner = now, w
		}
	} else if now.Sub(s.overAt) > resultHold { // the same characters play again
		s.reset()
		for _, b := range s.foes {
			if b != nil {
				b.reset()
			}
		}
		s.overAt, s.winner = time.Time{}, -1
	}
	mx := make([]int, len(s.g.P))
	for i := range mx {
		mx[i] = s.g.maxHP(i)
	}
	msg, _ := json.Marshal(struct {
		T  string       `json:"t"`
		P  []Player     `json:"p"`
		E  []Event      `json:"e,omitempty"`
		B  []Ball       `json:"b,omitempty"`
		F  []Frost      `json:"f,omitempty"`
		FZ []FireZone   `json:"fz,omitempty"`
		GH []ghostFire  `json:"gh,omitempty"`
		SB []shadowBall `json:"sb,omitempty"`
		W  int          `json:"w"`
		EZ [2]int       `json:"ez"`
		MX []int        `json:"mx"` // each slot's full HP, which the page draws the bars against
	}{"s", s.g.P, events, s.g.Balls, s.g.Frost, s.g.Fires, s.g.Ghosts, s.g.Shadows, s.winner, s.g.Ease, mx})
	return msg
}

// serveSquad is the conversation with one person of a squad, from the greeting until the page goes. c.squad says which squad and
// which person.
func (h *hub) serveSquad(c *client, conn *websocket.Conn) {
	s, idx := c.squad, c.squadIdx
	s.mu.Lock()
	c.send <- hiMessage(s.g.cfg, s.mode, s.g.court, squadSlot(idx))
	s.broadcastLobby()
	s.mu.Unlock()

	defer h.leaveSquad(s, idx)
	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			return
		}
		c.touch()
		var m struct {
			Pick   *int `json:"pick"`
			Bonus  *int `json:"bonus"`
			Repick bool `json:"repick"`
			Input
		}
		if json.Unmarshal(data, &m) != nil {
			continue
		}
		s.mu.Lock()
		if m.Pick != nil || m.Bonus != nil || m.Repick {
			switch {
			case m.Pick != nil:
				s.choose(idx, *m.Pick)
			case m.Bonus != nil:
				s.setBonus(*m.Bonus)
			default:
				s.repick()
			}
			s.broadcastLobby() // also when refused, so the page learns what is taken
			s.mu.Unlock()
			continue
		}
		in := m.Input
		in.DX, in.DY = clampDir(in.DX), clampDir(in.DY)
		in.Shade = false
		in.Kick = false
		slot := squadSlot(idx)
		if s.g.P[slot].Kind == ash {
			in.Comet = false
		}
		s.g.P[slot].In = in
		s.mu.Unlock()
	}
}
