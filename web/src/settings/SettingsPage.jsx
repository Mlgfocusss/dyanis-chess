import { useEffect, useMemo, useRef, useState } from "react";
import { ChessboardDnDProvider, SparePiece } from "react-chessboard";
import Icon from "../ui/Icon.jsx";
import Segmented from "../ui/Segmented.jsx";
import ThemedBoard from "../components/ThemedBoard.jsx";
import { defaultsOf, useSettings } from "./SettingsContext.jsx";
import { themeName } from "../themes/names.js";
import { applyInterface } from "./model.js";
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
import InterfacePanel from "./InterfacePanel.jsx";
import ProfilePanel from "./ProfilePanel.jsx";
import AccountPanel from "./AccountPanel.jsx";
import { Row, SaveBar, Switch } from "./controls.jsx";
import { useT } from "../i18n/index.js";
import "./settings.css";

// A position with every kind of piece, for the preview board.
const PREVIEW_FEN = "r1bqk2r/pppp1ppp/2n2n2/2b1p3/2B1P3/2N2N2/PPPP1PPP/R1BQK2R w KQkq - 6 5";

const TABS = [
  { id: "boards", label: "settings.tabBoards" },
  { id: "pieces", label: "settings.tabPieces" },
  { id: "background", label: "settings.tabBackground" },
  { id: "presets", label: "settings.tabPresets" },
];

const SECTIONS = [
  { id: "board", label: "settings.secBoard", icon: "board" },
  { id: "play", label: "settings.secPlay", icon: "bot" },
  { id: "interface", label: "settings.secInterface", icon: "monitor" },
  { id: "profile", label: "settings.secProfile", icon: "user" },
  { id: "account", label: "settings.secAccount", icon: "lock" },
];

// "#settings/play" opens the gameplay page and so on; anything else is the board page.
function sectionFromHash() {
  const id = window.location.hash.split("/")[1];
  return SECTIONS.some((s) => s.id === id && s.id !== "board") ? id : "board";
}

export default function SettingsPage() {
  const t = useT();
  const { settings, save } = useSettings();
  const [draft, setDraft] = useState(settings);
  const [tab, setTab] = useState("boards");
  const [section, setSection] = useState(sectionFromHash);
  function openSection(id) {
    setSection(id);
    window.history.replaceState(null, "", id === "board" ? "#settings" : `#settings/${id}`);
  }
  // a link to another section (#settings/profile) while the settings are open
  useEffect(() => {
    const onHash = () => {
      if (window.location.hash.split("/")[0] === "#settings") setSection(sectionFromHash());
    };
    window.addEventListener("hashchange", onHash);
    return () => window.removeEventListener("hashchange", onHash);
  }, []);
  const [justSaved, setJustSaved] = useState(false);

  const dirty = useMemo(() => JSON.stringify(draft) !== JSON.stringify(settings), [draft, settings]);
  const set = (patch) => {
    setJustSaved(false);
    setDraft((d) => ({ ...d, ...patch }));
  };

  // While the interface page is open its unsaved choices are shown on the whole site; otherwise
  // (and when the page is left) the saved ones.
  // the language is saved at once (also by the switch in the menu): the draft follows it, so that it does not look unsaved
  useEffect(() => {
    setDraft((d) => (d.language === settings.language ? d : { ...d, language: settings.language }));
  }, [settings.language]);

  const savedRef = useRef(settings);
  savedRef.current = settings;
  useEffect(() => {
    applyInterface(document.documentElement, section === "interface" ? draft : savedRef.current);
    return () => applyInterface(document.documentElement, savedRef.current);
  }, [section, draft.theme, draft.textSize, draft.reduceMotion, draft.highContrast]); // eslint-disable-line react-hooks/exhaustive-deps

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
        <h1 className="page__title">{t("menu.settings")}</h1>
      </header>

      <div className="settings">
        <nav className="settings__menu" aria-label={t("settingsPage.11")}>
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
              <span>{t(s.label)}</span>
              {s.soon && <span className="nav__soon">{t("common.soon")}</span>}
            </button>
          ))}
        </nav>

        {section === "interface" ? (
          <section className="settings__body card">
            <InterfacePanel draft={draft} set={set} dirty={dirty} justSaved={justSaved} onSave={onSave} onCancel={onCancel} />
            <button type="button" className="settings__reset" onClick={() => set(defaultsOf("interface"))}>
              {t("settingsPage.13")}
            </button>
          </section>
        ) : section === "profile" ? (
          <section className="settings__body card">
            <ProfilePanel />
          </section>
        ) : section === "account" ? (
          <section className="settings__body card">
            <AccountPanel />
          </section>
        ) : section === "play" ? (
          <section className="settings__body card">
            <GameplayPanel draft={draft} set={set} dirty={dirty} justSaved={justSaved} onSave={onSave} onCancel={onCancel} />
            <button type="button" className="settings__reset" onClick={() => set(defaultsOf(true))}>
              {t("settingsPage.14")}
            </button>
          </section>
        ) : (
        <ChessboardDnDProvider>
          <section className="settings__body card">
            <h2 className="settings__h">{t("settings.secBoard")}</h2>
            <p className="settings__sub">{t("settingsPage.15")}</p>

            <div className="tabs" role="tablist">
              {TABS.map((tb) => (
                <button
                  key={tb.id}
                  type="button"
                  role="tab"
                  aria-selected={tab === tb.id}
                  className={`tabs__tab${tab === tb.id ? " tabs__tab--active" : ""}`}
                  onClick={() => setTab(tb.id)}
                >
                  {t(tb.label)}
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
              title={t("settingsPage.16")}
              hint={t("settingsPage.17")}
              control={
                <Switch
                  checked={draft.coordinates !== "off"}
                  onChange={(on) => set({ coordinates: on ? "inside" : "off" })}
                  label={t("settingsPage.18")}
                />
              }
            >
              {draft.coordinates !== "off" && (
                <Segmented
                  value={draft.coordinates}
                  onChange={(v) => set({ coordinates: v })}
                  options={[
                    ["inside", t("settingsPage.19")],
                    ["outside", t("settingsPage.20")],
                  ]}
                />
              )}
            </Row>

            <Row
              title={t("settingsPage.21")}
              hint={t("settingsPage.22")}
              control={
                <Switch
                  checked={draft.highlightLastMove}
                  onChange={(v) => set({ highlightLastMove: v })}
                  label={t("settingsPage.21")}
                />
              }
            />

            <Row title={t("settingsPage.23")} hint={t("settingsPage.24")}>
              <Segmented
                value={draft.moveMethod}
                onChange={(v) => set({ moveMethod: v })}
                options={[
                  ["both", t("settingsPage.25")],
                  ["drag", t("settingsPage.26")],
                  ["click", t("settingsPage.27")],
                ]}
              />
            </Row>

            <Row title={t("settingsPage.28")} hint={t("settingsPage.29")}>
              <Segmented
                value={draft.playLayout}
                onChange={(v) => set({ playLayout: v })}
                options={[
                  ["side", t("settingsPage.30")],
                  ["center", t("settingsPage.31")],
                ]}
              />
            </Row>

            <Row title={t("settingsPage.32")} hint={t("settingsPage.33")}>
              <Segmented
                value={draft.animation}
                onChange={(v) => set({ animation: v })}
                options={[
                  ["none", t("settingsPage.34")],
                  ["slow", t("settingsPage.35")],
                  ["medium", t("settingsPage.36")],
                  ["fast", t("settingsPage.37")],
                ]}
              />
            </Row>

            <Row
              title={t("settingsPage.38")}
              hint={t("settingsPage.39")}
              control={<Switch checked={draft.sound} onChange={(v) => set({ sound: v })} label={t("settingsPage.40")} />}
            >
              {draft.sound && (
                <div className="sound-row">
                  <label className="sound-row__label">
                    {t("settingsPage.41")}
                    <select
                      className="field"
                      value={draft.soundTheme}
                      onChange={(e) => set({ soundTheme: e.target.value })}
                    >
                      {soundThemes.map((th) => (
                        <option key={th.id} value={th.id}>
                          {themeName(t, "sounds", th)}
                        </option>
                      ))}
                    </select>
                  </label>
                  <button
                    type="button"
                    className="btn"
                    onClick={() => playSound("capture", draft.soundTheme)}
                  >
                    {t("settingsPage.42")}
                  </button>
                </div>
              )}
            </Row>

            <SaveBar dirty={dirty} justSaved={justSaved} onSave={onSave} onCancel={onCancel} />
            <button type="button" className="settings__reset" onClick={() => set(defaultsOf(false))}>
              {t("settingsPage.43")}
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
  const t = useT();
  return (
    <>
      <div className="thumbs">
        {boardThemes.map((b) => (
          <Thumb
            key={b.id}
            name={themeName(t, "boards", b)}
            selected={draft.boardTheme === b.id}
            onSelect={() => set({ boardTheme: b.id })}
          >
            <BoardSwatch theme={b} />
          </Thumb>
        ))}
      </div>
      <Warnings
        items={incompleteBoards}
        text={(i) => t("settingsPage.44", { name: i.name, p: i.missing.map((m) => m + ".*").join(", ") })}
      />
      <Hint>
        {t("settingsPage.45")} <code>web/src/themes/boards/</code> {t("settingsPage.46")} <code>themes</code>{t("settingsPage.47")}
      </Hint>
    </>
  );
}

function PiecesTab({ draft, set }) {
  const t = useT();
  return (
    <>
      <div className="thumbs">
        {pieceSets.map((p) => (
          <Thumb
            key={p.id}
            name={themeName(t, "pieces", p)}
            selected={draft.pieceSet === p.id}
            onSelect={() => set({ pieceSet: p.id })}
          >
            <PieceStrip set={p} />
          </Thumb>
        ))}
      </div>
      <Warnings
        items={incompletePieceSets}
        text={(i) => t("settingsPage.48", { name: i.name, p: i.missing.join(", ") })}
      />
      <Hint>
        {t("settingsPage.49")} <code>wP wN wB wR wQ wK bP bN bB bR bQ bK</code> {t("settingsPage.50")}{" "}
        <code>{t("settingsPage.51")}</code>{t("settingsPage.52")}
      </Hint>
    </>
  );
}

function BackgroundTab({ draft, set }) {
  const t = useT();
  return (
    <>
      <div className="thumbs">
        {backgrounds.map((b) => (
          <Thumb
            key={b.id}
            name={themeName(t, "backgrounds", b)}
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
        {t("settingsPage.53")} <code>web/src/themes/backgrounds/</code>{t("settingsPage.54")}
      </Hint>
    </>
  );
}

function PresetsTab({ draft, set }) {
  const t = useT();
  return (
    <div className="thumbs">
      {presets.map((p) => {
        const board = boardThemes.find((b) => b.id === p.boardTheme);
        const bg = backgrounds.find((b) => b.id === p.background);
        const selected = draft.boardTheme === p.boardTheme && draft.background === p.background;
        return (
          <Thumb
            key={p.id}
            name={themeName(t, "presets", p)}
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
  const t = useT();
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

