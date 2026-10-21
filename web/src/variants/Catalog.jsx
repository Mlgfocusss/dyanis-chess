import Icon from "../ui/Icon.jsx";
import { PROBLEMS, VARIANTS } from "./registry.js";

// "Другие шахматы": a card for each variant with a few lines of its rules and the two ways to play.
export default function Catalog({ missing = "" }) {
  return (
    <div className="page vcat">
      <header className="page__head">
        <h1 className="page__title">Другие шахматы</h1>
        <p className="page__sub">Необычные правила и расстановки. Играют люди: по ссылке с другом или с быстрым подбором. Партии без рейтинга и без разбора движком, он знает только классические шахматы.</p>
      </header>

      {missing && <div className="banner banner--error">Такого вида шахмат нет: «{missing}». Выберите из списка.</div>}

      {VARIANTS.length === 0 ? (
        <section className="card vcat__empty">
          <p>Виды шахмат пока не подключены.</p>
        </section>
      ) : (
        <div className="vcat__grid">
          {VARIANTS.map((v) => (
            <article key={v.id} className="card vcard" data-variant={v.id}>
              <header className="vcard__head">
                <span className="vcard__icon">
                  <Icon name={v.icon} size={22} />
                </span>
                <div>
                  <h2 className="vcard__name">{v.name}</h2>
                  <p className="vcard__short">{v.short}</p>
                </div>
              </header>
              {v.rules.length > 0 && (
                <ul className="vcard__rules">
                  {v.rules.slice(0, 3).map((r, i) => (
                    <li key={i}>{r}</li>
                  ))}
                </ul>
              )}
              <div className="vcard__actions">
                <a className="btn btn--primary" href={`#variants/${v.id}/link`}>
                  <Icon name="online" size={16} /> Играть по ссылке
                </a>
                {v.seats !== 4 && (
                  <a className="btn" href={`#variants/${v.id}/quick`}>
                    <Icon name="bolt" size={16} /> Быстрая игра
                  </a>
                )}
              </div>
            </article>
          ))}
        </div>
      )}

      {PROBLEMS.length > 0 && (
        <section className="card vcat__problems" aria-label="Ошибки в описании видов">
          <h2>Не подключено</h2>
          <ul>
            {PROBLEMS.map((p, i) => (
              <li key={i}>{p}</li>
            ))}
          </ul>
        </section>
      )}
    </div>
  );
}
