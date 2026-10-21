package trainer

// ThemeNames are the Russian names of the Lichess puzzle themes. They repeat THEME_NAMES of
// web/src/puzzles/themes.js (a test keeps the two lists equal): the texts of the trainer are made
// on the server, so it needs the names too. A theme that is not listed is shown by its id.
var ThemeNames = map[string]string{
	"advancedPawn":      "Проходная пешка",
	"advantage":         "Преимущество",
	"anastasiaMate":     "Мат Анастасии",
	"arabianMate":       "Арабский мат",
	"attackingF2F7":     "Атака на f2/f7",
	"attraction":        "Завлечение",
	"backRankMate":      "Мат по последней горизонтали",
	"bishopEndgame":     "Слоновый эндшпиль",
	"bodenMate":         "Мат Бодена",
	"capturingDefender": "Уничтожение защитника",
	"castling":          "Рокировка",
	"clearance":         "Освобождение поля",
	"crushing":          "Разгром",
	"defensiveMove":     "Защитный ход",
	"deflection":        "Отвлечение",
	"discoveredAttack":  "Вскрытое нападение",
	"doubleBishopMate":  "Мат двумя слонами",
	"doubleCheck":       "Двойной шах",
	"dovetailMate":      "Мат «ласточкин хвост»",
	"endgame":           "Эндшпиль",
	"enPassant":         "Взятие на проходе",
	"equality":          "Уравнять",
	"exposedKing":       "Открытый король",
	"fork":              "Вилка",
	"hangingPiece":      "Незащищённая фигура",
	"hookMate":          "Мат крюком",
	"interference":      "Перекрытие",
	"intermezzo":        "Промежуточный ход",
	"kingsideAttack":    "Атака на королевском фланге",
	"knightEndgame":     "Коневой эндшпиль",
	"long":              "Длинная задача",
	"master":            "Партии мастеров",
	"masterVsMaster":    "Мастер против мастера",
	"mate":              "Мат",
	"mateIn1":           "Мат в 1 ход",
	"mateIn2":           "Мат в 2 хода",
	"mateIn3":           "Мат в 3 хода",
	"mateIn4":           "Мат в 4 хода",
	"mateIn5":           "Мат в 5 ходов и больше",
	"middlegame":        "Миттельшпиль",
	"oneMove":           "Один ход",
	"opening":           "Дебют",
	"pawnEndgame":       "Пешечный эндшпиль",
	"pin":               "Связка",
	"promotion":         "Превращение",
	"queenEndgame":      "Ферзевый эндшпиль",
	"queenRookEndgame":  "Ферзь и ладья в эндшпиле",
	"queensideAttack":   "Атака на ферзевом фланге",
	"quietMove":         "Тихий ход",
	"rookEndgame":       "Ладейный эндшпиль",
	"sacrifice":         "Жертва",
	"short":             "Короткая задача",
	"skewer":            "Линейный удар",
	"smotheredMate":     "Спёртый мат",
	"superGM":           "Партии сильнейших гроссмейстеров",
	"trappedPiece":      "Ловушка для фигуры",
	"underPromotion":    "Превращение не в ферзя",
	"veryLong":          "Очень длинная задача",
	"xRayAttack":        "Рентген",
	"zugzwang":          "Цугцванг",
}

// ThemeName returns the Russian name of a puzzle theme.
func ThemeName(id string) string {
	if n, ok := ThemeNames[id]; ok {
		return n
	}
	return id
}
