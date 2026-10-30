package main

import (
	"encoding/json"
	"testing"
	"time"
)

// ---- team matches and the repick button, on the room itself (no sockets) ----

func teamRoom() *room {
	r := &room{g: NewGame(DefaultConfig()), winner: -1, pick: [2]int{-1, -1}}
	r.c = [2]*client{{send: make(chan []byte, 8)}, {send: make(chan []byte, 8)}}
	r.running = true // the loop goroutine is not wanted here: the tests call step themselves
	return r
}

func snapOf(t *testing.T, msg []byte) (m struct {
	W  int       `json:"w"`
	TM *teamInfo `json:"tm"`
	P  [2]Player `json:"p"`
	EZ [2]int    `json:"ez"`
}) {
	t.Helper()
	if err := json.Unmarshal(msg, &m); err != nil {
		t.Fatal(err)
	}
	return m
}

// knockOut ends the bout the way a skill would: slot i is left with no HP. step then notices.
func knockOut(r *room, i int, now time.Time) []byte {
	r.g.P[i].HP = 0
	return r.step(now)
}

func TestOrderMustBeAPermutationOfEveryCharacter(t *testing.T) {
	r := teamRoom()
	if r.setOrder(0, []int{0, 1, 2, 3, 4, 5}) {
		t.Fatal("an order was taken outside team mode")
	}
	r.setTeam(true)
	for _, bad := range [][]int{{0, 1, 2, 3, 4}, {0, 1, 2, 3, 4, 4}, {0, 1, 2, 3, 4, 6}, {-1, 1, 2, 3, 4, 5}, {0, 1, 2, 3, 4, 5, 0}, nil} {
		if r.setOrder(0, bad) {
			t.Errorf("order %v was taken", bad)
		}
	}
	if !r.setOrder(0, []int{2, 0, 3, 1, 4, 5}) || r.pick[0] != 2 {
		t.Fatalf("a good order was refused, or its first character is not the pick (%v)", r.pick)
	}
	if r.choose(1, 0) {
		t.Fatal("a single pick was taken in team mode")
	}
}

func TestTeamMatchStartsWhenBothOrdersAreIn(t *testing.T) {
	r := teamRoom()
	r.setTeam(true)
	r.setOrder(0, []int{2, 0, 3, 1, 4, 5})
	if r.playing() && r.g.P[0].Kind == 2 && r.g.P[1].Kind == 3 {
		t.Fatal("started with one order only")
	}
	r.setOrder(1, []int{3, 1, 0, 2, 4, 5})
	if !r.playing() || r.g.P[0].Kind != 2 || r.g.P[1].Kind != 3 {
		t.Fatalf("kinds %d,%d, want each side's first: 2 and 3", r.g.P[0].Kind, r.g.P[1].Kind)
	}
	if r.setOrder(0, []int{0, 1, 2, 3, 4, 5}) || r.setTeam(false) {
		t.Fatal("the order or the mode changed while a match is on")
	}
}

func TestKnockedOutCharacterIsReplacedInOrderAndSurvivorKeepsItsHP(t *testing.T) {
	r := teamRoom()
	r.setTeam(true)
	r.setOrder(0, []int{0, 1, 2, 3, 4, 5})
	r.setOrder(1, []int{3, 2, 1, 0, 4, 5})
	now := time.Now()
	r.step(now)
	r.g.P[0].HP = 37 // slot 0's Pikachu is hurt, then slot 1's Chikorita goes down
	m := snapOf(t, knockOut(r, 1, now))
	if m.W != 0 || m.TM == nil || m.TM.Cur != [2]int{0, 1} || m.TM.Over {
		t.Fatalf("after the first bout: winner %d, team %+v, want slot 0 and slot 1 on its second character", m.W, m.TM)
	}
	if len(m.TM.Fallen[1]) != 1 || m.TM.Fallen[1][0] != 3 || len(m.TM.Fallen[0]) != 0 {
		t.Fatalf("fallen %v, want only Chikorita (3) on slot 1", m.TM.Fallen)
	}
	m = snapOf(t, r.step(now.Add(roundReset-time.Millisecond)))
	if m.W != 0 {
		t.Fatal("the next bout began before the pause was over")
	}
	m = snapOf(t, r.step(now.Add(roundReset+time.Millisecond)))
	if m.W != -1 || m.P[0].Kind != 0 || m.P[1].Kind != 2 {
		t.Fatalf("winner %d kinds %d,%d, want a new bout of Pikachu (0) against Charizard (2)", m.W, m.P[0].Kind, m.P[1].Kind)
	}
	if m.P[0].HP != 37 || m.P[1].HP != DefaultConfig().maxHPOf(2) {
		t.Fatalf("HP %d and %d, want the survivor's 37 and a full Charizard", m.P[0].HP, m.P[1].HP)
	}
}

func TestMatchEndsWhenASideRunsOutAndHoldsTheResult(t *testing.T) {
	r := teamRoom()
	r.setTeam(true)
	r.setOrder(0, []int{0, 1, 2, 3, 4, 5})
	r.setOrder(1, []int{3, 2, 1, 0, 4, 5})
	now := time.Now()
	r.step(now)
	for n := 0; n < kinds-1; n++ {
		if m := snapOf(t, knockOut(r, 1, now)); m.TM.Over {
			t.Fatalf("over after %d knock-outs", n+1)
		}
		now = now.Add(roundReset + time.Millisecond)
		r.step(now)
	}
	m := snapOf(t, knockOut(r, 1, now))
	if !m.TM.Over || m.TM.Winner != 0 || len(m.TM.Fallen[1]) != kinds {
		t.Fatalf("team %+v, want the match over, won by slot 0, with all four of slot 1 fallen", m.TM)
	}
	if r.repick() == false {
		t.Fatal("repick refused once the match is over")
	}
}

func TestSimultaneousKnockOutOfBothLastCharactersGoesToSlotZero(t *testing.T) {
	r := teamRoom()
	r.setTeam(true)
	r.setOrder(0, []int{0, 1, 2, 3, 4, 5})
	r.setOrder(1, []int{0, 1, 2, 3, 4, 5})
	r.cur = [2]int{kinds - 1, kinds - 1}
	r.g.P[0].HP = 0
	m := snapOf(t, knockOut(r, 1, time.Now()))
	if !m.TM.Over || m.TM.Winner != 0 {
		t.Fatalf("team %+v, want over with the arbitrary winner 0", m.TM)
	}
}

func TestResultStaysUpLongerThanABoutBreakThenRestartsTheSameMatch(t *testing.T) {
	r := teamRoom()
	r.setTeam(true)
	r.setOrder(0, []int{1, 0, 2, 3, 4, 5})
	r.setOrder(1, []int{2, 0, 1, 3, 4, 5})
	now := time.Now()
	r.step(now)
	r.cur = [2]int{0, kinds - 1}
	snapOf(t, knockOut(r, 1, now))
	if m := snapOf(t, r.step(now.Add(roundReset+time.Millisecond))); m.W != 0 {
		t.Fatal("the result went away after a bout break: the repick button needs longer")
	}
	m := snapOf(t, r.step(now.Add(resultHold+time.Millisecond)))
	if m.W != -1 || m.TM.Over || m.TM.Cur != [2]int{} || m.P[0].Kind != 1 || m.P[1].Kind != 2 || m.P[0].HP != DefaultConfig().maxHPOf(1) {
		t.Fatalf("after the hold: winner %d team %+v kinds %d,%d, want a fresh match from each first character", m.W, m.TM, m.P[0].Kind, m.P[1].Kind)
	}
}

func TestRepickIsOnlyForAFinishedMatchAndFreesBothPlayers(t *testing.T) {
	r := teamRoom()
	r.choose(0, 0)
	r.choose(1, 1)
	now := time.Now()
	r.step(now)
	if r.repick() {
		t.Fatal("repick allowed in the middle of a match")
	}
	knockOut(r, 1, now)
	if !r.repick() || r.pick != [2]int{-1, -1} || r.playing() || !r.overAt.IsZero() {
		t.Fatalf("after the result: picks %v, playing %v, want both free to choose again", r.pick, r.playing())
	}
	if r.repick() {
		t.Fatal("a second repick (the other player pressing too) must do nothing")
	}

	// A team match between bouts is not finished: no repick until it is over.
	r.setTeam(true)
	r.setOrder(0, []int{0, 1, 2, 3, 4, 5})
	r.setOrder(1, []int{0, 1, 2, 3, 4, 5})
	r.step(now)
	knockOut(r, 1, now)
	if r.repick() {
		t.Fatal("repick allowed between the bouts of a team match")
	}
}

func TestSingleMatchesStillRestartAndTheSnapshotHasNoTeam(t *testing.T) {
	r := teamRoom()
	r.choose(0, 0)
	r.choose(1, 1)
	now := time.Now()
	r.step(now)
	m := snapOf(t, knockOut(r, 1, now))
	if m.TM != nil || m.W != 0 {
		t.Fatalf("winner %d team %+v, want a plain result", m.W, m.TM)
	}
	if m := snapOf(t, r.step(now.Add(resultHold-time.Second))); m.W != 0 {
		t.Fatal("the result went away before resultHold")
	}
	if m := snapOf(t, r.step(now.Add(resultHold+time.Second))); m.W != -1 || m.P[1].HP != DefaultConfig().maxHPOf(1) {
		t.Fatalf("winner %d hp %d, want a fresh round", m.W, m.P[1].HP)
	}
}

func TestRunningOutOnSlotZeroGivesTheMatchToSlotOne(t *testing.T) {
	r := teamRoom()
	r.setTeam(true)
	r.setOrder(0, []int{0, 1, 2, 3, 4, 5})
	r.setOrder(1, []int{0, 1, 2, 3, 4, 5})
	r.cur = [2]int{kinds - 1, 0}
	m := snapOf(t, knockOut(r, 0, time.Now()))
	if !m.TM.Over || m.TM.Winner != 1 || m.W != 1 {
		t.Fatalf("team %+v winner %d, want slot 1 to win", m.TM, m.W)
	}
}
