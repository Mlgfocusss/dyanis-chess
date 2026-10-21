// Fog of war (Chess.com rules): no check, the game ends when a king is taken; a player sees their own pieces
// and the squares their pieces could move to. The server sends only what the player may see (board, legal,
// last, the list of moves) and `extra.fog`, the squares in the fog; this file only paints the veil.
// Rules of the visibility: backend/internal/variants/fog.go. Nothing here may add attributes (title,
// aria-label, data-*) to a fogged square: the DOM must not say what the fog hides.
import Icon from "../../ui/Icon.jsx";
import { fogClasses } from "./fogview.js";
import "./fog.css";

function FogNote({ game, you }) {
  const ended = game.status === "ended";
  return (
    <section className="card fog-note">
      <h2>
        <Icon name="fog" size={18} /> Туман войны
      </h2>
      {ended ? (
        <p className="fog-note__over">Туман рассеялся: видна вся доска и все ходы.</p>
      ) : (
        <>
          <p>Видны только ваши фигуры и клетки, на которые они могут сходить.</p>
          <p>
            Шаха нет: король может встать под бой, партия кончается, когда его берут. Ходы соперника, которых вы не видели, записаны как
            «…».
          </p>
          {!you && <p>Зрители видят доску после конца партии.</p>}
        </>
      )}
    </section>
  );
}

export default {
  id: "fog",
  name: "Туман войны",
  short: "Видны только ваши фигуры и поля, на которые они могут сходить; шаха нет, побеждает взявший короля.",
  rules: [
    "Вы видите свои фигуры, поля, на которые они могут сходить (в том числе пустые), и фигуры соперника на этих полях.",
    "Остальные поля закрыты туманом: чужих фигур на них не видно. Пешка видит одно поле вперёд (два с начальной позиции) и диагональ, только если там стоит чужая фигура.",
    "Шаха и мата нет: король может встать под бой, а партия кончается, когда король взят.",
    "Ходы соперника, которых вы не видели, в списке записаны как «…»; после партии открывается всё.",
  ],
  icon: "fog",
  order: 10,
  captured: false,
  Side: FogNote,
  boardProps: ({ view }) => ({ squareClasses: fogClasses(view) }),
};
