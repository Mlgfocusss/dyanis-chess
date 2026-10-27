import { useEffect, useRef, useState } from "react";
import Icon from "../ui/Icon.jsx";
import { categoryName } from "../online/rating.js";
import {
  accuracyChart,
  accuracyNote,
  activeDays,
  activityCells,
  daysText,
  historyRows,
  isEmpty,
  percentText,
  recordBar,
  recordText,
  rushRows,
  sideText,
  weakRows,
  dayLabel,
} from "./stats.js";
import { locale, tr, useT, serverMsg } from "../i18n/index.js";
import "./stats.css";

// GET /api/stats: the numbers of the signed-in person (guests too).
export async function fetchStats() {
  const res = await fetch("/api/stats");
  let body = null;
  try {
    body = await res.json();
  } catch {
    // not JSON: handled below
  }
  if (!res.ok) throw new Error(serverMsg(body?.error) || tr("common.serverReplied", { status: res.status }));
  return body;
}

// The statistics of the person, loaded once when the profile opens: { stats, error, loading }.
export function useStats(userId) {
  const [state, setState] = useState({ stats: null, error: "", loading: true });
  useEffect(() => {
    if (!userId) return undefined;
    let alive = true;
    setState({ stats: null, error: "", loading: true });
    fetchStats().then(
      (stats) => alive && setState({ stats, error: "", loading: false }),
      (e) => alive && setState({ stats: null, error: e.message, loading: false }),
    );
    return () => {
      alive = false;
    };
  }, [userId]);
  return state;
}

// ---------------------------------------------------------------- small pieces

function Empty({ children, actions }) {
  return (
    <div className="stats__empty">
      <p>{children}</p>
      {actions && <div className="stats__actions">{actions}</div>}
    </div>
  );
}

function Tile({ label, value, sub, icon }) {
  return (
    <div className="stats__tile">
      <span className="stats__tlabel">
        {icon && <Icon name={icon} size={15} />}
        {label}
      </span>
      <b className="stats__tvalue">{value}</b>
      {sub && <span className="stats__tsub">{sub}</span>}
    </div>
  );
}

// Wins, draws and losses as one bar with the numbers under it.
function ResultBar({ title, rec }) {
  const t = useT();
  const bar = recordBar(rec);
  return (
    <div className="stats__result">
      <div className="stats__rhead">
        <span>{title}</span>
        <b>{rec.total}</b>
      </div>
      {rec.total > 0 ? (
        <div className="stats__bar" role="img" aria-label={t("statsTab.2", { title, wins: rec.wins, draws: rec.draws, losses: rec.losses })}>
          {bar.win > 0 && <span className="stats__seg stats__seg--win" style={{ width: `${bar.win}%` }} />}
          {bar.draw > 0 && <span className="stats__seg stats__seg--draw" style={{ width: `${bar.draw}%` }} />}
          {bar.loss > 0 && <span className="stats__seg stats__seg--loss" style={{ width: `${bar.loss}%` }} />}
        </div>
      ) : (
        <div className="stats__bar stats__bar--none" />
      )}
      <div className="stats__rnums">
        <span>
          <i className="stats__dot stats__dot--win" />
          {t("statsTab.3")} {rec.wins}
        </span>
        <span>
          <i className="stats__dot stats__dot--draw" />
          {t("statsTab.4")} {rec.draws}
        </span>
        <span>
          <i className="stats__dot stats__dot--loss" />
          {t("statsTab.5")} {rec.losses}
        </span>
      </div>
    </div>
  );
}

// ---------------------------------------------------------------- accuracy chart

const CHART_HEIGHT = 220;

function AccuracyChart({ reviews }) {
  const t = useT();
  const box = useRef(null);
  const [width, setWidth] = useState(0);
  const [hover, setHover] = useState(-1);
  useEffect(() => {
    const el = box.current;
    if (!el) return undefined;
    const measure = () => setWidth(Math.floor(el.clientWidth));
    measure();
    if (typeof ResizeObserver === "undefined") return undefined;
    const ro = new ResizeObserver(measure);
    ro.observe(el);
    return () => ro.disconnect();
  }, []);

  const chart = accuracyChart(reviews.series, reviews.average, width, CHART_HEIGHT);
  const { left, right, top, bottom } = chart.pad;
  const active = hover >= 0 ? chart.points[hover] : null;
  const tipLeft = active ? Math.min(Math.max(active.x, 90), Math.max(90, width - 90)) : 0;

  return (
    <div className="stats__chart" ref={box} onPointerLeave={() => setHover(-1)}>
      {width > 0 && chart.points.length > 0 && (
        <svg width={width} height={CHART_HEIGHT} role="img" aria-label={t("statsTab.6")}>
          {chart.ticks.map((t) => (
            <g key={t.value}>
              <line className="stats__grid" x1={left} x2={width - right} y1={t.y} y2={t.y} />
              <text className="stats__axis" x={left - 8} y={t.y + 4} textAnchor="end">
                {t.value}
              </text>
            </g>
          ))}
          {chart.avgY != null && <line className="stats__avg" x1={left} x2={width - right} y1={chart.avgY} y2={chart.avgY} />}
          {chart.points.length > 1 && <path className="stats__line" d={chart.line} />}
          {hover >= 0 && <line className="stats__cross" x1={active.x} x2={active.x} y1={top} y2={CHART_HEIGHT - bottom} />}
          {chart.points.map((p, i) => (
            <a
              key={p.id}
              href={`#review/${p.id}`}
              aria-label={t("statsTab.7", { p: i + 1, accuracy: p.accuracy, p2: sideText(p.side) })}
              onPointerEnter={() => setHover(i)}
              onFocus={() => setHover(i)}
              onBlur={() => setHover(-1)}
            >
              <circle className={`stats__pt${p.side === "both" ? " stats__pt--both" : ""}`} cx={p.x} cy={p.y} r={hover === i ? 6 : 4.5} />
              <circle className="stats__hit" cx={p.x} cy={p.y} r={14} />
            </a>
          ))}
          <text className="stats__axis" x={left} y={CHART_HEIGHT - 5} textAnchor="start">
            {t("statsTab.8")}
          </text>
          <text className="stats__axis" x={width - right} y={CHART_HEIGHT - 5} textAnchor="end">
            {t("statsTab.9")}
          </text>
        </svg>
      )}
      {active && (
        <div className="stats__tip" style={{ left: tipLeft, top: Math.max(0, active.y - 64) }}>
          <b>{active.accuracy}</b>
          <span>{sideText(active.side)}</span>
          <span>{new Date(active.at).toLocaleDateString(locale(), { day: "numeric", month: "short" })}</span>
        </div>
      )}
    </div>
  );
}

// ---------------------------------------------------------------- sections

function GamesSection({ games }) {
  const t = useT();
  if (!games.total) {
    return (
      <Empty
        actions={
          <>
            <a className="btn btn--primary" href="#online">
              {t("home.playOnline")}
            </a>
            <a className="btn btn--ghost" href="#bots">
              {t("statsTab.11")}
            </a>
          </>
        }
      >
        {t("statsTab.12")}
      </Empty>
    );
  }
  return (
    <>
      <div className="stats__results">
        <ResultBar title={t("statsTab.13")} rec={games} />
        <ResultBar title={t("statsTab.14")} rec={games.humans} />
        <ResultBar title={t("statsTab.15")} rec={games.bots} />
      </div>
      <h3 className="stats__sub">{t("statsTab.16")}</h3>
      <ul className="stats__cats">
        {[...games.categories, ...(games.duel ? [games.duel] : [])].map((c) => (
          <li key={c.category} className="stats__cat">
            <span className="stats__cname">{categoryName(c.category) || c.name}</span>
            <b className="stats__cvalue">{c.games ? `${c.rating}${c.provisional ? "?" : ""}` : "—"}</b>
            <span className="stats__csub">{c.games ? t("statsTab.17", { games: c.games, p: recordText(c) }) : t("profile.notPlayedYet")}</span>
          </li>
        ))}
      </ul>
      <p className="stats__note">
        {t("statsTab.19")}
      </p>
    </>
  );
}

function ReviewsSection({ reviews }) {
  const t = useT();
  if (!reviews.count) {
    return (
      <Empty
        actions={
          <a className="btn btn--primary" href="#review">
            {t("home.reviewGame")}
          </a>
        }
      >
        {t("statsTab.21")}
      </Empty>
    );
  }
  const note = accuracyNote(reviews);
  return (
    <>
      {reviews.series.length > 0 ? (
        <>
          <AccuracyChart reviews={reviews} />
          <details className="stats__table">
            <summary>{t("statsTab.22")}</summary>
            <ul>
              {[...reviews.series].reverse().map((p) => (
                <li key={p.id}>
                  <a href={`#review/${p.id}`}>{new Date(p.at).toLocaleDateString(locale(), { day: "numeric", month: "short" })}</a>
                  <b>{p.accuracy}</b>
                  <span>{sideText(p.side)}</span>
                </li>
              ))}
            </ul>
          </details>
        </>
      ) : (
        <p className="stats__note">{t("statsTab.23")}</p>
      )}
      {note && <p className="stats__note">{note} {t("statsTab.24")}</p>}
    </>
  );
}

function PuzzlesSection({ puzzles }) {
  const t = useT();
  const rush = rushRows(puzzles.rush);
  const duels = puzzles.duels;
  const points = puzzles.points || { total: 0, today: 0, streak: 0, best: 0 };
  const collecting = points.total > 0 || points.best > 0;
  if (!puzzles.attempts && !rush.length && !duels.total) {
    return (
      <Empty
        actions={
          <>
            <a className="btn btn--primary" href="#puzzles">
              {t("statsTab.25")}
            </a>
            <a className="btn btn--ghost" href="#puzzles/daily">
              {t("menu.puzzlesDaily")}
            </a>
          </>
        }
      >
        {t("statsTab.27")}
      </Empty>
    );
  }
  return (
    <div className="stats__tiles stats__tiles--inner">
      <Tile label={t("statsTab.28")} value={puzzles.attempts} sub={t("statsTab.29")} />
      <Tile label={t("statsTab.30")} value={puzzles.solved} sub={percentText(puzzles.solved, puzzles.attempts)} />
      <Tile label={t("statsTab.31")} value={puzzles.attempts ? puzzles.rating : "—"} />
      {collecting && <Tile label={t("statsTab.32")} value={points.total} sub={t("statsTab.33", { today: points.today })} icon="star" />}
      {collecting && <Tile label={t("statsTab.34")} value={points.best} sub={t("statsTab.35", { streak: points.streak })} icon="flame" />}
      {rush.map((r) => (
        <Tile key={r.mode} label={r.name} value={r.best} sub={t("statsTab.36", { runs: r.runs })} icon="bolt" />
      ))}
      {duels.total > 0 && <Tile label={t("statsTab.37")} value={recordText(duels)} sub={t("statsTab.38")} icon="duel" />}
    </div>
  );
}

function StreakSection({ streak }) {
  const t = useT();
  const cells = activityCells(streak);
  const active = activeDays(streak);
  return (
    <>
      <div className="stats__streakhead">
        <div className="stats__streakbig">
          <Icon name="flame" size={26} className={streak.current > 0 ? "stats__flame stats__flame--on" : "stats__flame"} />
          <div>
            <b>{streak.current > 0 ? daysText(streak.current) : t("statsTab.39")}</b>
            <span>
              {streak.current > 0
                ? streak.today
                  ? t("statsTab.40")
                  : t("statsTab.41")
                : t("statsTab.42")}
            </span>
          </div>
        </div>
        <div className="stats__streakbest">
          <span>{t("statsTab.34")}</span>
          <b>{streak.best > 0 ? daysText(streak.best) : "—"}</b>
        </div>
        <div className="stats__streakbest">
          <span>{t("statsTab.43")}</span>
          <b>{active}</b>
        </div>
      </div>
      <ol className="stats__strip" aria-label={t("statsTab.44")}>
        {cells.map((c) => (
          <li key={c.date} className={`stats__cell stats__cell--${c.level}`} title={c.label} aria-label={c.label} />
        ))}
      </ol>
      <div className="stats__stripaxis">
        <span>{cells.length ? dayLabel(cells[0].date) : ""}</span>
        <span>{t("statsTab.45")}</span>
      </div>
      <p className="stats__note">
        {t("statsTab.46")}
      </p>
    </>
  );
}

function WeakSection({ weak, attempts }) {
  const t = useT();
  const rows = weakRows(weak);
  if (!rows.length) {
    return (
      <Empty>
        {attempts > 0
          ? t("statsTab.47")
          : t("statsTab.48")}
      </Empty>
    );
  }
  return (
    <>
      <ul className="stats__weak">
        {rows.map((r) => (
          <li key={r.theme}>
            <a className="stats__wname" href="#puzzles/themes" title={t("statsTab.49")}>
              {r.name}
            </a>
            <span className="stats__wbar" aria-hidden="true">
              <i style={{ width: `${r.share}%` }} />
            </span>
            <span className="stats__wnum">
              <b>{r.failed}</b> {t("statsTab.50")} {r.attempts}
            </span>
          </li>
        ))}
      </ul>
      <p className="stats__note">{t("statsTab.51")}</p>
    </>
  );
}

function HistorySection({ history }) {
  const t = useT();
  const rows = historyRows(history);
  if (!rows.length) return <Empty>{t("statsTab.52")}</Empty>;
  return (
    <ul className="stats__hist">
      {rows.map((r, i) => (
        <li key={`${r.id}-${i}`}>
          <a href={r.href} title={t("statsTab.53")}>
            <span className={`stats__mark stats__mark--${r.solved ? "ok" : "bad"}`}>
              <Icon name={r.solved ? "check" : "close"} size={14} />
              <span className="stats__sr">{r.solved ? t("statsTab.54") : t("statsTab.55")}</span>
            </span>
            <span className="stats__hmain">
              <span className="stats__hid">{r.id}</span>
              <span className="stats__hwhen">{r.when}</span>
            </span>
            <b className={`stats__hdelta${r.delta.startsWith("+") ? " stats__hdelta--up" : r.delta.startsWith("−") ? " stats__hdelta--down" : ""}`}>
              {r.delta}
            </b>
            <span className="stats__hrating">{r.rating}</span>
          </a>
        </li>
      ))}
    </ul>
  );
}

// ---------------------------------------------------------------- the tab

export default function StatsTab({ stats, error, loading }) {
  const t = useT();
  if (loading) return <section className="card stats__status">{t("statsTab.56")}</section>;
  if (error || !stats) {
    return (
      <section className="card stats__status">
        <div className="banner banner--error" role="alert">
          {error || t("statsTab.57")}
        </div>
      </section>
    );
  }

  const { games, reviews, puzzles, streak } = stats;
  return (
    <div className="stats">
      {isEmpty(stats) && (
        <section className="card stats__hello">
          <h2>{t("statsTab.58")}</h2>
          <p>{t("statsTab.59")}</p>
        </section>
      )}

      <div className="stats__tiles">
        <Tile label={t("statsTab.60")} value={games.total} sub={games.total ? t("statsTab.61", { p: percentText(games.wins, games.total) }) : t("profile.notPlayedYet")} />
        <Tile label={t("statsTab.62")} value={reviews.count} sub={reviews.average != null ? t("statsTab.63", { average: reviews.average }) : t("statsTab.64")} />
        <Tile
          label={t("statsTab.65")}
          value={puzzles.attempts ? percentText(puzzles.solved, puzzles.attempts) : "—"}
          sub={puzzles.attempts ? t("statsTab.66", { solved: puzzles.solved, attempts: puzzles.attempts }) : t("statsTab.67")}
        />
        <Tile label={t("statsTab.31")} value={puzzles.attempts ? puzzles.rating : "—"} />
        <Tile label={t("statsTab.68")} value={streak.current} sub={t("statsTab.69", { best: streak.best })} icon="flame" />
      </div>

      <section className="card stats__sec">
        <header>
          <h2>{t("profile.tabGames")}</h2>
          <span className="stats__hint">{t("statsTab.71")}</span>
        </header>
        <GamesSection games={games} />
      </section>

      <section className="card stats__sec">
        <header>
          <h2>{t("statsTab.72")}</h2>
          <span className="stats__hint">
            {reviews.average != null ? t("statsTab.73", { average: reviews.average, length: reviews.series.length }) : t("statsTab.74")}
          </span>
        </header>
        <ReviewsSection reviews={reviews} />
      </section>

      <section className="card stats__sec">
        <header>
          <h2>{t("statsTab.68")}</h2>
          <span className="stats__hint">{t("statsTab.75")}</span>
        </header>
        <StreakSection streak={streak} />
      </section>

      <section className="card stats__sec">
        <header>
          <h2>{t("menu.puzzles")}</h2>
          <span className="stats__hint">{t("statsTab.77")}</span>
        </header>
        <PuzzlesSection puzzles={puzzles} />
      </section>

      <section className="card stats__sec">
        <header>
          <h2>{t("statsTab.78")}</h2>
          <span className="stats__hint">{t("statsTab.79")}</span>
        </header>
        <WeakSection weak={puzzles.weak} attempts={puzzles.attempts} />
      </section>

      <section className="card stats__sec">
        <header>
          <h2>{t("statsTab.80")}</h2>
          <span className="stats__hint">{t("statsTab.81")}</span>
        </header>
        <HistorySection history={puzzles.history} />
      </section>
    </div>
  );
}
