// Small controls shared by the settings panels.

export function Row({ title, hint, control, children }) {
  return (
    <div className="srow">
      <div className="srow__head">
        <div>
          <h3 className="srow__title">{title}</h3>
          {hint && <p className="srow__hint">{hint}</p>}
        </div>
        {control}
      </div>
      {children && <div className="srow__body">{children}</div>}
    </div>
  );
}

export function Switch({ checked, onChange, label }) {
  return (
    <button
      type="button"
      role="switch"
      aria-checked={checked}
      aria-label={label}
      className={`switch${checked ? " switch--on" : ""}`}
      onClick={() => onChange(!checked)}
    >
      <span className="switch__knob" />
    </button>
  );
}

export function SaveBar({ dirty, justSaved, onSave, onCancel }) {
  return (
    <div className="savebar">
      <button type="button" className="btn" onClick={onCancel} disabled={!dirty}>
        Отмена
      </button>
      <button type="button" className="btn btn--primary" onClick={onSave} disabled={!dirty}>
        Сохранить
      </button>
      {justSaved && !dirty && (
        <span className="savebar__ok" role="status">
          Сохранено
        </span>
      )}
    </div>
  );
}

// A row for a setting that waits for a feature of the site that is not built yet (correspondence
// games, friends and challenges, the trainer): shown, disabled, marked "скоро".
export function SoonRow({ title, hint }) {
  return (
    <div className="srow srow--soon">
      <div className="srow__head">
        <div>
          <h3 className="srow__title">
            {title} <span className="nav__soon">скоро</span>
          </h3>
          {hint && <p className="srow__hint">{hint}</p>}
        </div>
      </div>
    </div>
  );
}
