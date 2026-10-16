import { useCallback, useEffect, useState } from "react";
import { useAuth } from "../auth/AuthContext.jsx";
import Icon from "../ui/Icon.jsx";
import Segmented from "../ui/Segmented.jsx";
import Builder from "./Builder.jsx";
import GamesReview from "./GamesReview.jsx";
import PgnImport, { summaryOf } from "./PgnImport.jsx";
import Trainer from "./Trainer.jsx";
import { createRepertoire, deleteRepertoire, exportUrl, listRepertoires } from "./api.js";
import { learnedPercent } from "./tree.js";
import "./openings.css";

// "#openings" is the list, "#openings/<id>" builds one repository, "#openings/<id>/train"
// learns it, "#openings/<id>/games" puts the person's games against it, and
// "#openings/<id>/move/<node>[/<uci>]" opens the builder at a move of the tree (and
// first adds the move <uci> after it: what the opponent played in a game).
function routeFromHash() {
  const [page, id, mode, node, uci] = window.location.hash.slice(1).split("/");
  if (page !== "openings") return { id: 0, mode: "" };
  const n = Number(id);
  const route = { id: Number.isInteger(n) && n > 0 ? n : 0, mode: mode === "train" || mode === "games" ? mode : "" };
  if (route.id && mode === "move") {
    const at = Number(node);
    route.start = { node: Number.isInteger(at) && at >= 0 ? at : 0, uci: /^[a-h][1-8][a-h][1-8][qrbn]?$/.test(uci || "") ? uci : "" };
  }
  return route;
}

export default function OpeningsPage() {
  const [route, setRoute] = useState(routeFromHash);
  useEffect(() => {
    const onHash = () => setRoute(routeFromHash());
    window.addEventListener("hashchange", onHash);
    return () => window.removeEventListener("hashchange", onHash);
  }, []);
  if (route.id && route.mode === "train") return <Trainer key={route.id} id={route.id} />;
  if (route.id && route.mode === "games") return <GamesReview key={route.id} id={route.id} />;
  if (route.id) return <Builder key={`${route.id}:${route.start?.node}:${route.start?.uci}`} id={route.id} start={route.start} />;
  return <RepertoireList />;
}

const COLORS = [
  ["white", "За белых"],
  ["black", "За чёрных"],
];

function RepertoireList() {
  const { status, user, guest } = useAuth();
  const [items, setItems] = useState(null);
  const [error, setError] = useState("");
  const [name, setName] = useState("");
  const [color, setColor] = useState("white");
  const [busy, setBusy] = useState(false);
  const [imported, setImported] = useState(null); // what the last import did: { id, text }

  const load = useCallback(async () => {
    try {
      setItems(await listRepertoires());
      setError("");
    } catch (e) {
      setError(e.message);
    }
  }, []);
  useEffect(() => {
    if (status === "ready" && user) load();
    else if (status === "ready") setItems([]);
  }, [status, user, load]);

  async function create(e) {
    e.preventDefault();
    if (!name.trim()) return;
    setBusy(true);
    setError("");
    try {
      if (!user) await guest(); // a guest account keeps the repertoire
      const rep = await createRepertoire({ name, color });
      window.location.hash = `openings/${rep.id}`;
    } catch (err) {
      setError(err.message);
      setBusy(false);
    }
  }

  async function remove(rep) {
    if (!window.confirm(`Удалить репозиторий «${rep.name}» со всеми линиями? Это нельзя отменить.`)) return;
    try {
      await deleteRepertoire(rep.id);
      load();
    } catch (err) {
      setError(err.message);
    }
  }

  if (status === "unavailable") {
    return (
      <div className="page">
        <div className="banner banner--error">Дебюты недоступны: сервер запущен без базы данных.</div>
      </div>
    );
  }

  return (
    <div className="page openings">
      <header className="page__head">
        <h1 className="page__title">Дебюты</h1>
        <p className="page__sub">
          Соберите свой репозиторий ходов: линии, которые вы играете за белых и за чёрных. Потом тренажёр будет спрашивать ходы, пока вы не выучите их до автоматизма.
        </p>
      </header>

      {error && <div className="banner banner--error">{error}</div>}
      {imported && (
        <div className="banner banner--ok" role="status">
          {imported.text} <a href={`#openings/${imported.id}`}>Открыть в конструкторе</a>
        </div>
      )}

      {items && items.length > 0 && (
        <div className="replist">
          {items.map((r) => (
            <RepCard key={r.id} rep={r} onDelete={() => remove(r)} />
          ))}
        </div>
      )}
      {items && items.length === 0 && (
        <p className="openings__empty">
          Репозиториев пока нет. Создайте первый: например, «Итальянская партия» за белых или «Защита Каро-Канн» за чёрных.
        </p>
      )}

      <form className="card repnew" onSubmit={create}>
        <h2>Новый репозиторий ходов</h2>
        <input
          className="field"
          value={name}
          onChange={(e) => setName(e.target.value)}
          placeholder="Название, например «Староиндийская защита»"
          maxLength={80}
          aria-label="Название репозитория"
        />
        <Segmented label="Цвет" value={color} onChange={setColor} options={COLORS} />
        <button type="submit" className="btn btn--primary" disabled={busy || !name.trim() || status !== "ready"}>
          {busy ? "Создаю…" : "Создать и добавить ходы"}
        </button>
        {!user && status === "ready" && <p className="openings__note">Войдя через Google, вы сохраните репозиторий на любом устройстве. Пока он будет привязан к гостевому аккаунту.</p>}
      </form>

      {status === "ready" && (
        <PgnImport
          ensureAccount={user ? undefined : guest}
          onDone={(res) => {
            setImported({ id: res.repertoire.id, text: `«${res.repertoire.name}» создан. ${summaryOf(res)}` });
            load();
          }}
        />
      )}
    </div>
  );
}

function RepCard({ rep, onDelete }) {
  const c = rep.counts;
  const pct = learnedPercent(c);
  const todo = c.due + c.new;
  return (
    <section className="card repcard">
      <div className="repcard__head">
        <span className={`repcard__color repcard__color--${rep.color}`} title={rep.color === "white" ? "За белых" : "За чёрных"} />
        <h2>{rep.name}</h2>
        <button type="button" className="repcard__del" onClick={onDelete} title="Удалить репозиторий" aria-label="Удалить репозиторий">
          <Icon name="trash" size={16} />
        </button>
      </div>
      <p className="repcard__stats">
        Ходов в дереве: {c.moves}. Ваших: {c.mine}. Выучено: {c.learned} ({pct}%)
      </p>
      <div className="repbar" role="progressbar" aria-valuenow={pct} aria-valuemin={0} aria-valuemax={100}>
        <div className="repbar__fill" style={{ width: `${pct}%` }} />
      </div>
      <p className="repcard__todo">
        {c.mine === 0
          ? "Добавьте свои ходы в конструкторе."
          : todo === 0
            ? "На сегодня всё повторено."
            : `Сегодня: повторить ${c.due}, новых ${c.new}.`}
      </p>
      <div className="repcard__actions">
        <a className={`btn ${todo > 0 ? "btn--primary" : ""}`} href={`#openings/${rep.id}/train`} aria-disabled={c.mine === 0}>
          <Icon name="play" size={16} /> Тренировать{todo > 0 ? ` (${todo})` : ""}
        </a>
        <a className="btn" href={`#openings/${rep.id}`}>
          <Icon name="list" size={16} /> Конструктор
        </a>
        <a className="btn" href={`#openings/${rep.id}/games`} aria-disabled={c.mine === 0} title="Где вы вышли из репозитория в своих партиях">
          <Icon name="review" size={16} /> Мои партии
        </a>
        <a className="btn btn--ghost" href={exportUrl(rep.id)} download title="Скачать репозиторий файлом PGN: линии станут вариантами">
          <Icon name="copy" size={16} /> PGN
        </a>
      </div>
    </section>
  );
}
