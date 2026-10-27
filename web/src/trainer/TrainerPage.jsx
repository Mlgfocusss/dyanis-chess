import { useCallback, useEffect, useRef, useState } from "react";
import BotAvatar from "../bots/BotAvatar.jsx";
import Icon from "../ui/Icon.jsx";
import { useAuth } from "../auth/AuthContext.jsx";
import {
  accuracyLabel,
  classWord,
  costlyCaption,
  dayPercent,
  dayText,
  dynamicsChart,
  fetchTrainer,
  itemPercent,
  kindIcon,
  moveLabel,
  needText,
  num,
  phaseBars,
  progressText,
  rateTiles,
  rememberTheme,
  skippedNote,
  trendTone,
} from "./trainer.js";
import { locale, useT } from "../i18n/index.js";
import "./trainer.css";

const DIONIS = { id: "dionis", name: "Дионис Самусенко" };

// The "Тренер" page: Dionis looks at what the person has done on the site and makes a plan for the
// day, shows how they play (from their reviews) and says what to work on. It is rules over the
// person's own data, made on the server (backend/internal/trainer); no model is asked.
export default function TrainerPage() {
  const t = useT();
  const { status, user, guest } = useAuth();
  const [state, setState] = useState({ data: null, error: "", loading: true });
  const [guestError, setGuestError] = useState("");
  const userId = user?.id || "";

  const load = useCallback(() => {
    setState((s) => ({ ...s, error: "", loading: true }));
    let alive = true;
    fetchTrainer().then(
      (data) => alive && setState({ data, error: "", loading: false }),
      (e) => alive && setState({ data: null, error: e.message, loading: false })
    );
    return () => {
      alive = false;
    };
  }, []);

  useEffect(() => {
    if (status !== "ready" || !userId) return undefined;
    return load();
  }, [status, userId, load]);

  async function startAsGuest() {
    setGuestError("");
    try {
      await guest();
    } catch (e) {
      setGuestError(e.message);
    }
  }

  if (status === "loading") return <Shell>{null}</Shell>;
  if (status === "unavailable") {
    return (
      <Shell>
        <section className="card coach__status">{t("trainerPage.2")}</section>
      </Shell>
    );
  }
  if (!user) {
    return (
      <Shell>
        <section className="card coach__status coach__welcome">
          <BotAvatar bot={DIONIS} size={64} />
          <div>
            <h1 className="coach__hello">{t("trainerPage.3")}</h1>
            <p>{t("trainerPage.4")}</p>
            <div className="coach__actions">
              <button type="button" className="btn btn--primary" onClick={startAsGuest}>
                {t("trainerPage.5")}
              </button>
              <a className="btn" href="#account">
                {t("menu.signIn")}
              </a>
            </div>
            {guestError && <div className="banner banner--error">{guestError}</div>}
          </div>
        </section>
      </Shell>
    );
  }
  if (state.loading && !state.data) return <Shell>{<section className="card coach__status">{t("trainerPage.7")}</section>}</Shell>;
  if (state.error || !state.data) {
    return (
      <Shell>
        <section className="card coach__status">
          <div className="banner banner--error" role="alert">
            {state.error || t("trainerPage.8")}
          </div>
          <button type="button" className="btn" onClick={load}>
            {t("trainerPage.9")}
          </button>
        </section>
      </Shell>
    );
  }

  const d = state.data;
  return (
    <Shell>
      <Hero data={d} />
      <PlanSection plan={d.plan} day={d.day} />
      <PlaySection insights={d.insights} newcomer={d.newcomer} />
      <AdviceSection advice={d.advice} newcomer={d.newcomer} />
    </Shell>
  );
}

function Shell({ children }) {
  const t = useT();
  return <div className="page coach">{children}</div>;
}

// ---------------------------------------------------------------- hero

function Hero({ data }) {
  const t = useT();
  const { greeting, streak, day } = data;
  const ds = dayText(day);
  return (
    <header className="coach__hero card">
      <BotAvatar bot={DIONIS} size={72} />
      <div className="coach__say">
        <span className="coach__who">{t("dionisCard.2")}</span>
        <h1 className="coach__hello">{greeting.hello}</h1>
        {greeting.lines.map((line, i) => (
          <p key={i}>{line}</p>
        ))}
      </div>
      <ul className="coach__chips">
        <li className={`coach__chip${streak.current > 0 ? " coach__chip--warm" : ""}`}>
          <Icon name="flame" size={18} />
          <span>
            <b>{streak.current}</b>
            <i>{streak.current > 0 ? t("trainerPage.11") : t("trainerPage.12")}</i>
          </span>
        </li>
        {ds && (
          <li className={`coach__chip${day.done === day.total ? " coach__chip--done" : ""}`}>
            <Icon name="check" size={18} />
            <span>
              <b>
                {day.done}/{day.total}
              </b>
              <i>{t("trainerPage.13")}</i>
            </span>
          </li>
        )}
      </ul>
    </header>
  );
}

// ---------------------------------------------------------------- plan

function PlanSection({ plan, day }) {
  const t = useT();
  const firstOpen = plan.findIndex((it) => !it.done);
  const text = dayText(day);
  return (
    <section className="coach__sec" aria-labelledby="coach-plan">
      <div className="coach__sechead">
        <h2 id="coach-plan">{t("trainerPage.14")}</h2>
        {text && <span className="coach__hint">{text}</span>}
      </div>
      {day.total > 0 && (
        <div className="coach__bar coach__bar--day" role="img" aria-label={text}>
          <i style={{ width: `${dayPercent(day)}%` }} />
        </div>
      )}
      <ul className="plan">
        {plan.map((it, i) => (
          <PlanCard key={it.id} item={it} primary={i === firstOpen} />
        ))}
      </ul>
      <p className="coach__note">{t("trainerPage.15")}</p>
    </section>
  );
}

function PlanCard({ item, primary }) {
  const t = useT();
  const pct = itemPercent(item);
  return (
    <li className={`plan__item card${item.done ? " plan__item--done" : ""}`}>
      <span className="plan__ico">
        <Icon name={item.done ? "check" : kindIcon(item.kind)} size={22} />
      </span>
      <div className="plan__main">
        <div className="plan__titlerow">
          <h3>{item.title}</h3>
          {item.done && (
            <span className="plan__tag">
              <Icon name="check" size={13} /> {t("trainerPage.16")}
            </span>
          )}
        </div>
        <p>{item.text}</p>
        {pct !== null && (
          <div className="plan__prog">
            <div className="coach__bar" role="img" aria-label={`${item.title}: ${progressText(item)}`}>
              <i style={{ width: `${pct}%` }} />
            </div>
            <span>{progressText(item)}</span>
          </div>
        )}
      </div>
      <a className={`btn plan__go${primary ? " btn--primary" : " btn--ghost"}`} href={item.href} onClick={() => rememberTheme(item.theme)}>
        {item.actionLabel}
      </a>
    </li>
  );
}

// ---------------------------------------------------------------- how you play

function PlaySection({ insights, newcomer }) {
  const t = useT();
  return (
    <section className="coach__sec" aria-labelledby="coach-play">
      <div className="coach__sechead">
        <h2 id="coach-play">{t("trainerPage.17")}</h2>
        {insights.enough && <span className="coach__hint">{t("trainerPage.18")} {insights.used} {t("trainerPage.19")}</span>}
      </div>
      {insights.enough ? <Picture insights={insights} /> : <Invitation insights={insights} newcomer={newcomer} />}
    </section>
  );
}

function Invitation({ insights, newcomer }) {
  const t = useT();
  const note = skippedNote(insights);
  return (
    <div className="coach__empty card">
      <p className="coach__lead">{t("trainerPage.20")} {insights.need} {t("trainerPage.21")}</p>
      {!newcomer && <p className="coach__sub">{needText(insights)}</p>}
      {note && <p className="coach__note">{note}</p>}
      <div className="coach__actions">
        <a className="btn btn--primary" href="#bots">
          {t("home.playBot")}
        </a>
        <a className="btn" href="#review">
          {t("home.reviewGame")}
        </a>
      </div>
    </div>
  );
}

function Picture({ insights }) {
  const t = useT();
  const { trend, rate, average } = insights;
  const tone = trendTone(trend);
  const note = skippedNote(insights);
  return (
    <>
      <div className="coach__tiles">
        <div className="coach__tile card">
          <span className="coach__tlabel">{t("trainerPage.24")}</span>
          <b className="coach__tvalue">{average != null ? `${num(average)}%` : "—"}</b>
          <span className="coach__tsub">{t("trainerPage.25")}</span>
        </div>
        <div className={`coach__tile card coach__tile--${tone}`}>
          <span className="coach__tlabel">
            <Icon name="trend" size={15} /> {t("trainerPage.26")}
          </span>
          <b className="coach__tvalue">{trend ? trend.text : "—"}</b>
          <span className="coach__tsub">
            {trend ? t("trainerPage.27", { recentCount: trend.recentCount, previousCount: trend.previousCount }) : t("trainerPage.28")}
          </span>
        </div>
        {rateTiles(rate).map((tile) => (
          <div key={tile.id} className={`coach__tile card coach__tile--${tile.tone}`}>
            <span className="coach__tlabel">{tile.label}</span>
            <b className="coach__tvalue">{tile.value}</b>
            <span className="coach__tsub">{t("trainerPage.29")}</span>
          </div>
        ))}
      </div>

      <div className="coach__cols">
        <div className="coach__panel card">
          <h3>{t("trainerPage.30")}</h3>
          <Phases phases={insights.phases} />
          <p className="coach__note">{insights.rule}</p>
        </div>
        <div className="coach__panel card">
          <h3>{t("trainerPage.31")}</h3>
          <DynamicsChart series={insights.series} average={average} />
        </div>
      </div>

      <div className="coach__panel card">
        <h3>{t("trainerPage.32")}</h3>
        {insights.costly.length === 0 ? (
          <p className="coach__sub">{t("trainerPage.33")}</p>
        ) : (
          <ol className="costly">
            {insights.costly.map((c, i) => (
              <li key={`${c.review}-${c.ply}-${i}`}>
                <a className="costly__row" href={c.href} title={t("trainerPage.34")}>
                  <span className="costly__move">{moveLabel(c)}</span>
                  <span className="costly__main">
                    <span className={`costly__class costly__class--${c.class}`}>{classWord(c.class)}</span>
                    {c.best ? <span>{t("trainerPage.35")} {c.best}</span> : null}
                    <span className="costly__cap">{costlyCaption(c)}</span>
                  </span>
                  <b className="costly__loss">−{num(c.loss)}</b>
                </a>
              </li>
            ))}
          </ol>
        )}
        <p className="coach__note">{t("trainerPage.36")}</p>
      </div>
      {note && <p className="coach__note">{note}</p>}
    </>
  );
}

function Phases({ phases }) {
  const t = useT();
  const bars = phaseBars(phases);
  return (
    <ul className="phases" aria-label={t("trainerPage.30")}>
      {bars.map((b) => (
        <li key={b.id} className={`phases__col phases__col--${b.kind}`}>
          <span className="phases__val">{accuracyLabel(b.accuracy)}</span>
          <span className="phases__plot">
            <i style={{ height: `${b.height}%` }} />
          </span>
          <span className="phases__name">{b.name}</span>
          <span className="phases__moves">{b.moves} {t("trainerPage.37")}</span>
        </li>
      ))}
    </ul>
  );
}

const CHART_HEIGHT = 200;

function DynamicsChart({ series, average }) {
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

  const chart = dynamicsChart(series, average, width, CHART_HEIGHT);
  const { left, right, top, bottom } = chart.pad;
  const active = hover >= 0 ? chart.points[hover] : null;
  const tipLeft = active ? Math.min(Math.max(active.x, 70), Math.max(70, width - 70)) : 0;
  const avgY = chart.avgY;

  return (
    <div className="dyn" ref={box} onPointerLeave={() => setHover(-1)}>
      {width > 0 && chart.points.length > 0 && (
        <svg width={width} height={CHART_HEIGHT} role="img" aria-label={t("statsTab.6")}>
          {chart.ticks.map((t) => (
            <g key={t.value}>
              <line className="dyn__grid" x1={left} x2={width - right} y1={t.y} y2={t.y} />
              <text className="dyn__axis" x={left - 8} y={t.y + 4} textAnchor="end">
                {t.value}
              </text>
            </g>
          ))}
          {avgY != null && avgY >= top && avgY <= CHART_HEIGHT - bottom && <line className="dyn__avg" x1={left} x2={width - right} y1={avgY} y2={avgY} />}
          {chart.points.length > 1 && <path className="dyn__line" d={chart.line} />}
          {chart.points.map((p, i) => (
            <a
              key={p.id}
              href={`#review/${p.id}`}
              aria-label={t("trainerPage.39", { p: i + 1, p2: num(p.accuracy) })}
              onPointerEnter={() => setHover(i)}
              onFocus={() => setHover(i)}
              onBlur={() => setHover(-1)}
            >
              <circle className="dyn__pt" cx={p.x} cy={p.y} r={hover === i ? 6 : 4.5} />
              <circle className="dyn__hit" cx={p.x} cy={p.y} r={14} />
            </a>
          ))}
          <text className="dyn__axis" x={left} y={CHART_HEIGHT - 5} textAnchor="start">
            {t("statsTab.8")}
          </text>
          <text className="dyn__axis" x={width - right} y={CHART_HEIGHT - 5} textAnchor="end">
            {t("statsTab.9")}
          </text>
        </svg>
      )}
      {active && (
        <div className="dyn__tip" style={{ left: tipLeft, top: Math.max(0, active.y - 56) }}>
          <b>{num(active.accuracy)}%</b>
          <span>{new Date(active.at).toLocaleDateString(locale(), { day: "numeric", month: "short" })}</span>
        </div>
      )}
    </div>
  );
}

// ---------------------------------------------------------------- advice

function AdviceSection({ advice, newcomer }) {
  const t = useT();
  return (
    <section className="coach__sec" aria-labelledby="coach-advice">
      <div className="coach__sechead">
        <h2 id="coach-advice">{t("trainerPage.42")}</h2>
      </div>
      {advice.length === 0 ? (
        <div className="coach__empty card">
          <p className="coach__lead">{newcomer ? t("trainerPage.43") : t("trainerPage.44")}</p>
          <p className="coach__sub">{t("trainerPage.45")}</p>
        </div>
      ) : (
        <ul className="advice">
          {advice.map((a) => (
            <li key={a.id} className="advice__card card">
              <h3>{a.title}</h3>
              <p>{a.text}</p>
              <div className="coach__actions">
                <a className="btn btn--primary" href={a.href} onClick={() => rememberTheme(a.theme)}>
                  {a.actionLabel}
                </a>
                {(a.more || []).map((m) => (
                  <a key={m.href + m.label} className="btn btn--ghost" href={m.href} onClick={() => rememberTheme(m.theme)}>
                    {m.label}
                  </a>
                ))}
              </div>
            </li>
          ))}
        </ul>
      )}
    </section>
  );
}
