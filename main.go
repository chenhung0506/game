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
		r = &room{g: NewGameFor(h.cfg, mode), winner: -1}
		h.rooms[id] = r
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for i := range r.c {
		if r.c[i] == nil {
			r.c[i] = c
			r.g.Reset()
			r.overAt, r.winner = time.Time{}, -1
			if r.c[0] != nil && r.c[1] != nil && !r.running {
				r.running = true
				go r.loop()
			}
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
	if r.c[0] == nil && r.c[1] == nil {
		delete(h.rooms, id)
		return
	}
	// The remaining player waits for a new opponent.
	r.g.Reset()
	r.broadcast([]byte(`{"t":"wait"}`))
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
		if r.c[0] == nil || r.c[1] == nil {
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
			W int       `json:"w"`
		}{"s", r.g.P, events, r.winner})
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
	hb, th := r.g.cfg.Snorlax.HyperBeam, r.g.cfg.Pikachu.Thunder // tunable, so the client takes them from here instead of hard-coding
	hi, _ := json.Marshal(map[string]any{
		"t": "hi", "you": idx, "arena": map[string]float64{"w": r.g.court.Length, "h": r.g.court.Width}, // the x and y extents
		"radius":  []float64{r.g.cfg.Pikachu.Radius, r.g.cfg.Snorlax.Radius},
		"maxhp":   []int{r.g.cfg.Pikachu.MaxHP, r.g.cfg.Snorlax.MaxHP},
		"beam":    map[string]float64{"len": hb.Length, "w": hb.Width, "charge": hb.ChargeDuration, "angle": hb.SweepAngle},
		"thunder": map[string]float64{"radius": th.Radius, "delay": th.Delay, "interval": th.Interval, "strikes": float64(th.Strikes)},
		// full cooldowns, so the pad's buttons can show how much of each is left
		"cooldown": map[string]float64{"dash": r.g.cfg.Pikachu.Dash.Cooldown, "thunder": th.Cooldown, "jump": r.g.cfg.Snorlax.Jump.Cooldown, "beam": hb.Cooldown},
	})
	c.send <- hi
	if r.c[1-idx] == nil {
		c.send <- []byte(`{"t":"wait"}`)
	}
	r.mu.Unlock()

	defer h.leave(id, r, idx)
	conn.SetReadLimit(256)
	conn.SetReadDeadline(time.Now().Add(pongWait))
	conn.SetPongHandler(func(string) error {
		return conn.SetReadDeadline(time.Now().Add(pongWait))
	})
	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			return
		}
		var in Input
		if json.Unmarshal(data, &in) != nil {
			continue
		}
		in.DX, in.DY = clampDir(in.DX), clampDir(in.DY)
		r.mu.Lock()
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
