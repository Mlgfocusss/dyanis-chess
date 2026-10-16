package bots

import (
	"sort"
	"time"
)

// All is the list of bots, weakest first inside a category. The Elo values
// come from the ladder tool (backend/cmd/botladder); the parameters were
// tuned until the measured ladder matched them.
var all = []Bot{
	// ---- Новички
	{
		ID: "misha", Name: "Миша", Category: "beginners", Elo: 400, Points: 10,
		Desc:     "Только выучил, как ходят фигуры. Любит двигать пешки и часто забывает, что у него есть король.",
		Greeting: "Привет! Давай играть? Я только вчера научился.",
		OnWin:    "Ура, я выиграл! Сыграем ещё?",
		OnLose:   "Ой… Ты так здорово играешь!",
		Params:   Params{Depth: 1, Temp: 400, Random: 0.45, NoMate: true},
	},
	{
		ID: "liza", Name: "Лиза", Category: "beginners", Elo: 600, Points: 15,
		Desc:     "Знает правила и берёт всё, что плохо лежит, но почти не смотрит вперёд. Ферзя выводит рано.",
		Greeting: "Привет! Я люблю, когда на доске много фигур.",
		OnWin:    "Получилось! Спасибо за партию.",
		OnLose:   "Ты меня обыграл! Научишь так же?",
		Params:   Params{Depth: 1, Temp: 200, Random: 0.2},
	},
	{
		ID: "timur", Name: "Тимур", Category: "beginners", Elo: 900, Points: 20,
		Desc:     "Ходит в школьный шахматный кружок. Видит угрозы на ход вперёд, но попадается на вилки и связки.",
		Greeting: "Привет! Нас в кружке учат играть дебют, посмотрим, что получится.",
		OnWin:    "Я выиграл! Расскажу всем в кружке.",
		OnLose:   "Хорошо играешь. Мне надо ещё потренироваться.",
		Params:   Params{Depth: 2, Temp: 60},
	},

	// ---- Любители
	{
		ID: "anna", Name: "Анна", Category: "club", Elo: 1200, Points: 30,
		Desc:     "Играет по выходным для удовольствия. Знает дебют и простую тактику, но в эндшпиле теряет нить.",
		Greeting: "Привет! Люблю хорошие партии без спешки.",
		OnWin:    "Хорошая была борьба, спасибо!",
		OnLose:   "Красиво сыграно! Хочу реванш.",
		Params:   Params{Depth: 3, Temp: 55},
	},
	{
		ID: "artem", Name: "Артём", Category: "club", Elo: 1400, Points: 35,
		Desc:     "Играет в клубе. Считает варианты на три-четыре хода и редко подставляет фигуры, но слабоват в позиционной игре.",
		Greeting: "Привет! Играем всерьёз, без поддавков.",
		OnWin:    "Хорошая партия. Разберём её вместе?",
		OnLose:   "Сильно! Ты это заслужил.",
		Params:   Params{Depth: 3, Temp: 46},
	},
	{
		ID: "igor", Name: "Игорь", Category: "club", Elo: 1600, Points: 45,
		Desc:     "Опытный турнирный игрок. Хорошо чувствует тактику и использует каждую неточность соперника.",
		Greeting: "Добрый день. Постарайтесь не зевать.",
		OnWin:    "Была неточность в середине игры. Проверьте разбором.",
		OnLose:   "Сильная игра, поздравляю.",
		Params:   Params{Depth: 4, Temp: 30},
	},

	// ---- Сильные
	{
		ID: "viktor", Name: "Виктор", Category: "strong", Elo: 2100, Points: 60,
		Desc:     "Кандидат в мастера. Считает глубоко, играет точно и не прощает слабых полей в лагере.",
		Greeting: "Здравствуйте. Начнём.",
		OnWin:    "Хорошая попытка. Эндшпиль надо подтянуть.",
		OnLose:   "Это была отличная партия.",
		Params:   Params{Depth: 6, Temp: 10},
	},
	{
		ID: "sofia", Name: "София", Category: "strong", Elo: 2400, Points: 80,
		Desc:     "Мастер. Почти не ошибается в тактике, а позиционные преимущества накапливает неторопливо и неумолимо.",
		Greeting: "Приятно познакомиться. Играйте внимательно.",
		OnWin:    "Спасибо за игру. Посмотрите на ход, после которого стало сложно.",
		OnLose:   "Впечатляюще. Мои поздравления.",
		Params:   Params{MoveTime: 30 * time.Millisecond},
	},
	{
		ID: "max", Name: "Максимум", Category: "strong", Elo: 2900, Points: 150,
		Desc:     "Сам движок Dyanis Chess с нейросетью NNUE, без ограничений и поддавков: пик его силы. Если вы его обыграли, это событие.",
		Greeting: "Играю в полную силу.",
		OnWin:    "Партия сыграна. Разбор покажет, где было упущено.",
		OnLose:   "Невероятно. Вы обыграли движок.",
		Params:   Params{MoveTime: 500 * time.Millisecond},
	},

	// ---- Спортсмены
	{
		ID: "dionis", Name: "Дионис Самусенко", Category: "athletes", Elo: 1800, Points: 50,
		Desc:     "Белорусский шахматист, третий разряд. Лучше всего играет в рапид: любит активные фигуры, умеет быстро считать и редко теряется в острой позиции.",
		Greeting: "Привет! Рапид — моё любимое. Играем?",
		OnWin:    "Хорошая партия! Реванш?",
		OnLose:   "Отлично сыграно! Разберём, где я ошибся?",
		Params:   Params{Depth: 4, Temp: 15},
	},
}

// All returns the bots, grouped as Categories lists them and weakest first
// within a category.
func All() []Bot {
	out := make([]Bot, len(all))
	copy(out, all)
	order := map[string]int{}
	for i, c := range Categories {
		order[c.ID] = i
	}
	sort.SliceStable(out, func(i, j int) bool {
		if order[out[i].Category] != order[out[j].Category] {
			return order[out[i].Category] < order[out[j].Category]
		}
		return out[i].Elo < out[j].Elo
	})
	return out
}

// Get returns the bot with this id.
func Get(id string) (*Bot, bool) {
	for i := range all {
		if all[i].ID == id {
			b := all[i]
			return &b, true
		}
	}
	return nil, false
}
