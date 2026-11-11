package main

import (
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/websocket"
)

// dialWith opens a socket with the query the page would send.
func dialWith(t *testing.T, srv *httptest.Server, query string) *websocket.Conn {
	t.Helper()
	url := "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws?" + query
	c, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { c.Close() })
	return c
}

// A host opens a squad; the second person finds it in the menu and joins; both pick.
func startSquad(t *testing.T, srv *httptest.Server, ka, kb int) (host, mate *websocket.Conn) {
	t.Helper()
	host = dialWith(t, srv, "play=squad&mode=web")
	next(t, host, "hi")
	mate = dialWith(t, srv, "menu=1&mode=web")
	rooms := next(t, mate, "rooms")["rooms"].([]any)
	if len(rooms) != 1 || rooms[0].(map[string]any)["squad"] != true {
		t.Fatalf("the menu shows %v, want the one squad", rooms)
	}
	id := rooms[0].(map[string]any)["id"].(string)
	if err := mate.WriteMessage(websocket.TextMessage, []byte(fmt.Sprintf(`{"join":%q}`, id))); err != nil {
		t.Fatal(err)
	}
	next(t, mate, "hi")
	pick(t, host, ka)
	pick(t, mate, kb)
	return host, mate
}

func TestASquadOfTwoPeopleStartsAMatchWithTwoComputersAgainstThem(t *testing.T) {
	srv := httptest.NewServer(newMux(DefaultConfig()))
	defer srv.Close()
	host, mate := startSquad(t, srv, pikachu, snorlax)

	_ = mate
	s := next(t, host, "s")
	ps := s["p"].([]any)
	if len(ps) != 4 {
		t.Fatalf("the snapshot has %d fighters, want 4", len(ps))
	}
	k := func(i int) float64 { return ps[i].(map[string]any)["k"].(float64) }
	if k(0) != pikachu || k(2) != snorlax {
		t.Fatalf("the people play kinds %v and %v, want Pikachu (host, slot 0) and Snorlax (mate, slot 2)", k(0), k(2))
	}
	for _, i := range []int{1, 3} {
		if !botCanPlay(int(k(i))) {
			t.Errorf("computer in slot %d plays kind %v, which the computer cannot play", i, k(i))
		}
	}
}

func TestEachPersonOfASquadIsToldTheirOwnSlot(t *testing.T) {
	srv := httptest.NewServer(newMux(DefaultConfig()))
	defer srv.Close()
	host := dialWith(t, srv, "play=squad&mode=web")
	if got := next(t, host, "hi")["you"]; got != float64(0) {
		t.Fatalf("host is slot %v, want 0", got)
	}
	mate := dialWith(t, srv, "menu=1&mode=web")
	id := next(t, mate, "rooms")["rooms"].([]any)[0].(map[string]any)["id"].(string)
	mate.WriteMessage(websocket.TextMessage, []byte(fmt.Sprintf(`{"join":%q}`, id)))
	if got := next(t, mate, "hi")["you"]; got != float64(2) {
		t.Fatalf("the teammate is slot %v, want 2", got)
	}
}

func TestASquadWaitsForTheSecondPersonAndAThirdIsTurnedAway(t *testing.T) {
	srv := httptest.NewServer(newMux(DefaultConfig()))
	defer srv.Close()
	host := dialWith(t, srv, "play=squad&mode=web")
	next(t, host, "hi")
	pick(t, host, pikachu)
	m := next(t, host, "lobby")
	if m["squad"] != true || m["here"].([]any)[1] != false {
		t.Fatalf("lobby %v: the squad should wait for its second person", m)
	}
	// a person looking for a duel is never put in the squad
	other := dialWith(t, srv, "mode=web")
	if got := next(t, other, "hi")["you"]; got != float64(0) {
		t.Fatalf("a duel player was seated in slot %v", got)
	}
}

func TestWhenOnePersonLeavesASquadTheOtherGoesBackToTheLobby(t *testing.T) {
	srv := httptest.NewServer(newMux(DefaultConfig()))
	defer srv.Close()
	host, mate := startSquad(t, srv, pikachu, snorlax)
	next(t, host, "s")
	mate.Close()
	for {
		m := next(t, host, "lobby")
		if m["here"].([]any)[1] == false {
			if p := m["picks"].([]any); p[0] != float64(-1) {
				t.Fatalf("picks %v: both should choose again", p)
			}
			return
		}
	}
}
