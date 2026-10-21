import { useEffect, useRef, useState } from "react";
import Icon from "../ui/Icon.jsx";
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
  if (!res.ok) throw new Error(body?.error || `Сервер ответил ${res.status}`);
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
  const bar = recordBar(rec);
  return (
    <div className="stats__result">
      <div className="stats__rhead">
        <span>{title}</span>
        <b>{rec.total}</b>
      </div>
      {rec.total > 0 ? (
        <div className="stats__bar" role="img" aria-label={`${title}: побед ${rec.wins}, ничьих ${rec.draws}, поражений ${rec.losses}`}>
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
          побед {rec.wins}
        </span>
        <span>
          <i className="stats__dot stats__dot--draw" />
          ничьих {rec.draws}
        </span>
        <span>
          <i className="stats__dot stats__dot--loss" />
          поражений {rec.losses}
        </span>
      </div>
    </div>
  );
}

// ---------------------------------------------------------------- accuracy chart

const CHART_HEIGHT = 220;

function AccuracyChart({ reviews }) {
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
        <svg width={width} height={CHART_HEIGHT} role="img" aria-label="Точность в последних разборах, от старых к новым">
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
              aria-label={`Разбор ${i + 1}: точность ${p.accuracy}, ${sideText(p.side)}`}
              onPointerEnter={() => setHover(i)}
              onFocus={() => setHover(i)}
              onBlur={() => setHover(-1)}
            >
              <circle className={`stats__pt${p.side === "both" ? " stats__pt--both" : ""}`} cx={p.x} cy={p.y} r={hover === i ? 6 : 4.5} />
              <circle className="stats__hit" cx={p.x} cy={p.y} r={14} />
            </a>
          ))}
          <text className="stats__axis" x={left} y={CHART_HEIGHT - 5} textAnchor="start">
            давно
          </text>
          <text className="stats__axis" x={width - right} y={CHART_HEIGHT - 5} textAnchor="end">
            недавно
          </text>
        </svg>
      )}
      {active && (
        <div className="stats__tip" style={{ left: tipLeft, top: Math.max(0, active.y - 64) }}>
          <b>{active.accuracy}</b>
          <span>{sideText(active.side)}</span>
          <span>{new Date(active.at).toLocaleDateString("ru-RU", { day: "numeric", month: "short" })}</span>
        </div>
      )}
    </div>
  );
}

// ---------------------------------------------------------------- sections

function GamesSection({ games }) {
  if (!games.total) {
    return (
      <Empty
        actions={
          <>
            <a className="btn btn--primary" href="#online">
              Играть онлайн
            </a>
            <a className="btn btn--ghost" href="#bots">
              Сыграть с ботом
            </a>
          </>
        }
      >
        Пока нет данных, сыграйте партию.
      </Empty>
    );
  }
  return (
    <>
      <div className="stats__results">
        <ResultBar title="Все партии" rec={games} />
        <ResultBar title="С людьми" rec={games.humans} />
        <ResultBar title="С ботами" rec={games.bots} />
      </div>
      <h3 className="stats__sub">Рейтинги по видам игры</h3>
      <ul className="stats__cats">
        {[...games.categories, ...(games.duel ? [games.duel] : [])].map((c) => (
          <li key={c.category} className="stats__cat">
            <span className="stats__cname">{c.name}</span>
            <b className="stats__cvalue">{c.games ? `${c.rating}${c.provisional ? "?" : ""}` : "—"}</b>
            <span className="stats__csub">{c.games ? `партий ${c.games}: ${recordText(c)}` : "ещё не играли"}</span>
          </li>
        ))}
      </ul>
      <p className="stats__note">
        Рейтинг считается только по рейтинговым партиям с людьми. Строка «побед / ничьих / поражений» идёт в том же порядке.
      </p>
    </>
  );
}

function ReviewsSection({ reviews }) {
  if (!reviews.count) {
    return (
      <Empty
        actions={
          <a className="btn btn--primary" href="#review">
            Разобрать партию
          </a>
        }
      >
        Пока нет данных: разберите партию, и здесь появится ваша точность.
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
            <summary>Показать таблицей</summary>
            <ul>
              {[...reviews.series].reverse().map((p) => (
                <li key={p.id}>
                  <a href={`#review/${p.id}`}>{new Date(p.at).toLocaleDateString("ru-RU", { day: "numeric", month: "short" })}</a>
                  <b>{p.accuracy}</b>
                  <span>{sideText(p.side)}</span>
                </li>
              ))}
            </ul>
          </details>
        </>
      ) : (
        <p className="stats__note">Разборы есть, но сводки точности у них нет.</p>
      )}
      {note && <p className="stats__note">{note} Пунктир показывает среднее, точка открывает разбор; пустая точка значит, что взято среднее по обеим сторонам.</p>}
    </>
  );
}

function PuzzlesSection({ puzzles }) {
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
              Решать задачи
            </a>
            <a className="btn btn--ghost" href="#puzzles/daily">
              Задача дня
            </a>
          </>
        }
      >
        Пока нет данных, решите первую задачу.
      </Empty>
    );
  }
  return (
    <div className="stats__tiles stats__tiles--inner">
      <Tile label="Попыток" value={puzzles.attempts} sub="первых попыток по каждой задаче" />
      <Tile label="Решено" value={puzzles.solved} sub={percentText(puzzles.solved, puzzles.attempts)} />
      <Tile label="Рейтинг задач" value={puzzles.attempts ? puzzles.rating : "—"} />
      {collecting && <Tile label="Очки за задачи" value={points.total} sub={`сегодня: ${points.today}`} icon="star" />}
      {collecting && <Tile label="Лучшая серия" value={points.best} sub={`верных подряд, сейчас ${points.streak}`} icon="flame" />}
      {rush.map((r) => (
        <Tile key={r.mode} label={r.name} value={r.best} sub={`лучший результат, заездов ${r.runs}`} icon="bolt" />
      ))}
      {duels.total > 0 && <Tile label="Дуэли" value={recordText(duels)} sub="побед / ничьих / поражений" icon="duel" />}
    </div>
  );
}

function StreakSection({ streak }) {
  const cells = activityCells(streak);
  const active = activeDays(streak);
  return (
    <>
      <div className="stats__streakhead">
        <div className="stats__streakbig">
          <Icon name="flame" size={26} className={streak.current > 0 ? "stats__flame stats__flame--on" : "stats__flame"} />
          <div>
            <b>{streak.current > 0 ? daysText(streak.current) : "Серии нет"}</b>
            <span>
              {streak.current > 0
                ? streak.today
                  ? "решаете сегодня, так держать"
                  : "решите задачу сегодня, чтобы не прервать"
                : "решите задачу, чтобы начать серию"}
            </span>
          </div>
        </div>
        <div className="stats__streakbest">
          <span>Лучшая серия</span>
          <b>{streak.best > 0 ? daysText(streak.best) : "—"}</b>
        </div>
        <div className="stats__streakbest">
          <span>Дней с задачами за 30 дней</span>
          <b>{active}</b>
        </div>
      </div>
      <ol className="stats__strip" aria-label="Задачи по дням за последние 30 дней">
        {cells.map((c) => (
          <li key={c.date} className={`stats__cell stats__cell--${c.level}`} title={c.label} aria-label={c.label} />
        ))}
      </ol>
      <div className="stats__stripaxis">
        <span>{cells.length ? dayLabel(cells[0].date) : ""}</span>
        <span>сегодня</span>
      </div>
      <p className="stats__note">
        День засчитывается, если в нём была хотя бы одна задача: рейтинговая, задача дня, заезд штурма или дуэль. Дни считаются по
        календарю UTC (полночь по Гринвичу), серия не прерывается, пока не закончился сегодняшний день.
      </p>
    </>
  );
}

function WeakSection({ weak, attempts }) {
  const rows = weakRows(weak);
  if (!rows.length) {
    return (
      <Empty>
        {attempts > 0
          ? "Ошибок в задачах пока нет: слабых тем не видно."
          : "Пока нет данных, решите несколько задач, и здесь появятся темы, где вы чаще ошибаетесь."}
      </Empty>
    );
  }
  return (
    <>
      <ul className="stats__weak">
        {rows.map((r) => (
          <li key={r.theme}>
            <a className="stats__wname" href="#puzzles/themes" title="Тренировать по темам">
              {r.name}
            </a>
            <span className="stats__wbar" aria-hidden="true">
              <i style={{ width: `${r.share}%` }} />
            </span>
            <span className="stats__wnum">
              <b>{r.failed}</b> из {r.attempts}
            </span>
          </li>
        ))}
      </ul>
      <p className="stats__note">Темы, где больше всего неудачных первых попыток. Длину и стадию партии (короткая задача, миттельшпиль) не считаем.</p>
    </>
  );
}

function HistorySection({ history }) {
  const rows = historyRows(history);
  if (!rows.length) return <Empty>Пока нет данных: здесь будут последние 30 задач с изменением рейтинга.</Empty>;
  return (
    <ul className="stats__hist">
      {rows.map((r, i) => (
        <li key={`${r.id}-${i}`}>
          <a href={r.href} title="Открыть задачу">
            <span className={`stats__mark stats__mark--${r.solved ? "ok" : "bad"}`}>
              <Icon name={r.solved ? "check" : "close"} size={14} />
              <span className="stats__sr">{r.solved ? "решена" : "не решена"}</span>
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
  if (loading) return <section className="card stats__status">Загружаем статистику…</section>;
  if (error || !stats) {
    return (
      <section className="card stats__status">
        <div className="banner banner--error" role="alert">
          {error || "Не удалось загрузить статистику."}
        </div>
      </section>
    );
  }

  const { games, reviews, puzzles, streak } = stats;
  return (
    <div className="stats">
      {isEmpty(stats) && (
        <section className="card stats__hello">
          <h2>Пока нет данных</h2>
          <p>Сыграйте партию, разберите её или решите задачу: цифры и графики появятся здесь сами.</p>
        </section>
      )}

      <div className="stats__tiles">
        <Tile label="Партий" value={games.total} sub={games.total ? `побед ${percentText(games.wins, games.total)}` : "ещё не играли"} />
        <Tile label="Разобрано" value={reviews.count} sub={reviews.average != null ? `точность ${reviews.average}` : "разборов нет"} />
        <Tile
          label="Задач решено"
          value={puzzles.attempts ? percentText(puzzles.solved, puzzles.attempts) : "—"}
          sub={puzzles.attempts ? `${puzzles.solved} из ${puzzles.attempts}` : "задач нет"}
        />
        <Tile label="Рейтинг задач" value={puzzles.attempts ? puzzles.rating : "—"} />
        <Tile label="Серия дней" value={streak.current} sub={`лучшая: ${streak.best}`} icon="flame" />
      </div>

      <section className="card stats__sec">
        <header>
          <h2>Партии</h2>
          <span className="stats__hint">Онлайн и с ботами, победы / ничьи / поражения</span>
        </header>
        <GamesSection games={games} />
      </section>

      <section className="card stats__sec">
        <header>
          <h2>Точность в разборах</h2>
          <span className="stats__hint">
            {reviews.average != null ? `в среднем ${reviews.average}, последних ${reviews.series.length}` : "последние 20 разборов"}
          </span>
        </header>
        <ReviewsSection reviews={reviews} />
      </section>

      <section className="card stats__sec">
        <header>
          <h2>Серия дней</h2>
          <span className="stats__hint">Задачи подряд, день за днём</span>
        </header>
        <StreakSection streak={streak} />
      </section>

      <section className="card stats__sec">
        <header>
          <h2>Задачи</h2>
          <span className="stats__hint">Рейтинг, штурм и дуэли</span>
        </header>
        <PuzzlesSection puzzles={puzzles} />
      </section>

      <section className="card stats__sec">
        <header>
          <h2>Слабые темы</h2>
          <span className="stats__hint">Где вы чаще всего ошибаетесь</span>
        </header>
        <WeakSection weak={puzzles.weak} attempts={puzzles.attempts} />
      </section>

      <section className="card stats__sec">
        <header>
          <h2>История попыток</h2>
          <span className="stats__hint">Последние 30 задач, нажмите на задачу, чтобы открыть её</span>
        </header>
        <HistorySection history={puzzles.history} />
      </section>
    </div>
  );
}
