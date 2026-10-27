// King of the Hill (as on Lichess; backend/internal/variants/kingofthehill.go): ordinary chess where a king on one of
// the four central squares wins at once. The screen marks the hill and tells how far each king is from it.
import { describeDistance, hillClasses, hillDistance } from "./hill.js";
import { useT } from "../../i18n/index.js";
import "./hill.css";

const HILL_CLASSES = hillClasses(); // one object for ever: the board does not need a new one at every render

function HillCard({ view, you }) {
  const t = useT();
  const white = hillDistance(view?.board, "white");
  const black = hillDistance(view?.board, "black");
  const row = (side, label, n) => (
    <li key={side} className={you === side ? "vhill__me" : undefined}>
      {label}: <strong>{describeDistance(n)}</strong>
    </li>
  );
  return (
    <section className="card vhill__card">
      <h2>{t("vhill.1")}</h2>
      <p className="vhill__text">{t("vhill.2")}</p>
      <ul className="vhill__list">
        {row("white", t("vhill.3"), white)}
        {row("black", t("vhill.4"), black)}
      </ul>
    </section>
  );
}

export default {
  id: "kingofthehill",
  name: "Царь горы",
  short: "Обычные шахматы, но побеждает и тот, чей король первым дошёл до центра: d4, e4, d5 или e5.",
  icon: "hill",
  order: 30,
  rules: [
    "Все правила обычных шахмат: ходы, рокировка, мат, пат, ничья.",
    "Ещё один способ победить: король, который дошёл до одной из четырёх центральных клеток (d4, e4, d5, e5), выигрывает сразу.",
    "Король, как и всегда, не может встать на клетку под боем, поэтому центр приходится сначала отвоевать фигурами.",
    "Обычный мат тоже побеждает. Недостаток материала даёт ничью, как в обычных шахматах.",
  ],
  reasons: { "king-of-the-hill": ["Царь горы", "королём на горе"] },
  boardProps: () => ({ squareClasses: HILL_CLASSES }),
  Side: (ctx) => <HillCard view={ctx.view} you={ctx.you} />,
};
