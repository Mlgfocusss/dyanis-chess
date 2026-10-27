import { useEffect, useState } from "react";
import { useAuth } from "../auth/AuthContext.jsx";
import ThemedBoard from "../components/ThemedBoard.jsx";
import { fetchPuzzle } from "../puzzles/api.js";
import { fetchRatings } from "../online/api.js";
import { categoryName, ratingText } from "../online/rating.js";
import LangSwitch from "../i18n/LangSwitch.jsx";
import Icon from "../ui/Icon.jsx";
import { useT } from "../i18n/index.js";
import "./home.css";

// Morphy's "Opera Game" a move before the end: the queen sacrifice 16.Qb8+!! (the same brilliant
// badge the review gives). A still picture: the board does not move, so it costs nothing.
const SHOWCASE_FEN = "4kb1r/p2n1ppp/4q3/4p1B1/4P3/1Q6/PPP2PPP/2KR4 w k - 0 16";
// an SVG attribute, so a CSS variable would not work here: the value of --primary
const SHOWCASE_ARROWS = [["b3", "b8", "rgba(52, 211, 153, 0.85)"]];
const SHOWCASE_BADGE = { square: "b8", cls: "brilliant" };

const FACTS = [
  { value: "≈ 2900", labelKey: "home.fact1" },
  { value: "80 000+", labelKey: "home.fact2" },
  { value: "10", labelKey: "home.fact3" },
  { value: "NNUE", labelKey: "home.fact4" },
];

const MODES = [
  { href: "#online", icon: "online", titleKey: "home.mode1", textKey: "home.mode1t" },
  { href: "#bots", icon: "bots", titleKey: "home.mode2", textKey: "home.mode2t" },
  { href: "#play", icon: "bot", titleKey: "home.mode3", textKey: "home.mode3t" },
];

const FEATURES = [
  { href: "#review", icon: "review", titleKey: "menu.review", textKey: "home.feat1t" },
  { href: "#puzzles", icon: "puzzles", titleKey: "menu.puzzles", textKey: "home.feat2t" },
  { href: "#openings", icon: "openings", titleKey: "menu.openings", textKey: "home.feat3t" },
  { href: "#settings", icon: "sliders", titleKey: "home.feat4", textKey: "home.feat4t" },
];

function greeting(t) {
  const h = new Date().getHours();
  if (h < 5) return t("home.night");
  if (h < 12) return t("home.morning");
  if (h < 18) return t("home.day");
  return t("home.evening");
}

// The ratings of a person who is signed in, as a row of figures.
function RatingsStrip() {
  const t = useT();
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
    <ul className="home__ratings" aria-label={t("home.ratings")}>
      {list.map((r) => (
        <li key={r.category} className="home__rating">
          <span className="home__rname">{categoryName(r.category) || r.name}</span>
          <b className="home__rvalue">{r.games ? ratingText(r.rating, r.provisional) : "—"}</b>
        </li>
      ))}
    </ul>
  );
}

// The puzzle of the day as a still board; nothing is shown when the server cannot give one.
function DailyPuzzle() {
  const t = useT();
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
          <Icon name="trophy" size={16} /> {t("menu.puzzlesDaily")}
        </span>
        <h3>{puzzle.orientation === "white" ? t("home.whiteMove") : t("home.blackMove")}</h3>
        <p>{done ? (daily.status === "solved" ? t("home.dailySolved") : t("home.dailyTried")) : t("home.dailyNew")}</p>
        <span className="home__more">
          {done ? t("home.view") : t("home.solve")} <Icon name="next" size={16} />
        </span>
      </div>
    </a>
  );
}

export default function HomePage() {
  const t = useT();
  const { status, user } = useAuth();
  const name = user?.name;

  return (
    <div className="page home">
      <section className="home__hero">
        <div className="home__copy">
          <LangSwitch className="langsw--mobile" />
          <span className="home__badge">
            <Icon name="crown" size={16} /> Dyanis Chess
          </span>
          <h1 className="home__title">
            {name ? (
              <>
                {greeting(t)}, <span className="home__name">{name}</span>
              </>
            ) : (
              <>
                {t("home.titleA")} <span className="home__name">{t("home.titleB")}</span>
              </>
            )}
          </h1>
          <p className="home__lead">
            {name ? t("home.leadUser") : t("home.leadGuest")}
          </p>
          <div className="home__cta">
            <a className="btn btn--primary home__btn" href="#online">
              <Icon name="play" size={18} /> {t("home.playOnline")}
            </a>
            <a className="btn home__btn" href="#bots">
              <Icon name="bots" size={18} /> {t("home.playBot")}
            </a>
            <a className="btn btn--ghost home__btn" href="#review">
              <Icon name="review" size={18} /> {t("home.reviewGame")}
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
            <span className="home__eval">{t("home.mateIn2")}</span>
            {t("home.opera")}
          </figcaption>
        </figure>
      </section>

      <ul className="home__facts">
        {FACTS.map((f) => (
          <li key={f.labelKey}>
            <b>{f.value}</b>
            <span>{t(f.labelKey)}</span>
          </li>
        ))}
      </ul>

      <section className="home__section" aria-labelledby="home-play">
        <h2 id="home-play" className="home__h2">
          {t("home.playGame")}
        </h2>
        <div className="home__modes">
          {MODES.map((m) => (
            <a key={m.href} className="card home__mode" href={m.href}>
              <span className="home__ico">
                <Icon name={m.icon} size={22} />
              </span>
              <h3>{t(m.titleKey)}</h3>
              <p>{t(m.textKey)}</p>
            </a>
          ))}
        </div>
      </section>

      <section className="home__section home__split">
        <DailyPuzzle />
        <div className="home__features">
          <h2 className="home__h2">{t("home.trainAll")}</h2>
          <ul className="home__flist">
            {FEATURES.map((f) => (
              <li key={f.href}>
                <a className="home__feature" href={f.href}>
                  <span className="home__ico home__ico--sm">
                    <Icon name={f.icon} size={18} />
                  </span>
                  <span className="home__ftext">
                    <b>{t(f.titleKey)}</b>
                    <span>{t(f.textKey)}</span>
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
