import { useCallback, useEffect, useState } from "react";
import { useT } from "../i18n/index.js";
import Icon from "../ui/Icon.jsx";
import { blockPlayer, fetchBlocks, unblockPlayer } from "./api.js";
import { PersonCell } from "./People.jsx";
import "./community.css";

// "Заблокировать" on the page of a player or in a talk: asks once, then blocks. onDone runs after.
export function BlockButton({ person, className = "btn btn--danger-ghost", onDone }) {
  const t = useT();
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  async function go() {
    if (!window.confirm(t("safety.blockConfirm", { name: person.name }))) return;
    setBusy(true);
    setError("");
    try {
      await blockPlayer(person.id);
      onDone?.();
    } catch (e) {
      setError(e.message);
    }
    setBusy(false);
  }
  return (
    <>
      <button type="button" className={className} disabled={busy} onClick={go}>
        <Icon name="block" size={16} /> {t("safety.block")}
      </button>
      {error && <span className="cnote cnote--error">{error}</span>}
    </>
  );
}

// "Заблокированные": whom the person has blocked, with the way back.
export default function BlockedTab() {
  const t = useT();
  const [data, setData] = useState(null);
  const [error, setError] = useState("");
  const load = useCallback(() => {
    fetchBlocks().then(
      (d) => {
        setData(d);
        setError("");
      },
      (e) => setError(e.message),
    );
  }, []);
  useEffect(load, [load]);

  async function unblock(id) {
    try {
      await unblockPlayer(id);
      load();
    } catch (e) {
      setError(e.message);
    }
  }

  return (
    <div className="ctab">
      <section className="card csection">
        <header className="csection__head">
          <h2>{t("safety.blockedTitle")}</h2>
          {data && <span className="cnote">{t("safety.blockedCount", { n: data.items.length, max: data.max })}</span>}
        </header>
        <p className="cnote">{t("safety.blockedIntro")}</p>
        {error && <div className="banner banner--error">{error}</div>}
        {!data && !error && <p className="cnote">{t("safety.loading")}</p>}
        {data && data.items.length === 0 && <p className="cnote">{t("safety.blockedEmpty")}</p>}
        {data && data.items.length > 0 && (
          <ul className="clist">
            {data.items.map((p) => (
              <li key={p.id} className="clist__row">
                <PersonCell p={p} />
                <button type="button" className="btn" onClick={() => unblock(p.id)}>
                  {t("safety.unblock")}
                </button>
              </li>
            ))}
          </ul>
        )}
      </section>
    </div>
  );
}
