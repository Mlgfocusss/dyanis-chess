package trainer

import (
	"fmt"
	"strings"
)

// Greeting is what Dionis says at the top of the page: a hello by the time of day and a few short
// lines that depend on the person's data.
type Greeting struct {
	Hello string   `json:"hello"`
	Lines []string `json:"lines"`
}

// GreetingInput is what the phrases depend on.
type GreetingInput struct {
	Name     string
	Hour     int // the person's local hour, 0..23
	Newcomer bool
	Streak   int  // the current streak of days
	Best     int  // the best streak
	Today    bool // a puzzle was done today
	// Yesterday is the number of puzzles done yesterday.
	Yesterday int
	Trend     *Trend
	Day       Day
}

// HelloFor is the greeting for an hour of the day.
func HelloFor(hour int) string {
	switch {
	case hour >= 5 && hour < 12:
		return "Доброе утро"
	case hour >= 12 && hour < 18:
		return "Добрый день"
	case hour >= 18 && hour < 23:
		return "Добрый вечер"
	}
	return "Доброй ночи"
}

// Greet makes the words of Dionis. The phrases are picked by rules from the figures, so they are
// short and always about something that is true.
func Greet(in GreetingInput) Greeting {
	hello := HelloFor(in.Hour)
	if n := strings.TrimSpace(in.Name); n != "" {
		hello += ", " + n
	}
	g := Greeting{Hello: hello + "!", Lines: []string{}}
	if in.Newcomer {
		g.Lines = append(g.Lines,
			"Я Дионис, ваш тренер. Пока я вас не знаю, поэтому вот простой план на сегодня.",
			fmt.Sprintf("Сыграйте партию и разберите её. Когда наберётся %d разбора, я покажу ваши сильные и слабые стороны.", MinReviews))
		return g
	}

	switch {
	case in.Streak > 0 && in.Today:
		g.Lines = append(g.Lines, fmt.Sprintf("Серия: %d %s, и сегодня вы уже решали задачи. Так держать!", in.Streak, Plural(in.Streak, "день", "дня", "дней")))
	case in.Streak > 0:
		g.Lines = append(g.Lines, fmt.Sprintf("Серия: %d %s. Решите сегодня хотя бы одну задачу, чтобы не прервать её.", in.Streak, Plural(in.Streak, "день", "дня", "дней")))
	case in.Best > 0:
		g.Lines = append(g.Lines, "Серия прервалась, ничего страшного: одна задача сегодня начнёт новую.")
	default:
		g.Lines = append(g.Lines, "Серии дней пока нет. Одна задача в день, и она начнётся.")
	}

	switch {
	case in.Trend != nil && in.Trend.Delta >= 1:
		g.Lines = append(g.Lines, fmt.Sprintf("Ваша точность в последних партиях выросла: %s по сравнению с предыдущими. Это заметный успех.", in.Trend.Text))
	case in.Trend != nil && in.Trend.Delta <= -2:
		g.Lines = append(g.Lines, fmt.Sprintf("Точность в последних партиях просела (%s). Давайте разберёмся, где вы теряете очки.", in.Trend.Text))
	case in.Yesterday > 0:
		g.Lines = append(g.Lines, fmt.Sprintf("Вчера вы занимались задачами: %d %s. Хороший темп.", in.Yesterday, Plural(in.Yesterday, "задача", "задачи", "задач")))
	case in.Streak == 0:
		g.Lines = append(g.Lines, "Вчера задач не было. Сегодня хороший день, чтобы вернуться.")
	}

	switch {
	case in.Day.Total > 0 && in.Day.Done == in.Day.Total:
		g.Lines = append(g.Lines, "План на сегодня выполнен. Можно отдохнуть или сыграть для удовольствия.")
	case in.Day.Total > 0:
		g.Lines = append(g.Lines, fmt.Sprintf("Сегодня выполнено %d из %d.", in.Day.Done, in.Day.Total))
	}
	return g
}
