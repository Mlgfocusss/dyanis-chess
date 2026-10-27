import { useT } from "../i18n/index.js";

const MODE_KEYS = { rated: "pz.titleRated", themes: "pz.titleThemes", daily: "menu.puzzlesDaily" };

// The last puzzles finished (puzzles/recent.js): a press opens one to look at, with all its moves.
export default function RecentList({ list, current, onOpen }) {
  const t = useT();
  if (!list.length) return null;
  return (
    <details className="pz__recent">
      <summary className="pz__label">{t("pzrecent.title", { n: list.length })}</summary>
      <ul className="pz__reclist">
        {list.map((e) => (
          <li key={e.id}>
            <button type="button" className={`pz__rec${e.id === current ? " pz__rec--on" : ""}`} onClick={() => onOpen(e.id)} title={t("pzrecent.open")}>
              <span className={`pz__recmark pz__recmark--${e.ok ? "ok" : "bad"}`} aria-label={e.ok ? t("pzrecent.ok") : t("pzrecent.bad")}>
                {e.ok ? "✓" : "✗"}
              </span>
              <span className="pz__recid">#{e.id}</span>
              <span className="pz__recmode">{t(MODE_KEYS[e.mode])}</span>
              {e.rating > 0 && <span className="pz__recrate">{e.rating}</span>}
            </button>
          </li>
        ))}
      </ul>
    </details>
  );
}
