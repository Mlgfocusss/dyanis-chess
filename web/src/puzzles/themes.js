// Names of the Lichess puzzle themes in Russian. A theme not listed here is
// shown by its own id, so a newer database does not break the page.
export const THEME_NAMES = {
  advancedPawn: "Проходная пешка",
  advantage: "Преимущество",
  anastasiaMate: "Мат Анастасии",
  arabianMate: "Арабский мат",
  attackingF2F7: "Атака на f2/f7",
  attraction: "Завлечение",
  backRankMate: "Мат по последней горизонтали",
  bishopEndgame: "Слоновый эндшпиль",
  bodenMate: "Мат Бодена",
  capturingDefender: "Уничтожение защитника",
  castling: "Рокировка",
  clearance: "Освобождение поля",
  crushing: "Разгром",
  defensiveMove: "Защитный ход",
  deflection: "Отвлечение",
  discoveredAttack: "Вскрытое нападение",
  doubleBishopMate: "Мат двумя слонами",
  doubleCheck: "Двойной шах",
  dovetailMate: "Мат «ласточкин хвост»",
  endgame: "Эндшпиль",
  enPassant: "Взятие на проходе",
  equality: "Уравнять",
  exposedKing: "Открытый король",
  fork: "Вилка",
  hangingPiece: "Незащищённая фигура",
  hookMate: "Мат крюком",
  interference: "Перекрытие",
  intermezzo: "Промежуточный ход",
  kingsideAttack: "Атака на королевском фланге",
  knightEndgame: "Коневой эндшпиль",
  long: "Длинная задача",
  master: "Партии мастеров",
  masterVsMaster: "Мастер против мастера",
  mate: "Мат",
  mateIn1: "Мат в 1 ход",
  mateIn2: "Мат в 2 хода",
  mateIn3: "Мат в 3 хода",
  mateIn4: "Мат в 4 хода",
  mateIn5: "Мат в 5 ходов и больше",
  middlegame: "Миттельшпиль",
  oneMove: "Один ход",
  opening: "Дебют",
  pawnEndgame: "Пешечный эндшпиль",
  pin: "Связка",
  promotion: "Превращение",
  queenEndgame: "Ферзевый эндшпиль",
  queenRookEndgame: "Ферзь и ладья в эндшпиле",
  queensideAttack: "Атака на ферзевом фланге",
  quietMove: "Тихий ход",
  rookEndgame: "Ладейный эндшпиль",
  sacrifice: "Жертва",
  short: "Короткая задача",
  skewer: "Линейный удар",
  smotheredMate: "Спёртый мат",
  superGM: "Партии сильнейших гроссмейстеров",
  trappedPiece: "Ловушка для фигуры",
  underPromotion: "Превращение не в ферзя",
  veryLong: "Очень длинная задача",
  xRayAttack: "Рентген",
  zugzwang: "Цугцванг",
};

export function themeName(id) {
  return THEME_NAMES[id] || id;
}

// Themes that say how the puzzle was found, not what it is about: kept out of
// the chips under a solved puzzle, which should name the idea.
const TECHNICAL = new Set(["short", "long", "veryLong", "oneMove", "master", "masterVsMaster", "superGM", "crushing", "advantage", "equality"]);

export function ideaThemes(ids) {
  const idea = (ids || []).filter((t) => !TECHNICAL.has(t));
  return idea.length ? idea : ids || [];
}
