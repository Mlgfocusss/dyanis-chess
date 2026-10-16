import { useRef, useState } from "react";
import Icon from "../ui/Icon.jsx";
import Segmented from "../ui/Segmented.jsx";
import { importInto, importRepertoire } from "./api.js";
import { importSummary, nameFromFile } from "./pgnImport.js";

const COLORS = [
  ["", "Как в файле"],
  ["white", "За белых"],
  ["black", "За чёрных"],
];

const MAX_BYTES = 4 << 20; // the server's limit

// Imports a PGN file (or pasted text) as a new repository, or, with repertoireId, into that one.
// Variations of the file become the branches of the tree. onDone gets what the server answered.
export default function PgnImport({ repertoireId, ensureAccount, onDone, onCancel }) {
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
      setError("Файл больше 4 МБ.");
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
      <h2>{repertoireId ? "Импорт PGN в этот репозиторий" : "Импорт из PGN"}</h2>
      <p className="openings__note">
        Файл PGN с вариантами: основная линия и ответвления станут деревом ходов. Подойдёт экспорт из Lichess (изучения), ChessBase, Chessable или из этого сайта. Партии с
        собственной начальной позицией пропускаются.
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
        placeholder={"[Event \"Итальянская партия\"]\n[Orientation \"white\"]\n\n1. e4 e5 2. Nf3 Nc6 3. Bc4 (3. Bb5 a6) 3... Bc5 *"}
        spellCheck={false}
        rows={6}
        aria-label="Текст PGN"
      />
      {!repertoireId && (
        <>
          <input
            className="field"
            value={name}
            onChange={(e) => setName(e.target.value)}
            placeholder="Название (по умолчанию из файла)"
            maxLength={80}
            aria-label="Название репозитория"
          />
          <Segmented label="Цвет" value={color} onChange={setColor} options={COLORS} />
        </>
      )}
      {error && <div className="banner banner--error">{error}</div>}
      <div className="pgnimport__actions">
        <button type="submit" className="btn btn--primary" disabled={busy || !text.trim()}>
          {busy ? "Загружаю…" : "Импортировать"}
        </button>
        {onCancel && (
          <button type="button" className="btn btn--ghost" onClick={onCancel} disabled={busy}>
            Отмена
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
