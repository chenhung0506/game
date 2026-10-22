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
	srv := httptest.NewServer(newMux())
	defer srv.Close()

	a := dial(t, srv, "abc")
	if got := next(t, a, "hi")["you"]; got != float64(0) {
		t.Fatalf("first player index = %v, want 0", got)
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
	srv := httptest.NewServer(newMux())
	defer srv.Close()

	a, b := dial(t, srv, "mv"), dial(t, srv, "mv")
	next(t, a, "s")

	a.WriteMessage(websocket.TextMessage, []byte(`{"dx":1,"dy":0}`))
	var x float64
	for i := 0; i < 30 && x <= 200; i++ {
		p := next(t, b, "s")["p"].([]any)[0].(map[string]any)
		x = p["x"].(float64)
	}
	if x <= 200 {
		t.Fatalf("player 0 x = %v, want > 200 after holding right", x)
	}

	a.Close()
	next(t, b, "wait")
}

func TestBadRoomIDRejected(t *testing.T) {
	srv := httptest.NewServer(newMux())
	defer srv.Close()
	url := "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws?room=Bad%20Room"
	if _, resp, err := websocket.DefaultDialer.Dial(url, nil); err == nil || resp == nil || resp.StatusCode != 400 {
		t.Fatalf("want HTTP 400 handshake failure, got err=%v resp=%v", err, resp)
	}
}
