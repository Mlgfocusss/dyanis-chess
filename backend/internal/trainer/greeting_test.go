package trainer

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

func TestHelloFor(t *testing.T) {
	for h, want := range map[int]string{0: "Доброй ночи", 4: "Доброй ночи", 5: "Доброе утро", 11: "Доброе утро", 12: "Добрый день", 17: "Добрый день",
		18: "Добрый вечер", 22: "Добрый вечер", 23: "Доброй ночи"} {
		if got := HelloFor(h); got != want {
			t.Errorf("HelloFor(%d) = %q, want %q", h, got, want)
		}
	}
}

func TestGreet(t *testing.T) {
	g := Greet(GreetingInput{Name: "Анна", Hour: 19, Newcomer: true})
	if g.Hello != "Добрый вечер, Анна!" || len(g.Lines) != 2 || !strings.Contains(g.Lines[0], "Я Дионис") {
		t.Errorf("newcomer: %+v", g)
	}

	// a streak that goes on, and today is done
	g = Greet(GreetingInput{Name: "Анна", Hour: 8, Streak: 4, Best: 9, Today: true, Yesterday: 6, Day: Day{Done: 1, Total: 3}})
	if g.Hello != "Доброе утро, Анна!" || g.Lines[0] != "Серия: 4 дня, и сегодня вы уже решали задачи. Так держать!" {
		t.Errorf("streak: %+v", g)
	}
	if g.Lines[1] != "Вчера вы занимались задачами: 6 задач. Хороший темп." || g.Lines[2] != "Сегодня выполнено 1 из 3." {
		t.Errorf("lines: %+v", g.Lines)
	}
	// a streak at risk
	if g := Greet(GreetingInput{Streak: 5}); !strings.Contains(g.Lines[0], "Серия: 5 дней. Решите сегодня") {
		t.Errorf("at risk: %+v", g.Lines)
	}
	// a broken streak, a quiet yesterday
	g = Greet(GreetingInput{Best: 3})
	if !strings.Contains(g.Lines[0], "Серия прервалась") || g.Lines[1] != "Вчера задач не было. Сегодня хороший день, чтобы вернуться." {
		t.Errorf("broken: %+v", g.Lines)
	}
	// no streak ever
	if g := Greet(GreetingInput{}); !strings.Contains(g.Lines[0], "Серии дней пока нет") {
		t.Errorf("never: %+v", g.Lines)
	}
	// success beats yesterday; a drop is said gently
	up := Greet(GreetingInput{Streak: 2, Today: true, Yesterday: 3, Trend: &Trend{Delta: 3.2, Text: "+3,2 пункта"}})
	if !strings.Contains(up.Lines[1], "выросла: +3,2 пункта") {
		t.Errorf("up: %+v", up.Lines)
	}
	down := Greet(GreetingInput{Streak: 2, Today: true, Trend: &Trend{Delta: -2.5, Text: "−2,5 пункта"}})
	if !strings.Contains(down.Lines[1], "просела (−2,5 пункта)") {
		t.Errorf("down: %+v", down.Lines)
	}
	// a small change is not worth a word
	flat := Greet(GreetingInput{Streak: 2, Today: true, Yesterday: 2, Trend: &Trend{Delta: 0.4, Text: "+0,4 пункта"}})
	if strings.Contains(strings.Join(flat.Lines, " "), "пункта") {
		t.Errorf("flat: %+v", flat.Lines)
	}
	// the whole plan done
	done := Greet(GreetingInput{Streak: 2, Today: true, Day: Day{Done: 3, Total: 3}})
	if last := done.Lines[len(done.Lines)-1]; !strings.Contains(last, "План на сегодня выполнен") {
		t.Errorf("done: %+v", done.Lines)
	}
	// no name: a hello without a comma
	if g := Greet(GreetingInput{Hour: 13}); g.Hello != "Добрый день!" {
		t.Errorf("no name: %q", g.Hello)
	}
}

// ThemeNames repeat the names of web/src/puzzles/themes.js: a theme added there has to be added here.
func TestThemeNamesMatchTheSite(t *testing.T) {
	src, err := os.ReadFile("../../../web/src/puzzles/themes.js")
	if err != nil {
		t.Skipf("the site's sources are not here: %v", err)
	}
	re := regexp.MustCompile(`(?m)^  (\w+): "([^"]+)",$`)
	found := 0
	for _, m := range re.FindAllStringSubmatch(string(src), -1) {
		found++
		if got, ok := ThemeNames[m[1]]; !ok || got != m[2] {
			t.Errorf("theme %s: site says %q, trainer has %q (%v)", m[1], m[2], got, ok)
		}
	}
	if found != len(ThemeNames) {
		t.Errorf("the site lists %d themes, the trainer %d", found, len(ThemeNames))
	}
	if ThemeName("fork") != "Вилка" || ThemeName("unknownTheme") != "unknownTheme" {
		t.Errorf("ThemeName")
	}
}
