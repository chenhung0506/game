package main

import (
	"math"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/websocket"
)

func dialPlay(t *testing.T, srv *httptest.Server, play string) *websocket.Conn {
	t.Helper()
	url := "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws?mode=web&menu=1&play=" + play
	c, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { c.Close() })
	return c
}

func TestPlayingTheComputerOpensARoomWithOneInItAndAPickStartsTheMatch(t *testing.T) {
	srv := httptest.NewServer(newMux(DefaultConfig()))
	defer srv.Close()
	a := dialPlay(t, srv, "computer")
	if next(t, a, "hi")["you"] != float64(0) {
		t.Fatal("not seated first")
	}
	pick(t, a, snorlax)
	s := next(t, a, "s")
	if k := s["p"].([]any)[1].(map[string]any)["k"].(float64); !botCanPlay(int(k)) {
		t.Fatalf("the computer plays kind %v", k)
	}
	// A second computer-mode player does not walk into that room: it gets one of its own.
	b := dialPlay(t, srv, "computer")
	if next(t, b, "hi")["you"] != float64(0) {
		t.Fatal("a computer-mode player was put in someone else's room")
	}
	// A player looking for a person is shown both rooms, marked as against the computer, and takes the computer's place.
	c := dialMenu(t, srv)
	rooms := next(t, c, "rooms")["rooms"].([]any)
	if len(rooms) != 2 || rooms[0].(map[string]any)["bot"] != true {
		t.Fatalf("menu = %v, want two rooms against the computer", rooms)
	}
}

func TestThePracticeRoomHoldsADummyAshMayBePlayedAndNobodyElseGetsIn(t *testing.T) {
	cfg := DefaultConfig()
	srv := httptest.NewServer(newMux(cfg))
	defer srv.Close()
	a := dialPlay(t, srv, "dummy")
	next(t, a, "hi")
	pick(t, a, ash)
	s := next(t, a, "s")
	ps := s["p"].([]any)
	if ps[0].(map[string]any)["k"] != float64(ash) || ps[1].(map[string]any)["k"] != float64(dummy) || ps[1].(map[string]any)["hp"] != float64(cfg.Dummy.MaxHP) {
		t.Fatalf("players = %v, want a person's Ash against a dummy at full HP", ps)
	}
	// Whoever comes next, by whatever way, is not put in there.
	b := dialPlay(t, srv, "player") // no menu either: the practice room is not on it, so it is seated in a room of its own at once
	if next(t, b, "hi")["you"] != float64(0) {
		t.Fatal("put in the practice room")
	}
	h := newHub(cfg)
	r := h.open(modeWeb)
	if r.canPick(ash) {
		t.Fatal("a person may play Ash outside the practice room")
	}
	r.dummy = true
	if !r.canPick(ash) || r.canPick(dummy) || r.canPick(gyarados) {
		t.Fatal("the practice room lets a person play Ash and nothing but the characters")
	}
	if r.setBot(0, snorlax) {
		t.Fatal("the dummy was swapped for another opponent")
	}
}

func TestADummyIsNotMovedByAKnockbackButTakesTheHit(t *testing.T) {
	cfg := DefaultConfig()
	g := duel(cfg, ash, dummy)
	g.P[1].HP = g.maxHP(1)
	g.P[1].X = 600
	press(g, 0, Input{Mega: true})
	steps(g, int(cfg.Ash.MegatonPunch.ChargeDuration/dt)+2)
	if g.P[1].X != 600 || g.P[1].HP != cfg.Dummy.MaxHP-cfg.Ash.MegatonPunch.Damage {
		t.Fatalf("dummy at x=%v HP=%d", g.P[1].X, g.P[1].HP)
	}
}

func TestAshIsCarriedForwardWithTheFistAndStopsAtTheBody(t *testing.T) {
	cfg := DefaultConfig()
	mp := cfg.Ash.MegatonPunch
	g := megaDuel(cfg, 600) // the opponent is out of the fist's reach
	press(g, 0, Input{Mega: true})
	steps(g, int(mp.ChargeDuration/dt)+2)
	if math.Abs(g.P[0].X-(300+mp.LungeDistance)) > 1e-6 || g.P[0].Y != 250 {
		t.Fatalf("Ash at %v,%v, want %v ahead", g.P[0].X, g.P[0].Y, mp.LungeDistance)
	}
	g = megaDuel(cfg, 100) // right up against it: it cannot be carried into the body
	press(g, 0, Input{Mega: true})
	steps(g, int(mp.ChargeDuration/dt)+2)
	if gap := g.P[1].X - g.P[0].X; gap < g.radius(0)+g.radius(1)-1e-6 {
		t.Fatalf("Ash ended %v from the opponent, inside its body", gap)
	}
	cfg.Ash.MegatonPunch.LungeDistance = 0
	g = megaDuel(cfg, 600)
	press(g, 0, Input{Mega: true})
	steps(g, int(mp.ChargeDuration/dt)+2)
	if g.P[0].X != 300 {
		t.Fatal("it moved with no lunge")
	}
}

func TestTheFistsDrawnStartAndEndAreSentToTheClient(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Ash.MegatonPunch.FistStart, cfg.Ash.MegatonPunch.FistEnd = -80, 300
	srv := httptest.NewServer(newMux(cfg))
	defer srv.Close()
	mega := next(t, dialPlay(t, srv, "dummy"), "hi")["mega"].(map[string]any)
	if mega["fstart"] != float64(-80) || mega["fend"] != float64(300) {
		t.Fatalf("mega = %v", mega)
	}
}

func TestTheHitTextCanBeSwitchedOff(t *testing.T) {
	for _, on := range []bool{true, false} {
		cfg := DefaultConfig()
		cfg.Ash.MegatonPunch.ShowText = on
		srv := httptest.NewServer(newMux(cfg))
		hi := next(t, dialPlay(t, srv, "dummy"), "hi")
		if got := hi["mega"].(map[string]any)["text"] == float64(1); got != on {
			t.Fatalf("show_text %v: client sent %v", on, hi["mega"])
		}
		srv.Close()
	}
}

func TestTheWarningLineCanBeSwitchedOffInTheConfigTheClientIsSent(t *testing.T) {
	for _, on := range []bool{true, false} {
		cfg := DefaultConfig()
		cfg.Ash.MegatonPunch.ShowWarning = on
		srv := httptest.NewServer(newMux(cfg))
		hi := next(t, dialPlay(t, srv, "dummy"), "hi")
		if got := hi["mega"].(map[string]any)["warn"] == float64(1); got != on {
			t.Fatalf("show_warning %v: client sent %v", on, hi["mega"])
		}
		srv.Close()
	}
}
