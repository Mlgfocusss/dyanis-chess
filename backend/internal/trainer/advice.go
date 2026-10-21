package trainer

import (
	"fmt"
	"math"

	"dyanis-chess/backend/internal/stats"
)

// Link is a button of a recommendation.
type Link struct {
	Href  string `json:"href"`
	Label string `json:"label"`
	// Theme is the puzzle theme the link opens (the page chooses it before it goes there).
	Theme string `json:"theme,omitempty"`
}

// Advice is one card of "Что подтянуть".
type Advice struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Text        string `json:"text"`
	Href        string `json:"href"`
	ActionLabel string `json:"actionLabel"`
	Theme       string `json:"theme,omitempty"`
	// More are further buttons of the card.
	More []Link `json:"more,omitempty"`
}

// AdviceInput is what the recommendations are made of.
type AdviceInput struct {
	Newcomer   bool
	Insights   Insights
	WeakThemes []stats.Theme // most failures first, without technical themes
	Deviation  Deviation
	Streak     stats.Streak
}

// BuildAdvice applies the rules, in the order of how much they matter, and keeps MaxAdvice cards.
// A newcomer gets none: there is nothing to go on yet.
func BuildAdvice(in AdviceInput) []Advice {
	out := []Advice{}
	if in.Newcomer {
		return out
	}
	if a, ok := blunderAdvice(in.Insights); ok {
		out = append(out, a)
	}
	if a, ok := phaseAdvice(in.Insights); ok {
		out = append(out, a)
	}
	if a, ok := themeAdvice(in.WeakThemes); ok {
		out = append(out, a)
	}
	if a, ok := openingAdvice(in.Deviation); ok {
		out = append(out, a)
	}
	if a, ok := streakAdvice(in.Streak); ok {
		out = append(out, a)
	}
	if len(out) < MinAdvice {
		out = append(out, keepGoing(in.Insights, len(out) == 0))
	}
	if len(out) > MaxAdvice {
		out = out[:MaxAdvice]
	}
	return out
}

// blunders writes "3,4 зевка": a fraction takes the genitive singular.
func blunders(per100 float64) string {
	word := "зевка"
	if per100 == math.Trunc(per100) {
		word = Plural(int(per100), "зевок", "зевка", "зевков")
	}
	return Num(per100) + " " + word
}

func blunderAdvice(in Insights) (Advice, bool) {
	if !in.Enough || in.Rate.Blunders < BlundersPer100 {
		return Advice{}, false
	}
	return Advice{
		ID: "blunders", Title: "Слишком много зевков",
		Text: fmt.Sprintf("В среднем %s на 100 ходов: один такой ход часто решает партию. Перед ходом проверьте, что можно взять и чем грозит соперник.", blunders(in.Rate.Blunders)),
		Href: "#puzzles", ActionLabel: "Тактические задачи",
		More: []Link{{Href: "#puzzles/rush", Label: "Тактический штурм"}},
	}, true
}

var phasePrepositional = map[Phase]string{PhaseOpening: "дебюте", PhaseMiddlegame: "миттельшпиле", PhaseEndgame: "эндшпиле"}

func phaseAdvice(in Insights) (Advice, bool) {
	if !in.Enough {
		return Advice{}, false
	}
	var best, worst *PhaseStat
	for i := range in.Phases {
		p := &in.Phases[i]
		if p.Accuracy == nil {
			continue
		}
		if best == nil || *p.Accuracy > *best.Accuracy {
			best = p
		}
		if worst == nil || *p.Accuracy < *worst.Accuracy {
			worst = p
		}
	}
	if best == nil || worst == nil || *best.Accuracy-*worst.Accuracy < PhaseGap {
		return Advice{}, false
	}
	w := Phase(worst.ID)
	a := Advice{
		ID: "phase-" + worst.ID, Title: "Слабая фаза: " + lowerFirst(worst.Name),
		Text: fmt.Sprintf("В %s точность %s%% против %s%% в %s.", phasePrepositional[w], Num(*worst.Accuracy), Num(*best.Accuracy), phasePrepositional[Phase(best.ID)]),
		More: []Link{{Href: "#lessons", Label: "Уроки"}},
	}
	switch w {
	case PhaseOpening:
		a.Href, a.ActionLabel = "#openings", "Выучить дебюты"
	case PhaseMiddlegame:
		a.Href, a.ActionLabel, a.Theme = "#puzzles/themes", "Задачи на миттельшпиль", "middlegame"
	default:
		a.Href, a.ActionLabel, a.Theme = "#puzzles/themes", "Задачи на эндшпиль", "endgame"
	}
	return a, true
}

func lowerFirst(s string) string {
	r := []rune(s)
	if len(r) > 0 && r[0] >= 'А' && r[0] <= 'Я' {
		r[0] += 'а' - 'А'
	}
	return string(r)
}

func themeAdvice(weak []stats.Theme) (Advice, bool) {
	type row struct {
		theme   string
		percent int
	}
	var rows []row
	for _, t := range weak {
		if t.Attempts < ThemeMinAttempts {
			continue
		}
		p := int(math.Round(100 * float64(t.Failed) / float64(t.Attempts)))
		if p >= ThemeFailPercent {
			rows = append(rows, row{t.Theme, p})
		}
		if len(rows) == ThemeCards {
			break
		}
	}
	if len(rows) == 0 {
		return Advice{}, false
	}
	text := ""
	for i, r := range rows {
		if i > 0 {
			text += ", "
		}
		text += fmt.Sprintf("%s %d%%", ThemeName(r.theme), r.percent)
	}
	return Advice{
		ID: "themes", Title: "Слабые темы в задачах",
		Text: "Доля неудачных первых попыток: " + text + ".",
		Href: "#puzzles/themes", ActionLabel: "Тренировать «" + ThemeName(rows[0].theme) + "»", Theme: rows[0].theme,
	}, true
}

func openingAdvice(d Deviation) (Advice, bool) {
	if d.Compared < DeviationMinGames || 100*d.Deviated < DeviationPercent*d.Compared {
		return Advice{}, false
	}
	return Advice{
		ID: "openings", Title: "Вы сходите с линии дебюта",
		Text: fmt.Sprintf("В %d из %d недавних партий вы сыграли не тот ход, что в вашем репозитории. Повторите линии, чтобы рука не ошибалась.", d.Deviated, d.Compared),
		Href: "#openings", ActionLabel: "К дебютам",
	}, true
}

func streakAdvice(s stats.Streak) (Advice, bool) {
	if s.Current > 0 {
		return Advice{}, false
	}
	text := "Серии дней пока нет. Решайте хотя бы одну задачу в день, и она начнётся: привычка важнее длинных занятий раз в неделю."
	if s.Best > 0 {
		text = fmt.Sprintf("Ваша лучшая серия была %d %s, сейчас её нет. Решайте хотя бы одну задачу в день, чтобы вернуть привычку.", s.Best, Plural(s.Best, "день", "дня", "дней"))
	}
	return Advice{ID: "streak", Title: "Решайте задачу каждый день", Text: text, Href: "#puzzles", ActionLabel: "Решить задачу"}, true
}

func keepGoing(in Insights, nothing bool) Advice {
	a := Advice{ID: "keep", Title: "Продолжайте в том же духе", Href: "#review", ActionLabel: "Разобрать партию"}
	switch {
	case !in.Enough:
		a.Title = "Мне нужно больше партий"
		a.Text = fmt.Sprintf("Разберите ещё несколько партий (нужно хотя бы %d с вашим именем), и я начну замечать, в какой фазе и на каких ходах вы теряете очки.", MinReviews)
	case nothing && in.Average != nil:
		a.Text = fmt.Sprintf("Явных слабых мест не вижу, средняя точность %s%%. Продолжайте играть, разбирать партии и решать задачи.", Num(*in.Average))
	default:
		a.Text = "Разбирайте каждую важную партию и решайте задачи: так слабые места находятся раньше, чем успевают повториться."
	}
	return a
}
