import Icon from "../ui/Icon.jsx";
import { PROBLEMS, VARIANTS } from "./registry.js";
import { tx, useT } from "../i18n/index.js";

// "Другие шахматы": a card for each variant with a few lines of its rules and the two ways to play.
export default function Catalog({ missing = "" }) {
  const t = useT();
  return (
    <div className="page vcat">
      <header className="page__head">
        <h1 className="page__title">{t("menu.variants")}</h1>
        <p className="page__sub">{t("vcatalog.2")}</p>
      </header>

      {missing && <div className="banner banner--error">{t("vcatalog.3")}{missing}{t("vcatalog.4")}</div>}

      {VARIANTS.length === 0 ? (
        <section className="card vcat__empty">
          <p>{t("vcatalog.5")}</p>
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
                  <h2 className="vcard__name">{tx(v.name)}</h2>
                  <p className="vcard__short">{tx(v.short)}</p>
                </div>
              </header>
              {v.rules.length > 0 && (
                <ul className="vcard__rules">
                  {v.rules.slice(0, 3).map((r, i) => (
                    <li key={i}>{tx(r)}</li>
                  ))}
                </ul>
              )}
              <div className="vcard__actions">
                <a className="btn btn--primary" href={`#variants/${v.id}/link`}>
                  <Icon name="online" size={16} /> {t("vcatalog.link")}
                </a>
                {v.seats !== 4 && (
                  <a className="btn" href={`#variants/${v.id}/quick`}>
                    <Icon name="bolt" size={16} /> {t("menu.quick")}
                  </a>
                )}
              </div>
            </article>
          ))}
        </div>
      )}

      {PROBLEMS.length > 0 && (
        <section className="card vcat__problems" aria-label={t("vcatalog.8")}>
          <h2>{t("vcatalog.9")}</h2>
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
