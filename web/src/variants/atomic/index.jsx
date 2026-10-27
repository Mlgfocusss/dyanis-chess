// Atomic chess (as on Lichess; the rules are in backend/internal/variants/atomic.go). A capture is an explosion:
// the screen tints the squares of the last blast and the card lists the pieces it destroyed.
import { blastClasses, blastOf, describeBlast } from "./blast.js";
import { useT } from "../../i18n/index.js";
import "./atomic.css";

function BlastCard({ view }) {
  const t = useT();
  const d = describeBlast(blastOf(view?.extra));
  if (!d) return null;
  return (
    <section className="card vatomic__card" aria-live="polite">
      <h2>{t("vatomic.1")}</h2>
      <p className="vatomic__center">
        {t("vatomic.2")} <strong>{d.center}</strong>{t("vatomic.3")} {d.white}{t("vatomic.4")} {d.black}.
      </p>
      <ul className="vatomic__lost">
        {d.lost.map((l) => (
          <li key={l.square}>
            {l.name} <span className="vatomic__sq">{l.square}</span>
          </li>
        ))}
      </ul>
    </section>
  );
}

export default {
  id: "atomic",
  name: "Атомные шахматы",
  short: "Каждое взятие взрывает всё вокруг, кроме пешек; побеждает тот, кто взорвал короля соперника.",
  icon: "atomic",
  order: 20,
  rules: [
    "Каждое взятие — взрыв. Бьющая фигура, взятая фигура и все фигуры на соседних клетках, кроме пешек, исчезают.",
    "Пешки рядом со взрывом остаются. Пешка исчезает, только если она бьёт или её бьют (взятие на проходе тоже взрыв).",
    "Король не может бить, и нельзя взятием взорвать собственного короля.",
    "Побеждает тот, кто взорвал короля соперника, даже если его собственный король под шахом. Мат и пат как в обычных шахматах.",
    "Когда короли стоят рядом, шаха нет: взять одного короля значило бы взорвать и другого. Короли могут вставать рядом.",
    "При взятии с превращением пешка сразу взрывается, поэтому фигуру выбирать не нужно.",
  ],
  captured: false, // pieces vanish in blasts, "taken pieces" and the material balance mean nothing here
  reasons: { "king-exploded": ["Король взорван", "взрывом короля"] },
  boardProps: (ctx) => ({ squareClasses: blastClasses(blastOf(ctx.view?.extra)) }),
  Side: (ctx) => <BlastCard view={ctx.view} />,
};
