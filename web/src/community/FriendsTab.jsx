import { useCallback, useEffect, useRef, useState } from "react";
import { useAuth } from "../auth/AuthContext.jsx";
import Icon from "../ui/Icon.jsx";
import { acceptFriend, declineFriend, fetchFriends, muteFriend, removeFriend, requestFriend, searchPeople } from "./api.js";
import { useCommunity } from "./CommunityContext.jsx";
import { communityHash } from "./logic.js";
import { ChallengeForm, PersonCell } from "./People.jsx";
import { useT } from "../i18n/index.js";

// "Друзья": requests, the list (write, challenge, mute challenges, remove) and a search for new ones.
export default function FriendsTab() {
  const t = useT();
  const { user } = useAuth();
  const { refresh } = useCommunity();
  const [data, setData] = useState(null);
  const [error, setError] = useState("");
  const [challenging, setChallenging] = useState(""); // the id of the friend with the open challenge form
  const [confirming, setConfirming] = useState(""); // the id whose removal waits for a second press
  const [copied, setCopied] = useState(false);

  const load = useCallback(() => {
    fetchFriends().then(
      (d) => {
        setData(d);
        setError("");
      },
      (e) => setError(e.message),
    );
  }, []);
  useEffect(() => {
    load();
    const t = setInterval(() => !document.hidden && load(), 15000);
    return () => clearInterval(t);
  }, [load]);

  async function act(fn, ...args) {
    setError("");
    try {
      await fn(...args);
    } catch (e) {
      setError(e.message);
    }
    load();
    refresh();
  }

  async function copyLink() {
    const url = `${window.location.origin}/#user/${user.id}`;
    try {
      await navigator.clipboard.writeText(url);
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    } catch {
      window.prompt(t("friendsTab.1"), url);
    }
  }

  return (
    <div className="ctab">
      {error && <div className="banner banner--error">{error}</div>}

      <section className="card csection">
        <header className="csection__head">
          <h2>{t("friendsTab.2")}</h2>
          <button type="button" className="btn btn--ghost" onClick={copyLink} title={t("friendsTab.3")}>
            <Icon name={copied ? "check" : "link"} size={16} /> {copied ? t("gameRoom.16") : t("friendsTab.5")}
          </button>
        </header>
        <Search onAdded={() => { load(); refresh(); }} friends={data} />
      </section>

      {data?.incoming?.length > 0 && (
        <section className="card csection">
          <header className="csection__head">
            <h2>{t("friendsTab.6")}{data.incoming.length})</h2>
          </header>
          <ul className="clist">
            {data.incoming.map((r) => (
              <li key={r.id} className="clist__row">
                <PersonCell p={r} />
                <span className="clist__actions">
                  <button type="button" className="btn btn--primary" onClick={() => act(acceptFriend, r.id)}>
                    {t("gameRoom.64")}
                  </button>
                  <button type="button" className="btn" onClick={() => act(declineFriend, r.id)}>
                    {t("gameRoom.65")}
                  </button>
                </span>
              </li>
            ))}
          </ul>
        </section>
      )}

      <section className="card csection">
        <header className="csection__head">
          <h2>{t("friendsTab.9")}{data ? ` (${data.friends.length})` : ""}</h2>
        </header>
        {!data && !error && <p className="cnote">{t("leadersPage.6")}</p>}
        {data && data.friends.length === 0 && (
          <p className="cnote">
            {t("friendsTab.11")}
          </p>
        )}
        <ul className="clist">
          {data?.friends.map((f) => (
            <li key={f.id} className="clist__block">
              <div className="clist__row">
                <PersonCell p={f}>
                  <span className="cperson__sub">{f.online ? t("friendsTab.12") : t("friendsTab.13")}{f.muted ? t("friendsTab.14") : ""}</span>
                </PersonCell>
                <span className="clist__actions">
                  <a className="btn" href={communityHash("messages", f.id)}>
                    <Icon name="message" size={16} /> {t("friendsTab.15")}
                    {f.unread > 0 && <span className="cbadge cbadge--inline">{f.unread}</span>}
                  </a>
                  <button type="button" className="btn btn--primary" onClick={() => setChallenging(challenging === f.id ? "" : f.id)} aria-expanded={challenging === f.id}>
                    <Icon name="duel" size={16} /> {t("friendsTab.16")}
                  </button>
                  <button
                    type="button"
                    className={`btn btn--ghost${f.muted ? " btn--on" : ""}`}
                    onClick={() => act(muteFriend, f.id, !f.muted)}
                    title={f.muted ? t("friendsTab.17") : t("friendsTab.18")}
                  >
                    {f.muted ? t("friendsTab.19") : t("friendsTab.20")}
                  </button>
                  {confirming === f.id ? (
                    <>
                      <button type="button" className="btn btn--danger" onClick={() => { setConfirming(""); act(removeFriend, f.id); }}>
                        {t("friendsTab.21")}
                      </button>
                      <button type="button" className="btn btn--ghost" onClick={() => setConfirming("")}>
                        {t("gameRoom.67")}
                      </button>
                    </>
                  ) : (
                    <button type="button" className="btn btn--danger-ghost" onClick={() => setConfirming(f.id)} aria-label={t("friendsTab.23", { name: f.name })} title={t("friendsTab.24")}>
                      <Icon name="trash" size={16} />
                    </button>
                  )}
                </span>
              </div>
              {challenging === f.id && (
                <ChallengeForm
                  person={f}
                  onClose={() => setChallenging("")}
                  onSent={() => {
                    setChallenging("");
                    window.location.hash = communityHash("challenges");
                  }}
                />
              )}
            </li>
          ))}
        </ul>
      </section>

      {data?.outgoing?.length > 0 && (
        <section className="card csection">
          <header className="csection__head">
            <h2>{t("friendsTab.25")}</h2>
          </header>
          <ul className="clist">
            {data.outgoing.map((r) => (
              <li key={r.id} className="clist__row">
                <PersonCell p={r}>
                  <span className="cperson__sub">{t("friendsTab.26")}</span>
                </PersonCell>
                <button type="button" className="btn btn--ghost" onClick={() => act(removeFriend, r.id)}>
                  {t("friendsTab.27")}
                </button>
              </li>
            ))}
          </ul>
        </section>
      )}
    </div>
  );
}

// The search: the beginning of a name, results after a short pause.
function Search({ friends, onAdded }) {
  const t = useT();
  const [q, setQ] = useState("");
  const [found, setFound] = useState(null);
  const [error, setError] = useState("");
  const [sent, setSent] = useState({});
  const seq = useRef(0);

  useEffect(() => {
    const text = q.trim();
    if ([...text].length < 2) {
      setFound(null);
      return undefined;
    }
    const mine = ++seq.current;
    const t = setTimeout(() => {
      searchPeople(text).then(
        (items) => mine === seq.current && (setFound(items), setError("")),
        (e) => mine === seq.current && setError(e.message),
      );
    }, 350);
    return () => clearTimeout(t);
  }, [q]);

  const friendIds = new Set([...(friends?.friends || []), ...(friends?.outgoing || []), ...(friends?.incoming || [])].map((x) => x.id));
  const isFriend = new Set((friends?.friends || []).map((x) => x.id));

  async function add(p) {
    setError("");
    try {
      const r = await requestFriend(p.id);
      setSent((s) => ({ ...s, [p.id]: r.status }));
      onAdded();
    } catch (e) {
      setError(e.message);
    }
  }

  return (
    <>
      <input className="field" type="search" value={q} onChange={(e) => setQ(e.target.value)} placeholder={t("friendsTab.28")} aria-label={t("friendsTab.29")} maxLength={40} />
      {error && <p className="cnote cnote--error">{error}</p>}
      {found && found.length === 0 && <p className="cnote">{t("friendsTab.30")}</p>}
      {found && found.length > 0 && (
        <ul className="clist">
          {found.map((p) => (
            <li key={p.id} className="clist__row">
              <PersonCell p={p}>{p.kind === "guest" && <span className="cperson__sub">{t("friendsTab.31")}</span>}</PersonCell>
              {isFriend.has(p.id) || sent[p.id] === "friends" ? (
                <span className="cnote">{t("friendsTab.32")}</span>
              ) : friendIds.has(p.id) || sent[p.id] === "requested" ? (
                <span className="cnote">{t("friendsTab.26")}</span>
              ) : (
                <button type="button" className="btn" onClick={() => add(p)}>
                  <Icon name="users" size={16} /> {t("friendsTab.33")}
                </button>
              )}
            </li>
          ))}
        </ul>
      )}
    </>
  );
}
