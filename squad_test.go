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

// Two people who both press 2v2 meet: the second is shown the waiting squads and can join one or open its own.
func TestPressing2v2WithASquadWaitingShowsItAndJoiningMakesYouTheTeammate(t *testing.T) {
	srv := httptest.NewServer(newMux(DefaultConfig()))
	defer srv.Close()
	host := dialWith(t, srv, "play=squad&menu=1&mode=web")
	next(t, host, "hi") // nobody was waiting: its own squad, at once

	mate := dialWith(t, srv, "play=squad&menu=1&mode=web")
	rooms := next(t, mate, "rooms")["rooms"].([]any)
	if len(rooms) != 1 || rooms[0].(map[string]any)["squad"] != true {
		t.Fatalf("shown %v, want the one waiting squad", rooms)
	}
	mate.WriteMessage(websocket.TextMessage, []byte(fmt.Sprintf(`{"join":%q}`, rooms[0].(map[string]any)["id"])))
	if got := next(t, mate, "hi")["you"]; got != float64(2) {
		t.Fatalf("the joiner is slot %v, want 2", got)
	}
}

func TestAPersonWhoPressed2v2CanOpenASquadOfTheirOwnInstead(t *testing.T) {
	srv := httptest.NewServer(newMux(DefaultConfig()))
	defer srv.Close()
	host := dialWith(t, srv, "play=squad&menu=1&mode=web")
	next(t, host, "hi")
	other := dialWith(t, srv, "play=squad&menu=1&mode=web")
	next(t, other, "rooms")
	other.WriteMessage(websocket.TextMessage, []byte(`{"new":true}`))
	if got := next(t, other, "hi")["you"]; got != float64(0) {
		t.Fatalf("a squad of its own seats its opener in slot %v, want 0", got)
	}
	third := dialWith(t, srv, "play=squad&menu=1&mode=web")
	if rooms := next(t, third, "rooms")["rooms"].([]any); len(rooms) != 2 {
		t.Fatalf("shown %d squads, want both", len(rooms))
	}
}

// A connection that does not ask for the menu (older pages, tests) always opens its own squad.
func TestA2v2ConnectionWithoutTheMenuOpensItsOwnSquadEvenWhenOthersWait(t *testing.T) {
	srv := httptest.NewServer(newMux(DefaultConfig()))
	defer srv.Close()
	a := dialWith(t, srv, "play=squad&mode=web")
	next(t, a, "hi")
	b := dialWith(t, srv, "play=squad&mode=web")
	if got := next(t, b, "hi")["you"]; got != float64(0) {
		t.Fatalf("slot %v, want its own squad's slot 0", got)
	}
}

// ---- 1v2: one person against the two computers ----

func TestAPersonAloneAgainstTwoComputersStartsAsSoonAsTheyPickAndSlotTwoIsEmpty(t *testing.T) {
	srv := httptest.NewServer(newMux(DefaultConfig()))
	defer srv.Close()
	c := dialWith(t, srv, "play=vs2&mode=web")
	if got := next(t, c, "hi")["you"]; got != float64(0) {
		t.Fatalf("slot %v, want 0", got)
	}
	pick(t, c, pikachu)
	ps := next(t, c, "s")["p"].([]any)
	if len(ps) != 4 {
		t.Fatalf("%d fighters, want 4 slots", len(ps))
	}
	hp := func(i int) float64 { return ps[i].(map[string]any)["hp"].(float64) }
	if hp(2) != 0 {
		t.Fatalf("the empty slot has %v HP, want it down from the start", hp(2))
	}
	if hp(0) <= 0 || hp(1) <= 0 || hp(3) <= 0 {
		t.Fatalf("the person and both computers must be standing: %v %v %v", hp(0), hp(1), hp(3))
	}
}

func TestASoloSquadIsNeverListedAndNobodyJoinsIt(t *testing.T) {
	srv := httptest.NewServer(newMux(DefaultConfig()))
	defer srv.Close()
	c := dialWith(t, srv, "play=vs2&mode=web")
	next(t, c, "hi")
	other := dialWith(t, srv, "menu=1&mode=web") // looks for someone: the solo squad is not offered
	if got := next(t, other, "hi")["you"]; got != float64(0) {
		t.Fatalf("a duel player was put in slot %v of the solo squad", got)
	}
}

func TestASoloSquadMatchIsWonByTheComputersWhenThePersonFallsAndPlaysAgain(t *testing.T) {
	cfg := DefaultConfig()
	h := newHub(cfg)
	s := &squad{id: "q1", g: NewTeamGame(cfg, modeWeb, 4), winner: -1, pick: [2]int{pikachu, -1}, solo: true}
	s.c[0] = &client{send: make(chan []byte, 8)}
	s.foes = [2]*Bot{newBot(snorlax, 1), newBot(snorlax, 2)}
	h.squads[s.id] = s
	if !s.playing() {
		t.Fatal("one person who has picked should be playing")
	}
	s.reset()
	if s.g.Winner() != -1 {
		t.Fatalf("a fresh solo match has a winner (%d)", s.g.Winner())
	}
	s.g.P[0].HP = 0
	if s.g.Winner() != 1 {
		t.Fatalf("with the person down the computers' side wins, got %d", s.g.Winner())
	}
}

// ---- 1v2: the extra-HP slider ----

func TestTheExtraHPOfASoloSquadRaisesOnlyItsPersonsHPAndTheSnapshotSaysTheFullHP(t *testing.T) {
	srv := httptest.NewServer(newMux(DefaultConfig()))
	defer srv.Close()
	c := dialWith(t, srv, "play=vs2&mode=web")
	next(t, c, "hi")
	if err := c.WriteMessage(websocket.TextMessage, []byte(`{"bonus":50}`)); err != nil {
		t.Fatal(err)
	}
	for { // the first lobby message is the greeting's; the one that follows carries the bonus
		if next(t, c, "lobby")["bonus"] == float64(50) {
			break
		}
	}
	pick(t, c, pikachu)
	s := next(t, c, "s")
	ps, mx := s["p"].([]any), s["mx"].([]any)
	base := float64(DefaultConfig().maxHPOf(pikachu))
	if want := base * 1.5; ps[0].(map[string]any)["hp"] != want || mx[0] != want {
		t.Fatalf("person's HP %v and full HP %v, want both %v", ps[0].(map[string]any)["hp"], mx[0], want)
	}
	foe := ps[1].(map[string]any)
	if foe["hp"] != mx[1] || mx[1] != float64(DefaultConfig().maxHPOf(int(foe["k"].(float64)))) {
		t.Fatalf("a computer's HP %v / full %v should be its character's own", foe["hp"], mx[1])
	}
}

func TestTheExtraHPIsRefusedUnlessItIsASoloSquadBetweenMatchesAndAValidStep(t *testing.T) {
	cfg := DefaultConfig()
	solo := &squad{g: NewTeamGame(cfg, modeWeb, 4), solo: true, pick: [2]int{-1, -1}}
	for _, bad := range []int{-10, 310, 15, 301} {
		if solo.setBonus(bad) {
			t.Errorf("a bonus of %d%% was accepted", bad)
		}
	}
	if !solo.setBonus(300) || solo.bonus != 300 {
		t.Fatal("300% is the most and must be accepted")
	}
	solo.c[0], solo.pick[0] = &client{send: make(chan []byte, 1)}, pikachu
	if solo.setBonus(0) {
		t.Error("the bonus changed while a match was on")
	}
	duo := &squad{g: NewTeamGame(cfg, modeWeb, 4), pick: [2]int{-1, -1}}
	if duo.setBonus(50) {
		t.Error("a 2v2 accepted an HP bonus")
	}
}

func TestAnHPBoostScalesMaxHPAndSurvivesAReset(t *testing.T) {
	g := newTeamTestGame(pikachu, snorlax, pikachu, snorlax)
	g.Boost[0] = 100
	g.Reset()
	if want := 2 * g.cfg.maxHPOf(pikachu); g.P[0].HP != want || g.maxHP(0) != want {
		t.Fatalf("HP %d / max %d, want %d", g.P[0].HP, g.maxHP(0), want)
	}
	g.Ease[0] = 50
	if want := g.cfg.maxHPOf(pikachu) * 150 / 100; g.maxHP(0) != want { // a handicap and a boost simply add up
		t.Fatalf("max HP %d, want %d", g.maxHP(0), want)
	}
}
