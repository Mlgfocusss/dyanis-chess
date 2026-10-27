import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import Board from "../components/Board.jsx";
import { useMoveSound } from "../settings/SettingsContext.jsx";
import { fetchLine } from "../review/api.js";
import { uciSquares } from "../review/classes.js";
import { sideOf } from "../review/branch.js";
import Icon from "../ui/Icon.jsx";
import { scrollWithin } from "../ui/scroll.js";
import { addMove, deleteMove, exportUrl, fetchGaps, fetchSavedGaps, getRepertoire, renameRepertoire } from "./api.js";
import GapsPanel from "./GapsPanel.jsx";
import PgnImport, { summaryOf } from "./PgnImport.jsx";
import Suggestions from "./Suggestions.jsx";
import { keyOf, markedNodes, nextGap, treeSignature, visibleGaps } from "./gaps.js";
import { loadPreset, ratingsOf, storePreset } from "./suggest.js";
import { openingName, openingTitle } from "./names.js";
import { buildTree, childWith, countsOf, fenAt, learnedPercent, lineText, pathTo, statusOf, subtreeIds, whoMoves } from "./tree.js";
import { useT } from "../i18n/index.js";

const STATUS_TEXT = { opponent: "openings.status.opponent", new: "openings.status.new", due: "openings.status.due", learning: "openings.status.learning", learned: "openings.status.learned" };

// The colour of an arrow from a token of the stylesheet (the board draws arrows with a colour string).
function tokenArrow(rgbToken, alpha) {
  try {
    const v = getComputedStyle(document.documentElement).getPropertyValue(rgbToken).trim();
    if (v) return `rgba(${v}, ${alpha})`;
  } catch {
    // no stylesheet: the default below
  }
  return "orange";
}

// The builder: the board and the tree of lines. A move made on the board is
// added to the repertoire (or, if it is already there, the board just follows it).
// fillStart: the page was opened to fill the repository hole after hole ("Continue filling" of the list).
export default function Builder({ id, start, fillStart = false }) {
  const t = useT();
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
  // holes: the popular replies of the opponent that the repository has no answer to
  const [preset, setPreset] = useState(loadPreset);
  const ratings = ratingsOf(preset);
  const [scan, setScan] = useState(null); // the last answer of the server's search
  const [scanError, setScanError] = useState("");
  const [refreshing, setRefreshing] = useState(false);
  const [fill, setFill] = useState(fillStart); // lead from one hole to the next as answers are added
  const [fillStatus, setFillStatus] = useState(""); // "", "searching", "done"
  const [focus, setFocus] = useState(null); // key of the hole being worked on
  const scanning = useRef(false);
  const rerun = useRef(null);
  const jumpArmed = useRef(null); // the position after the answer just added: the jump is wanted only if the person is still there
  const lastScanSig = useRef(null);
  const initDone = useRef(false);
  const live = useRef({}); // the latest tree and position, for the callbacks of searches that finish later
  const runRef = useRef(() => {});

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
  const items = useMemo(() => visibleGaps(scan, tree), [scan, tree]);
  const marked = useMemo(() => markedNodes(items), [items]);
  const color = data?.repertoire.color || "white";
  const side = orientation || color;
  const fen = fenAt(tree, cur);
  const path = useMemo(() => pathTo(tree, cur), [tree, cur]);
  const children = tree.children.get(cur) || [];
  const turn = whoMoves(color, cur, tree);
  const sig = treeSignature(moves || []);
  const gapsHere = useMemo(() => new Map(items.filter((g) => g.kind === "missing" && g.at === cur).map((g) => [g.uci, g])), [items, cur]);
  live.current = { tree, cur, fill, items, focus, sig };

  function goToGap(g) {
    setCur(g.at);
    setFocus(keyOf(g));
    setAdded(null);
  }
  function nextHole() {
    const g = nextGap(live.current.items, live.current.focus);
    if (g) goToGap(g);
  }

  // The search for holes. One runs at a time; a request made meanwhile runs after it. With
  // jump, the page goes to the heaviest hole when the answer comes (if the person is still where
  // they were when asking).
  const runScan = useCallback(
    (opts = {}) => {
      if (scanning.current) {
        rerun.current = { jump: !!(rerun.current?.jump || opts.jump) };
        return;
      }
      scanning.current = true;
      lastScanSig.current = live.current.sig;
      setRefreshing(true);
      if (opts.jump) setFillStatus("searching");
      fetchGaps(id, { ratings })
        .then((res) => {
          setScan(res);
          setScanError("");
          if (!opts.jump) return;
          const lv = live.current;
          const armed = jumpArmed.current;
          jumpArmed.current = null;
          if (armed !== null && lv.cur !== armed) {
            setFillStatus("");
            return;
          }
          const list = visibleGaps(res, lv.tree);
          if (!list.length) {
            setFillStatus("done");
            return;
          }
          setFillStatus("");
          setCur(list[0].at);
          setFocus(keyOf(list[0]));
          setAdded(null);
        })
        .catch((e) => {
          if (e.status !== 409) setScanError(e.message);
          setFillStatus("");
        })
        .finally(() => {
          scanning.current = false;
          setRefreshing(false);
          const next = rerun.current;
          rerun.current = null;
          if (next) setTimeout(() => runRef.current(next), 0);
        });
    },
    [id, ratings],
  );
  runRef.current = runScan;

  // first look: the saved search at once, then a fresh one
  useEffect(() => {
    if (!data || initDone.current) return;
    initDone.current = true;
    lastScanSig.current = live.current.sig; // the search below is for this tree
    let jumped = false;
    fetchSavedGaps(id)
      .then((saved) => {
        if (!saved) return;
        setScan(saved);
        if (fillStart) {
          const list = visibleGaps(saved, live.current.tree);
          if (list.length) {
            jumped = true;
            setCur(list[0].at);
            setFocus(keyOf(list[0]));
          }
        }
      })
      .catch(() => {})
      .finally(() => {
        if (fillStart && window.location.hash.endsWith("/fill")) window.history.replaceState(null, "", `#openings/${id}`);
        runRef.current({ jump: fillStart && !jumped });
      });
  }, [data, id, fillStart]);

  // the tree changed (a move added or a line deleted): look again a moment later
  useEffect(() => {
    if (!data || !initDone.current) return undefined;
    if (lastScanSig.current === sig) return undefined;
    const timer = setTimeout(() => {
      if (lastScanSig.current !== live.current.sig) runRef.current({});
    }, 1200);
    return () => clearTimeout(timer);
  }, [sig, data]);

  // another rating range: the weights change
  const lastRatings = useRef(ratings);
  useEffect(() => {
    if (lastRatings.current === ratings) return;
    lastRatings.current = ratings;
    runRef.current({});
  }, [ratings]);

  function choosePreset(p) {
    setPreset(p);
    storePreset(p);
  }
  function toggleFill(on) {
    setFill(on);
    setFillStatus("");
    if (on && items.length && !items.some((g) => keyOf(g) === focus)) goToGap(items[0]);
  }

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
        if (r.created) {
          setAdded({ id: r.move.id, san: r.move.san, mine: r.move.mine });
          // filling hole after hole: the person's answer is in, the next hole is wanted
          if (r.move.mine && live.current.fill) {
            jumpArmed.current = r.move.id;
            runRef.current({ jump: true });
          }
        }
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
    const text = n > 1 ? t("builder.6", { san: m.san, p: n - 1 }) : t("builder.7", { san: m.san });
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
      else if (e.key === "g" || e.key === "G") nextHole();
      else return;
      e.preventDefault();
    }
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [tree, cur, children, items, focus]);

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
  if (!data) return <div className="room openings openings--loading">{error ? <div className="banner banner--error">{error}</div> : t("builder.10")}</div>;

  const rep = data.repertoire;
  const pct = learnedPercent(counts);
  const todo = counts.due + counts.new;
  const lastMove = tree.byId.get(cur);
  const hovered = uciSquares(hover);
  const focusedGap = items.find((g) => keyOf(g) === focus) || null;
  const gapMove = focusedGap && focusedGap.kind === "missing" && focusedGap.at === cur ? uciSquares(focusedGap.uci) : null;
  const arrows = [];
  if (hovered) arrows.push([hovered.from, hovered.to, "rgba(96, 165, 250, 0.9)"]);
  else if (gapMove) arrows.push([gapMove.from, gapMove.to, tokenArrow("--warn-rgb", 0.9)]);
  const mover = sideOf(fen);

  return (
    <div className="room openings">
      <div className="room__grid">
        <div className="room__main">
          <div className="pbar">
            <span className={`repcard__color repcard__color--${side === "white" ? "black" : "white"}`} />
            <div className="pbar__who">
              <span className="pbar__name">{side === color ? t("openings.opponent") : t("botsPage.8")}</span>
              <span className="pbar__sub">{side === "white" ? t("online.black") : t("online.white")}</span>
            </div>
          </div>
          <div className="room__board">
            <Board
              key={boardKey}
              state={boardState}
              orientation={side}
              disabled={false}
              lastMove={lastMove ? uciSquares(lastMove.uci) : null}
              arrows={arrows}
              onPlayerMove={play}
            />
          </div>
          <div className="pbar">
            <span className={`repcard__color repcard__color--${side}`} />
            <div className="pbar__who">
              <span className="pbar__name">{side === color ? t("botsPage.8") : t("openings.opponent")}</span>
              <span className="pbar__sub">{side === "white" ? t("online.white") : t("online.black")}</span>
            </div>
          </div>
          <div className="openings__nav">
            <button type="button" onClick={() => setCur(0)} disabled={cur === 0} title={t("panelNav.2")} aria-label={t("panelNav.3")}>
              <Icon name="first" />
            </button>
            <button type="button" onClick={() => setCur(lastMove?.parent ?? 0)} disabled={cur === 0} title={t("panelNav.4")} aria-label={t("panelNav.5")}>
              <Icon name="prev" />
            </button>
            <button type="button" onClick={() => children[0] && setCur(children[0].id)} disabled={!children.length} title={t("panelNav.10")} aria-label={t("panelNav.11")}>
              <Icon name="next" />
            </button>
            <button type="button" onClick={() => setOrientation(side === "white" ? "black" : "white")} title={t("panelNav.22")} aria-label={t("panelNav.22")}>
              <Icon name="flip" />
            </button>
          </div>
        </div>

        <aside className="room__side">
          {error && <div className="banner banner--error">{error}</div>}

          <section className="card openings__head">
            <a className="openings__back" href="#openings">
              <Icon name="back" size={16} /> {t("builder.22")}
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
                aria-label={t("openingsPage.11")}
              />
            ) : (
              <h1
                className="openings__title"
                title={t("builder.24")}
                onClick={() => {
                  setDraftName(rep.name);
                  setEditing(true);
                }}
              >
                {rep.name}
              </h1>
            )}
            <p className="room__meta">
              {color === "white" ? t("openings.forWhite") : t("openings.forBlack")} {t("builder.27")} {counts.moves}{t("builder.28")} {counts.mine}{t("builder.29")} {counts.learned} ({pct}%)
            </p>
            <a className={`btn ${todo > 0 ? "btn--primary" : ""}`} href={`#openings/${id}/train`}>
              <Icon name="play" size={16} /> {t("openingsPage.24")}{todo > 0 ? ` (${todo})` : ""}
            </a>
            <a className="btn" href={`#openings/${id}/games`} title={t("openingsPage.26")}>
              <Icon name="review" size={16} /> {t("gamesPage.myGames")}
            </a>
            <div className="room__row">
              <a className="btn" href={exportUrl(id)} download title={t("openingsPage.28")}>
                <Icon name="copy" size={16} /> {t("builder.34")}
              </a>
              <button type="button" className="btn" onClick={() => setImporting((v) => !v)} aria-expanded={importing}>
                <Icon name="upload" size={16} /> {t("builder.35")}
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
                <Icon name="check" size={16} /> {t("builder.36")} <b>{added.san}</b>.
                {counts.new > 0 ? t("builder.37", { new: counts.new }) : ""}
              </p>
              <div className="room__row">
                {counts.new + counts.due > 0 && (
                  <a className="btn btn--primary" href={`#openings/${id}/train`}>
                    <Icon name="play" size={16} /> {t("openingsPage.24")}
                  </a>
                )}
                <button type="button" className="btn" onClick={undoAdded}>
                  {t("builder.38")}
                </button>
                <button type="button" className="btn btn--ghost" onClick={() => setAdded(null)}>
                  {t("builder.39")}
                </button>
              </div>
            </section>
          )}

          <GapsPanel
            tree={tree}
            cur={cur}
            items={items}
            scan={scan}
            state={scan ? "ready" : scanError ? "error" : "loading"}
            refreshing={refreshing}
            error={scanError}
            focusKey={focus}
            fill={fill}
            fillStatus={fillStatus}
            hasMoves={moves.length > 0}
            onFill={toggleFill}
            onGo={goToGap}
            onNext={nextHole}
            onAdd={(g) => play(g.uci)}
            onRecheck={() => runScan({})}
          />

          <section className="card openings__pos">
            <p className="room__state">{turn === "mine" ? t("gameRoom.41") : t("gameRoom.42")}</p>
            {opening && (
              <p className="room__opening" title={openingTitle(opening)}>
                <span className="review__eco">{opening.eco}</span> {openingName(opening)}
              </p>
            )}
            {path.length > 0 && <p className="openings__line">{lineText(path)}</p>}

            <Suggestions fen={fen} side={mover} turn={turn} children={children} onPick={play} onHover={setHover} gaps={gapsHere} preset={preset} onPreset={choosePreset} />

            <div className="room__row">
              <button type="button" className="btn btn--danger-ghost" onClick={removeCurrent} disabled={cur === 0}>
                <Icon name="trash" size={16} /> {t("builder.42")}
              </button>
            </div>
          </section>

          <section className="card openings__tree">
            <h2>{t("builder.46")}</h2>
            {moves.length === 0 ? <p className="room__meta">{t("builder.47")}</p> : <TreeList tree={tree} parent={0} cur={cur} onSelect={setCur} marked={marked} />}
          </section>
        </aside>
      </div>
    </div>
  );
}

// The tree of lines, indented by branch.
function TreeList({ tree, parent, cur, onSelect, marked, depth = 0 }) {
  const kids = tree.children.get(parent) || [];
  if (!kids.length) return null;
  return (
    <ul className={`reptree${depth ? " reptree--nested" : ""}`}>
      {kids.map((m) => (
        <li key={m.id}>
          <TreeItem move={m} active={m.id === cur} hole={marked.has(m.id)} onSelect={onSelect} />
          <TreeList tree={tree} parent={m.id} cur={cur} onSelect={onSelect} marked={marked} depth={depth + 1} />
        </li>
      ))}
    </ul>
  );
}

function TreeItem({ move, active, hole, onSelect }) {
  const t = useT();
  const ref = useRef(null);
  useEffect(() => {
    if (active) scrollWithin(ref.current); // only the box of the tree, never the page
  }, [active]);
  const no = Math.ceil(move.ply / 2);
  const status = statusOf(move);
  return (
    <button ref={ref} type="button" className={`reptree__move repchip repchip--${status}${active ? " repchip--on" : ""}`} onClick={() => onSelect(move.id)} title={hole ? `${t(STATUS_TEXT[status])} · ${t("openings2.gaps.treeMark")}` : t(STATUS_TEXT[status])}>
      <span className="reptree__no">{move.ply % 2 === 1 ? `${no}.` : `${no}...`}</span>
      {move.san}
      {hole && <span className="reptree__hole" aria-label={t("openings2.gaps.treeMark")} />}
    </button>
  );
}
