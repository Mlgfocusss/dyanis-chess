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
import "./trainer.css";

const DIONIS = { id: "dionis", name: "Дионис Самусенко" };

// The "Тренер" page: Dionis looks at what the person has done on the site and makes a plan for the
// day, shows how they play (from their reviews) and says what to work on. It is rules over the
// person's own data, made on the server (backend/internal/trainer); no model is asked.
export default function TrainerPage() {
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
        <section className="card coach__status">Тренер работает только при подключённой базе данных: на этом сервере её нет.</section>
      </Shell>
    );
  }
  if (!user) {
    return (
      <Shell>
        <section className="card coach__status coach__welcome">
          <BotAvatar bot={DIONIS} size={64} />
          <div>
            <h1 className="coach__hello">Я Дионис, ваш тренер</h1>
            <p>Я смотрю, как вы играете, составляю план на день и подсказываю, что подтянуть. Чтобы я мог запоминать ваши партии и задачи, нужен аккаунт: хватит гостевого.</p>
            <div className="coach__actions">
              <button type="button" className="btn btn--primary" onClick={startAsGuest}>
                Продолжить гостем
              </button>
              <a className="btn" href="#account">
                Войти
              </a>
            </div>
            {guestError && <div className="banner banner--error">{guestError}</div>}
          </div>
        </section>
      </Shell>
    );
  }
  if (state.loading && !state.data) return <Shell>{<section className="card coach__status">Дионис смотрит ваши партии и задачи…</section>}</Shell>;
  if (state.error || !state.data) {
    return (
      <Shell>
        <section className="card coach__status">
          <div className="banner banner--error" role="alert">
            {state.error || "Не удалось загрузить тренера."}
          </div>
          <button type="button" className="btn" onClick={load}>
            Повторить
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
  return <div className="page coach">{children}</div>;
}

// ---------------------------------------------------------------- hero

function Hero({ data }) {
  const { greeting, streak, day } = data;
  const ds = dayText(day);
  return (
    <header className="coach__hero card">
      <BotAvatar bot={DIONIS} size={72} />
      <div className="coach__say">
        <span className="coach__who">Тренер Дионис</span>
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
            <i>{streak.current > 0 ? "дней подряд" : "серии нет"}</i>
          </span>
        </li>
        {ds && (
          <li className={`coach__chip${day.done === day.total ? " coach__chip--done" : ""}`}>
            <Icon name="check" size={18} />
            <span>
              <b>
                {day.done}/{day.total}
              </b>
              <i>на сегодня</i>
            </span>
          </li>
        )}
      </ul>
    </header>
  );
}

// ---------------------------------------------------------------- plan

function PlanSection({ plan, day }) {
  const firstOpen = plan.findIndex((it) => !it.done);
  const text = dayText(day);
  return (
    <section className="coach__sec" aria-labelledby="coach-plan">
      <div className="coach__sechead">
        <h2 id="coach-plan">План на сегодня</h2>
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
      <p className="coach__note">День считается по календарю UTC (полночь по Гринвичу), как серия дней в профиле.</p>
    </section>
  );
}

function PlanCard({ item, primary }) {
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
              <Icon name="check" size={13} /> Выполнено
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
  return (
    <section className="coach__sec" aria-labelledby="coach-play">
      <div className="coach__sechead">
        <h2 id="coach-play">Как вы играете</h2>
        {insights.enough && <span className="coach__hint">по {insights.used} последним разборам</span>}
      </div>
      {insights.enough ? <Picture insights={insights} /> : <Invitation insights={insights} newcomer={newcomer} />}
    </section>
  );
}

function Invitation({ insights, newcomer }) {
  const note = skippedNote(insights);
  return (
    <div className="coach__empty card">
      <p className="coach__lead">Сыграйте и разберите {insights.need} партии, тогда я покажу ваши сильные и слабые стороны.</p>
      {!newcomer && <p className="coach__sub">{needText(insights)}</p>}
      {note && <p className="coach__note">{note}</p>}
      <div className="coach__actions">
        <a className="btn btn--primary" href="#bots">
          Играть с ботом
        </a>
        <a className="btn" href="#review">
          Разобрать партию
        </a>
      </div>
    </div>
  );
}

function Picture({ insights }) {
  const { trend, rate, average } = insights;
  const tone = trendTone(trend);
  const note = skippedNote(insights);
  return (
    <>
      <div className="coach__tiles">
        <div className="coach__tile card">
          <span className="coach__tlabel">Средняя точность</span>
          <b className="coach__tvalue">{average != null ? `${num(average)}%` : "—"}</b>
          <span className="coach__tsub">за вашу сторону</span>
        </div>
        <div className={`coach__tile card coach__tile--${tone}`}>
          <span className="coach__tlabel">
            <Icon name="trend" size={15} /> Динамика
          </span>
          <b className="coach__tvalue">{trend ? trend.text : "—"}</b>
          <span className="coach__tsub">
            {trend ? `последние ${trend.recentCount} против предыдущих ${trend.previousCount}` : "нужно побольше партий для сравнения"}
          </span>
        </div>
        {rateTiles(rate).map((t) => (
          <div key={t.id} className={`coach__tile card coach__tile--${t.tone}`}>
            <span className="coach__tlabel">{t.label}</span>
            <b className="coach__tvalue">{t.value}</b>
            <span className="coach__tsub">на 100 ходов</span>
          </div>
        ))}
      </div>

      <div className="coach__cols">
        <div className="coach__panel card">
          <h3>Точность по фазам партии</h3>
          <Phases phases={insights.phases} />
          <p className="coach__note">{insights.rule}</p>
        </div>
        <div className="coach__panel card">
          <h3>Точность по разборам</h3>
          <DynamicsChart series={insights.series} average={average} />
        </div>
      </div>

      <div className="coach__panel card">
        <h3>Самые дорогие ошибки</h3>
        {insights.costly.length === 0 ? (
          <p className="coach__sub">Серьёзных ошибок в этих разборах нет. Так держать!</p>
        ) : (
          <ol className="costly">
            {insights.costly.map((c, i) => (
              <li key={`${c.review}-${c.ply}-${i}`}>
                <a className="costly__row" href={c.href} title="Открыть разбор партии">
                  <span className="costly__move">{moveLabel(c)}</span>
                  <span className="costly__main">
                    <span className={`costly__class costly__class--${c.class}`}>{classWord(c.class)}</span>
                    {c.best ? <span>лучше было {c.best}</span> : null}
                    <span className="costly__cap">{costlyCaption(c)}</span>
                  </span>
                  <b className="costly__loss">−{num(c.loss)}</b>
                </a>
              </li>
            ))}
          </ol>
        )}
        <p className="coach__note">Число справа показывает, сколько процентных пунктов вероятности выигрыша стоил ход. Ссылка открывает разбор той партии.</p>
      </div>
      {note && <p className="coach__note">{note}</p>}
    </>
  );
}

function Phases({ phases }) {
  const bars = phaseBars(phases);
  return (
    <ul className="phases" aria-label="Точность по фазам партии">
      {bars.map((b) => (
        <li key={b.id} className={`phases__col phases__col--${b.kind}`}>
          <span className="phases__val">{accuracyLabel(b.accuracy)}</span>
          <span className="phases__plot">
            <i style={{ height: `${b.height}%` }} />
          </span>
          <span className="phases__name">{b.name}</span>
          <span className="phases__moves">{b.moves} ход.</span>
        </li>
      ))}
    </ul>
  );
}

const CHART_HEIGHT = 200;

function DynamicsChart({ series, average }) {
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
        <svg width={width} height={CHART_HEIGHT} role="img" aria-label="Точность в последних разборах, от старых к новым">
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
              aria-label={`Разбор ${i + 1}: точность ${num(p.accuracy)}`}
              onPointerEnter={() => setHover(i)}
              onFocus={() => setHover(i)}
              onBlur={() => setHover(-1)}
            >
              <circle className="dyn__pt" cx={p.x} cy={p.y} r={hover === i ? 6 : 4.5} />
              <circle className="dyn__hit" cx={p.x} cy={p.y} r={14} />
            </a>
          ))}
          <text className="dyn__axis" x={left} y={CHART_HEIGHT - 5} textAnchor="start">
            давно
          </text>
          <text className="dyn__axis" x={width - right} y={CHART_HEIGHT - 5} textAnchor="end">
            недавно
          </text>
        </svg>
      )}
      {active && (
        <div className="dyn__tip" style={{ left: tipLeft, top: Math.max(0, active.y - 56) }}>
          <b>{num(active.accuracy)}%</b>
          <span>{new Date(active.at).toLocaleDateString("ru-RU", { day: "numeric", month: "short" })}</span>
        </div>
      )}
    </div>
  );
}

// ---------------------------------------------------------------- advice

function AdviceSection({ advice, newcomer }) {
  return (
    <section className="coach__sec" aria-labelledby="coach-advice">
      <div className="coach__sechead">
        <h2 id="coach-advice">Что подтянуть</h2>
      </div>
      {advice.length === 0 ? (
        <div className="coach__empty card">
          <p className="coach__lead">{newcomer ? "Пока мне не на что смотреть." : "Пока советов нет."}</p>
          <p className="coach__sub">Сыграйте несколько партий, решите задачи и разберите игру: тогда здесь появятся советы по вашим цифрам.</p>
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
