import BotAvatar from "../bots/BotAvatar.jsx";
import Icon from "../ui/Icon.jsx";
import PuzzleTabs from "./PuzzleTabs.jsx";
import { pointsText, signed } from "./puzzle.js";
import { useT } from "../i18n/index.js";
import "./puzzles-panel.css";

const DIONIS = { id: "dionis", name: "Дионис Самусенко" };

// The frame of every puzzle page, the same idea as the lesson player: the board on the left, a panel
// as tall as the board on the right. `head` is what stands at the right of the title (the clock),
// `children` the scrolling part of the panel and `bottom` the big button with the small ones under it.
// className goes to the root; mode (the active tab) puts the tabs of the puzzle section under the title.
export function Shell({ className = "", mode, icon, title, head, board, children, bottom }) {
  return (
    <div className={`pz ${className}`}>
      <div className="pz__grid">
        <div className="pz__boardwrap">
          <div className="room__board pz__board">{board}</div>
        </div>
        <aside className="pz__panel card">
          <header className="pz__head">
            <span className="pz__ico">
              <Icon name={icon} size={20} />
            </span>
            <span className="pz__title">{title}</span>
            {head}
          </header>
          {mode && <PuzzleTabs mode={mode} />}
          <div className="pz__scroll">{children}</div>
          {bottom && <div className="pz__bottom">{bottom}</div>}
        </aside>
      </div>
    </div>
  );
}

// The page while it has nothing to show (loading, an error): the same frame, no board.
export function Placeholder({ children }) {
  return <div className="pz pz--loading">{children}</div>;
}

// Dionis and his words in a bubble. tone colours the bubble: "" | "ok" | "bad" | "warn".
export function Say({ kind, tone = "", children }) {
  return (
    <div className="pz__coach" aria-live="polite">
      <BotAvatar bot={DIONIS} size={64} />
      <div className={`pz__bubble${tone ? ` pz__bubble--${tone}` : ""}`}>
        <b className="pz__kind">{kind}</b>
        {children && <p className="pz__text">{children}</p>}
      </div>
    </div>
  );
}

// The big button at the bottom of the panel. auto: the page will press it by itself (the bar fills in `ms`).
export function BigButton({ onClick, disabled, auto = false, ms = 2000, href, children }) {
  const cls = `btn btn--primary pz__next${auto ? " pz__next--auto" : ""}`;
  const style = auto ? { "--auto-ms": `${ms}ms` } : undefined;
  const label = <span className="pz__next-label">{children}</span>;
  if (href) {
    return (
      <a className={cls} href={href} style={style}>
        {label}
      </a>
    );
  }
  return (
    <button type="button" className={cls} style={style} onClick={onClick} disabled={disabled}>
      {label}
    </button>
  );
}

export function NextLabel({ children }) {
  return (
    <>
      {children} <Icon name="next" size={20} />
    </>
  );
}

// The numbers of the rated puzzles (see scoreView in puzzle.js): the main one in a tile, the other
// modestly under it. A change of this try stands next to the number.
function ScoreTile({ item }) {
  const t = useT();
  return (
    <div className="pz__score">
      <span className="pz__label">{item.kind === "rating" ? t("pzShell.2") : t("statsTab.32")}</span>
      <div className="pz__row">
        <span className="pz__num">{item.value}</span>
        {item.change !== null && (
          <span className={`pz__chip-delta pz__chip-delta--${item.change > 0 ? "up" : item.change < 0 ? "down" : "same"}`}>
            {item.kind === "rating" ? signed(item.change) : item.change > 0 ? `+${item.change}` : "0"}
          </span>
        )}
      </div>
      {item.kind === "rating" && item.from !== null && (
        <span className="pz__sub">
          {t("pzShell.4")} {item.from}{t("pzShell.5")} {item.value}
        </span>
      )}
      {item.kind === "points" && (
        <span className="pz__sub">
          {t("pzShell.6")} <b>{item.streak}</b>{t("pzShell.7")} {pointsText(item.today)}.
        </span>
      )}
    </div>
  );
}

export function Score({ view }) {
  if (!view?.main) return null;
  const { main, side } = view;
  return (
    <div className="pz__scores">
      <ScoreTile item={main} />
      {side && <ScoreTile item={side} />}
    </div>
  );
}
