package glicko

import (
	"math"
	"testing"
)

func near(t *testing.T, name string, got, want, tol float64) {
	t.Helper()
	if math.Abs(got-want) > tol {
		t.Errorf("%s = %.5f, want %.5f", name, got, want)
	}
}

// The worked example of Glickman's paper ("Example of the Glicko-2 system"): a player rated 1500
// with RD 200 and volatility 0.06 beats a 1400/30 player and loses to 1550/100 and 1700/300.
func TestPaperExample(t *testing.T) {
	// RD is clamped to the floor here, so use the paper's numbers through the exported API only
	r := Update(Rating{R: 1500, RD: 200, Sigma: 0.06}, []Result{
		{Rating{R: 1400, RD: 30, Sigma: 0.06}, 1},
		{Rating{R: 1550, RD: 100, Sigma: 0.06}, 0},
		{Rating{R: 1700, RD: 300, Sigma: 0.06}, 0},
	})
	near(t, "rating", r.R, 1464.06, 0.01)
	near(t, "RD", r.RD, 151.52, 0.01)
	near(t, "volatility", r.Sigma, 0.05999, 0.00001)
}

func TestWinRaisesLossLowersAndUncertaintyShrinks(t *testing.T) {
	a, b := New(), New()
	win := Update(a, []Result{{b, 1}})
	loss := Update(b, []Result{{a, 0}})
	if win.R <= a.R || loss.R >= b.R {
		t.Errorf("win %v loss %v", win.R, loss.R)
	}
	if win.RD >= a.RD || loss.RD >= b.RD {
		t.Errorf("RD did not shrink: %v %v", win.RD, loss.RD)
	}
	// equal and symmetric newcomers move the same amount in opposite directions
	near(t, "symmetry", win.R-a.R, b.R-loss.R, 1e-9)
	// a draw between equals changes nothing in the rating
	d := Update(a, []Result{{b, 0.5}})
	near(t, "draw", d.R, a.R, 1e-9)
}

func TestAnEstablishedPlayerMovesLessThanANewcomer(t *testing.T) {
	newcomer := Rating{R: 1500, RD: 350, Sigma: 0.06}
	veteran := Rating{R: 1500, RD: 50, Sigma: 0.06}
	opp := Rating{R: 1500, RD: 50, Sigma: 0.06}
	n := Update(newcomer, []Result{{opp, 1}})
	v := Update(veteran, []Result{{opp, 1}})
	if n.R-1500 <= 2*(v.R-1500) {
		t.Errorf("newcomer +%.1f, veteran +%.1f", n.R-1500, v.R-1500)
	}
}

func TestBeatingAStrongerPlayerPaysMore(t *testing.T) {
	me := Rating{R: 1500, RD: 80, Sigma: 0.06}
	weak := Update(me, []Result{{Rating{R: 1300, RD: 80, Sigma: 0.06}, 1}})
	strong := Update(me, []Result{{Rating{R: 1700, RD: 80, Sigma: 0.06}, 1}})
	if strong.R-me.R <= weak.R-me.R {
		t.Errorf("beating 1700 gave +%.1f, beating 1300 +%.1f", strong.R-me.R, weak.R-me.R)
	}
}

func TestInactivityRaisesUncertaintyUpToTheCeiling(t *testing.T) {
	r := Rating{R: 1600, RD: 60, Sigma: 0.06}
	later := Inflate(r, 30)
	if later.RD <= r.RD || later.R != r.R {
		t.Errorf("30 days: %+v", later)
	}
	if Inflate(r, 0) != r || Inflate(r, -3) != r {
		t.Error("no time, no change")
	}
	if got := Inflate(r, 1e9).RD; got != MaxRD {
		t.Errorf("ceiling: %v", got)
	}
	// no games in a period: only the uncertainty changes
	idle := Update(r, nil)
	if idle.R != r.R || idle.RD <= r.RD {
		t.Errorf("idle period: %+v", idle)
	}
}

func TestRDStaysInBoundsAndProvisionalFollowsIt(t *testing.T) {
	r := Rating{R: 1500, RD: 60, Sigma: 0.06}
	for i := 0; i < 400; i++ {
		r = Update(r, []Result{{Rating{R: 1500, RD: 40, Sigma: 0.06}, float64(i % 2)}})
	}
	if r.RD < MinRD {
		t.Errorf("RD fell below the floor: %v", r.RD)
	}
	if New().IsProvisional() != true || (Rating{RD: 60}).IsProvisional() {
		t.Error("provisional flag")
	}
}
