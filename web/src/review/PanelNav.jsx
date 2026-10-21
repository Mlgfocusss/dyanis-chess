import { useEffect, useRef, useState } from "react";
import Icon from "../ui/Icon.jsx";

// The buttons at the foot of the review panel: to the start, back, play or pause, forward, to the end
// (Home, left, space, right, End on the keyboard).
export function PanelNav({ atStart, atEnd, playing, canPlay = true, onFirst, onPrev, onNext, onLast, onPlay, className = "" }) {
  return (
    <div className={`rnav ${className}`} role="group" aria-label="Навигация по партии">
      <button type="button" onClick={onFirst} disabled={atStart} title="В начало (Home)" aria-label="В начало">
        <Icon name="first" />
      </button>
      <button type="button" onClick={onPrev} disabled={atStart} title="Назад (←)" aria-label="Назад">
        <Icon name="prev" />
      </button>
      <button
        type="button"
        className="rnav__play"
        onClick={onPlay}
        disabled={!canPlay}
        title={playing ? "Пауза (пробел)" : "Проиграть партию (пробел)"}
        aria-label={playing ? "Пауза" : "Проиграть партию"}
        aria-pressed={playing}
      >
        <Icon name={playing ? "pause" : "play"} />
      </button>
      <button type="button" onClick={onNext} disabled={atEnd} title="Вперёд (→)" aria-label="Вперёд">
        <Icon name="next" />
      </button>
      <button type="button" onClick={onLast} disabled={atEnd} title="В конец (End)" aria-label="В конец">
        <Icon name="last" />
      </button>
    </div>
  );
}

// The second row: the previous and the next mistake, the board turned over, the person's own branch and
// the "share" menu. share: [{key, label, icon, onClick, href?}]; copied: the key of what was just copied.
export function PanelTools({ canPrevMoment, canNextMoment, onPrevMoment, onNextMoment, onFlip, onBranch, branchOn, onLink, share, copied, hideMoments = false }) {
  return (
    <div className="rtools">
      {!hideMoments && (
        <>
          <button type="button" className="rtools__btn rtools__btn--moment" onClick={onPrevMoment} disabled={!canPrevMoment} title="Предыдущая ошибка (Shift + ←)">
            <Icon name="prev" size={14} />
            <Icon name="alert" size={16} />
            <span className="rtools__sr">Предыдущая ошибка</span>
          </button>
          <button type="button" className="rtools__btn rtools__btn--moment" onClick={onNextMoment} disabled={!canNextMoment} title="Следующая ошибка (Shift + →)">
            <Icon name="alert" size={16} />
            <Icon name="next" size={14} />
            <span className="rtools__sr">Следующая ошибка</span>
          </button>
        </>
      )}
      <span className="rtools__gap" />
      <span className="rtools__linkwrap">
        <button type="button" className="rtools__btn rtools__btn--link" onClick={onLink} title="Скопировать ссылку: по ней другой человек откроет этот разбор" aria-describedby="rtools-toast">
          <Icon name="link" size={16} />
          <span className="rtools__text">Ссылка</span>
        </button>
        <span id="rtools-toast" className="rtools__toast" role="status" aria-live="polite">
          {copied === "link" ? "Ссылка скопирована" : ""}
        </span>
      </span>
      <button type="button" className="rtools__btn rtools__btn--icon" onClick={onFlip} title="Перевернуть доску (F)" aria-label="Перевернуть доску">
        <Icon name="flip" size={18} />
      </button>
      <button
        type="button"
        className={`rtools__btn rtools__btn--icon${branchOn ? " rtools__btn--on" : ""}`}
        onClick={onBranch}
        title={branchOn ? "Вернуться к партии (Esc)" : "Своя ветка: ходите на доске из этой позиции"}
        aria-label={branchOn ? "Вернуться к партии" : "Своя ветка"}
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
      <button type="button" className="rtools__btn rtools__btn--icon" onClick={() => setOpen((o) => !o)} aria-expanded={open} aria-haspopup="menu" aria-label="Поделиться" title="Поделиться: ссылка, PGN, FEN">
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
                <span>{copied === it.key ? "Скопировано" : it.label}</span>
              </button>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}
