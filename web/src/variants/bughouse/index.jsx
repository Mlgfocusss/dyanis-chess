// Bughouse (Swedish chess), as on Chess.com: four players, two teams of two on two boards. A team has White on
// one board and Black on the other; the piece a player takes goes to the pocket of the partner, who may drop it
// instead of a move. Each player has a clock of their own. The room is BughouseRoom.jsx (VariantRoom hands a
// game of this variant over to it): two boards, the pockets, the four seats while the game waits, one chat.
// `seats: 4` tells the lobby and the catalogue there is no quick game (nobody to pair four people with).
export default {
  id: "bughouse",
  name: "Шведские шахматы (Bughouse)",
  short: "Четверо играют командами по двое на двух досках: взятая вами фигура сразу достаётся партнёру, и он может выставить её на свою доску.",
  icon: "bughouse",
  order: 70,
  seats: 4,
  rules: [
    "Играют две команды по двое. У команды белые на одной доске и чёрные на другой; партнёры сидят рядом и играют разными цветами.",
    "Фигура, которую вы взяли, сразу попадает в резерв партнёра (превращённая пешка возвращается пешкой). Вместо хода можно выставить фигуру из резерва на любое свободное поле; пешку нельзя ставить на первую и восьмую горизонтали.",
    "У каждого игрока свои часы. Партия кончается, когда на любой из двух досок поставлен мат, у кого-то упали часы или кто-то сдался; проигрывает вся команда.",
    "Ничьей по согласию нет. Игрок без ходов, которому не объявлен шах (пат), ждёт: ход остаётся за ним, часы идут, и партнёр может освободить его взятой фигурой. Мат ставится сразу, даже если партнёр мог бы прислать фигуру.",
    "Партию можно отменить, пока не сделан ни один ход. Играют по ссылке: создатель делится ею, места занимаются по порядку или по выбору, партия начинается, когда сели все четверо.",
  ],
  reasons: { checkmate: ["Мат", "матом"], timeout: ["Время вышло", "по времени"], resign: ["Сдача", "по сдаче"] },
};
