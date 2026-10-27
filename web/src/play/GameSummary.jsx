import Icon from "../ui/Icon.jsx";
import { useT } from "../i18n/index.js";
import "./summary.css";

const SCORE = { "1-0": ["1", "0"], "0-1": ["0", "1"], "1/2-1/2": ["½", "½"] }; // [white, black]

function Player({ name, side, score, you }) {
  const t = useT();
  return (
    <div className="summary__player">
      <span className={`summary__piece summary__piece--${side}`} aria-hidden="true" />
      <span className="summary__pname">
        {name}
        {you && <span className="summary__you">{t("leadersPage.13")}</span>}
      </span>
      <span className="summary__score">{score}</span>
    </div>
  );
}

// The card that appears over the board when a game ends: who won and how,
// the two players, and what to do next (look at the review, play again).
// Used by the game against the bot and by online games.
//   info        a description from play/game.js (describeResult / describeFinished)
//   white,black display names; you: the viewer's colour or "" for a spectator
//   rematch     { label, onClick, disabled } to replace the "Реванш" button
//   newLabel    text of the second button ("Новая партия" by default)
//   onReview    the analysis of the game; without it (the chess variants are not analysed) the
//               button and the line under it are left out
export default function GameSummary({
  info,
  white,
  black,
  you = "",
  moves,
  signedIn,
  busy,
  error,
  hint,
  reward = "",
  newLabel,
  rematch,
  onReview,
  onNew,
  onRematch,
  onClose,
}) {
  const t = useT();
  const [ws, bs] = SCORE[info.result] || ["", ""];
  const players = [
    { name: white, side: "white", score: ws, you: you === "white" },
    { name: black, side: "black", score: bs, you: you === "black" },
  ];
  const rm = rematch || { label: t("gameRoom.39"), onClick: onRematch };

  return (
    <div className="summary__backdrop">
      <section className={`summary summary--${info.outcome}`} role="dialog" aria-label={t("gameRoom.71")}>
        <button type="button" className="summary__close" onClick={onClose} aria-label={t("summary.5")}>
          <Icon name="close" size={18} />
        </button>

        <header className="summary__head">
          <h2 className="summary__title">{info.title}</h2>
          <p className="summary__sub">{info.subtitle}</p>
        </header>

        <div className="summary__players">
          {players.map((p) => (
            <Player key={p.side} {...p} />
          ))}
        </div>

        <dl className="summary__facts">
          <div>
            <dt>{t("summary.6")}</dt>
            <dd>{info.result === "1/2-1/2" ? "½–½" : info.result.replace("-", "–")}</dd>
          </div>
          <div>
            <dt>{t("summary.7")}</dt>
            <dd>{info.reason}</dd>
          </div>
          <div>
            <dt>{t("summary.8")}</dt>
            <dd>{Math.ceil(moves / 2)}</dd>
          </div>
        </dl>

        {reward && (
          <div className="summary__reward">
            <Icon name="trophy" size={18} /> {reward}
          </div>
        )}
        {error && <div className="banner banner--error">{error}</div>}

        <div className="summary__actions">
          {onReview && (
            <button type="button" className="btn btn--primary btn--block" onClick={onReview} disabled={busy || moves === 0}>
              <Icon name="review" size={18} />
              {busy ? t("summary.9") : t("summary.10")}
            </button>
          )}
          <div className="summary__row">
            <button type="button" className="btn" onClick={onNew}>
              {newLabel ?? t("gameRoom.newGame")}
            </button>
            {(rm.onClick || onRematch) && (
              <button type="button" className="btn" onClick={rm.onClick} disabled={rm.disabled}>
                {rm.label}
              </button>
            )}
          </div>
        </div>

        {(onReview || hint) && (
          <p className="summary__hint">
            {hint ||
              (moves === 0
                ? t("summary.11")
                : signedIn
                  ? t("summary.12")
                  : t("summary.13"))}
          </p>
        )}
      </section>
    </div>
  );
}
