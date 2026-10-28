package main

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func dial(t *testing.T, srv *httptest.Server, room string) *websocket.Conn {
	t.Helper()
	url := "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws?room=" + room
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
	// The client draws the beam's warning line from these, not from hard-coded numbers.
	if beam, ok := hi["beam"].(map[string]any); !ok || beam["len"] != float64(450) || beam["w"] != float64(50) || beam["charge"] != float64(0.6) || beam["angle"] != float64(30) {
		t.Fatalf("hi.beam = %v, want the configured length/width/charge", hi["beam"])
	}
	// The pad's buttons draw their countdown from these.
	if cd, ok := hi["cooldown"].(map[string]any); !ok || cd["dash"] != float64(0.6) || cd["thunder"] != float64(4) || cd["jump"] != float64(1.5) || cd["beam"] != float64(5) {
		t.Fatalf("hi.cooldown = %v, want dash 0.6 / thunder 4 / jump 1.5 / beam 5", hi["cooldown"])
	}
	next(t, a, "wait")

	b := dial(t, srv, "abc")
	if got := next(t, b, "hi")["you"]; got != float64(1) {
		t.Fatalf("second player index = %v, want 1", got)
	}
	next(t, a, "s") // snapshots flow once both are present
	next(t, b, "s")

	c := dial(t, srv, "abc")
	next(t, c, "full")

	// A different room is independent.
	d := dial(t, srv, "other")
	next(t, d, "wait")
}

func TestInputMovesPlayerAndLeaveSendsWait(t *testing.T) {
	srv := httptest.NewServer(newMux(DefaultConfig()))
	defer srv.Close()

	a, b := dial(t, srv, "mv"), dial(t, srv, "mv")
	next(t, a, "s")
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
	next(t, b, "wait")
}

func TestBadRoomIDRejected(t *testing.T) {
	srv := httptest.NewServer(newMux(DefaultConfig()))
	defer srv.Close()
	url := "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws?room=Bad%20Room"
	if _, resp, err := websocket.DefaultDialer.Dial(url, nil); err == nil || resp == nil || resp.StatusCode != 400 {
		t.Fatalf("want HTTP 400 handshake failure, got err=%v resp=%v", err, resp)
	}
}
