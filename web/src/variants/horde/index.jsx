// Horde (as on Lichess; backend/internal/variants/horde.go): White has 36 pawns and no king, Black the usual army.
// Nothing on the board differs from the common one; the card tells the size of the horde and what each side needs.
import { describeHorde, hordeSize } from "./horde.js";
import { useT } from "../../i18n/index.js";
import "./horde.css";

function HordeCard({ view, you }) {
  const t = useT();
  const n = hordeSize(view);
  return (
    <section className="card vhorde__card">
      <h2>{t("vhorde.1")}</h2>
      <p className="vhorde__size">
        {t("vhorde.2")} <strong>{describeHorde(n)}</strong>.
      </p>
      <p className="vhorde__text">
        {you === "white"
          ? t("vhorde.3")
          : you === "black"
            ? t("vhorde.4")
            : t("vhorde.5")}
      </p>
    </section>
  );
}

export default {
  id: "horde",
  name: "Орда",
  short: "У белых 36 пешек и нет короля; белые побеждают матом, чёрные, взяв все белые фигуры.",
  icon: "horde",
  order: 40,
  rules: [
    "У белых нет короля, но есть 36 пешек: четыре первые горизонтали и пешки b5, c5, f5, g5. У чёрных обычная армия. Белые ходят первыми.",
    "Пешки на первой горизонтали ходят на две клетки, если обе свободны; такую пешку нельзя взять на проходе. Остальные пешки ходят как обычно.",
    "Белые побеждают, поставив мат чёрному королю. Чёрные побеждают, взяв все белые фигуры.",
    "Белым не бывает шаха. Если у белых нет ходов, это пат и ничья. Ничьей из-за недостатка материала нет: чёрные всегда могут съесть остатки орды.",
  ],
  captured: false, // the horde has too many pieces for the "taken" rows and the balance to mean anything
  reasons: { "horde-destroyed": ["Орда уничтожена", "взятием всей орды"] },
  Side: (ctx) => <HordeCard view={ctx.view} you={ctx.you} />,
};
