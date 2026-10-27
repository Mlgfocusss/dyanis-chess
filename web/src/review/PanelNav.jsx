import { useEffect, useRef, useState } from "react";
import Icon from "../ui/Icon.jsx";
import { useT } from "../i18n/index.js";

// The buttons at the foot of the review panel: to the start, back, play or pause, forward, to the end
// (Home, left, space, right, End on the keyboard).
export function PanelNav({ atStart, atEnd, playing, canPlay = true, onFirst, onPrev, onNext, onLast, onPlay, className = "" }) {
  const t = useT();
  return (
    <div className={`rnav ${className}`} role="group" aria-label={t("panelNav.1")}>
      <button type="button" onClick={onFirst} disabled={atStart} title={t("panelNav.2")} aria-label={t("panelNav.3")}>
        <Icon name="first" />
      </button>
      <button type="button" onClick={onPrev} disabled={atStart} title={t("panelNav.4")} aria-label={t("panelNav.5")}>
        <Icon name="prev" />
      </button>
      <button
        type="button"
        className="rnav__play"
        onClick={onPlay}
        disabled={!canPlay}
        title={playing ? t("panelNav.6") : t("panelNav.7")}
        aria-label={playing ? t("panelNav.8") : t("panelNav.9")}
        aria-pressed={playing}
      >
        <Icon name={playing ? "pause" : "play"} />
      </button>
      <button type="button" onClick={onNext} disabled={atEnd} title={t("panelNav.10")} aria-label={t("panelNav.11")}>
        <Icon name="next" />
      </button>
      <button type="button" onClick={onLast} disabled={atEnd} title={t("panelNav.12")} aria-label={t("panelNav.13")}>
        <Icon name="last" />
      </button>
    </div>
  );
}

// The second row: the previous and the next mistake, the board turned over, the person's own branch and
// the "share" menu. share: [{key, label, icon, onClick, href?}]; copied: the key of what was just copied.
export function PanelTools({ canPrevMoment, canNextMoment, onPrevMoment, onNextMoment, onFlip, onBranch, branchOn, onLink, share, copied, hideMoments = false }) {
  const t = useT();
  return (
    <div className="rtools">
      {!hideMoments && (
        <>
          <button type="button" className="rtools__btn rtools__btn--moment" onClick={onPrevMoment} disabled={!canPrevMoment} title={t("panelNav.14")}>
            <Icon name="prev" size={14} />
            <Icon name="alert" size={16} />
            <span className="rtools__sr">{t("panelNav.15")}</span>
          </button>
          <button type="button" className="rtools__btn rtools__btn--moment" onClick={onNextMoment} disabled={!canNextMoment} title={t("panelNav.16")}>
            <Icon name="alert" size={16} />
            <Icon name="next" size={14} />
            <span className="rtools__sr">{t("panelNav.17")}</span>
          </button>
        </>
      )}
      <span className="rtools__gap" />
      <span className="rtools__linkwrap">
        <button type="button" className="rtools__btn rtools__btn--link" onClick={onLink} title={t("panelNav.18")} aria-describedby="rtools-toast">
          <Icon name="link" size={16} />
          <span className="rtools__text">{t("panelNav.19")}</span>
        </button>
        <span id="rtools-toast" className="rtools__toast" role="status" aria-live="polite">
          {copied === "link" ? t("panelNav.20") : ""}
        </span>
      </span>
      <button type="button" className="rtools__btn rtools__btn--icon" onClick={onFlip} title={t("panelNav.21")} aria-label={t("panelNav.22")}>
        <Icon name="flip" size={18} />
      </button>
      <button
        type="button"
        className={`rtools__btn rtools__btn--icon${branchOn ? " rtools__btn--on" : ""}`}
        onClick={onBranch}
        title={branchOn ? t("panelNav.23") : t("panelNav.24")}
        aria-label={branchOn ? t("panelNav.25") : t("reviewView.branch")}
        aria-pressed={branchOn}
      >
        <Icon name="branch" size={18} />
      </button>
      <ShareMenu items={share} copied={copied} />
    </div>
  );
}

// "Поделиться": a small menu above the button with the things that can be copied or opened elsewhere.
function ShareMenu({ items, copied }) {
  const t = useT();
  const [open, setOpen] = useState(false);
  const box = useRef(null);
  useEffect(() => {
    if (!open) return undefined;
    const onDown = (e) => box.current && !box.current.contains(e.target) && setOpen(false);
    const onKey = (e) => {
      if (e.key === "Escape") {
        setOpen(false);
        e.stopPropagation();
      }
    };
    document.addEventListener("pointerdown", onDown);
    window.addEventListener("keydown", onKey, true);
    return () => {
      document.removeEventListener("pointerdown", onDown);
      window.removeEventListener("keydown", onKey, true);
    };
  }, [open]);
  return (
    <div className="share" ref={box}>
      <button type="button" className="rtools__btn rtools__btn--icon" onClick={() => setOpen((o) => !o)} aria-expanded={open} aria-haspopup="menu" aria-label={t("panelNav.27")} title={t("panelNav.28")}>
        <Icon name="share" size={18} />
      </button>
      {open && (
        <ul className="share__menu" role="menu">
          {items.map((it) => (
            <li key={it.key} role="none">
              <button
                type="button"
                role="menuitem"
                className="share__item"
                onClick={() => {
                  it.onClick();
                  if (!it.keepOpen) setOpen(false);
                }}
              >
                <Icon name={copied === it.key ? "check" : it.icon} size={16} />
                <span>{copied === it.key ? t("gameRoom.16") : it.label}</span>
              </button>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}
