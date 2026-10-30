package main

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func dial(t *testing.T, srv *httptest.Server, room string) *websocket.Conn {
	t.Helper()
	return dialIn(t, srv, room, "")
}

// dialIn opens the socket the way the page does from a given view ("mobile", "web" or "" for none).
func dialIn(t *testing.T, srv *httptest.Server, room, mode string) *websocket.Conn {
	t.Helper()
	url := "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws?room=" + room
	if mode != "" {
		url += "&mode=" + mode
	}
	c, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { c.Close() })
	return c
}

// next reads messages until one has type want, or fails after 2s.
func next(t *testing.T, c *websocket.Conn, want string) map[string]any {
	t.Helper()
	c.SetReadDeadline(time.Now().Add(2 * time.Second))
	for {
		_, data, err := c.ReadMessage()
		if err != nil {
			t.Fatalf("waiting for %q: %v", want, err)
		}
		var m map[string]any
		json.Unmarshal(data, &m)
		if m["t"] == want {
			return m
		}
	}
}

// pick sends the character choice the page sends from its selection screen.
func pick(t *testing.T, c *websocket.Conn, kind int) {
	t.Helper()
	if err := c.WriteMessage(websocket.TextMessage, []byte(fmt.Sprintf(`{"pick":%d}`, kind))); err != nil {
		t.Fatal(err)
	}
}

// lobbyWith reads lobby messages until the picks are want (-1 = none yet).
func lobbyWith(t *testing.T, c *websocket.Conn, want [2]int) {
	t.Helper()
	for {
		m := next(t, c, "lobby")
		p := m["picks"].([]any)
		if p[0] == float64(want[0]) && p[1] == float64(want[1]) {
			return
		}
	}
}

// startMatch connects two players to a room and has them pick the given characters.
func startMatch(t *testing.T, srv *httptest.Server, room string, ka, kb int) (a, b *websocket.Conn) {
	t.Helper()
	a, b = dial(t, srv, room), dial(t, srv, room)
	pick(t, a, ka)
	pick(t, b, kb)
	next(t, a, "s")
	next(t, b, "s")
	return a, b
}

func TestTwoPlayersMatchAndThirdIsRejected(t *testing.T) {
	srv := httptest.NewServer(newMux(DefaultConfig()))
	defer srv.Close()

	a := dial(t, srv, "abc")
	hi := next(t, a, "hi")
	if hi["you"] != float64(0) {
		t.Fatalf("first player index = %v, want 0", hi["you"])
	}
	if th, ok := hi["thunder"].(map[string]any); !ok || th["radius"] != float64(60) || th["delay"] != float64(1) || th["interval"] != float64(0.5) || th["strikes"] != float64(3) {
		t.Fatalf("hi.thunder = %v, want the configured radius/delay/interval/strikes", hi["thunder"])
	}
	if arena, ok := hi["arena"].(map[string]any); !ok || arena["w"] != float64(2000) || arena["h"] != float64(1000) {
		t.Fatalf("hi.arena = %v, want 2000x1000", hi["arena"])
	}
	if rd, ok := hi["radius"].([]any); !ok || len(rd) != 4 || rd[0] != float64(16) || rd[1] != float64(24) || rd[2] != float64(26) || rd[3] != float64(18) {
		t.Fatalf("hi.radius = %v, want [16 24 26 18]: the client draws each body at its size, indexed by character", hi["radius"])
	}
	if pu, ok := hi["punch"].(map[string]any); !ok || pu["charge"] != float64(0.4) || pu["radius"] != float64(40) || pu["freeze"] != float64(1.5) {
		t.Fatalf("hi.punch = %v, want charge 0.4 / radius 40 / freeze 1.5", hi["punch"])
	}
	if cd, ok := hi["cooldown"].(map[string]any); !ok || cd["punch"] != float64(5) {
		t.Fatalf("hi.cooldown.punch = %v, want 5", hi["cooldown"])
	}
	if mh, ok := hi["maxhp"].([]any); !ok || len(mh) != 4 || mh[0] != float64(100) || mh[1] != float64(100) || mh[2] != float64(120) || mh[3] != float64(110) {
		t.Fatalf("hi.maxhp = %v, want [100 100 120 110]: the client scales the HP bars with it", hi["maxhp"])
	}
	if lf, ok := hi["leaf"].(map[string]any); !ok || lf["radius"] != float64(90) || lf["scale"] != float64(2.5) {
		t.Fatalf("hi.leaf = %v, want the Leaf Slash radius 90 and leaf scale 2.5", hi["leaf"])
	}
	if rz, ok := hi["razor"].(map[string]any); !ok || rz["radius"] != float64(10) || rz["scale"] != float64(1) || rz["hp"] != float64(5) {
		t.Fatalf("hi.razor = %v, want radius 10, scale 1, hp 5", hi["razor"])
	}
	// The client draws the beam's warning line from these, not from hard-coded numbers.
	if beam, ok := hi["beam"].(map[string]any); !ok || beam["len"] != float64(450) || beam["w"] != float64(50) || beam["charge"] != float64(0.6) || beam["angle"] != float64(30) {
		t.Fatalf("hi.beam = %v, want the configured length/width/charge", hi["beam"])
	}
	// The pad's buttons draw their countdown from these.
	if cd, ok := hi["cooldown"].(map[string]any); !ok || cd["dash"] != float64(0.6) || cd["thunder"] != float64(4) || cd["jump"] != float64(1.5) || cd["beam"] != float64(5) {
		t.Fatalf("hi.cooldown = %v, want dash 0.6 / thunder 4 / jump 1.5 / beam 5", hi["cooldown"])
	}
	if cd := hi["cooldown"].(map[string]any); cd["fly"] != float64(8) || cd["fire"] != float64(8) || cd["toss"] != float64(10) {
		t.Fatalf("hi.cooldown = %v, want Charizard's fly 8 / fire 8 / toss 10", cd)
	}
	if v, ok := hi["volt"].(map[string]any); !ok || v["uses"] != float64(3) || v["window"] != float64(2) {
		t.Fatalf("hi.volt = %v, want 3 uses in a 2s window: the button shows the uses left", hi["volt"])
	}
	if l := next(t, a, "lobby"); l["here"].([]any)[1] != false {
		t.Fatalf("lobby = %v, want the opponent absent", l)
	}

	b := dial(t, srv, "abc")
	if got := next(t, b, "hi")["you"]; got != float64(1) {
		t.Fatalf("second player index = %v, want 1", got)
	}
	if l := next(t, a, "lobby"); l["here"].([]any)[1] != true {
		t.Fatalf("lobby = %v, want the opponent here once it joined", l)
	}
	pick(t, a, charizard)
	pick(t, b, pikachu)
	next(t, a, "s") // snapshots flow once both are here and both have picked
	next(t, b, "s")

	c := dial(t, srv, "abc")
	next(t, c, "full")

	// A different room is independent.
	d := dial(t, srv, "other")
	next(t, d, "lobby")
}

// Nobody plays before both have picked, and the picks decide who is who.
func TestPickingCharactersStartsTheMatch(t *testing.T) {
	srv := httptest.NewServer(newMux(DefaultConfig()))
	defer srv.Close()
	a, b := dial(t, srv, "pk"), dial(t, srv, "pk")

	pick(t, a, snorlax)
	lobbyWith(t, b, [2]int{snorlax, -1}) // the opponent sees what was picked
	pick(t, b, 7)                        // no such character
	pick(t, b, -1)
	lobbyWith(t, b, [2]int{snorlax, -1})

	pick(t, b, charizard)
	ps := next(t, a, "s")["p"].([]any)
	if k0, k1 := ps[0].(map[string]any)["k"], ps[1].(map[string]any)["k"]; k0 != float64(snorlax) || k1 != float64(charizard) {
		t.Fatalf("kinds in the snapshot = %v, %v, want snorlax in slot 0 and charizard in slot 1", k0, k1)
	}
	if hp := ps[1].(map[string]any)["hp"]; hp != float64(120) {
		t.Fatalf("charizard's hp = %v, want its max_hp 120", hp)
	}

	// Once the match is on, picks are ignored: nobody changes character mid-fight.
	pick(t, a, pikachu)
	for i := 0; i < 5; i++ {
		ps = next(t, b, "s")["p"].([]any)
		if k := ps[0].(map[string]any)["k"]; k != float64(snorlax) {
			t.Fatalf("slot 0's kind = %v after a pick during the match, want it unchanged", k)
		}
	}
}

// Leaving sends the other player back to the character choice, keeping its pick; the next one chooses
// something else and the match starts again with them.
func TestLeavingReturnsToTheLobby(t *testing.T) {
	srv := httptest.NewServer(newMux(DefaultConfig()))
	defer srv.Close()
	a, b := startMatch(t, srv, "lv", pikachu, snorlax)

	b.Close()
	lobbyWith(t, a, [2]int{pikachu, -1})

	c := dial(t, srv, "lv")
	pick(t, c, charizard)
	ps := next(t, a, "s")["p"].([]any)
	if k := ps[1].(map[string]any)["k"]; k != float64(charizard) {
		t.Fatalf("slot 1's kind = %v, want charizard", k)
	}
}

func TestInputMovesPlayerAndLeaveSendsWait(t *testing.T) {
	srv := httptest.NewServer(newMux(DefaultConfig()))
	defer srv.Close()

	a, b := startMatch(t, srv, "mv", pikachu, snorlax)
	x0 := next(t, b, "s")["p"].([]any)[0].(map[string]any)["x"].(float64) // the real spawn point

	a.WriteMessage(websocket.TextMessage, []byte(`{"dx":1,"dy":0}`))
	x := x0
	for i := 0; i < 30 && x <= x0+10; i++ {
		x = next(t, b, "s")["p"].([]any)[0].(map[string]any)["x"].(float64)
	}
	if x <= x0+10 {
		t.Fatalf("player 0 x = %v (started at %v), want it to move right when holding right", x, x0)
	}

	a.Close()
	lobbyWith(t, b, [2]int{-1, snorlax})
}

// The room takes the court of the view it was opened from, whatever the second player opens it with.
func TestRoomCourtFollowsTheViewThatOpenedIt(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Court.Mobile = CourtSize{Width: 600, Length: 1200}
	cfg.Court.Web = CourtSize{Width: 900, Length: 1600}
	srv := httptest.NewServer(newMux(cfg))
	defer srv.Close()
	arenaOf := func(hi map[string]any) (w, h float64) {
		a := hi["arena"].(map[string]any)
		return a["w"].(float64), a["h"].(float64)
	}
	for _, tc := range []struct {
		name, first, second string
		w, h                float64
	}{
		{"phone opens", "mobile", "web", 1200, 600},
		{"web opens", "web", "mobile", 1600, 900},
		{"no mode counts as phone", "", "web", 1200, 600},
	} {
		t.Run(tc.name, func(t *testing.T) {
			room := "c" + tc.first + tc.second // short, lower-case, unique per row
			a := dialIn(t, srv, room, tc.first)
			if w, h := arenaOf(next(t, a, "hi")); w != tc.w || h != tc.h {
				t.Fatalf("first player's arena = %vx%v, want %vx%v", w, h, tc.w, tc.h)
			}
			b := dialIn(t, srv, room, tc.second)
			if w, h := arenaOf(next(t, b, "hi")); w != tc.w || h != tc.h {
				t.Fatalf("second player's arena = %vx%v, want the room's %vx%v", w, h, tc.w, tc.h)
			}
		})
	}
}

func TestBadModeRejected(t *testing.T) {
	srv := httptest.NewServer(newMux(DefaultConfig()))
	defer srv.Close()
	url := "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws?room=abc&mode=tablet"
	if _, resp, err := websocket.DefaultDialer.Dial(url, nil); err == nil || resp == nil || resp.StatusCode != 400 {
		t.Fatalf("want HTTP 400 handshake failure, got err=%v resp=%v", err, resp)
	}
}

func TestBadRoomIDRejected(t *testing.T) {
	srv := httptest.NewServer(newMux(DefaultConfig()))
	defer srv.Close()
	url := "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws?room=Bad%20Room"
	if _, resp, err := websocket.DefaultDialer.Dial(url, nil); err == nil || resp == nil || resp.StatusCode != 400 {
		t.Fatalf("want HTTP 400 handshake failure, got err=%v resp=%v", err, resp)
	}
}

// Both players may pick the same character.
func TestBothPlayersMayPickTheSameCharacter(t *testing.T) {
	srv := httptest.NewServer(newMux(DefaultConfig()))
	defer srv.Close()
	a, b := startMatch(t, srv, "same", snorlax, snorlax)
	ps := next(t, a, "s")["p"].([]any)
	if k0, k1 := ps[0].(map[string]any)["k"], ps[1].(map[string]any)["k"]; k0 != float64(snorlax) || k1 != float64(snorlax) {
		t.Fatalf("kinds = %v, %v, want snorlax twice", k0, k1)
	}
	_ = b
}
