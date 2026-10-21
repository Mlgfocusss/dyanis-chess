package trainer

import (
	"strings"
	"testing"

	"dyanis-chess/backend/internal/stats"
)

func fp(x float64) *float64 { return &x }

func good() Insights {
	return Insights{
		Enough: true, Used: 5, Average: fp(82),
		Phases: []PhaseStat{{ID: "opening", Name: "Дебют", Accuracy: fp(88), Moves: 40}, {ID: "middlegame", Name: "Миттельшпиль", Accuracy: fp(84), Moves: 60}, {ID: "endgame", Name: "Эндшпиль", Accuracy: fp(61), Moves: 30}},
		Rate:   Rate{Moves: 200, Blunders: 1},
	}
}

func adviceIDs(a []Advice) string {
	var out []string
	for _, x := range a {
		out = append(out, x.ID)
	}
	return strings.Join(out, ",")
}

func TestAdvicePhase(t *testing.T) {
	a := BuildAdvice(AdviceInput{Insights: good(), Streak: stats.Streak{Current: 3}})
	if adviceIDs(a) != "phase-endgame,keep" {
		t.Fatalf("advice: %s", adviceIDs(a))
	}
	p := a[0]
	if p.Text != "В эндшпиле точность 61% против 88% в дебюте." || p.Title != "Слабая фаза: эндшпиль" || p.Theme != "endgame" || p.Href != "#puzzles/themes" {
		t.Errorf("phase card: %+v", p)
	}
	if len(p.More) != 1 || p.More[0].Href != "#lessons" {
		t.Errorf("lessons button: %+v", p.More)
	}

	// the gap is a threshold: 7,9 points is not weak, 8 is
	in := good()
	in.Phases[2].Accuracy = fp(88 - PhaseGap + 0.1)
	if a := BuildAdvice(AdviceInput{Insights: in, Streak: stats.Streak{Current: 3}}); strings.Contains(adviceIDs(a), "phase") {
		t.Errorf("below the gap: %s", adviceIDs(a))
	}
	in.Phases[2].Accuracy = fp(88 - PhaseGap)
	if a := BuildAdvice(AdviceInput{Insights: in, Streak: stats.Streak{Current: 3}}); !strings.Contains(adviceIDs(a), "phase-endgame") {
		t.Errorf("at the gap: %s", adviceIDs(a))
	}
	// the opening is sent to the repositories, the middlegame to its puzzles
	in = good()
	in.Phases[0].Accuracy, in.Phases[2].Accuracy = fp(60), fp(90)
	if a := BuildAdvice(AdviceInput{Insights: in, Streak: stats.Streak{Current: 1}}); a[0].ID != "phase-opening" || a[0].Href != "#openings" {
		t.Errorf("opening: %+v", a[0])
	}
	in.Phases[0].Accuracy, in.Phases[1].Accuracy = fp(90), fp(60)
	if a := BuildAdvice(AdviceInput{Insights: in, Streak: stats.Streak{Current: 1}}); a[0].ID != "phase-middlegame" || a[0].Theme != "middlegame" {
		t.Errorf("middlegame: %+v", a[0])
	}
	// phases without a number are not compared; one phase alone has nothing to be compared with
	in = good()
	in.Phases[0].Accuracy, in.Phases[1].Accuracy = nil, nil
	if a := BuildAdvice(AdviceInput{Insights: in, Streak: stats.Streak{Current: 1}}); strings.Contains(adviceIDs(a), "phase") {
		t.Errorf("one phase: %s", adviceIDs(a))
	}
	// not enough reviews: no verdict on the play
	in = good()
	in.Enough = false
	if a := BuildAdvice(AdviceInput{Insights: in, Streak: stats.Streak{Current: 1}}); strings.Contains(adviceIDs(a), "phase") || a[0].ID != "keep" {
		t.Errorf("not enough: %s", adviceIDs(a))
	}
}

func TestAdviceBlunders(t *testing.T) {
	in := good()
	in.Rate.Blunders = 3.4
	a := BuildAdvice(AdviceInput{Insights: in, Streak: stats.Streak{Current: 1}})
	if a[0].ID != "blunders" || !strings.Contains(a[0].Text, "3,4 зевка на 100 ходов") || a[0].ActionLabel != "Тактические задачи" || a[0].More[0].Href != "#puzzles/rush" {
		t.Errorf("blunders: %+v", a[0])
	}
	in.Rate.Blunders = BlundersPer100 - 0.1
	if a := BuildAdvice(AdviceInput{Insights: in, Streak: stats.Streak{Current: 1}}); strings.Contains(adviceIDs(a), "blunders") {
		t.Errorf("below the rate: %s", adviceIDs(a))
	}
	if blunders(3) != "3 зевка" || blunders(5) != "5 зевков" || blunders(1) != "1 зевок" {
		t.Errorf("words: %s %s %s", blunders(3), blunders(5), blunders(1))
	}
}

func TestAdviceThemes(t *testing.T) {
	weak := []stats.Theme{
		{Theme: "fork", Attempts: 5, Failed: 3},   // 60%
		{Theme: "pin", Attempts: 4, Failed: 2},    // 50%
		{Theme: "skewer", Attempts: 2, Failed: 2}, // too few tries
		{Theme: "mate", Attempts: 10, Failed: 3},  // 30%: not weak
		{Theme: "deflection", Attempts: 3, Failed: 1},
		{Theme: "sacrifice", Attempts: 5, Failed: 2},
		{Theme: "zugzwang", Attempts: 5, Failed: 2},
	}
	a := BuildAdvice(AdviceInput{Insights: good(), WeakThemes: weak, Streak: stats.Streak{Current: 2}})
	var th *Advice
	for i := range a {
		if a[i].ID == "themes" {
			th = &a[i]
		}
	}
	if th == nil {
		t.Fatalf("no themes card: %s", adviceIDs(a))
	}
	// up to three, with their share of failures: the second and third are 50% and 40%
	if th.Text != "Доля неудачных первых попыток: Вилка 60%, Связка 50%, Жертва 40%." || th.Theme != "fork" || th.ActionLabel != "Тренировать «Вилка»" {
		t.Errorf("themes: %+v", th)
	}
	if a := BuildAdvice(AdviceInput{Insights: good(), WeakThemes: weak[3:5], Streak: stats.Streak{Current: 2}}); strings.Contains(adviceIDs(a), "themes") {
		t.Errorf("no weak themes: %s", adviceIDs(a))
	}
}

func TestAdviceOpeningsAndStreak(t *testing.T) {
	cases := []struct {
		d    Deviation
		want bool
	}{{Deviation{Compared: 9, Deviated: 4}, true}, {Deviation{Compared: 10, Deviated: 3}, true}, {Deviation{Compared: 10, Deviated: 2}, false},
		{Deviation{Compared: 2, Deviated: 2}, false}, {Deviation{}, false}}
	for _, c := range cases {
		a := BuildAdvice(AdviceInput{Insights: good(), Deviation: c.d, Streak: stats.Streak{Current: 2}})
		if has := strings.Contains(adviceIDs(a), "openings"); has != c.want {
			t.Errorf("%+v: %s", c.d, adviceIDs(a))
		}
	}
	a := BuildAdvice(AdviceInput{Insights: good(), Deviation: Deviation{Compared: 9, Deviated: 4}, Streak: stats.Streak{Current: 2}})
	for _, x := range a {
		if x.ID == "openings" && !strings.Contains(x.Text, "В 4 из 9 недавних партий") {
			t.Errorf("openings: %+v", x)
		}
	}

	// no streak
	a = BuildAdvice(AdviceInput{Insights: good(), Streak: stats.Streak{}})
	if !strings.Contains(adviceIDs(a), "streak") {
		t.Fatalf("no streak: %s", adviceIDs(a))
	}
	for _, x := range a {
		if x.ID == "streak" && (x.Title != "Решайте задачу каждый день" || !strings.Contains(x.Text, "хотя бы одну задачу в день")) {
			t.Errorf("streak: %+v", x)
		}
	}
	a = BuildAdvice(AdviceInput{Insights: good(), Streak: stats.Streak{Best: 6}})
	for _, x := range a {
		if x.ID == "streak" && !strings.Contains(x.Text, "6 дней") {
			t.Errorf("best streak: %+v", x)
		}
	}
	if a := BuildAdvice(AdviceInput{Insights: good(), Streak: stats.Streak{Current: 1}}); strings.Contains(adviceIDs(a), "streak") {
		t.Errorf("with a streak: %s", adviceIDs(a))
	}
}

func TestAdviceBounds(t *testing.T) {
	// a newcomer gets none
	if a := BuildAdvice(AdviceInput{Newcomer: true, Insights: good()}); a == nil || len(a) != 0 {
		t.Errorf("newcomer: %v", a)
	}
	// everything wrong at once: five cards at most, the most important first
	in := good()
	in.Rate.Blunders = 5
	all := BuildAdvice(AdviceInput{
		Insights: in, WeakThemes: []stats.Theme{{Theme: "fork", Attempts: 5, Failed: 4}},
		Deviation: Deviation{Compared: 10, Deviated: 8}, Streak: stats.Streak{},
	})
	if adviceIDs(all) != "blunders,phase-endgame,themes,openings,streak" || len(all) != MaxAdvice {
		t.Errorf("all: %s", adviceIDs(all))
	}
	// nothing wrong: at least MinAdvice cards, the last one says to carry on
	calm := BuildAdvice(AdviceInput{Insights: Insights{Enough: true, Average: fp(90), Phases: good().Phases[:0], Rate: Rate{Moves: 100}}, Streak: stats.Streak{Current: 4}})
	if len(calm) != 1 || calm[0].ID != "keep" || !strings.Contains(calm[0].Text, "90") || !strings.Contains(calm[0].Text, "Продолжайте") {
		t.Errorf("calm: %+v", calm)
	}
	// one card from the rules is padded to the minimum
	one := BuildAdvice(AdviceInput{Insights: good(), Streak: stats.Streak{Current: 4}})
	if len(one) != MinAdvice || one[1].ID != "keep" {
		t.Errorf("padded: %s", adviceIDs(one))
	}
	// not enough reviews: the card asks for games
	few := BuildAdvice(AdviceInput{Insights: Insights{Used: 1, Need: MinReviews}, Streak: stats.Streak{Current: 1}})
	if len(few) != 1 || few[0].Title != "Мне нужно больше партий" || few[0].Href != "#review" {
		t.Errorf("few: %+v", few)
	}
}
