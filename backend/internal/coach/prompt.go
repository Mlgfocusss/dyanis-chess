package coach

import (
	"fmt"
	"strings"

	"dyanis-chess/backend/internal/analysis"
)

const systemPrompt = `Ты шахматный тренер на сайте Dyanis Chess. Ученик разбирает свою партию. Тебе даны его ход и точные данные шахматного движка: оценки и лучшие линии. Объясни ход человеческим языком, по-русски.

Правила.
1. Опирайся только на данные ниже. Не придумывай ходов, вариантов и угроз, которых нет в линиях. Называй ходы только те, что есть в данных, в обычной нотации (Nf3, exd5).
2. Не пиши оценку в пешках числом. Говори словами («теряете фигуру», «позиция остаётся равной», «получаете мат») и можно называть шансы на победу в процентах. Материал назван фигурами («лёгкая фигура», «ладья», «ферзь», «пешка»): пиши так же словами, числа очков и «пешек» вместо фигуры не используй. Обращайся к ученику на «вы», никогда на «ты».
3. Ход плохой (неточность, ошибка, зевок, упущенная возможность): в первом абзаце объясни, что именно не так с ходом и чем этим пользуется соперник. Назови первые два-три хода его линии и чем она кончается: потерей материала, матом, сильной атакой. Во втором абзаце скажи, что нужно было играть вместо этого и в чём идея лучшего хода.
4. Ход хороший, лучший, блестящий или сильный: одним абзацем объясни идею хода и чего он добивается. Если это теория дебюта, так и скажи и назови идею.
5. Причину называй только ту, что видна из линии. Если причина неочевидна, опиши результат линии, не выдумывая мотивов.
6. Всего 3-5 предложений, не больше 110 слов. Простой язык, без разметки, списков, заголовков и эмодзи.`

var classRU = map[analysis.Class]string{
	analysis.Brilliant:  "блестящий ход",
	analysis.Great:      "сильный ход (единственный хороший)",
	analysis.Best:       "лучший ход",
	analysis.Excellent:  "отличный ход",
	analysis.Good:       "хороший ход",
	analysis.Book:       "ход из теории дебюта",
	analysis.Inaccuracy: "неточность",
	analysis.Mistake:    "ошибка",
	analysis.Blunder:    "зевок (грубая ошибка)",
	analysis.Miss:       "упущенная возможность",
}

// Prompt writes the question put to the model for these facts.
func Prompt(f Facts) string {
	white := f.Mover == "white"
	who := "белые"
	if !white {
		who = "чёрные"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Ход %d, ходят %s.\n\nПозиция до хода.\n%s\n\n", f.MoveNumber, who, f.Position)

	label := classRU[f.Class]
	if label == "" {
		label = string(f.Class)
	}
	fmt.Fprintf(&b, "Ход ученика: %s. Движок оценил его как: %s.\n", Numbered(f.MoveNumber, white, []string{f.Played}), label)
	if f.Forced {
		b.WriteString("Это был единственный допустимый ход.\n")
	}
	if f.Book && f.Opening != "" {
		fmt.Fprintf(&b, "Дебют: %s (это теория).\n", f.Opening)
	}
	fmt.Fprintf(&b, "Шансы ученика на победу: до хода %.0f%%, после хода %.0f%%", f.WinBefore, f.WinAfter)
	if f.Loss >= 1 {
		fmt.Fprintf(&b, " (ход потерял %.0f%%)", f.Loss)
	}
	fmt.Fprintf(&b, ".\nОценка позиции для ученика (в пешках, плюс значит, что он лучше): до хода %s, после хода %s.\n", f.EvalBefore, f.EvalAfter)

	switch {
	case f.BestSAN != "":
		fmt.Fprintf(&b, "\nЛучший ход по движку: %s.\nЛучшая линия: %s.\n", f.BestSAN, Numbered(f.MoveNumber, white, f.BestLine))
		if d := f.MaterialBest - f.MaterialBefore; d != 0 {
			fmt.Fprintf(&b, "К концу лучшей линии материал ученика %s.\n", materialWords(d))
		}
	case len(f.BestLine) > 0:
		fmt.Fprintf(&b, "\nХод ученика совпал с первым ходом лучшей линии движка: %s.\n", Numbered(f.MoveNumber, white, f.BestLine))
	}

	if f.Terminal != "" {
		fmt.Fprintf(&b, "\nПосле этого хода партия закончилась: %s.\n", terminalWords(f.Terminal))
	} else if len(f.ReplyLine) > 0 {
		// After the mover's move the opponent is on the move.
		n := f.MoveNumber
		if !white {
			n++
		}
		fmt.Fprintf(&b, "\nКак соперник отвечает на ход ученика (линия движка): %s.\n", Numbered(n, !white, f.ReplyLine))
		if d := f.MaterialReply - f.MaterialBefore; d != 0 {
			fmt.Fprintf(&b, "К концу этой линии материал ученика %s.\n", materialWords(d))
		}
	}
	b.WriteString("\nОбъясни этот ход ученику.")
	return b.String()
}

func materialWords(d int) string {
	if d > 0 {
		return "растёт: " + materialAmount(d)
	}
	return "уменьшается: " + materialAmount(-d)
}

// materialAmount says a material difference, counted in pawns, as the pieces it is worth.
func materialAmount(n int) string {
	switch n {
	case 1:
		return "на одну пешку"
	case 2:
		return "на две пешки"
	case 3:
		return "на лёгкую фигуру (коня или слона)"
	case 4:
		return "на лёгкую фигуру и пешку"
	case 5:
		return "на ладью"
	}
	if n >= 9 {
		return "на ферзя или больше"
	}
	return "на " + pawns(n) + " (примерно фигура и пешки)"
}

// pawns is "1 пешку", "3 пешки", "5 пешек".
func pawns(n int) string {
	switch {
	case n%100 >= 11 && n%100 <= 14:
		return fmt.Sprintf("%d пешек", n)
	case n%10 == 1:
		return fmt.Sprintf("%d пешку", n)
	case n%10 >= 2 && n%10 <= 4:
		return fmt.Sprintf("%d пешки", n)
	}
	return fmt.Sprintf("%d пешек", n)
}

func terminalWords(t string) string {
	switch t {
	case "checkmate":
		return "мат"
	case "stalemate":
		return "пат"
	}
	return "ничья"
}
