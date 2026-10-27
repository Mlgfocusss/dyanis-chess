import { useCallback, useEffect, useMemo, useState } from "react";
import { useAuth } from "../auth/AuthContext.jsx";
import Icon from "../ui/Icon.jsx";
import Segmented from "../ui/Segmented.jsx";
import PgnImport, { summaryOf } from "./PgnImport.jsx";
import { createRepertoire, deleteRepertoire, exportUrl, importRepertoire, listRepertoires } from "./api.js";
import { cardHoles, listTotals } from "./gaps.js";
import { presetById, presetsFor } from "./presets.js";
import { learnedPercent } from "./tree.js";
import { tOpts, useT } from "../i18n/index.js";

const COLORS = [
  ["white", "openings.forWhite"],
  ["black", "openings.forBlack"],
];

// The start page of "Дебюты": the strip with the totals, a card for each repository (how much of the likely
// games it covers, how many holes it has, what is to repeat today, and the next step), and the start of a new one.
export default function RepList() {
  const t = useT();
  const { status, user, guest } = useAuth();
  const [items, setItems] = useState(null);
  const [error, setError] = useState("");
  const [imported, setImported] = useState(null); // what the last import did: { id, text }
  const [busy, setBusy] = useState(false);

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

  // creates a repository (a guest account keeps it) and opens it: with first moves, to fill the holes
  async function create({ name, color, pgn }) {
    setBusy(true);
    setError("");
    try {
      if (!user) await guest();
      if (pgn) {
        const res = await importRepertoire({ name, color, pgn });
        window.location.hash = `openings/${res.repertoire.id}/fill`;
      } else {
        const rep = await createRepertoire({ name, color });
        window.location.hash = `openings/${rep.id}`;
      }
    } catch (err) {
      setError(err.message);
      setBusy(false);
    }
  }

  async function remove(rep) {
    if (!window.confirm(t("openingsPage.3", { name: rep.name }))) return;
    try {
      await deleteRepertoire(rep.id);
      load();
    } catch (err) {
      setError(err.message);
    }
  }

  const totals = useMemo(() => listTotals(items || []), [items]);

  if (status === "unavailable") {
    return (
      <div className="page">
        <div className="banner banner--error">{t("openingsPage.4")}</div>
      </div>
    );
  }

  const empty = items && items.length === 0;
  return (
    <div className="page openings">
      <header className="page__head">
        <h1 className="page__title">{t("menu.openings")}</h1>
        <p className="page__sub">{t("openingsPage.6")}</p>
      </header>

      {error && <div className="banner banner--error">{error}</div>}
      {imported && (
        <div className="banner banner--ok" role="status">
          {imported.text} <a href={`#openings/${imported.id}`}>{t("openingsPage.7")}</a>
        </div>
      )}

      <div className="repdash">
        <div className="repdash__main">
          {items && items.length > 0 && <Totals totals={totals} />}

          {items && items.length > 0 && (
            <div className="replist">
              {items.map((r) => (
                <RepCard key={r.id} rep={r} onDelete={() => remove(r)} />
              ))}
            </div>
          )}

          {empty && <FirstRepository disabled={busy || status !== "ready"} onCreate={create} />}
        </div>

        <aside className="repdash__side">
          <NewRepository disabled={busy || status !== "ready"} guestNote={!user && status === "ready"} onCreate={create} />

          {status === "ready" && (
            <PgnImport
              ensureAccount={user ? undefined : guest}
              onDone={(res) => {
                setImported({ id: res.repertoire.id, text: t("openingsPage.16", { name: res.repertoire.name, p: summaryOf(res) }) });
                load();
              }}
            />
          )}
        </aside>
      </div>
    </div>
  );
}

function Totals({ totals }) {
  const t = useT();
  const tiles = [
    ["reps", totals.reps, t("openings2.list.totalReps"), ""],
    ["moves", totals.moves, t("openings2.list.totalMoves"), ""],
    ["holes", totals.holes, t("openings2.list.totalHoles"), totals.holes > 0 ? "warn" : "ok"],
    ["due", totals.due, t("openings2.list.totalDue"), totals.fresh > 0 ? t("openings2.list.totalNew", { n: totals.fresh }) : ""],
  ];
  return (
    <div className="repsum" role="list">
      {tiles.map(([key, value, label, extra]) => (
        <div key={key} className={`repsum__tile${key === "holes" && totals.holes > 0 ? " repsum__tile--warn" : ""}`} role="listitem">
          <span className="repsum__num">{value}</span>
          <span className="repsum__label">{label}</span>
          {key === "due" && extra && <span className="repsum__extra">{extra}</span>}
        </div>
      ))}
    </div>
  );
}

// Openings to start from: one click makes the repository with the first moves in it.
function PresetChips({ color, onPick, disabled, selected }) {
  const t = useT();
  return (
    <div className="presets" role="group" aria-label={t("openings2.start.openingLabel")}>
      {presetsFor(color).map((p) => (
        <button key={p.id} type="button" className={`preset${selected === p.id ? " preset--on" : ""}`} onClick={() => onPick(p)} disabled={disabled} aria-pressed={selected === p.id}>
          {t(p.name)}
        </button>
      ))}
    </div>
  );
}

// No repository yet: an invitation instead of an empty page.
function FirstRepository({ onCreate, disabled }) {
  const t = useT();
  return (
    <section className="card repstart">
      <div className="repstart__head">
        <span className="repstart__icon" aria-hidden="true">
          <Icon name="openings" size={28} />
        </span>
        <div>
          <h2>{t("openings2.start.title")}</h2>
          <p>{t("openings2.start.sub")}</p>
        </div>
      </div>
      <div className="repstart__quick">
        <button type="button" className="btn btn--primary repstart__btn" disabled={disabled} onClick={() => onCreate({ name: t("openings2.start.nameWhite"), color: "white" })}>
          <span className="repcard__color repcard__color--white" /> {t("openings2.start.white")}
        </button>
        <button type="button" className="btn repstart__btn" disabled={disabled} onClick={() => onCreate({ name: t("openings2.start.nameBlack"), color: "black" })}>
          <span className="repcard__color repcard__color--black" /> {t("openings2.start.black")}
        </button>
      </div>
      {COLORS.map(([color, label]) => (
        <div key={color} className="repstart__row">
          <span className="repstart__rowlabel">{t(label)}</span>
          <PresetChips color={color} disabled={disabled} onPick={(p) => onCreate({ name: t(p.name), color: p.color, pgn: p.pgn })} />
        </div>
      ))}
    </section>
  );
}

// The form for one more repository: a name, a colour and, if wanted, an opening to start from.
function NewRepository({ onCreate, disabled, guestNote }) {
  const t = useT();
  const [name, setName] = useState("");
  const [color, setColor] = useState("white");
  const [presetId, setPresetId] = useState("");
  const preset = presetById(presetId);

  function pickColor(c) {
    setColor(c);
    if (preset && preset.color !== c) {
      if (name === t(preset.name)) setName("");
      setPresetId("");
    }
  }
  function pickPreset(p) {
    if (p.id === presetId) {
      if (name === t(p.name)) setName("");
      setPresetId("");
      return;
    }
    if (!name.trim() || (preset && name === t(preset.name))) setName(t(p.name));
    setPresetId(p.id);
  }
  function submit(e) {
    e.preventDefault();
    if (!name.trim()) return;
    onCreate({ name, color, pgn: preset?.pgn });
  }

  return (
    <form className="card repnew" onSubmit={submit}>
      <h2>{t("openingsPage.9")}</h2>
      <input className="field" value={name} onChange={(e) => setName(e.target.value)} placeholder={t("openingsPage.10")} maxLength={80} aria-label={t("openingsPage.11")} />
      <Segmented label={t("online.colour")} value={color} onChange={pickColor} options={tOpts(t, COLORS)} />
      <div className="repnew__presets">
        <span className="repnew__label">{t("openings2.start.openingLabel")}</span>
        <PresetChips color={color} selected={presetId} onPick={pickPreset} disabled={disabled} />
      </div>
      <button type="submit" className="btn btn--primary" disabled={disabled || !name.trim()}>
        {disabled && name.trim() ? t("lobby.24") : preset ? t("openings2.start.createWith") : t("openingsPage.14")}
      </button>
      {guestNote && <p className="openings__note">{t("openingsPage.15")}</p>}
    </form>
  );
}

function Meter({ label, value, tone, hint, muted }) {
  return (
    <div className={`repmeter${muted ? " repmeter--muted" : ""}`}>
      <div className="repmeter__row">
        <span>{label}</span>
        <b>{value == null ? hint : `${value}%`}</b>
      </div>
      <div className="repbar" role="progressbar" aria-label={label} aria-valuenow={value ?? 0} aria-valuemin={0} aria-valuemax={100}>
        <div className={`repbar__fill repbar__fill--${tone}`} style={{ width: `${value ?? 0}%` }} />
      </div>
    </div>
  );
}

function RepCard({ rep, onDelete }) {
  const t = useT();
  const c = rep.counts;
  const g = cardHoles(rep.gaps);
  const learned = learnedPercent(c);
  const todo = c.due + c.new;
  const empty = c.moves === 0;
  const white = rep.color === "white";
  let hint = "";
  if (empty) hint = t("openings2.card.hintEmpty");
  else if (!g.checked) hint = g.holes > 0 ? t("openings2.card.hintOpen", { n: g.holes }) : t("openings2.card.hintUnchecked");
  else if (g.stale) hint = t("openings2.card.hintStale");
  else if (g.source === "theory") hint = t("openings2.card.hintTheory");
  else if (g.partial) hint = t("openings2.card.hintPartial");
  else if (g.holes === 0) hint = t("openings2.card.hintDone");

  return (
    <section className="card repcard">
      <div className="repcard__head">
        <span className={`repcard__color repcard__color--${rep.color}`} title={white ? t("openings.forWhite") : t("openings.forBlack")} />
        <div className="repcard__title">
          <h2 title={rep.name}>{rep.name}</h2>
          <span className="repcard__side">{white ? t("openings.forWhite") : t("openings.forBlack")}</span>
        </div>
        <button type="button" className="repcard__del" onClick={onDelete} title={t("openingsPage.17")} aria-label={t("openingsPage.17")}>
          <Icon name="trash" size={16} />
        </button>
      </div>

      <dl className="repfacts">
        <div>
          <dt>{t("openings2.card.moves")}</dt>
          <dd>{c.moves}</dd>
        </div>
        <div className={g.holes > 0 ? "repfacts__warn" : ""}>
          <dt>{t("openings2.card.holes")}</dt>
          <dd>{empty ? "—" : g.checked || g.holes > 0 ? `${g.holes}${g.stale || g.partial ? "+" : ""}` : "—"}</dd>
        </div>
        <div>
          <dt>{t("openings2.card.today")}</dt>
          <dd>{c.mine === 0 ? "—" : todo}</dd>
        </div>
      </dl>

      {!empty && (
        <div className="repmeters">
          <Meter label={t("openings2.card.coverage")} value={g.coverage} tone="cover" hint={t("openings2.card.notChecked")} muted={g.stale || g.coverage == null} />
          <Meter label={t("openings2.card.learned")} value={c.mine ? learned : null} tone="learn" hint="—" />
        </div>
      )}
      {hint && <p className="repcard__hint">{hint}</p>}
      {!empty && c.mine > 0 && (
        <p className="repcard__todo">{todo === 0 ? t("openingsPage.22") : t("openingsPage.23", { due: c.due, new: c.new })}</p>
      )}

      <div className="repcard__actions">
        {empty ? (
          <a className="btn btn--primary" href={`#openings/${rep.id}`}>
            <Icon name="list" size={16} /> {t("openings2.card.addFirst")}
          </a>
        ) : (
          <a className={`btn ${g.holes > 0 || !g.checked ? "btn--primary" : ""}`} href={`#openings/${rep.id}/fill`} title={t("openings2.card.fillTitle")}>
            <Icon name="branch" size={16} /> {g.checked || g.holes > 0 ? t("openings2.card.fill") : t("openings2.card.check")}
          </a>
        )}
        <a className={`btn ${todo > 0 && g.holes === 0 ? "btn--primary" : ""}`} href={`#openings/${rep.id}/train`} aria-disabled={c.mine === 0}>
          <Icon name="play" size={16} /> {t("openingsPage.24")}
          {todo > 0 ? ` (${todo})` : ""}
        </a>
      </div>
      <div className="repcard__links">
        <a href={`#openings/${rep.id}`}>
          <Icon name="list" size={14} /> {t("openingsPage.25")}
        </a>
        <a href={`#openings/${rep.id}/games`} aria-disabled={c.mine === 0} title={t("openingsPage.26")}>
          <Icon name="review" size={14} /> {t("gamesPage.myGames")}
        </a>
        <a href={exportUrl(rep.id)} download title={t("openingsPage.28")}>
          <Icon name="copy" size={14} /> PGN
        </a>
      </div>
    </section>
  );
}
