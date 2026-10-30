package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"regexp"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const (
	tickRate   = 30
	maxRooms   = 1000            // rooms live in memory and anyone can open one
	roundReset = 2 * time.Second // between two bouts of a team match
	resultHold = 6 * time.Second // how long the result stays up before the next match starts by itself; the repick button works meanwhile
	pongWait   = 60 * time.Second
	pingEvery  = 20 * time.Second
)

var roomIDRe = regexp.MustCompile(`^[a-z0-9]{1,16}$`)

// CheckOrigin is left nil: gorilla then only accepts same-origin browsers.
var upgrader = websocket.Upgrader{ReadBufferSize: 1024, WriteBufferSize: 1024}

type client struct {
	conn *websocket.Conn
	send chan []byte
}

type room struct {
	mu      sync.Mutex
	g       *Game
	c       [2]*client
	pick    [2]int // the character each slot chose; -1 until it does
	running bool
	overAt  time.Time // zero while a round is in progress
	winner  int

	// Team match: both players send the order they field all `kinds` characters in; when one is knocked out the
	// next in order steps in, and the match is over when a side has run out. A survivor keeps its HP.
	team        bool
	order       [2][kinds]int
	ease        [2]int // each slot's handicap in percent, chosen in the lobby
	cur         [2]int // index into order of the character each side is fielding
	matchOver   bool
	matchWinner int
}

type hub struct {
	mu    sync.Mutex
	rooms map[string]*room
	cfg   Config
}

func newHub(cfg Config) *hub { return &hub{rooms: map[string]*room{}, cfg: cfg} }

// join claims a slot in the room. Lock order is always hub.mu -> room.mu.
func (h *hub) join(id, mode string, c *client) (r *room, idx int, ok bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	r = h.rooms[id]
	if r == nil {
		if len(h.rooms) >= maxRooms {
			return nil, 0, false
		}
		r = &room{g: NewGameFor(h.cfg, mode), winner: -1, pick: [2]int{-1, -1}}
		h.rooms[id] = r
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for i := range r.c {
		if r.c[i] == nil {
			r.c[i] = c
			r.pick[i] = -1
			r.ease[i] = 0
			return r, i, true
		}
	}
	return nil, 0, false
}

func (h *hub) leave(id string, r *room, idx int) {
	h.mu.Lock()
	defer h.mu.Unlock()
	r.mu.Lock()
	defer r.mu.Unlock()
	if c := r.c[idx]; c != nil {
		close(c.send) // only closed under r.mu, which is also held while broadcasting
		r.c[idx] = nil
	}
	r.pick[idx] = -1
	r.matchOver = false
	if r.c[0] == nil && r.c[1] == nil {
		delete(h.rooms, id)
		return
	}
	// The remaining player keeps its pick and waits in the lobby for a new opponent, who picks something else.
	r.overAt, r.winner = time.Time{}, -1
	r.broadcastLobby()
}

// playing is whether a match is on: both players are here and both have picked. Must be called with r.mu held.
func (r *room) playing() bool {
	return r.c[0] != nil && r.c[1] != nil && r.pick[0] >= 0 && r.pick[1] >= 0
}

// choose records slot idx's pick and starts the match once both players have one. It reports whether the
// pick was taken: an unknown character is refused, and so is any pick while a match is on. Both players may play the same one. Must be called with r.mu held.
func (r *room) choose(idx, kind int) bool {
	if r.playing() || r.team || kind < 0 || kind >= kinds {
		return false
	}
	r.pick[idx] = kind
	r.startIfReady()
	return true
}

// setOrder records the order slot idx fields its characters in (a team match); the first one is its pick. It is
// refused unless the room is in team mode, no match is on, and the order has every character exactly once.
// Must be called with r.mu held.
func (r *room) setOrder(idx int, order []int) bool {
	if r.playing() || !r.team || len(order) != kinds {
		return false
	}
	var seen [kinds]bool
	for _, k := range order {
		if k < 0 || k >= kinds || seen[k] {
			return false
		}
		seen[k] = true
	}
	copy(r.order[idx][:], order)
	r.pick[idx] = order[0]
	r.startIfReady()
	return true
}

// setTeam switches the room between single matches and team matches. Everyone has to choose again. It is refused
// while a match is on. Must be called with r.mu held.
func (r *room) setTeam(team bool) bool {
	if r.playing() {
		return false
	}
	r.team, r.pick = team, [2]int{-1, -1}
	return true
}

// setEase records slot idx's handicap, which counts from the next match. It is refused unless it is a multiple of
// easeStep up to easeMax, and while a match is on. Must be called with r.mu held.
func (r *room) setEase(idx, pct int) bool {
	if r.playing() || !validEase(pct) {
		return false
	}
	r.ease[idx] = pct
	return true
}

// repick sends both players back to the lobby, to choose again. It is only allowed once the result of a match is
// showing. Must be called with r.mu held.
func (r *room) repick() bool {
	if r.overAt.IsZero() || !r.matchEnded() {
		return false
	}
	r.pick = [2]int{-1, -1}
	r.overAt, r.winner, r.matchOver = time.Time{}, -1, false
	return true
}

// matchEnded is whether the bout that just finished ends the match: always for a single match.
func (r *room) matchEnded() bool { return !r.team || r.matchOver }

// startIfReady starts the match once both players have chosen. Must be called with r.mu held.
func (r *room) startIfReady() {
	if !r.playing() {
		return
	}
	r.cur, r.matchOver = [2]int{}, false
	r.g.Ease = r.ease
	if r.team {
		r.g.SetKinds(r.order[0][0], r.order[1][0])
	} else {
		r.g.SetKinds(r.pick[0], r.pick[1])
	}
	r.overAt, r.winner = time.Time{}, -1
	if !r.running {
		r.running = true
		go r.loop()
	}
}

// boutOver is called when a bout ends: whoever was knocked out is replaced by the next character in its order,
// and the match is over when a side has none left. Must be called with r.mu held.
func (r *room) boutOver() {
	for i := range r.g.P {
		if r.g.P[i].HP <= 0 {
			r.cur[i]++
		}
	}
	if r.cur[0] >= kinds || r.cur[1] >= kinds {
		r.matchOver = true
		switch {
		case r.cur[0] >= kinds && r.cur[1] >= kinds:
			r.matchWinner = r.winner // the last two knocked out together: slot 0, like a single match
		case r.cur[0] >= kinds:
			r.matchWinner = 1
		default:
			r.matchWinner = 0
		}
	}
}

// nextRound starts the next bout, or the next match once the result has been up long enough. In a team match
// a character that won keeps the HP it has left. Must be called with r.mu held.
func (r *room) nextRound() {
	switch {
	case !r.team:
		r.g.Reset()
	case r.matchOver:
		r.cur, r.matchOver = [2]int{}, false
		r.g.SetKinds(r.order[0][0], r.order[1][0])
	default:
		hp := [2]int{r.g.P[0].HP, r.g.P[1].HP}
		r.g.SetKinds(r.order[0][r.cur[0]], r.order[1][r.cur[1]])
		for i, h := range hp {
			if h > 0 {
				r.g.P[i].HP = h
			}
		}
	}
	r.overAt, r.winner = time.Time{}, -1
}

// teamInfo is what a snapshot says about a team match. Who is next is each side's secret, so only the characters
// that have fallen are named.
type teamInfo struct {
	Cur    [2]int   `json:"c"`
	Fallen [2][]int `json:"f"`
	Over   bool     `json:"o"`
	Winner int      `json:"mw"`
}

// broadcastLobby tells both players who is here and what they picked; the page shows its character
// choice until snapshots start. Must be called with r.mu held.
func (r *room) broadcastLobby() {
	msg, _ := json.Marshal(map[string]any{"t": "lobby", "picks": r.pick, "here": [2]bool{r.c[0] != nil, r.c[1] != nil}, "team": r.team, "ease": r.ease})
	r.broadcast(msg)
}

// broadcast must be called with r.mu held. A client whose buffer is full
// simply misses this snapshot; the next one supersedes it.
func (r *room) broadcast(msg []byte) {
	for _, c := range r.c {
		if c == nil {
			continue
		}
		select {
		case c.send <- msg:
		default:
		}
	}
}

func (r *room) loop() {
	t := time.NewTicker(time.Second / tickRate)
	defer t.Stop()
	for range t.C {
		r.mu.Lock()
		if !r.playing() {
			r.running = false
			r.mu.Unlock()
			return
		}
		r.broadcast(r.step(time.Now()))
		r.mu.Unlock()
	}
}

// step advances the match by one tick at the given time and returns the snapshot to send. Must be called with
// r.mu held while a match is on.
func (r *room) step(now time.Time) []byte {
	var events []Event
	if r.overAt.IsZero() {
		events = r.g.Step(1.0 / tickRate)
		if w := r.g.Winner(); w >= 0 {
			r.overAt, r.winner = now, w
			if r.team {
				r.boutOver()
			}
		}
	} else {
		hold := roundReset
		if r.matchEnded() {
			hold = resultHold
		}
		if now.Sub(r.overAt) > hold {
			r.nextRound()
		}
	}
	var tm *teamInfo
	if r.team {
		tm = &teamInfo{Cur: r.cur, Over: r.matchOver, Winner: r.matchWinner}
		for i := range tm.Fallen {
			tm.Fallen[i] = append([]int{}, r.order[i][:min(r.cur[i], kinds)]...)
		}
	}
	msg, _ := json.Marshal(struct {
		T  string    `json:"t"`
		P  [2]Player `json:"p"`
		E  []Event   `json:"e,omitempty"`
		B  []Ball    `json:"b,omitempty"`
		W  int       `json:"w"`
		TM *teamInfo `json:"tm,omitempty"`
		EZ [2]int    `json:"ez"`
	}{"s", r.g.P, events, r.g.Balls, r.winner, tm, r.g.Ease})
	return msg
}

func (h *hub) serveWS(w http.ResponseWriter, req *http.Request) {
	id := req.URL.Query().Get("room")
	if !roomIDRe.MatchString(id) {
		http.Error(w, "bad room id", http.StatusBadRequest)
		return
	}
	mode := req.URL.Query().Get("mode") // the view the player opened the page in; it only matters to whoever opens the room
	if mode == "" {
		mode = modeMobile
	}
	if mode != modeMobile && mode != modeWeb {
		http.Error(w, "bad mode", http.StatusBadRequest)
		return
	}
	conn, err := upgrader.Upgrade(w, req, nil)
	if err != nil {
		return
	}
	c := &client{conn: conn, send: make(chan []byte, 8)}
	r, idx, ok := h.join(id, mode, c)
	if !ok {
		conn.WriteMessage(websocket.TextMessage, []byte(`{"t":"full"}`))
		conn.Close()
		return
	}
	go c.writePump()

	r.mu.Lock()
	cfg := r.g.cfg
	hb, th, fb, ts := cfg.Snorlax.HyperBeam, cfg.Pikachu.Thunder, cfg.Charizard.FireBreath, cfg.Charizard.SeismicToss // tunable, so the client takes them from here instead of hard-coding
	hi, _ := json.Marshal(map[string]any{
		"t": "hi", "you": idx, "arena": map[string]float64{"w": r.g.court.Length, "h": r.g.court.Width}, // the x and y extents
		// per character: pikachu, snorlax, charizard, chikorita
		"radius":  []float64{cfg.Pikachu.Radius, cfg.Snorlax.Radius, cfg.Charizard.Radius, cfg.Chikorita.Radius},
		"maxhp":   []int{cfg.Pikachu.MaxHP, cfg.Snorlax.MaxHP, cfg.Charizard.MaxHP, cfg.Chikorita.MaxHP},
		"leaf":    map[string]float64{"radius": cfg.Chikorita.LeafSlash.SlashRadius, "scale": cfg.Chikorita.LeafSlash.LeafScale},
		"razor":   map[string]float64{"radius": cfg.Chikorita.RazorLeaf.Radius, "scale": cfg.Chikorita.RazorLeaf.LeafScale, "hp": float64(cfg.Chikorita.RazorLeaf.LeafHP)},
		"vine":    map[string]float64{"len": cfg.Chikorita.VineWhip.Length, "w": cfg.Chikorita.VineWhip.Width, "angle": cfg.Chikorita.VineWhip.SweepAngle, "duration": cfg.Chikorita.VineWhip.SweepDuration, "charge": cfg.Chikorita.VineWhip.ChargeDuration, "recharge": cfg.Chikorita.VineWhip.RechargeDuration},
		"punch":   map[string]float64{"charge": cfg.Snorlax.IcePunch.ChargeDuration, "radius": cfg.Snorlax.IcePunch.Radius, "freeze": cfg.Snorlax.IcePunch.FreezeDuration, "distance": cfg.Snorlax.IcePunch.Distance},
		"ball":    map[string]float64{"radius": cfg.Pikachu.ElectroBall.Radius, "hp": float64(cfg.Pikachu.ElectroBall.HP), "charge": cfg.Pikachu.ElectroBall.ChargeTime},
		"beam":    map[string]float64{"len": hb.Length, "w": hb.Width, "charge": hb.ChargeDuration, "angle": hb.SweepAngle},
		"thunder": map[string]float64{"radius": th.Radius, "delay": th.Delay, "interval": th.Interval, "strikes": float64(th.Strikes)},
		"fire":    map[string]float64{"radius": fb.Radius, "hp": float64(fb.HP)},
		"toss":    map[string]float64{"radius": ts.Radius, "high": ts.High},
		"fly":     map[string]float64{"height": cfg.Charizard.Fly.Height},
		"volt":    map[string]float64{"radius": cfg.Pikachu.VoltTackle.Radius, "uses": float64(cfg.Pikachu.VoltTackle.MaxUses), "window": cfg.Pikachu.VoltTackle.UseWindow},
		// full cooldowns, so the pad's buttons can show how much of each is left
		"cooldown": map[string]float64{"dash": cfg.Pikachu.VoltTackle.Cooldown, "thunder": th.Cooldown, "jump": cfg.Snorlax.Jump.Cooldown, "beam": hb.Cooldown, "ball": cfg.Pikachu.ElectroBall.Cooldown, "punch": cfg.Snorlax.IcePunch.Cooldown,
			"fly": cfg.Charizard.Fly.Cooldown, "fire": fb.Cooldown, "toss": ts.Cooldown, "leaf": cfg.Chikorita.LeafSlash.Cooldown, "razor": cfg.Chikorita.RazorLeaf.Cooldown, "vine": cfg.Chikorita.VineWhip.Cooldown},
	})
	c.send <- hi
	r.broadcastLobby() // both players learn who joined
	r.mu.Unlock()

	defer h.leave(id, r, idx)
	conn.SetReadLimit(512)
	conn.SetReadDeadline(time.Now().Add(pongWait))
	conn.SetPongHandler(func(string) error {
		return conn.SetReadDeadline(time.Now().Add(pongWait))
	})
	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			return
		}
		// The other messages from the page are the lobby's: the character choice {"pick":2}, the match type
		// {"team":true}, the team's order {"order":[2,0,3,1]} and {"repick":true} after a result, the handicap {"ease":30}; the rest is held keys.
		var m struct {
			Pick   *int  `json:"pick"`
			Team   *bool `json:"team"`
			Order  []int `json:"order"`
			Repick bool  `json:"repick"`
			Ease   *int  `json:"ease"`
			Input
		}
		if json.Unmarshal(data, &m) != nil {
			continue
		}
		r.mu.Lock()
		if m.Pick != nil || m.Team != nil || m.Order != nil || m.Repick || m.Ease != nil {
			switch {
			case m.Pick != nil:
				r.choose(idx, *m.Pick)
			case m.Team != nil:
				r.setTeam(*m.Team)
			case m.Order != nil:
				r.setOrder(idx, m.Order)
			case m.Ease != nil:
				r.setEase(idx, *m.Ease)
			default:
				r.repick()
			}
			r.broadcastLobby() // also when refused, so the page learns what is taken
			r.mu.Unlock()
			continue
		}
		in := m.Input
		in.DX, in.DY = clampDir(in.DX), clampDir(in.DY)
		r.g.P[idx].In = in
		r.mu.Unlock()
	}
}

func clampDir(v int) int { return min(max(v, -1), 1) }

// writePump is the only writer on the connection (gorilla requires that).
func (c *client) writePump() {
	ping := time.NewTicker(pingEvery)
	defer func() { ping.Stop(); c.conn.Close() }()
	for {
		select {
		case msg, ok := <-c.send:
			if !ok {
				return
			}
			c.conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
			if c.conn.WriteMessage(websocket.TextMessage, msg) != nil {
				return
			}
		case <-ping.C:
			c.conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
			if c.conn.WriteMessage(websocket.PingMessage, nil) != nil {
				return
			}
		}
	}
}

func newMux(cfg Config) *http.ServeMux {
	h := newHub(cfg)
	mux := http.NewServeMux()
	mux.HandleFunc("/ws", h.serveWS)
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) { w.Write([]byte("ok")) })
	mux.Handle("/", http.FileServer(http.Dir("static")))
	return mux
}

func main() {
	cfgPath := envOr("CONFIG", "config.json")
	cfg, err := LoadConfig(cfgPath)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}
	log.Printf("config %s: %+v", cfgPath, cfg)

	addr := ":" + envOr("PORT", "8080")
	log.Printf("game listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, newMux(cfg)))
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
