import { useCallback, useEffect, useState } from "react";
import { useAuth } from "../auth/AuthContext.jsx";
import Icon from "../ui/Icon.jsx";
import { useT } from "../i18n/index.js";
import { acceptFriend, declineFriend, fetchRelation, muteFriend, removeFriend, requestFriend, unblockPlayer } from "./api.js";
import { BlockButton } from "./BlockedTab.jsx";
import { useCommunity } from "./CommunityContext.jsx";
import { communityHash } from "./logic.js";
import { ChallengeForm } from "./People.jsx";
import ReportButton from "./ReportButton.jsx";
import "./community.css";

// The buttons on the page of another player: add to friends, write, challenge, stop receiving their challenges,
// report, block.
export default function RelationBar({ person }) {
  const t = useT();
  const { user } = useAuth();
  const { refresh } = useCommunity();
  const [rel, setRel] = useState(null);
  const [error, setError] = useState("");
  const [challenging, setChallenging] = useState(false);
  const [sent, setSent] = useState(false);
  const [busy, setBusy] = useState(false);

  const load = useCallback(() => {
    fetchRelation(person.id).then(setRel, (e) => setError(e.message));
  }, [person.id]);
  useEffect(() => {
    if (user && user.id !== person.id) load();
  }, [user, person.id, load]);

  if (!user || user.id === person.id) return null;

  async function act(fn, ...args) {
    setBusy(true);
    setError("");
    try {
      await fn(...args);
    } catch (e) {
      setError(e.message);
    }
    setBusy(false);
    load();
    refresh();
  }

  return (
    <section className="card csection relbar">
      {error && <div className="banner banner--error">{error}</div>}
      {!rel && !error && <p className="cnote">{t("leadersPage.6")}</p>}
      {rel?.relation === "none" && (
        <div className="relbar__row">
          <button type="button" className="btn btn--primary" disabled={busy} onClick={() => act(requestFriend, person.id)}>
            <Icon name="users" size={16} /> {t("friendsTab.33")}
          </button>
          <span className="cnote">{t("relation.3")}</span>
        </div>
      )}
      {rel?.relation === "outgoing" && (
        <div className="relbar__row">
          <span className="cnote">{t("relation.4")}</span>
          <button type="button" className="btn btn--ghost" disabled={busy} onClick={() => act(removeFriend, person.id)}>
            {t("friendsTab.27")}
          </button>
        </div>
      )}
      {rel?.relation === "incoming" && (
        <div className="relbar__row">
          <span className="cnote">{person.name} {t("relation.6")}</span>
          <button type="button" className="btn btn--primary" disabled={busy} onClick={() => act(acceptFriend, person.id)}>
            {t("gameRoom.64")}
          </button>
          <button type="button" className="btn" disabled={busy} onClick={() => act(declineFriend, person.id)}>
            {t("gameRoom.65")}
          </button>
        </div>
      )}
      {rel?.relation === "friend" && (
        <div className="relbar__row">
          <span className="cnote">{rel.online ? t("relation.9") : t("relation.10")}</span>
          <a className="btn" href={communityHash("messages", person.id)}>
            <Icon name="message" size={16} /> {t("friendsTab.15")}
          </a>
          <button type="button" className="btn btn--primary" onClick={() => setChallenging((v) => !v)} aria-expanded={challenging}>
            <Icon name="duel" size={16} /> {t("relation.12")}
          </button>
          <button
            type="button"
            className={`btn btn--ghost${rel.muted ? " btn--on" : ""}`}
            disabled={busy}
            onClick={() => act(muteFriend, person.id, !rel.muted)}
            title={t("relation.13")}
          >
            {rel.muted ? t("friendsTab.19") : t("friendsTab.20")}
          </button>
          <button type="button" className="btn btn--danger-ghost" disabled={busy} onClick={() => window.confirm(t("relation.16", { name: person.name })) && act(removeFriend, person.id)}>
            {t("friendsTab.24")}
          </button>
        </div>
      )}
      {rel?.relation === "blocked" && (
        <div className="relbar__row">
          <span className="cnote">{t("safety.blockedNote")}</span>
          <button type="button" className="btn" disabled={busy} onClick={() => act(unblockPlayer, person.id)}>
            {t("safety.unblock")}
          </button>
        </div>
      )}
      {rel && rel.relation !== "friend" && rel.relation !== "self" && rel.relation !== "blocked" && (
        <div className="relbar__row">
          <button type="button" className="btn" onClick={() => setChallenging((v) => !v)} aria-expanded={challenging}>
            <Icon name="duel" size={16} /> {t("relation.12")}
          </button>
        </div>
      )}
      {rel && rel.relation !== "self" && (
        <div className="relbar__row relbar__row--safety">
          <ReportButton kind="user" targetId={person.id} />
          {rel.relation !== "blocked" && <BlockButton person={person} onDone={() => { load(); refresh(); }} />}
        </div>
      )}
      {sent && (
        <p className="cnote">
          {t("relation.18")}<a href={communityHash("challenges")}>{t("relation.19")}</a>{t("relation.20")}
        </p>
      )}
      {challenging && (
        <ChallengeForm
          person={person}
          onClose={() => setChallenging(false)}
          onSent={() => {
            setChallenging(false);
            setSent(true);
          }}
        />
      )}
    </section>
  );
}
