import { useEffect, useState } from "react";
import { useAuth } from "../auth/AuthContext.jsx";
import Icon from "../ui/Icon.jsx";
import { useT } from "../i18n/index.js";
import BlockedTab from "./BlockedTab.jsx";
import ChallengesTab from "./ChallengesTab.jsx";
import { useCommunity } from "./CommunityContext.jsx";
import CorrTab from "./CorrTab.jsx";
import FriendsTab from "./FriendsTab.jsx";
import MessagesTab from "./MessagesTab.jsx";
import { communityHash, parseCommunityHash } from "./logic.js";
import "./community.css";

const TABS = [
  ["friends", "menu.friends", "users", (s) => s.friendRequests],
  ["messages", "menu.messages", "message", (s) => s.messages],
  ["challenges", "menu.challenges", "duel", (s) => s.challenges],
  ["correspondence", "menu.correspondence", "clock", (s) => s.yourTurn],
  ["blocked", null, "block", () => 0], // the name comes from the dictionary: safety.tab
];

// "Сообщество": friends, private messages between friends, challenges to a game and the correspondence
// games. The leaderboard and the forum are pages of their own in the same menu group.
export default function CommunityPage() {
  const t = useT();
  const { status, user, guest } = useAuth();
  const { summary } = useCommunity();
  const [where, setWhere] = useState(() => parseCommunityHash(window.location.hash));
  const [error, setError] = useState("");

  useEffect(() => {
    const onHash = () => setWhere(parseCommunityHash(window.location.hash));
    window.addEventListener("hashchange", onHash);
    return () => window.removeEventListener("hashchange", onHash);
  }, []);

  return (
    <div className="page community">
      <header className="page__head">
        <h1 className="page__title">{t("menu.community")}</h1>
        <p className="page__sub">{t("commPage.6")}</p>
      </header>

      {!user ? (
        <section className="card csection cintro">
          <Icon name="users" size={32} />
          <h2>{t("commPage.7")}</h2>
          <p>{t("commPage.8")}</p>
          {error && <div className="banner banner--error">{error}</div>}
          <button
            type="button"
            className="btn btn--primary"
            disabled={status !== "ready"}
            onClick={() => guest().catch((e) => setError(e.message))}
          >
            {t("auth.asGuest")}
          </button>
        </section>
      ) : (
        <>
          <div className="ctabs" role="tablist" aria-label={t("commPage.10")}>
            {TABS.map(([id, name, icon, count]) => {
              const n = summary ? count(summary) : 0;
              return (
                <a
                  key={id}
                  role="tab"
                  href={communityHash(id)}
                  aria-selected={where.tab === id}
                  className={`ctabs__tab${where.tab === id ? " ctabs__tab--on" : ""}`}
                >
                  <Icon name={icon} size={16} />
                  {name ? t(name) : t("safety.tab")}
                  {n > 0 && <span className="cbadge">{n}</span>}
                </a>
              );
            })}
          </div>
          {where.tab === "friends" && <FriendsTab />}
          {where.tab === "messages" && <MessagesTab friendId={where.id} />}
          {where.tab === "challenges" && <ChallengesTab />}
          {where.tab === "correspondence" && <CorrTab />}
          {where.tab === "blocked" && <BlockedTab />}
        </>
      )}
    </div>
  );
}
