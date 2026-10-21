package trainer

import (
	"fmt"
	"time"

	"dyanis-chess/backend/internal/stats"
)

// PlanItem is one point of the plan for today.
type PlanItem struct {
	ID    string `json:"id"`
	Kind  string `json:"kind"` // puzzles, repertoire, review, play, lesson
	Title string `json:"title"`
	Text  string `json:"text"`
	Done  bool   `json:"done"`
	// Progress of Goal; both 0 for a point that cannot be measured (the lesson).
	Progress    int    `json:"progress"`
	Goal        int    `json:"goal"`
	Href        string `json:"href"`
	ActionLabel string `json:"actionLabel"`
	// Theme is the puzzle theme the link opens (the page chooses it before it goes there).
	Theme string `json:"theme,omitempty"`
	// Tracked says whether the site can tell that the point is done; the lesson cannot be told yet.
	Tracked bool `json:"tracked"`
}

// Day is how much of the plan is done.
type Day struct {
	Done  int `json:"done"`
	Total int `json:"total"`
}

// Repo is a repository of moves in the plan.
type Repo struct {
	ID   int64
	Name string
	Due  int // cards that are due
	New  int // cards never asked
	Mine int // the person's own cards
}

// RecentGame is a game of the last days that could be reviewed.
type RecentGame struct {
	ID       string
	Opponent string
	Result   string // "win", "loss", "draw" for the person
	Reviewed bool
	At       time.Time
}

// PlanInput is what the plan is made of: today's counts and the person's materials.
type PlanInput struct {
	// Newcomer: the person has no games, reviews or puzzles yet; they get the starting plan.
	Newcomer         bool
	WeakTheme        *stats.Theme // the theme with the most failures, nil when there is none
	PuzzlesToday     int          // puzzles tried today
	ThemeToday       int          // of them, on WeakTheme
	Repos            []Repo
	RepReviewedToday int
	Games            []RecentGame // the last RecentGameDays days, newest first
	ReviewsToday     int          // reviews the person started today
}

// BuildPlan makes the plan for the day: puzzles, the repositories, a game to review (or to play),
// and a lesson last. A newcomer's plan starts with the lesson.
func BuildPlan(in PlanInput) []PlanItem {
	lesson := lessonItem(in.Newcomer)
	puzzles := puzzleItem(in)
	review := reviewItem(in)
	if in.Newcomer {
		return []PlanItem{lesson, puzzles, review}
	}
	var out []PlanItem
	out = append(out, puzzles)
	if it, ok := repertoireItem(in); ok {
		out = append(out, it)
	}
	return append(out, review, lesson)
}

// Tally counts the measurable points done and all measurable points.
func Tally(plan []PlanItem) Day {
	var d Day
	for _, it := range plan {
		if !it.Tracked {
			continue
		}
		d.Total++
		if it.Done {
			d.Done++
		}
	}
	return d
}

func lessonItem(newcomer bool) PlanItem {
	text := "Короткий урок по шахматам: пять минут в день дают больше, чем час раз в месяц."
	if newcomer {
		text = "Начните с урока: он объяснит основы и покажет, как устроен сайт."
	}
	return PlanItem{ID: "lesson", Kind: "lesson", Title: "Пройдите урок", Text: text, Href: "#lessons", ActionLabel: "К урокам"}
}

func puzzleItem(in PlanInput) PlanItem {
	if w := in.WeakTheme; w != nil && !in.Newcomer {
		progress := min(in.ThemeToday, PuzzleGoal)
		it := PlanItem{
			ID: "weak-theme", Kind: "puzzles", Tracked: true, Goal: PuzzleGoal, Progress: progress, Done: progress >= PuzzleGoal,
			Title: fmt.Sprintf("Решите %d задач по теме «%s»", PuzzleGoal, ThemeName(w.Theme)),
			Text:  fmt.Sprintf("Ваша самая слабая тема: неудачных первых попыток %d из %d.", w.Failed, w.Attempts),
			Href:  "#puzzles/themes", ActionLabel: "Решать", Theme: w.Theme,
		}
		if it.Done {
			it.Text = "Норма по слабой теме выполнена, отличная работа."
		}
		return it
	}
	progress := min(in.PuzzlesToday, PuzzleGoal)
	it := PlanItem{
		ID: "puzzles", Kind: "puzzles", Tracked: true, Goal: PuzzleGoal, Progress: progress, Done: progress >= PuzzleGoal,
		Title: fmt.Sprintf("Решите %d задач", PuzzleGoal), Text: "Задачи подбираются под ваш рейтинг, а день с задачей продлевает серию.",
		Href: "#puzzles", ActionLabel: "Решать",
	}
	if in.Newcomer {
		it.Text = "Сайт подберёт задачи по вашей силе и покажет, где вы ошибаетесь."
	}
	if it.Done {
		it.Text = "Задачи на сегодня решены, серия дней продолжается."
	}
	return it
}

func repertoireItem(in PlanInput) (PlanItem, bool) {
	var due, fresh, mine int
	var bestDue, bestNew *Repo
	for i := range in.Repos {
		r := &in.Repos[i]
		due += r.Due
		fresh += r.New
		mine += r.Mine
		if r.Due > 0 && (bestDue == nil || r.Due > bestDue.Due) {
			bestDue = r
		}
		if r.New > 0 && (bestNew == nil || r.New > bestNew.New) {
			bestNew = r
		}
	}
	if mine == 0 {
		return PlanItem{}, false // no repository, or one without a move of the person's own
	}
	trainHref := func(r *Repo) string { return fmt.Sprintf("#openings/%d/train", r.ID) }
	switch {
	case due > 0:
		goal := min(RepeatCap, in.RepReviewedToday+due)
		progress := min(in.RepReviewedToday, goal)
		return PlanItem{
			ID: "repertoire", Kind: "repertoire", Tracked: true, Goal: goal, Progress: progress, Done: progress >= goal,
			Title: "Повторите ходы дебютов",
			Text:  fmt.Sprintf("Пора повторить %s. Больше всего в репозитории «%s»: %d.", cards(due), bestDue.Name, bestDue.Due),
			Href:  trainHref(bestDue), ActionLabel: "Повторять",
		}, true
	case in.RepReviewedToday > 0:
		n := in.RepReviewedToday
		return PlanItem{
			ID: "repertoire", Kind: "repertoire", Tracked: true, Goal: n, Progress: n, Done: true,
			Title: "Повторите ходы дебютов", Text: fmt.Sprintf("Повторено сегодня: %s. Просроченных больше нет.", cards(n)),
			Href: "#openings", ActionLabel: "К дебютам",
		}, true
	case fresh > 0:
		return PlanItem{
			ID: "repertoire-new", Kind: "repertoire", Tracked: true, Goal: NewMovesGoal, Progress: 0,
			Title: fmt.Sprintf("Выучите %d новых хода", NewMovesGoal),
			Text:  fmt.Sprintf("Повторять пока нечего, зато в репозитории «%s» есть ходы, которых вы ещё не учили.", bestNew.Name),
			Href:  trainHref(bestNew), ActionLabel: "Учить",
		}, true
	}
	return PlanItem{}, false // everything rests until later
}

// cards writes "1 карточку", "3 карточки", "7 карточек".
func cards(n int) string {
	return fmt.Sprintf("%d %s", n, Plural(n, "карточку", "карточки", "карточек"))
}

func reviewItem(in PlanInput) PlanItem {
	if len(in.Games) == 0 {
		it := PlanItem{
			ID: "play", Kind: "play", Tracked: true, Goal: 1, Title: "Сыграйте партию",
			Text: "За последнюю неделю партий на сайте нет. Сыграйте с ботом: потом движок разберёт её и покажет ошибки.",
			Href: "#bots", ActionLabel: "Играть с ботом",
		}
		if in.Newcomer {
			it.Title = "Сыграйте партию с ботом"
			it.Text = "Выберите бота по силе и сыграйте: после партии её можно разобрать с движком."
		}
		return it
	}
	var unreviewed *RecentGame
	for i := range in.Games {
		if !in.Games[i].Reviewed {
			unreviewed = &in.Games[i]
			break
		}
	}
	it := PlanItem{ID: "review", Kind: "review", Tracked: true, Goal: 1, Title: "Разберите партию", Href: "#profile", ActionLabel: "К партиям"}
	switch {
	case in.ReviewsToday > 0:
		it.Done, it.Progress = true, 1
		it.Text = "Сегодня вы уже разобрали партию: так ошибки запоминаются."
	case unreviewed == nil:
		it.Done, it.Progress = true, 1
		it.Text = "Свои последние партии вы уже разобрали. Сыграйте ещё одну, и будет что разбирать."
		it.Href, it.ActionLabel = "#bots", "Сыграть ещё"
	default:
		it.Text = fmt.Sprintf("Партия с соперником %s (%s) ещё не разобрана. Найдём, где она решилась.", opponentName(unreviewed.Opponent), resultWord(unreviewed.Result))
		it.Href, it.ActionLabel = "#online/"+unreviewed.ID, "Открыть партию"
	}
	return it
}

func opponentName(n string) string {
	if n == "" {
		return "соперника"
	}
	return n
}

func resultWord(r string) string {
	switch r {
	case "win":
		return "победа"
	case "loss":
		return "поражение"
	}
	return "ничья"
}
