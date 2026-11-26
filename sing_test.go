package main

import "testing"

// singNow fills slot 0's charge and presses its ultimate: Sing.
func singNow(g *Game) {
	g.P[0].Charge = g.cfg.Jigglypuff.Ultimate.Max
	press(g, 0, Input{Ult: true})
}

func notesOf(g *Game) (n int) {
	for _, b := range g.Balls {
		if b.K == ballNote {
			n++
		}
	}
	return
}

func TestSingLetsOutTheNotesAndEmptiesTheCharge(t *testing.T) {
	g := jig(400)
	sg := g.cfg.Jigglypuff.Sing
	press(g, 0, Input{Ult: true}) // no charge: nothing
	if notesOf(g) != 0 {
		t.Fatal("it sang without the charge")
	}
	singNow(g)
	if got := notesOf(g); got != sg.Count {
		t.Fatalf("%d notes, want %d", got, sg.Count)
	}
	if g.P[0].Charge != 0 {
		t.Fatalf("the charge should be spent: %v", g.P[0].Charge)
	}
	g.Step(dt)
	press(g, 0, Input{Ult: true}) // again at once: no charge left
	if got := notesOf(g); got > sg.Count {
		t.Fatalf("a second song with no charge: %d notes", got)
	}
}

func TestNotesChaseTheFoeAndHurtNothing(t *testing.T) {
	g := jig(300)
	hp := g.P[1].HP
	singNow(g)
	reached := false
	for i := 0; i < int(10*tickRate); i++ {
		g.Step(dt)
		reached = reached || g.P[1].Marks > 0 || g.P[1].DozeT > 0
	}
	if g.P[1].HP != hp {
		t.Fatalf("a note hurt the foe: HP %d, was %d", g.P[1].HP, hp)
	}
	if !reached {
		t.Fatal("the notes never reached the foe")
	}
}

func TestEnoughMarksPutTheFoeToSleepForTheConfiguredTime(t *testing.T) {
	g := jig(120)
	sg := &g.cfg.Jigglypuff.Sing
	sg.Count, sg.MarksToSleep, sg.SleepDuration, sg.Duration = 6, 3, 2, 20
	singNow(g)
	slept := false
	for i := 0; i < int(10*tickRate) && !slept; i++ {
		g.Step(dt)
		slept = g.P[1].DozeT > 0
	}
	if !slept {
		t.Fatalf("the foe never fell asleep (marks %d)", g.P[1].Marks)
	}
	if g.P[1].StunT < 1.5 || g.P[1].Marks != 0 {
		t.Fatalf("asleep: StunT %v, marks %d (want it unable to act for about 2 s, marks cleared)", g.P[1].StunT, g.P[1].Marks)
	}
	steps(g, int(3*tickRate))
	if g.P[1].DozeT != 0 || g.P[1].StunT != 0 {
		t.Fatalf("it should have woken: DozeT %v, StunT %v", g.P[1].DozeT, g.P[1].StunT)
	}
}

func TestMarksRunOutAfterMarkDuration(t *testing.T) {
	g := jig(300)
	g.cfg.Jigglypuff.Sing.MarkDuration = 1
	g.P[1].Marks, g.P[1].MarkT = 2, 1
	steps(g, int(1.2*tickRate))
	if g.P[1].Marks != 0 {
		t.Fatalf("marks %d, want them gone", g.P[1].Marks)
	}
}

func TestANoteThatTouchesASleepingFoeIsSpentForNothing(t *testing.T) {
	g := jig(100)
	g.P[1].DozeT = 5
	singNow(g)
	steps(g, int(8*tickRate))
	if g.P[1].Marks != 0 {
		t.Fatalf("a sleeping foe took %d marks", g.P[1].Marks)
	}
	if notesOf(g) != 0 {
		t.Fatalf("%d notes are left hovering over a sleeping foe", notesOf(g))
	}
}

func TestANoteWithNoHPLeftIsGone(t *testing.T) {
	g := jig(900)
	singNow(g)
	for i := range g.Balls {
		g.Balls[i].HP = 0
	}
	g.Step(dt)
	if notesOf(g) != 0 {
		t.Fatalf("%d notes survived with no HP", notesOf(g))
	}
}

func TestSingWorksBetweenTheBlowsOfAChainButNotMidSwing(t *testing.T) {
	g := jig(400)
	swing(g, 0)
	singNow(g)
	if notesOf(g) == 0 || !g.P[0].SlapChain {
		t.Fatalf("it should sing inside use_window and keep the chain: notes %d, chain %v", notesOf(g), g.P[0].SlapChain)
	}
	g2 := jig(400)
	press(g2, 0, Input{Skill: true})
	g2.Step(dt)
	g2.P[0].Charge = g2.cfg.Jigglypuff.Ultimate.Max
	press(g2, 0, Input{Ult: true})
	if notesOf(g2) != 0 {
		t.Fatal("a swing in progress is rooted: it cannot sing")
	}
}

func TestSingConfigIsChecked(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Jigglypuff.Sing.Count = 0
	if cfg.validate() == nil {
		t.Fatal("count 0 should be refused")
	}
	cfg = DefaultConfig()
	cfg.Jigglypuff.Sing.MarksToSleep = 0
	if cfg.validate() == nil {
		t.Fatal("marks_to_sleep 0 should be refused")
	}
}

func TestNotesWeaveInAnSAroundTheirPathWhenSwayWidthIsSet(t *testing.T) {
	maxOff := func(width float64) float64 {
		g := jig(700)
		sg := &g.cfg.Jigglypuff.Sing
		sg.Count, sg.SwayWidth, sg.SwayLength, sg.Duration = 1, width, 120, 30
		g.P[1].X, g.P[1].Y = g.P[0].X+700, g.P[0].Y // straight along +x from where the note starts
		singNow(g)
		worst := 0.0
		for i := 0; i < int(4*tickRate); i++ {
			g.Step(dt)
			for _, b := range g.Balls {
				if b.K == ballNote && b.Back {
					if d := b.Y - g.P[0].Y; d > worst {
						worst = d
					} else if -d > worst {
						worst = -d
					}
				}
			}
		}
		return worst
	}
	if straight := maxOff(0); straight > 1 {
		t.Fatalf("with no sway the note strayed %v from its line", straight)
	}
	if swayed := maxOff(40); swayed < 30 || swayed > 41 {
		t.Fatalf("with sway_width 40 the note strayed %v to a side, want about 40", swayed)
	}
}

func TestANotesSwayStillReachesTheFoe(t *testing.T) {
	g := jig(300)
	sg := &g.cfg.Jigglypuff.Sing
	sg.SwayWidth, sg.SwayLength, sg.Duration = 60, 100, 30
	singNow(g)
	marked := false
	for i := 0; i < int(12*tickRate) && !marked; i++ {
		g.Step(dt)
		marked = g.P[1].Marks > 0 || g.P[1].DozeT > 0
	}
	if !marked {
		t.Fatal("swaying notes never touched the foe")
	}
}
