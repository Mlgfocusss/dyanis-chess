import { useRef, useState } from "react";
import Icon from "../ui/Icon.jsx";
import Segmented from "../ui/Segmented.jsx";
import { importInto, importRepertoire } from "./api.js";
import { importSummary, nameFromFile } from "./pgnImport.js";
import { tOpts, useT, tr } from "../i18n/index.js";

const COLORS = [
  ["", "openings.asInFile"],
  ["white", "openings.forWhite"],
  ["black", "openings.forBlack"],
];

const MAX_BYTES = 4 << 20; // the server's limit

// Imports a PGN file (or pasted text) as a new repository, or, with repertoireId, into that one.
// Variations of the file become the branches of the tree. onDone gets what the server answered.
export default function PgnImport({ repertoireId, ensureAccount, onDone, onCancel }) {
  const t = useT();
  const [text, setText] = useState("");
  const [fileName, setFileName] = useState("");
  const [name, setName] = useState("");
  const [color, setColor] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const input = useRef(null);

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
    if (!repertoireId && !name) setName(nameFromFile(file.name));
  }

  async function submit(e) {
    e.preventDefault();
    if (!text.trim()) return;
    setBusy(true);
    setError("");
    try {
      if (ensureAccount) await ensureAccount();
      const res = repertoireId ? await importInto(repertoireId, text) : await importRepertoire({ name, color, pgn: text });
      onDone(res);
      setText("");
      setFileName("");
    } catch (err) {
      setError(err.message);
    } finally {
      setBusy(false);
    }
  }

  return (
    <form className="card pgnimport" onSubmit={submit}>
      <h2>{repertoireId ? t("pgnImportUi.5") : t("pgnImportUi.6")}</h2>
      <p className="openings__note">
        {t("pgnImportUi.7")}
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
        placeholder={t("pgnImportUi.10")}
        spellCheck={false}
        rows={6}
        aria-label={t("pgnImportUi.11")}
      />
      {!repertoireId && (
        <>
          <input
            className="field"
            value={name}
            onChange={(e) => setName(e.target.value)}
            placeholder={t("pgnImportUi.12")}
            maxLength={80}
            aria-label={t("openingsPage.11")}
          />
          <Segmented label={t("online.colour")} value={color} onChange={setColor} options={tOpts(t, COLORS)} />
        </>
      )}
      {error && <div className="banner banner--error">{error}</div>}
      <div className="pgnimport__actions">
        <button type="submit" className="btn btn--primary" disabled={busy || !text.trim()}>
          {busy ? t("pgnImportUi.15") : t("pgnImportUi.16")}
        </button>
        {onCancel && (
          <button type="button" className="btn btn--ghost" onClick={onCancel} disabled={busy}>
            {t("common.cancel")}
          </button>
        )}
      </div>
    </form>
  );
}

// What the import did, as a line of text for a banner.
export function summaryOf(res) {
  return importSummary(res.import);
}
