import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import Board from "../components/Board.jsx";
import { useMoveSound } from "../settings/SettingsContext.jsx";
import { fetchLine } from "../review/api.js";
import { uciSquares } from "../review/classes.js";
import { sideOf } from "../review/branch.js";
import Icon from "../ui/Icon.jsx";
import { scrollWithin } from "../ui/scroll.js";
import { addMove, deleteMove, exportUrl, getRepertoire, renameRepertoire } from "./api.js";
import PgnImport, { summaryOf } from "./PgnImport.jsx";
import Suggestions from "./Suggestions.jsx";
import { openingName, openingTitle } from "./names.js";
import { buildTree, childWith, countsOf, fenAt, gapsOf, learnedPercent, lineText, pathTo, statusOf, subtreeIds, whoMoves } from "./tree.js";

const STATUS_TEXT = { opponent: "ход соперника", new: "новый", due: "пора повторить", learning: "учится", learned: "выучен" };

// The builder: the board and the tree of lines. A move made on the board is
// added to the repertoire (or, if it is already there, the board just follows it).
export default function Builder({ id, start }) {
  const [data, setData] = useState(null); // { repertoire, moves }
  const [cur, setCur] = useState(0); // the move the board stands after; 0 = the start position
  const [error, setError] = useState("");
  const [notFound, setNotFound] = useState(false);
  const [boardKey, setBoardKey] = useState(0);
  const [legalCache, setLegalCache] = useState({});
  const [opening, setOpening] = useState(null);
  const [hover, setHover] = useState(null); // the suggested move the mouse is over: shown as an arrow
  const [added, setAdded] = useState(null); // the move just added to the repertoire: { id, san }
  const [orientation, setOrientation] = useState(null);
  const [importing, setImporting] = useState(false); // the PGN import form is open
  const [importMsg, setImportMsg] = useState(""); // what the last import did
  const [editing, setEditing] = useState(false);
  const [draftName, setDraftName] = useState("");
  const playMoveSound = useMoveSound();
  const asking = useRef(new Set());

  const load = useCallback(async () => {
    try {
      const d = await getRepertoire(id);
      setData({ repertoire: d.repertoire, moves: d.moves });
    } catch (e) {
      if (e.status === 404 || e.status === 401) setNotFound(true);
      else setError(e.message);
    }
  }, [id]);
  useEffect(() => {
    load();
  }, [load]);

  // Coming from the review of a game: stand at the move where the game left the
  // repository, after first adding the move the opponent played there.
  const started = useRef(false);
  useEffect(() => {
    if (!data || !start || started.current) return;
    started.current = true;
    const at = data.moves.some((m) => m.id === start.node) ? start.node : 0;
    window.history.replaceState(null, "", `#openings/${id}`); // a reload must not do it again
    if (!start.uci) {
      setCur(at);
      return;
    }
    addMove(id, at, start.uci)
      .then((r) => {
        setData((d) => (d.moves.some((m) => m.id === r.move.id) ? d : { ...d, moves: [...d.moves, r.move] }));
        setCur(r.move.id);
        if (r.created) setAdded({ id: r.move.id, san: r.move.san, mine: r.move.mine });
      })
      .catch((e) => {
        setError(e.message);
        setCur(at);
      });
  }, [data, start, id]);

  const moves = data?.moves;
  const tree = useMemo(() => buildTree(moves || []), [moves]);
  const counts = useMemo(() => countsOf(moves || []), [moves]);
  const gaps = useMemo(() => gapsOf(tree), [tree]);
  const color = data?.repertoire.color || "white";
  const side = orientation || color;
  const fen = fenAt(tree, cur);
  const path = useMemo(() => pathTo(tree, cur), [tree, cur]);
  const children = tree.children.get(cur) || [];
  const turn = whoMoves(color, cur, tree);

  // the legal moves of the position on the board come from the server
  useEffect(() => {
    if (!data || legalCache[fen] || asking.current.has(fen)) return;
    asking.current.add(fen);
    fetchLine({ fen })
      .then((r) => setLegalCache((c) => ({ ...c, [fen]: r.legal })))
      .catch((e) => setError(e.message))
      .finally(() => asking.current.delete(fen));
  }, [data, fen, legalCache]);

  // the name of the opening
  useEffect(() => {
    if (!path.length) {
      setOpening(null);
      return undefined;
    }
    let alive = true;
    fetch(`/api/opening?san=${encodeURIComponent(path.map((m) => m.san).join(","))}`)
      .then((r) => (r.ok ? r.json() : null))
      .then((j) => alive && setOpening(j?.opening || null))
      .catch(() => {});
    return () => {
      alive = false;
    };
  }, [path]);

  const boardState = useMemo(() => ({ fen, legalMoves: (legalCache[fen] || []).map((uci) => ({ uci })) }), [fen, legalCache]);

  function play(uci) {
    const known = childWith(tree, cur, uci);
    if (known) {
      playMoveSound(known.san);
      setCur(known.id);
      return { ok: true };
    }
    setError("");
    addMove(id, cur, uci)
      .then((r) => {
        setData((d) => (d.moves.some((m) => m.id === r.move.id) ? d : { ...d, moves: [...d.moves, r.move] }));
        playMoveSound(r.move.san);
        setCur(r.move.id);
        if (r.created) setAdded({ id: r.move.id, san: r.move.san, mine: r.move.mine });
      })
      .catch((e) => {
        setError(e.message);
        setBoardKey((k) => k + 1); // put the piece back
      });
    return { ok: true };
  }

  // takes back the move that was just added (and whatever was built on it since)
  async function undoAdded() {
    const a = added;
    setAdded(null);
    const m = tree.byId.get(a.id);
    if (!m) return;
    try {
      await deleteMove(id, a.id);
      const doomed = new Set(subtreeIds(tree, a.id));
      setData((d) => ({ ...d, moves: d.moves.filter((x) => !doomed.has(x.id)) }));
      if (doomed.has(cur)) setCur(m.parent);
    } catch (e) {
      setError(e.message);
    }
  }

  async function removeCurrent() {
    const m = tree.byId.get(cur);
    if (!m) return;
    const n = subtreeIds(tree, cur).length;
    const text = n > 1 ? `Удалить ход ${m.san} и ещё ${n - 1} ход(ов) после него?` : `Удалить ход ${m.san}?`;
    if (!window.confirm(text)) return;
    try {
      await deleteMove(id, cur);
      const doomed = new Set(subtreeIds(tree, cur));
      setData((d) => ({ ...d, moves: d.moves.filter((x) => !doomed.has(x.id)) }));
      setCur(m.parent);
    } catch (e) {
      setError(e.message);
    }
  }

  async function saveName() {
    setEditing(false);
    const n = draftName.trim();
    if (!n || n === data.repertoire.name) return;
    try {
      const r = await renameRepertoire(id, n);
      setData((d) => ({ ...d, repertoire: { ...d.repertoire, name: r.name } }));
    } catch (e) {
      setError(e.message);
    }
  }

  // keyboard: left goes back a move, right goes on along the first line
  useEffect(() => {
    function onKey(e) {
      const tag = e.target.tagName;
      if (tag === "TEXTAREA" || tag === "INPUT" || e.altKey || e.ctrlKey || e.metaKey) return;
      if (e.key === "ArrowLeft") setCur(tree.byId.get(cur)?.parent ?? 0);
      else if (e.key === "ArrowRight" && children.length) setCur(children[0].id);
      else if (e.key === "Home") setCur(0);
      else return;
      e.preventDefault();
    }
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [tree, cur, children]);

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
  if (!data) return <div className="room openings openings--loading">{error ? <div className="banner banner--error">{error}</div> : "Загружаю репозиторий…"}</div>;

  const rep = data.repertoire;
  const pct = learnedPercent(counts);
  const todo = counts.due + counts.new;
  const lastMove = tree.byId.get(cur);
  const hovered = uciSquares(hover);
  const mover = sideOf(fen);

  return (
    <div className="room openings">
      <div className="room__grid">
        <div className="room__main">
          <div className="pbar">
            <span className={`repcard__color repcard__color--${side === "white" ? "black" : "white"}`} />
            <div className="pbar__who">
              <span className="pbar__name">{side === color ? "Соперник" : "Вы"}</span>
              <span className="pbar__sub">{side === "white" ? "Чёрные" : "Белые"}</span>
            </div>
          </div>
          <div className="room__board">
            <Board
              key={boardKey}
              state={boardState}
              orientation={side}
              disabled={false}
              lastMove={lastMove ? uciSquares(lastMove.uci) : null}
              arrows={hovered ? [[hovered.from, hovered.to, "rgba(96, 165, 250, 0.9)"]] : []}
              onPlayerMove={play}
            />
          </div>
          <div className="pbar">
            <span className={`repcard__color repcard__color--${side}`} />
            <div className="pbar__who">
              <span className="pbar__name">{side === color ? "Вы" : "Соперник"}</span>
              <span className="pbar__sub">{side === "white" ? "Белые" : "Чёрные"}</span>
            </div>
          </div>
          <div className="openings__nav">
            <button type="button" onClick={() => setCur(0)} disabled={cur === 0} title="В начало (Home)" aria-label="В начало">
              <Icon name="first" />
            </button>
            <button type="button" onClick={() => setCur(lastMove?.parent ?? 0)} disabled={cur === 0} title="Назад (←)" aria-label="Назад">
              <Icon name="prev" />
            </button>
            <button type="button" onClick={() => children[0] && setCur(children[0].id)} disabled={!children.length} title="Вперёд (→)" aria-label="Вперёд">
              <Icon name="next" />
            </button>
            <button type="button" onClick={() => setOrientation(side === "white" ? "black" : "white")} title="Перевернуть доску" aria-label="Перевернуть доску">
              <Icon name="flip" />
            </button>
          </div>
        </div>

        <aside className="room__side">
          {error && <div className="banner banner--error">{error}</div>}

          <section className="card openings__head">
            <a className="openings__back" href="#openings">
              <Icon name="back" size={16} /> Все репозитории
            </a>
            {editing ? (
              <input
                className="field"
                autoFocus
                value={draftName}
                maxLength={80}
                onChange={(e) => setDraftName(e.target.value)}
                onBlur={saveName}
                onKeyDown={(e) => e.key === "Enter" && e.currentTarget.blur()}
                aria-label="Название репозитория"
              />
            ) : (
              <h1
                className="openings__title"
                title="Нажмите, чтобы переименовать"
                onClick={() => {
                  setDraftName(rep.name);
                  setEditing(true);
                }}
              >
                {rep.name}
              </h1>
            )}
            <p className="room__meta">
              {color === "white" ? "За белых" : "За чёрных"} · ходов {counts.moves}, ваших {counts.mine}, выучено {counts.learned} ({pct}%)
            </p>
            <a className={`btn ${todo > 0 ? "btn--primary" : ""}`} href={`#openings/${id}/train`}>
              <Icon name="play" size={16} /> Тренировать{todo > 0 ? ` (${todo})` : ""}
            </a>
            <a className="btn" href={`#openings/${id}/games`} title="Где вы вышли из репозитория в своих партиях">
              <Icon name="review" size={16} /> Мои партии
            </a>
            <div className="room__row">
              <a className="btn" href={exportUrl(id)} download title="Скачать репозиторий файлом PGN: линии станут вариантами">
                <Icon name="copy" size={16} /> Скачать PGN
              </a>
              <button type="button" className="btn" onClick={() => setImporting((v) => !v)} aria-expanded={importing}>
                <Icon name="upload" size={16} /> Импорт PGN
              </button>
            </div>
            {importMsg && (
              <p className="openings__msg openings__msg--ok" role="status">
                {importMsg}
              </p>
            )}
          </section>

          {importing && (
            <PgnImport
              repertoireId={id}
              onCancel={() => setImporting(false)}
              onDone={(res) => {
                setImportMsg(summaryOf(res));
                setImporting(false);
                load();
              }}
            />
          )}

          {added && added.id === cur && (
            <section className="card openings__added" role="status">
              <p>
                <Icon name="check" size={16} /> Добавлено в репозиторий: <b>{added.san}</b>.
                {counts.new > 0 ? ` Новых ходов для заучивания: ${counts.new}.` : ""}
              </p>
              <div className="room__row">
                {counts.new + counts.due > 0 && (
                  <a className="btn btn--primary" href={`#openings/${id}/train`}>
                    <Icon name="play" size={16} /> Тренировать
                  </a>
                )}
                <button type="button" className="btn" onClick={undoAdded}>
                  Отменить добавление
                </button>
                <button type="button" className="btn btn--ghost" onClick={() => setAdded(null)}>
                  Продолжить линию
                </button>
              </div>
            </section>
          )}

          <section className="card openings__pos">
            <p className="room__state">{turn === "mine" ? "Ваш ход" : "Ход соперника"}</p>
            {opening && (
              <p className="room__opening" title={openingTitle(opening)}>
                <span className="review__eco">{opening.eco}</span> {openingName(opening)}
              </p>
            )}
            {path.length > 0 && <p className="openings__line">{lineText(path)}</p>}

            <Suggestions fen={fen} side={mover} turn={turn} children={children} onPick={play} onHover={setHover} />

            <div className="room__row">
              <button type="button" className="btn btn--danger-ghost" onClick={removeCurrent} disabled={cur === 0}>
                <Icon name="trash" size={16} /> Удалить этот ход
              </button>
            </div>
          </section>

          {gaps.length > 0 && (
            <section className="card openings__gaps">
              <h2>Нет вашего ответа ({gaps.length})</h2>
              <p className="room__meta">Линии заканчиваются ходом соперника: добавьте, как вы будете отвечать.</p>
              {gaps.slice(0, 8).map((g) => (
                <button key={g.id} type="button" className="gap" onClick={() => setCur(g.id)}>
                  {lineText(pathTo(tree, g.id))}
                </button>
              ))}
              {gaps.length > 8 && <p className="room__meta">и ещё {gaps.length - 8}</p>}
            </section>
          )}

          <section className="card openings__tree">
            <h2>Все линии</h2>
            {moves.length === 0 ? <p className="room__meta">Репозиторий пуст: сделайте первый ход на доске.</p> : <TreeList tree={tree} parent={0} cur={cur} onSelect={setCur} />}
          </section>
        </aside>
      </div>
    </div>
  );
}

// The tree of lines, indented by branch.
function TreeList({ tree, parent, cur, onSelect, depth = 0 }) {
  const kids = tree.children.get(parent) || [];
  if (!kids.length) return null;
  return (
    <ul className={`reptree${depth ? " reptree--nested" : ""}`}>
      {kids.map((m) => (
        <li key={m.id}>
          <TreeItem move={m} active={m.id === cur} onSelect={onSelect} />
          <TreeList tree={tree} parent={m.id} cur={cur} onSelect={onSelect} depth={depth + 1} />
        </li>
      ))}
    </ul>
  );
}

function TreeItem({ move, active, onSelect }) {
  const ref = useRef(null);
  useEffect(() => {
    if (active) scrollWithin(ref.current); // only the box of the tree, never the page
  }, [active]);
  const no = Math.ceil(move.ply / 2);
  const status = statusOf(move);
  return (
    <button ref={ref} type="button" className={`reptree__move repchip repchip--${status}${active ? " repchip--on" : ""}`} onClick={() => onSelect(move.id)} title={STATUS_TEXT[status]}>
      <span className="reptree__no">{move.ply % 2 === 1 ? `${no}.` : `${no}...`}</span>
      {move.san}
    </button>
  );
}
