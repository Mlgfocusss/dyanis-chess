package trainer

import (
	"strings"
	"testing"
	"time"

	"dyanis-chess/backend/internal/stats"
)

func ids(plan []PlanItem) string {
	var out []string
	for _, it := range plan {
		out = append(out, it.ID)
	}
	return strings.Join(out, ",")
}

func find(plan []PlanItem, id string) *PlanItem {
	for i := range plan {
		if plan[i].ID == id {
			return &plan[i]
		}
	}
	return nil
}

func TestPlanForANewcomer(t *testing.T) {
	plan := BuildPlan(PlanInput{Newcomer: true})
	if ids(plan) != "lesson,puzzles,play" {
		t.Fatalf("plan: %s", ids(plan))
	}
	if plan[0].Href != "#lessons" || plan[1].Href != "#puzzles" || plan[2].Href != "#bots" || plan[2].Title != "Сыграйте партию с ботом" {
		t.Errorf("links: %+v", plan)
	}
	if d := Tally(plan); d.Total != 2 || d.Done != 0 {
		t.Errorf("the lesson cannot be measured: %+v", d)
	}
	// even with a weak theme the newcomer is not sent to a theme
	if p := BuildPlan(PlanInput{Newcomer: true, WeakTheme: &stats.Theme{Theme: "fork", Attempts: 3, Failed: 3}}); p[1].Theme != "" {
		t.Errorf("newcomer's puzzles: %+v", p[1])
	}
}

func TestPlanPuzzles(t *testing.T) {
	weak := &stats.Theme{Theme: "fork", Attempts: 7, Failed: 4}
	plan := BuildPlan(PlanInput{WeakTheme: weak, PuzzlesToday: 9, ThemeToday: 3})
	it := find(plan, "weak-theme")
	if it == nil || ids(plan) != "weak-theme,play,lesson" {
		t.Fatalf("plan: %s", ids(plan))
	}
	if it.Title != "Решите 5 задач по теме «Вилка»" || it.Goal != 5 || it.Progress != 3 || it.Done || it.Theme != "fork" || it.Href != "#puzzles/themes" || !it.Tracked {
		t.Errorf("weak theme: %+v", it)
	}
	if !strings.Contains(it.Text, "4 из 7") {
		t.Errorf("text: %q", it.Text)
	}
	// puzzles on other themes do not move this bar; the bar stops at the goal
	done := find(BuildPlan(PlanInput{WeakTheme: weak, ThemeToday: 8}), "weak-theme")
	if !done.Done || done.Progress != 5 {
		t.Errorf("done: %+v", done)
	}

	// no weak theme: five puzzles of any kind
	any := find(BuildPlan(PlanInput{PuzzlesToday: 2}), "puzzles")
	if any == nil || any.Goal != 5 || any.Progress != 2 || any.Href != "#puzzles" || any.Theme != "" || any.Done {
		t.Errorf("any puzzles: %+v", any)
	}
}

func TestPlanRepertoire(t *testing.T) {
	repos := []Repo{{ID: 3, Name: "Сицилианская", Due: 2, New: 1, Mine: 10}, {ID: 5, Name: "Итальянская", Due: 7, New: 0, Mine: 6}}
	it := find(BuildPlan(PlanInput{Repos: repos, RepReviewedToday: 4}), "repertoire")
	if it == nil || it.Href != "#openings/5/train" || it.Goal != 13 || it.Progress != 4 || it.Done {
		t.Fatalf("due: %+v", it)
	}
	if !strings.Contains(it.Text, "9 карточек") || !strings.Contains(it.Text, "«Итальянская»: 7") {
		t.Errorf("text: %q", it.Text)
	}
	// the goal is capped, the bar follows it
	big := find(BuildPlan(PlanInput{Repos: []Repo{{ID: 1, Name: "A", Due: 90, Mine: 90}}, RepReviewedToday: 25}), "repertoire")
	if big.Goal != RepeatCap || big.Progress != RepeatCap || !big.Done {
		t.Errorf("cap: %+v", big)
	}
	// everything repeated today
	rested := find(BuildPlan(PlanInput{Repos: []Repo{{ID: 1, Name: "A", Mine: 4}}, RepReviewedToday: 6}), "repertoire")
	if rested == nil || !rested.Done || rested.Goal != 6 || rested.Progress != 6 {
		t.Errorf("rested: %+v", rested)
	}
	// nothing due, nothing repeated, new moves waiting
	fresh := find(BuildPlan(PlanInput{Repos: []Repo{{ID: 8, Name: "Новая", New: 5, Mine: 5}}}), "repertoire-new")
	if fresh == nil || fresh.Goal != NewMovesGoal || fresh.Href != "#openings/8/train" || fresh.Done {
		t.Errorf("new moves: %+v", fresh)
	}
	// nothing to do, no repository, or a repository without moves of one's own: no point
	for name, in := range map[string]PlanInput{
		"resting": {Repos: []Repo{{ID: 1, Name: "A", Mine: 4}}},
		"none":    {},
		"empty":   {Repos: []Repo{{ID: 1, Name: "A"}}},
	} {
		if p := BuildPlan(in); find(p, "repertoire") != nil || find(p, "repertoire-new") != nil {
			t.Errorf("%s: %s", name, ids(p))
		}
	}
}

func TestPlanReview(t *testing.T) {
	at := time.Now()
	games := []RecentGame{
		{ID: "g2", Opponent: "Лиза", Result: "loss", Reviewed: false, At: at},
		{ID: "g1", Opponent: "Борис", Result: "win", Reviewed: true, At: at.Add(-time.Hour)},
	}
	it := find(BuildPlan(PlanInput{Games: games}), "review")
	if it == nil || it.Done || it.Href != "#online/g2" || !strings.Contains(it.Text, "Лиза") || !strings.Contains(it.Text, "поражение") {
		t.Fatalf("latest unreviewed: %+v", it)
	}
	// the newest game is reviewed already: the next unreviewed one is offered
	games[0].Reviewed = true
	games[1].Reviewed = false
	if it := find(BuildPlan(PlanInput{Games: games}), "review"); it.Href != "#online/g1" {
		t.Errorf("older unreviewed: %+v", it)
	}
	// all reviewed
	games[1].Reviewed = true
	if it := find(BuildPlan(PlanInput{Games: games}), "review"); !it.Done || it.Progress != 1 || it.Href != "#bots" {
		t.Errorf("all reviewed: %+v", it)
	}
	// a review made today ends the point whatever is left
	games[0].Reviewed = false
	if it := find(BuildPlan(PlanInput{Games: games, ReviewsToday: 1}), "review"); !it.Done {
		t.Errorf("reviewed today: %+v", it)
	}
	// no games for a week: play one
	p := find(BuildPlan(PlanInput{}), "play")
	if p == nil || p.Href != "#bots" || p.Done || p.Title != "Сыграйте партию" {
		t.Errorf("play: %+v", p)
	}
}

func TestPlanOrderAndTally(t *testing.T) {
	in := PlanInput{
		WeakTheme: &stats.Theme{Theme: "pin", Attempts: 4, Failed: 3}, ThemeToday: 5,
		Repos: []Repo{{ID: 1, Name: "A", Due: 3, Mine: 8}}, RepReviewedToday: 3,
		Games: []RecentGame{{ID: "g", Reviewed: true}},
	}
	plan := BuildPlan(in)
	if ids(plan) != "weak-theme,repertoire,review,lesson" {
		t.Fatalf("order: %s", ids(plan))
	}
	if d := Tally(plan); d.Total != 3 || d.Done != 2 {
		t.Errorf("tally: %+v", d)
	}
	if last := plan[len(plan)-1]; last.Tracked || last.Goal != 0 || last.Href != "#lessons" {
		t.Errorf("lesson: %+v", last)
	}
	// the lesson is always last
	for _, in := range []PlanInput{{}, {Newcomer: false, PuzzlesToday: 3}, {Repos: []Repo{{ID: 1, Mine: 3, New: 3}}}} {
		if p := BuildPlan(in); p[len(p)-1].ID != "lesson" {
			t.Errorf("not last: %s", ids(p))
		}
	}
}
