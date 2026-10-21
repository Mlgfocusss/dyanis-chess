import { useEffect, useState } from "react";
import { useAuth } from "../auth/AuthContext.jsx";
import ThemedBoard from "../components/ThemedBoard.jsx";
import { fetchPuzzle } from "../puzzles/api.js";
import { fetchRatings } from "../online/api.js";
import { ratingText } from "../online/rating.js";
import Icon from "../ui/Icon.jsx";
import "./home.css";

// Morphy's "Opera Game" a move before the end: the queen sacrifice 16.Qb8+!! (the same brilliant
// badge the review gives). A still picture: the board does not move, so it costs nothing.
const SHOWCASE_FEN = "4kb1r/p2n1ppp/4q3/4p1B1/4P3/1Q6/PPP2PPP/2KR4 w k - 0 16";
// an SVG attribute, so a CSS variable would not work here: the value of --primary
const SHOWCASE_ARROWS = [["b3", "b8", "rgba(52, 211, 153, 0.85)"]];
const SHOWCASE_BADGE = { square: "b8", cls: "brilliant" };

const FACTS = [
  { value: "≈ 2900", label: "рейтинг движка по шкале Stockfish" },
  { value: "80 000+", label: "задач с рейтингом и темами" },
  { value: "10", label: "ботов от 400 до 2900" },
  { value: "NNUE", label: "своя нейросеть, написанная с нуля" },
];

const MODES = [
  { href: "#online", icon: "online", title: "Играть онлайн", text: "Соперник с близким рейтингом или партия с другом по ссылке." },
  { href: "#bots", icon: "bots", title: "Играть с ботами", text: "Десять характеров, у каждого свой уровень и стиль." },
  { href: "#play", icon: "bot", title: "Против движка", text: "Сильнейший соперник прямо в браузере, без сервера." },
];

const FEATURES = [
  { href: "#review", icon: "review", title: "Разбор партии", text: "Оценка каждого хода, график, точность, значки от зевка до блестящего хода и название дебюта." },
  { href: "#puzzles", icon: "puzzles", title: "Задачи", text: "Рейтинговые задачи, темы, задача дня, тактический штурм и дуэль с живым соперником." },
  { href: "#openings", icon: "openings", title: "Дебюты", text: "Соберите свой репозиторий ходов и учите его с интервальным повторением." },
  { href: "#settings", icon: "sliders", title: "Свой вид", text: "Темы досок, фигуры и звуки, премувы, подтверждение хода и режим «Доска в центре»." },
];

function greeting() {
  const h = new Date().getHours();
  if (h < 5) return "Доброй ночи";
  if (h < 12) return "Доброе утро";
  if (h < 18) return "Добрый день";
  return "Добрый вечер";
}

// The ratings of a person who is signed in, as a row of figures.
function RatingsStrip() {
  const [list, setList] = useState(null);
  useEffect(() => {
    let alive = true;
    fetchRatings().then(
      (r) => alive && setList(r),
      () => alive && setList([])
    );
    return () => {
      alive = false;
    };
  }, []);
  // nothing to show until the first rated game: four dashes would only look empty
  if (!list || !list.some((r) => r.games)) return null;
  return (
    <ul className="home__ratings" aria-label="Ваши рейтинги">
      {list.map((r) => (
        <li key={r.category} className="home__rating">
          <span className="home__rname">{r.name}</span>
          <b className="home__rvalue">{r.games ? ratingText(r.rating, r.provisional) : "—"}</b>
        </li>
      ))}
    </ul>
  );
}

// The puzzle of the day as a still board; nothing is shown when the server cannot give one.
function DailyPuzzle() {
  const [data, setData] = useState(null);
  useEffect(() => {
    let alive = true;
    fetchPuzzle({ daily: true }).then(
      (d) => alive && setData(d),
      () => {}
    );
    return () => {
      alive = false;
    };
  }, []);
  if (!data?.puzzle?.steps?.length) return null;
  const { puzzle, daily } = data;
  const fen = puzzle.steps[0].fen; // after the opponent's move: what the person sees first
  const done = daily?.status && daily.status !== "new";
  return (
    <a className="card home__daily" href="#puzzles/daily">
      <div className="home__daily-board" aria-hidden="true">
        <ThemedBoard position={fen} orientation={puzzle.orientation} arePiecesDraggable={false} />
      </div>
      <div className="home__daily-body">
        <span className="home__kicker">
          <Icon name="trophy" size={16} /> Задача дня
        </span>
        <h3>{puzzle.orientation === "white" ? "Ход белых" : "Ход чёрных"}</h3>
        <p>{done ? (daily.status === "solved" ? "Сегодня вы её решили." : "Сегодняшняя попытка уже сделана.") : "Одна позиция в день. Найдёте лучший ход?"}</p>
        <span className="home__more">
          {done ? "Посмотреть" : "Решить"} <Icon name="next" size={16} />
        </span>
      </div>
    </a>
  );
}

export default function HomePage() {
  const { status, user } = useAuth();
  const name = user?.name;

  return (
    <div className="page home">
      <section className="home__hero">
        <div className="home__copy">
          <span className="home__badge">
            <Icon name="crown" size={16} /> Dyanis Chess
          </span>
          <h1 className="home__title">
            {name ? (
              <>
                {greeting()}, <span className="home__name">{name}</span>
              </>
            ) : (
              <>
                Шахматы на <span className="home__name">собственном движке</span>
              </>
            )}
          </h1>
          <p className="home__lead">
            {name
              ? "Сыграйте партию, решите задачу или разберите вчерашнюю игру: движок найдёт, где она перевернулась."
              : "Играйте онлайн и с ботами, разбирайте партии и учите дебюты. Движок и нейросеть написаны с нуля, и каждый ход оценивает именно он."}
          </p>
          <div className="home__cta">
            <a className="btn btn--primary home__btn" href="#online">
              <Icon name="play" size={18} /> Играть онлайн
            </a>
            <a className="btn home__btn" href="#bots">
              <Icon name="bots" size={18} /> Играть с ботом
            </a>
            <a className="btn btn--ghost home__btn" href="#review">
              <Icon name="review" size={18} /> Разобрать партию
            </a>
          </div>
          {user && status === "ready" && <RatingsStrip />}
        </div>

        <figure className="home__showcase">
          <div className="home__board" aria-hidden="true">
            <ThemedBoard
              position={SHOWCASE_FEN}
              arePiecesDraggable={false}
              customArrows={SHOWCASE_ARROWS}
              badge={SHOWCASE_BADGE}
            />
          </div>
          <figcaption className="home__caption">
            <span className="home__eval">Мат в два хода</span>
            Опера Морфи: 16.Ферзь b8+!! Разбор находит такие ходы сам.
          </figcaption>
        </figure>
      </section>

      <ul className="home__facts">
        {FACTS.map((f) => (
          <li key={f.label}>
            <b>{f.value}</b>
            <span>{f.label}</span>
          </li>
        ))}
      </ul>

      <section className="home__section" aria-labelledby="home-play">
        <h2 id="home-play" className="home__h2">
          Сыграть партию
        </h2>
        <div className="home__modes">
          {MODES.map((m) => (
            <a key={m.href} className="card home__mode" href={m.href}>
              <span className="home__ico">
                <Icon name={m.icon} size={22} />
              </span>
              <h3>{m.title}</h3>
              <p>{m.text}</p>
            </a>
          ))}
        </div>
      </section>

      <section className="home__section home__split">
        <DailyPuzzle />
        <div className="home__features">
          <h2 className="home__h2">Всё для тренировки</h2>
          <ul className="home__flist">
            {FEATURES.map((f) => (
              <li key={f.href}>
                <a className="home__feature" href={f.href}>
                  <span className="home__ico home__ico--sm">
                    <Icon name={f.icon} size={18} />
                  </span>
                  <span className="home__ftext">
                    <b>{f.title}</b>
                    <span>{f.text}</span>
                  </span>
                  <Icon name="next" size={16} className="home__arrow" />
                </a>
              </li>
            ))}
          </ul>
        </div>
      </section>
    </div>
  );
}
