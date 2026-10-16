import { useEffect, useMemo, useRef, useState } from "react";
import Icon from "../ui/Icon.jsx";
import { compareGames, fetchGames, getRepertoire, sendReview } from "./api.js";
import { deviationLine, outcomeOf } from "./games.js";

const MAX_BYTES = 4 << 20; // the server's limit
const NAME_KEY = "dyanis.openings.myName";

function savedName() {
  try {
    return localStorage.getItem(NAME_KEY) || "";
  } catch {
    return "";
  }
}

const COUNT_TEXT = {
  "you-deviated": "вы отклонились",
  "opponent-deviated": "соперник вышел из подготовки",
  "book-ended": "линия закончилась",
  "in-book": "вся партия по репозиторию",
};

// The person's recent games put against one repository: where each game left it,
// and what to do about that: take the move back into training, or add what the
// opponent played to the repository.
export default function GamesReview({ id }) {
  const [rep, setRep] = useState(null);
  const [data, setData] = useState(null);
  const [error, setError] = useState("");
  const [notFound, setNotFound] = useState(false);
  const [sent, setSent] = useState({}); // game id -> true once its move went back to training
  const [pasting, setPasting] = useState(false); // the form for games of the person's own PGN is open
  const [text, setText] = useState("");
  const [fileName, setFileName] = useState("");
  const [myName, setMyName] = useState(savedName);
  const [busy, setBusy] = useState(false);
  const [pasted, setPasted] = useState(null); // the answer for pasted games; replaces the list of the site's games
  const input = useRef(null);

  useEffect(() => {
    let alive = true;
    Promise.all([getRepertoire(id), fetchGames(id)])
      .then(([r, g]) => {
        if (!alive) return;
        setRep(r.repertoire);
        setData(g);
      })
      .catch((e) => {
        if (!alive) return;
        if (e.status === 404 || e.status === 401) setNotFound(true);
        else setError(e.message);
      });
    return () => {
      alive = false;
    };
  }, [id]);

  async function backToTraining(game) {
    const mine = game.deviation.expected.find((m) => m.mine);
    if (!mine) return;
    try {
      await sendReview(id, mine.id, false); // forgetting it in a real game is a failure
      setSent((s) => ({ ...s, [game.id]: true }));
    } catch (e) {
      setError(e.message);
    }
  }

  async function pick(e) {
    const file = e.target.files?.[0];
    e.target.value = ""; // the same file can be chosen again
    if (!file) return;
    if (file.size > MAX_BYTES) {
      setError("Файл больше 4 МБ.");
      return;
    }
    setError("");
    setText(await file.text());
    setFileName(file.name);
  }

  async function compare(e) {
    e.preventDefault();
    if (!text.trim()) return;
    setBusy(true);
    setError("");
    try {
      const res = await compareGames(id, text, myName.trim());
      try {
        localStorage.setItem(NAME_KEY, myName.trim());
      } catch {
        // the name is only a convenience
      }
      setPasted(res);
      setSent({});
    } catch (err) {
      setError(err.message);
    } finally {
      setBusy(false);
    }
  }

  const source = pasted || data;
  const counts = source?.counts || {};
  const shown = useMemo(() => source?.games || [], [source]);

  if (notFound) {
    return (
      <div className="page">
        <div className="banner banner--error">Такого репозитория нет или он принадлежит другому аккаунту.</div>
        <a className="btn btn--primary" href="#openings">
          К списку репозиториев
        </a>
      </div>
    );
  }
  if (!data || !rep) return <div className="page openings openings--loading">{error ? <div className="banner banner--error">{error}</div> : "Сравниваю ваши партии с репозиторием…"}</div>;

  return (
    <div className="page openings gamesrev">
      <a className="openings__back" href={`#openings/${id}`}>
        <Icon name="back" size={16} /> К репозиторию «{rep.name}»
      </a>
      <header className="page__head">
        <h1 className="page__title">Мои партии против репозитория</h1>
        <p className="page__sub">
          {pasted
            ? "Партии из вашего PGN"
            : "Сайт берёт ваши последние партии"}{" "}
          {rep.color === "white" ? "белыми" : "чёрными"}
          {pasted ? "" : " (онлайн и с ботами)"} и показывает, где каждая вышла из «{rep.name}»: забытый ход, неожиданный ответ соперника или конец линии.
        </p>
      </header>

      {error && <div className="banner banner--error">{error}</div>}

      <div className="room__row">
        {!pasting && (
          <button type="button" className="btn" onClick={() => setPasting(true)}>
            <Icon name="upload" size={16} /> Проверить свои партии из PGN
          </button>
        )}
        {pasted && (
          <button type="button" className="btn btn--ghost" onClick={() => setPasted(null)}>
            Вернуться к партиям с сайта
          </button>
        )}
      </div>

      {pasting && (
        <form className="card pgnimport" onSubmit={compare}>
          <h2>Партии из PGN</h2>
          <p className="openings__note">
            Вставьте партии, сыгранные где угодно (экспорт из chess.com, Lichess, ChessBase): сайт покажет, где каждая вышла из репозитория. Они нигде не сохраняются. Если в файле есть партии
            обоими цветами, впишите своё имя, как оно в тегах White и Black: остальные партии будут пропущены.
          </p>
          <div className="pgnimport__file">
            <input ref={input} type="file" accept=".pgn,.txt,text/plain,application/x-chess-pgn" onChange={pick} hidden />
            <button type="button" className="btn" onClick={() => input.current?.click()}>
              <Icon name="upload" size={16} /> Выбрать файл
            </button>
            <span className="pgnimport__name">{fileName || "или вставьте текст ниже"}</span>
          </div>
          <textarea
            className="field pgnimport__text"
            value={text}
            onChange={(e) => {
              setText(e.target.value);
              setFileName("");
            }}
            placeholder={"[White \"Вы\"]\n[Black \"Соперник\"]\n\n1. e4 e5 2. Nf3 Nc6 3. Bb5 a6 1-0"}
            spellCheck={false}
            rows={6}
            aria-label="Текст PGN"
          />
          <input className="field" value={myName} onChange={(e) => setMyName(e.target.value)} placeholder="Ваше имя в партиях (необязательно)" maxLength={80} aria-label="Ваше имя в партиях" />
          <div className="pgnimport__actions">
            <button type="submit" className="btn btn--primary" disabled={busy || !text.trim()}>
              {busy ? "Сравниваю…" : "Сравнить с репозиторием"}
            </button>
            <button type="button" className="btn btn--ghost" onClick={() => setPasting(false)} disabled={busy}>
              Закрыть
            </button>
          </div>
        </form>
      )}

      {pasted && (
        <p className="openings__note">
          В тексте партий: {pasted.read}
          {pasted.skipped > 0 ? `, пропущено: ${pasted.skipped} (чужой цвет, своя начальная позиция или меньше двух ходов)` : ""}.{pasted.warning ? ` ${pasted.warning[0].toUpperCase()}${pasted.warning.slice(1)}.` : ""}
        </p>
      )}

      {shown.length > 0 && (
        <p className="gamesrev__counts">
          {Object.entries(COUNT_TEXT)
            .filter(([k]) => counts[k])
            .map(([k, text]) => (
              <span key={k} className={`gamesrev__count gamesrev__count--${k}`}>
                {text}: {counts[k]}
              </span>
            ))}
        </p>
      )}

      {shown.length === 0 && pasted && <p className="openings__empty">Ни одной партии для сравнения: проверьте имя и цвет репозитория.</p>}
      {shown.length === 0 && !pasted && (
        <p className="openings__empty">
          Партий {rep.color === "white" ? "белыми" : "чёрными"} пока нет. Сыграйте онлайн или с ботом, и здесь появится разбор: где вы вышли из своих линий.
        </p>
      )}

      <div className="gamesrev__list">
        {shown.map((g) => {
          const d = g.deviation;
          const text = deviationLine(d);
          const out = outcomeOf(g.result, rep.color);
          return (
            <section key={g.id} className={`card gamesrev__game gamesrev__game--${d.status}`}>
              <div className="gamesrev__head">
                <span className={`gamesrev__result gamesrev__result--${out.kind}`}>{out.text}</span>
                <span className="gamesrev__who">
                  против {g.opponent || "соперника"}
                  {g.bot ? " (бот)" : ""}
                </span>
                <span className="gamesrev__when">
                  {g.endedMs ? new Date(g.endedMs).toLocaleDateString("ru-RU") : g.date || (pasted ? "дата неизвестна" : "идёт")} · ходов {Math.ceil(d.moves / 2)}
                </span>
              </div>
              <p className="gamesrev__text">{text.headline}</p>
              {text.line && <p className="openings__line">{text.line}</p>}
              <div className="room__row">
                {d.status === "you-deviated" && d.expected.some((m) => m.mine) && (
                  <button type="button" className="btn btn--primary" onClick={() => backToTraining(g)} disabled={sent[g.id]}>
                    {sent[g.id] ? "Вернули в тренировку" : "Вернуть этот ход в тренировку"}
                  </button>
                )}
                {(d.status === "opponent-deviated" || d.status === "book-ended") && d.played && (
                  <a className="btn btn--primary" href={`#openings/${id}/move/${d.node}/${d.played.uci}`}>
                    <Icon name="list" size={16} /> {d.status === "opponent-deviated" ? "Добавить этот ответ и свой ход" : "Продолжить линию"}
                  </a>
                )}
                {d.status !== "in-book" && d.status !== "empty" && d.status !== "broken" && (
                  <a className="btn" href={`#openings/${id}/move/${d.node}`}>
                    Открыть позицию в конструкторе
                  </a>
                )}
              </div>
            </section>
          );
        })}
      </div>
    </div>
  );
}
