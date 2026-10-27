import { useEffect, useMemo, useRef, useState } from "react";
import Icon from "../ui/Icon.jsx";
import { compareGames, fetchGames, getRepertoire, sendReview } from "./api.js";
import { deviationLine, outcomeOf, transposeNote } from "./games.js";
import { locale, useT } from "../i18n/index.js";

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
  "you-deviated": "gamesReview.deviated",
  "opponent-deviated": "gamesReview.oppLeft",
  "book-ended": "gamesReview.ended",
  "in-book": "gamesReview.inBook",
};

// The person's recent games put against one repository: where each game left it,
// and what to do about that: take the move back into training, or add what the
// opponent played to the repository.
export default function GamesReview({ id }) {
  const t = useT();
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
      setError(t("pgnImportUi.4"));
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
        <div className="banner banner--error">{t("builder.8")}</div>
        <a className="btn btn--primary" href="#openings">
          {t("builder.9")}
        </a>
      </div>
    );
  }
  if (!data || !rep) return <div className="page openings openings--loading">{error ? <div className="banner banner--error">{error}</div> : t("gamesReview.8")}</div>;

  return (
    <div className="page openings gamesrev">
      <a className="openings__back" href={`#openings/${id}`}>
        <Icon name="back" size={16} /> {t("trainer.19")}{rep.name}»
      </a>
      <header className="page__head">
        <h1 className="page__title">{t("gamesReview.10")}</h1>
        <p className="page__sub">
          {pasted
            ? t("gamesReview.11")
            : t("gamesReview.12")}{" "}
          {rep.color === "white" ? t("gamesReview.13") : t("gamesReview.14")}
          {pasted ? "" : t("gamesReview.15")} {t("gamesReview.16")}{rep.name}{t("gamesReview.17")}
        </p>
      </header>

      {error && <div className="banner banner--error">{error}</div>}

      <div className="room__row">
        {!pasting && (
          <button type="button" className="btn" onClick={() => setPasting(true)}>
            <Icon name="upload" size={16} /> {t("gamesReview.18")}
          </button>
        )}
        {pasted && (
          <button type="button" className="btn btn--ghost" onClick={() => setPasted(null)}>
            {t("gamesReview.19")}
          </button>
        )}
      </div>

      {pasting && (
        <form className="card pgnimport" onSubmit={compare}>
          <h2>{t("gamesReview.20")}</h2>
          <p className="openings__note">
            {t("gamesReview.21")}
          </p>
          <div className="pgnimport__file">
            <input ref={input} type="file" accept=".pgn,.txt,text/plain,application/x-chess-pgn" onChange={pick} hidden />
            <button type="button" className="btn" onClick={() => input.current?.click()}>
              <Icon name="upload" size={16} /> {t("pgnImportUi.8")}
            </button>
            <span className="pgnimport__name">{fileName || t("pgnImportUi.9")}</span>
          </div>
          <textarea
            className="field pgnimport__text"
            value={text}
            onChange={(e) => {
              setText(e.target.value);
              setFileName("");
            }}
            placeholder={t("gamesReview.24")}
            spellCheck={false}
            rows={6}
            aria-label={t("pgnImportUi.11")}
          />
          <input className="field" value={myName} onChange={(e) => setMyName(e.target.value)} placeholder={t("gamesReview.26")} maxLength={80} aria-label={t("gamesReview.27")} />
          <div className="pgnimport__actions">
            <button type="submit" className="btn btn--primary" disabled={busy || !text.trim()}>
              {busy ? t("gamesReview.28") : t("gamesReview.29")}
            </button>
            <button type="button" className="btn btn--ghost" onClick={() => setPasting(false)} disabled={busy}>
              {t("gamesReview.close")}
            </button>
          </div>
        </form>
      )}

      {pasted && (
        <p className="openings__note">
          {t("gamesReview.31")} {pasted.read}
          {pasted.skipped > 0 ? t("gamesReview.32", { skipped: pasted.skipped }) : ""}.{pasted.warning ? ` ${pasted.warning[0].toUpperCase()}${pasted.warning.slice(1)}.` : ""}
        </p>
      )}

      {shown.length > 0 && (
        <p className="gamesrev__counts">
          {Object.entries(COUNT_TEXT)
            .filter(([k]) => counts[k])
            .map(([k, text]) => (
              <span key={k} className={`gamesrev__count gamesrev__count--${k}`}>
                {t(text)}: {counts[k]}
              </span>
            ))}
        </p>
      )}

      {shown.length === 0 && pasted && <p className="openings__empty">{t("gamesReview.33")}</p>}
      {shown.length === 0 && !pasted && (
        <p className="openings__empty">
          {t("gamesReview.34")} {rep.color === "white" ? t("gamesReview.13") : t("gamesReview.14")} {t("gamesReview.35")}
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
                  {t("onlineGames.10")} {g.opponent || t("onlineGames.11")}
                  {g.bot ? t("gamesReview.38") : ""}
                </span>
                <span className="gamesrev__when">
                  {g.endedMs ? new Date(g.endedMs).toLocaleDateString(locale()) : g.date || (pasted ? t("gamesReview.39") : t("onlineGames.1"))} {t("gamesReview.41")} {Math.ceil(d.moves / 2)}
                </span>
              </div>
              <p className="gamesrev__text">{text.headline}</p>
              {text.line && <p className="openings__line">{text.line}</p>}
              {transposeNote(d) && (
                <p className="gamesrev__trans">
                  <Icon name="branch" size={14} /> {transposeNote(d)}
                </p>
              )}
              <div className="room__row">
                {d.status === "you-deviated" && d.expected.some((m) => m.mine) && (
                  <button type="button" className="btn btn--primary" onClick={() => backToTraining(g)} disabled={sent[g.id]}>
                    {sent[g.id] ? t("gamesReview.42") : t("gamesReview.43")}
                  </button>
                )}
                {(d.status === "opponent-deviated" || d.status === "book-ended") && d.played && (
                  <a className="btn btn--primary" href={`#openings/${id}/move/${d.node}/${d.played.uci}`}>
                    <Icon name="list" size={16} /> {d.status === "opponent-deviated" ? t("gamesReview.44") : t("builder.39")}
                  </a>
                )}
                {d.status !== "in-book" && d.status !== "empty" && d.status !== "broken" && (
                  <a className="btn" href={`#openings/${id}/move/${d.node}`}>
                    {t("gamesReview.46")}
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
