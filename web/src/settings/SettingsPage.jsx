import { useMemo, useState } from "react";
import { ChessboardDnDProvider, SparePiece } from "react-chessboard";
import Icon from "../ui/Icon.jsx";
import Segmented from "../ui/Segmented.jsx";
import ThemedBoard from "../components/ThemedBoard.jsx";
import { defaultsOf, useSettings } from "./SettingsContext.jsx";
import { playSound } from "../sound/sound.js";
import {
  backgroundCss,
  backgrounds,
  boardThemes,
  incompleteBoards,
  incompletePieceSets,
  pieceSets,
  presets,
  soundThemes,
} from "../themes/index.js";
import GameplayPanel from "./GameplayPanel.jsx";
import { Row, SaveBar, Switch } from "./controls.jsx";
import "./settings.css";

// A position with every kind of piece, for the preview board.
const PREVIEW_FEN = "r1bqk2r/pppp1ppp/2n2n2/2b1p3/2B1P3/2N2N2/PPPP1PPP/R1BQK2R w KQkq - 6 5";

const TABS = [
  { id: "boards", label: "Доски" },
  { id: "pieces", label: "Фигуры" },
  { id: "background", label: "Фон" },
  { id: "presets", label: "Готовые настройки" },
];

const SECTIONS = [
  { id: "board", label: "Доска и фигуры", icon: "board" },
  { id: "play", label: "Игровой процесс", icon: "bot" },
  { id: "interface", label: "Интерфейс", icon: "review", soon: true },
  { id: "profile", label: "Профиль", icon: "online", soon: true },
  { id: "account", label: "Учётная запись", icon: "coach", soon: true },
];

export default function SettingsPage() {
  const { settings, save } = useSettings();
  const [draft, setDraft] = useState(settings);
  const [tab, setTab] = useState("boards");
  // "#settings/play" opens the gameplay page
  const [section, setSection] = useState(() => (window.location.hash.split("/")[1] === "play" ? "play" : "board"));
  function openSection(id) {
    setSection(id);
    window.history.replaceState(null, "", id === "play" ? "#settings/play" : "#settings");
  }
  const [justSaved, setJustSaved] = useState(false);

  const dirty = useMemo(() => JSON.stringify(draft) !== JSON.stringify(settings), [draft, settings]);
  const set = (patch) => {
    setJustSaved(false);
    setDraft((d) => ({ ...d, ...patch }));
  };

  function onSave() {
    save(draft);
    setJustSaved(true);
  }
  function onCancel() {
    setDraft(settings);
    setJustSaved(false);
  }

  return (
    <div className="page">
      <header className="page__head">
        <h1 className="page__title">Настройки</h1>
      </header>

      <div className="settings">
        <nav className="settings__menu" aria-label="Разделы настроек">
          {SECTIONS.map((s) => (
            <button
              key={s.id}
              type="button"
              disabled={s.soon}
              className={`settings__menuitem${s.id === section ? " settings__menuitem--active" : ""}`}
              aria-current={s.id === section ? "page" : undefined}
              onClick={() => openSection(s.id)}
            >
              <Icon name={s.icon} />
              <span>{s.label}</span>
              {s.soon && <span className="nav__soon">скоро</span>}
            </button>
          ))}
        </nav>

        {section === "play" ? (
          <section className="settings__body card">
            <GameplayPanel draft={draft} set={set} dirty={dirty} justSaved={justSaved} onSave={onSave} onCancel={onCancel} />
            <button type="button" className="settings__reset" onClick={() => set(defaultsOf(true))}>
              Вернуть стандартные настройки игрового процесса
            </button>
          </section>
        ) : (
        <ChessboardDnDProvider>
          <section className="settings__body card">
            <h2 className="settings__h">Доска и фигуры</h2>
            <p className="settings__sub">Настроить внешний вид своих шахмат.</p>

            <div className="tabs" role="tablist">
              {TABS.map((t) => (
                <button
                  key={t.id}
                  type="button"
                  role="tab"
                  aria-selected={tab === t.id}
                  className={`tabs__tab${tab === t.id ? " tabs__tab--active" : ""}`}
                  onClick={() => setTab(t.id)}
                >
                  {t.label}
                </button>
              ))}
            </div>

            <div className="picker">
              <div className="picker__grid">
                {tab === "boards" && <BoardsTab draft={draft} set={set} />}
                {tab === "pieces" && <PiecesTab draft={draft} set={set} />}
                {tab === "background" && <BackgroundTab draft={draft} set={set} />}
                {tab === "presets" && <PresetsTab draft={draft} set={set} />}
              </div>
              <div className="picker__preview">
                <ThemedBoard id="PreviewBoard" settings={draft} position={PREVIEW_FEN} arePiecesDraggable={false} />
              </div>
            </div>

            <SaveBar dirty={dirty} justSaved={justSaved} onSave={onSave} onCancel={onCancel} />

            <hr className="settings__rule" />

            <Row
              title="Показывать координаты доски"
              hint="Буквы a–h и цифры 1–8: на крайних полях доски (внутри) или рядом с доской (снаружи)."
              control={
                <Switch
                  checked={draft.coordinates !== "off"}
                  onChange={(on) => set({ coordinates: on ? "inside" : "off" })}
                  label="Показывать координаты"
                />
              }
            >
              {draft.coordinates !== "off" && (
                <Segmented
                  value={draft.coordinates}
                  onChange={(v) => set({ coordinates: v })}
                  options={[
                    ["inside", "Внутри"],
                    ["outside", "Снаружи"],
                  ]}
                />
              )}
            </Row>

            <Row
              title="Подсветка последнего хода"
              hint="Подсвечивает поля, с которого и на которое сходила фигура."
              control={
                <Switch
                  checked={draft.highlightLastMove}
                  onChange={(v) => set({ highlightLastMove: v })}
                  label="Подсветка последнего хода"
                />
              }
            />

            <Row title="Передвижение фигур" hint="Как ходить на доске в игре с ботом.">
              <Segmented
                value={draft.moveMethod}
                onChange={(v) => set({ moveMethod: v })}
                options={[
                  ["both", "Перетаскивать или нажимать"],
                  ["drag", "Только перетаскивать"],
                  ["click", "Только нажимать"],
                ]}
              />
            </Row>

            <Row title="Расположение доски в игре с ботом" hint="Сбоку: доска слева, панель справа. По центру: доска посередине, а партия и управление по бокам.">
              <Segmented
                value={draft.playLayout}
                onChange={(v) => set({ playLayout: v })}
                options={[
                  ["side", "Сбоку"],
                  ["center", "По центру"],
                ]}
              />
            </Row>

            <Row title="Анимации фигур" hint="Скорость, с которой фигуры переходят на новое поле.">
              <Segmented
                value={draft.animation}
                onChange={(v) => set({ animation: v })}
                options={[
                  ["none", "Без анимации"],
                  ["slow", "Медленно"],
                  ["medium", "Средне"],
                  ["fast", "Быстро"],
                ]}
              />
            </Row>

            <Row
              title="Звуки"
              hint="Звук хода, взятия, шаха, рокировки и конца партии."
              control={<Switch checked={draft.sound} onChange={(v) => set({ sound: v })} label="Включить звук" />}
            >
              {draft.sound && (
                <div className="sound-row">
                  <label className="sound-row__label">
                    Звуковая тема
                    <select
                      className="field"
                      value={draft.soundTheme}
                      onChange={(e) => set({ soundTheme: e.target.value })}
                    >
                      {soundThemes.map((t) => (
                        <option key={t.id} value={t.id}>
                          {t.name}
                        </option>
                      ))}
                    </select>
                  </label>
                  <button
                    type="button"
                    className="btn"
                    onClick={() => playSound("capture", draft.soundTheme)}
                  >
                    Проверить звук
                  </button>
                </div>
              )}
            </Row>

            <SaveBar dirty={dirty} justSaved={justSaved} onSave={onSave} onCancel={onCancel} />
            <button type="button" className="settings__reset" onClick={() => set(defaultsOf(false))}>
              Вернуть стандартные настройки доски
            </button>
          </section>
        </ChessboardDnDProvider>
        )}
      </div>
    </div>
  );
}

// ------------------------------------------------------------------ tabs

function BoardsTab({ draft, set }) {
  return (
    <>
      <div className="thumbs">
        {boardThemes.map((b) => (
          <Thumb
            key={b.id}
            name={b.name}
            selected={draft.boardTheme === b.id}
            onSelect={() => set({ boardTheme: b.id })}
          >
            <BoardSwatch theme={b} />
          </Thumb>
        ))}
      </div>
      <Warnings
        items={incompleteBoards}
        text={(i) => `Доска «${i.name}» не показана: в папке нет файлов ${i.missing.map((m) => m + ".*").join(", ")}.`}
      />
      <Hint>
        Свои доски: положите картинки в <code>web/src/themes/boards/</code> (см. README в папке <code>themes</code>) и
        пересоберите сайт.
      </Hint>
    </>
  );
}

function PiecesTab({ draft, set }) {
  return (
    <>
      <div className="thumbs">
        {pieceSets.map((p) => (
          <Thumb
            key={p.id}
            name={p.name}
            selected={draft.pieceSet === p.id}
            onSelect={() => set({ pieceSet: p.id })}
          >
            <PieceStrip set={p} />
          </Thumb>
        ))}
      </div>
      <Warnings
        items={incompletePieceSets}
        text={(i) => `Набор «${i.name}» не показан: не хватает фигур ${i.missing.join(", ")}.`}
      />
      <Hint>
        Свои фигуры: папка с 12 файлами <code>wP wN wB wR wQ wK bP bN bB bR bQ bK</code> в{" "}
        <code>web/src/themes/pieces/&lt;название&gt;/</code>, затем пересборка.
      </Hint>
    </>
  );
}

function BackgroundTab({ draft, set }) {
  return (
    <>
      <div className="thumbs">
        {backgrounds.map((b) => (
          <Thumb
            key={b.id}
            name={b.name}
            selected={draft.background === b.id}
            onSelect={() => set({ background: b.id })}
          >
            <div
              className="swatch"
              style={{
                backgroundImage: b.id === "none" ? "none" : backgroundCss(b),
                backgroundColor: b.id === "none" ? "var(--bg)" : undefined,
              }}
            />
          </Thumb>
        ))}
      </div>
      <Hint>
        Свои фоны: картинки в <code>web/src/themes/backgrounds/</code>. Поверх фото накладывается тёмная
        вуаль, чтобы текст читался.
      </Hint>
    </>
  );
}

function PresetsTab({ draft, set }) {
  return (
    <div className="thumbs">
      {presets.map((p) => {
        const board = boardThemes.find((b) => b.id === p.boardTheme);
        const bg = backgrounds.find((b) => b.id === p.background);
        const selected = draft.boardTheme === p.boardTheme && draft.background === p.background;
        return (
          <Thumb
            key={p.id}
            name={p.name}
            selected={selected}
            onSelect={() => set({ boardTheme: p.boardTheme, background: p.background })}
          >
            <div
              className="swatch swatch--preset"
              style={{ backgroundImage: bg ? backgroundCss(bg) : "none", backgroundColor: "var(--bg)" }}
            >
              {board && <BoardSwatch theme={board} small />}
            </div>
          </Thumb>
        );
      })}
    </div>
  );
}

// ------------------------------------------------------------ small parts

function Thumb({ name, selected, onSelect, children }) {
  return (
    <button
      type="button"
      className={`thumb${selected ? " thumb--selected" : ""}`}
      onClick={onSelect}
      aria-pressed={selected}
      title={name}
    >
      <span className="thumb__art">{children}</span>
      <span className="thumb__name">{name}</span>
      {selected && (
        <span className="thumb__check" aria-hidden="true">
          <Icon name="check" size={14} />
        </span>
      )}
    </button>
  );
}

// A small 8x8 board of a theme (whole-board pictures are shown as they are).
const CELLS = Array.from({ length: 64 }, (_, i) => ((i >> 3) + (i & 7)) % 2 === 0);
function BoardSwatch({ theme, small = false }) {
  let light = { backgroundColor: theme.light };
  let dark = { backgroundColor: theme.dark };
  let wrap = {};
  let cells = true;
  if (theme.kind === "tiles") {
    light = { backgroundImage: `url("${theme.light}")`, backgroundSize: "cover" };
    dark = { backgroundImage: `url("${theme.dark}")`, backgroundSize: "cover" };
  } else if (theme.kind === "image") {
    cells = false;
    wrap = { backgroundImage: `url("${theme.image}")`, backgroundSize: "100% 100%" };
  }
  return (
    <span className={`bsw${small ? " bsw--small" : ""}`} style={wrap}>
      {cells && CELLS.map((isLight, i) => <i key={i} style={isLight ? light : dark} />)}
    </span>
  );
}

// A row of four pieces of a set. The standard set is drawn by the board
// library itself; custom sets are plain pictures.
function PieceStrip({ set }) {
  const codes = ["wK", "wQ", "bN", "bR"];
  return (
    <span className="pstrip">
      {codes.map((c) =>
        set.builtin ? (
          <SparePiece key={c} piece={c} width={34} dndId={`thumb-${c}`} />
        ) : (
          <img key={c} src={set.files[c]} alt="" width={34} height={34} draggable={false} />
        )
      )}
    </span>
  );
}

function Warnings({ items, text }) {
  if (!items.length) return null;
  return (
    <ul className="warnings">
      {items.map((i) => (
        <li key={i.id}>{text(i)}</li>
      ))}
    </ul>
  );
}

function Hint({ children }) {
  return <p className="picker__hint">{children}</p>;
}

