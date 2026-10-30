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
	maxRooms   = 1000 // rooms live in memory and anyone can open one
	roundReset = 2 * time.Second
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
	if r.playing() || kind < 0 || kind >= kinds {
		return false
	}
	r.pick[idx] = kind
	if r.playing() {
		r.g.SetKinds(r.pick[0], r.pick[1])
		r.overAt, r.winner = time.Time{}, -1
		if !r.running {
			r.running = true
			go r.loop()
		}
	}
	return true
}

// broadcastLobby tells both players who is here and what they picked; the page shows its character
// choice until snapshots start. Must be called with r.mu held.
func (r *room) broadcastLobby() {
	msg, _ := json.Marshal(map[string]any{"t": "lobby", "picks": r.pick, "here": [2]bool{r.c[0] != nil, r.c[1] != nil}})
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
		var events []Event
		if r.overAt.IsZero() {
			events = r.g.Step(1.0 / tickRate)
			if w := r.g.Winner(); w >= 0 {
				r.overAt, r.winner = time.Now(), w
			}
		} else if time.Since(r.overAt) > roundReset {
			r.g.Reset()
			r.overAt, r.winner = time.Time{}, -1
		}
		msg, _ := json.Marshal(struct {
			T string    `json:"t"`
			P [2]Player `json:"p"`
			E []Event   `json:"e,omitempty"`
			B []Ball    `json:"b,omitempty"`
			W int       `json:"w"`
		}{"s", r.g.P, events, r.g.Balls, r.winner})
		r.broadcast(msg)
		r.mu.Unlock()
	}
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
		"razor":   map[string]float64{"radius": cfg.Chikorita.RazorLeaf.Radius, "hp": float64(cfg.Chikorita.RazorLeaf.LeafHP)},
		"vine":    map[string]float64{"len": cfg.Chikorita.VineWhip.Length, "w": cfg.Chikorita.VineWhip.Width, "angle": cfg.Chikorita.VineWhip.SweepAngle, "duration": cfg.Chikorita.VineWhip.SweepDuration},
		"punch":   map[string]float64{"charge": cfg.Snorlax.IcePunch.ChargeDuration, "radius": cfg.Snorlax.IcePunch.Radius, "freeze": cfg.Snorlax.IcePunch.FreezeDuration, "distance": cfg.Snorlax.IcePunch.Distance},
		"ball":    map[string]float64{"radius": cfg.Pikachu.ElectroBall.Radius, "hp": float64(cfg.Pikachu.ElectroBall.HP), "charge": cfg.Pikachu.ElectroBall.ChargeTime},
		"beam":    map[string]float64{"len": hb.Length, "w": hb.Width, "charge": hb.ChargeDuration, "angle": hb.SweepAngle},
		"thunder": map[string]float64{"radius": th.Radius, "delay": th.Delay, "interval": th.Interval, "strikes": float64(th.Strikes)},
		"fire":    map[string]float64{"radius": fb.Radius, "hp": float64(fb.HP)},
		"toss":    map[string]float64{"radius": ts.Radius, "high": ts.High},
		"fly":     map[string]float64{"height": cfg.Charizard.Fly.Height},
		// full cooldowns, so the pad's buttons can show how much of each is left
		"cooldown": map[string]float64{"dash": cfg.Pikachu.Dash.Cooldown, "thunder": th.Cooldown, "jump": cfg.Snorlax.Jump.Cooldown, "beam": hb.Cooldown, "ball": cfg.Pikachu.ElectroBall.Cooldown, "punch": cfg.Snorlax.IcePunch.Cooldown,
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
		// The one other message from the page is the character choice {"pick":2}; the rest is held keys.
		var m struct {
			Pick *int `json:"pick"`
			Input
		}
		if json.Unmarshal(data, &m) != nil {
			continue
		}
		r.mu.Lock()
		if m.Pick != nil {
			r.choose(idx, *m.Pick)
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
