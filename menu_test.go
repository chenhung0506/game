package main

import (
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// dialMenu opens the socket the way the page does: asking to choose a room when people are waiting.
func dialMenu(t *testing.T, srv *httptest.Server) *websocket.Conn {
	t.Helper()
	url := "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws?mode=web&menu=1"
	c, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { c.Close() })
	return c
}

func roomIDs(m map[string]any) []string {
	var ids []string
	for _, r := range m["rooms"].([]any) {
		ids = append(ids, r.(map[string]any)["id"].(string))
	}
	return ids
}

func TestMenuIsOnlyShownWhenSomeoneIsWaitingAndLetsThePlayerChoose(t *testing.T) {
	srv := httptest.NewServer(newMux(DefaultConfig()))
	defer srv.Close()

	a := dialMenu(t, srv)
	if next(t, a, "hi")["you"] != float64(0) {
		t.Fatal("nobody is waiting: the first player opens a room without a menu")
	}
	b := dialMenu(t, srv)
	ids := roomIDs(next(t, b, "rooms"))
	if len(ids) != 1 {
		t.Fatalf("menu = %v, want the one waiting room", ids)
	}
	// Opening a room of its own, though people are waiting, is allowed.
	if err := b.WriteMessage(websocket.TextMessage, []byte(`{"new":true}`)); err != nil {
		t.Fatal(err)
	}
	if next(t, b, "hi")["you"] != float64(0) {
		t.Fatal("b should be alone in its own room")
	}
	c := dialMenu(t, srv)
	if got := roomIDs(next(t, c, "rooms")); len(got) != 2 {
		t.Fatalf("menu = %v, want both waiting rooms", got)
	}
	// Picking the first one seats c beside a, and a learns of it.
	if err := c.WriteMessage(websocket.TextMessage, []byte(`{"join":"`+ids[0]+`"}`)); err != nil {
		t.Fatal(err)
	}
	if next(t, c, "hi")["you"] != float64(1) {
		t.Fatal("c should sit in slot 1 beside a")
	}
	lobbyWith(t, a, [2]int{-1, -1})
}

func TestAFullRoomPickedFromAStaleMenuSendsTheListAgain(t *testing.T) {
	srv := httptest.NewServer(newMux(DefaultConfig()))
	defer srv.Close()

	a := dialMenu(t, srv)
	next(t, a, "hi")
	b := dialMenu(t, srv)
	id := roomIDs(next(t, b, "rooms"))[0]
	c := dialMenu(t, srv)
	next(t, c, "rooms")
	b.WriteMessage(websocket.TextMessage, []byte(`{"join":"`+id+`"}`))
	next(t, b, "hi")
	c.WriteMessage(websocket.TextMessage, []byte(`{"join":"`+id+`"}`)) // already full
	// Nobody is waiting any more, so c opens a room instead of being stuck in a menu with nothing in it.
	if next(t, c, "hi")["you"] != float64(0) {
		t.Fatal("c should have been given a room of its own")
	}
}

func TestMenuMarksAWaitingPlayerWhoStoppedAnswering(t *testing.T) {
	h := newHub(DefaultConfig())
	ghost := &client{send: make(chan []byte, 8)}
	ghost.touch()
	h.join(modeWeb, ghost)
	ghost.seen.Store(time.Now().Add(-2 * staleAfter).UnixNano())
	live := &client{send: make(chan []byte, 8)}
	live.touch()
	h.openRoom(modeWeb, live)
	list := h.waitingRooms()
	if len(list) != 2 || list[0].Stale || !list[1].Stale {
		t.Fatalf("list = %+v, want the live room first and the stale one last", list)
	}
}

func TestADroppedConnectionIsNoticedWithinAFewSeconds(t *testing.T) {
	if pongWait > 15*time.Second || pingEvery*2 > pongWait {
		t.Fatalf("pongWait %v, pingEvery %v: a ghost must go within seconds yet survive a missed ping", pongWait, pingEvery)
	}
}
