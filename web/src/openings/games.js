// Words for a game compared with a repository, with no imports so that `npm test`
// covers them.

// The result of a game from the point of view of the colour the person played:
// { kind: "win" | "loss" | "draw" | "open", text }.
export function outcomeOf(result, color) {
  if (result === "1/2-1/2") return { kind: "draw", text: "Ничья" };
  if (result === "1-0" || result === "0-1") {
    const whiteWon = result === "1-0";
    const won = whiteWon === (color === "white");
    return won ? { kind: "win", text: "Победа" } : { kind: "loss", text: "Поражение" };
  }
  return { kind: "open", text: "Не окончена" };
}

// "1. e4 e5 2. Nf3" for the SAN moves of a line (the first move is White's).
function numbered(sans, extra) {
  const all = extra ? [...sans, extra] : sans;
  return all.map((s, i) => (i % 2 === 0 ? `${i / 2 + 1}. ${s}` : s)).join(" ");
}

// The words of a deviation: { headline, line }. d is the deviation of the server
// ({ status, ply, line, played, expected }).
export function deviationLine(d) {
  const no = Math.ceil(d.ply / 2);
  const black = d.ply % 2 === 0;
  const mv = d.played ? `${no}${black ? "..." : "."} ${d.played.san}` : "";
  const expected = d.expected.map((m) => m.san).join(", ");
  switch (d.status) {
    case "in-book":
      return { headline: "Вся партия сыграна по вашему репозиторию.", line: numbered(d.line) };
    case "you-deviated":
      return {
        headline: `Вы отклонились на ходу ${mv}: в репозитории здесь ${expected}.`,
        line: numbered(d.line, d.played && `[${d.played.san}]`),
      };
    case "opponent-deviated":
      return {
        headline: `Соперник вышел из вашей подготовки на ходу ${mv}. ${expected ? `Вы ждали: ${expected}.` : ""} Ответа на этот ход в репозитории нет.`,
        line: numbered(d.line, d.played && `[${d.played.san}]`),
      };
    case "book-ended":
      return {
        headline: `Линия репозитория закончилась на ходу ${mv}: дальше ходов нет.`,
        line: numbered(d.line, d.played && `[${d.played.san}]`),
      };
    case "empty":
      return { headline: "В репозитории пока нет ходов.", line: "" };
    default:
      return { headline: "Эту партию не удалось сравнить с репозиторием.", line: "" };
  }
}
