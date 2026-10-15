import { useCallback, useEffect, useState } from "react";
import { useAuth } from "../auth/AuthContext.jsx";
import { deleteAnalysis, listAnalyses } from "../review/api.js";
import Icon from "../ui/Icon.jsx";
import OnlineGames from "./OnlineGames.jsx";
import "./games.css";

function formatDate(iso) {
  const d = new Date(iso);
  return Number.isNaN(d.getTime())
    ? ""
    : d.toLocaleDateString("ru-RU", { day: "numeric", month: "short", year: "numeric" });
}

function title(g) {
  const w = g.white || "Белые";
  const b = g.black || "Чёрные";
  return `${w} — ${b}`;
}

function Row({ item, onDelete, busy }) {
  const { game, summary, status } = item;
  const failed = status === "error";
  const running = status === "queued" || status === "running";
  return (
    <li className="games__row">
      <a className="games__link" href={`#review/${item.id}`}>
        <span className="games__result">{game.result && game.result !== "*" ? game.result : "—"}</span>
        <span className="games__main">
          <span className="games__title">{title(game)}</span>
          <span className="games__sub">
            {formatDate(item.created)}
            {summary?.opening ? ` · ${summary.opening}` : ""}
          </span>
        </span>
        {summary && (
          <span className="games__acc" title="Точность: белые / чёрные">
            <b>{Math.round(summary.accuracy[0])}</b>
            <i>/</i>
            <b>{Math.round(summary.accuracy[1])}</b>
          </span>
        )}
        {failed && <span className="games__state games__state--bad">не удался</span>}
        {running && <span className="games__state">идёт разбор…</span>}
      </a>
      <button
        type="button"
        className="games__delete"
        aria-label={`Удалить партию ${title(game)}`}
        title="Удалить"
        disabled={busy || running}
        onClick={() => onDelete(item)}
      >
        <Icon name="trash" size={18} />
      </button>
    </li>
  );
}

// The "Партии" tab of the profile: the reviews started while signed in, newest first.
// embedded: shown as a tab of the profile, which has its own heading.
export default function GamesPage({ embedded = false }) {
  const { status, user } = useAuth();
  const [items, setItems] = useState(null);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);

  const load = useCallback(async () => {
    try {
      setItems(await listAnalyses());
      setError("");
    } catch (e) {
      setError(e.message);
    }
  }, []);

  useEffect(() => {
    if (user) load();
  }, [user, load]);

  // A review that is still running: look again shortly.
  const running = items?.some((i) => i.status === "queued" || i.status === "running");
  useEffect(() => {
    if (!running) return undefined;
    const t = setTimeout(load, 1500);
    return () => clearTimeout(t);
  }, [running, items, load]);

  async function remove(item) {
    if (!window.confirm(`Удалить разбор «${title(item.game)}»? Ссылка на него перестанет работать.`)) return;
    setBusy(true);
    try {
      await deleteAnalysis(item.id);
      setItems((list) => list.filter((i) => i.id !== item.id));
    } catch (e) {
      setError(e.message);
    } finally {
      setBusy(false);
    }
  }

  if (status === "loading") return embedded ? null : <div className="page" />;

  return (
    <div className={embedded ? "games games--embedded" : "page games"}>
      {!embedded && (
        <header className="page__head">
          <h1 className="page__title">Мои партии</h1>
          <p className="page__sub">Разборы, которые вы запускали, будучи в аккаунте.</p>
        </header>
      )}

      {status === "unavailable" && (
        <div className="banner banner--error">Список недоступен: сервер запущен без базы данных.</div>
      )}

      {status === "ready" && !user && (
        <section className="card games__empty">
          <p>Войдите в аккаунт или продолжите как гость, и здесь появятся ваши разборы.</p>
          <a className="btn btn--primary" href="#account">
            Войти
          </a>
        </section>
      )}

      {embedded && user && <OnlineGames />}
      {embedded && user && <h2 className="games__h">Разборы</h2>}

      {error && <div className="banner banner--error">{error}</div>}

      {user && items && items.length === 0 && !error && (
        <section className="card games__empty">
          <p>Пока пусто. Разберите первую партию, и она появится здесь.</p>
          <a className="btn btn--primary" href="#review">
            Разобрать партию
          </a>
        </section>
      )}

      {user && items && items.length > 0 && (
        <ul className="card games__list">
          {items.map((it) => (
            <Row key={it.id} item={it} onDelete={remove} busy={busy} />
          ))}
        </ul>
      )}
    </div>
  );
}
