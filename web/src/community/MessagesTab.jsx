import { useCallback, useEffect, useRef, useState } from "react";
import Icon from "../ui/Icon.jsx";
import { fetchConversations, fetchMessages, sendMessage } from "./api.js";
import { useCommunity } from "./CommunityContext.jsx";
import { ago, communityHash } from "./logic.js";
import Avatar from "../auth/Avatar.jsx";
import { PersonCell } from "./People.jsx";
import { BlockButton } from "./BlockedTab.jsx";
import ReportButton from "./ReportButton.jsx";
import { locale, useT } from "../i18n/index.js";

const MAX = 1000;

// "Сообщения": the talks on the left, the open talk on the right. Messages are only between friends.
export default function MessagesTab({ friendId }) {
  const t = useT();
  const { refresh } = useCommunity();
  const [convs, setConvs] = useState(null);
  const [error, setError] = useState("");

  const loadConvs = useCallback(() => {
    fetchConversations().then(setConvs, (e) => setError(e.message));
  }, []);
  useEffect(() => {
    loadConvs();
    const t = setInterval(() => !document.hidden && loadConvs(), 10000);
    return () => clearInterval(t);
  }, [loadConvs]);

  return (
    <div className={`cmsg${friendId ? " cmsg--open" : ""}`}>
      <aside className="cmsg__list card" aria-label={t("messages.1")}>
        {error && <p className="cnote cnote--error">{error}</p>}
        {convs && convs.length === 0 && (
          <p className="cnote">
            {t("messages.2")}
            <br />
            <a href={communityHash("friends")}>{t("messages.3")}</a>
          </p>
        )}
        <ul className="cconvs">
          {convs?.map((c) => (
            <li key={c.id}>
              <a className={`cconv${c.id === friendId ? " cconv--on" : ""}`} href={communityHash("messages", c.id)}>
                <span className="cperson__pic">
                  <Avatar user={c} size={36} />
                  {c.online && <span className="cperson__dot" title={t("people.2")} />}
                </span>
                <span className="cconv__body">
                  <span className="cconv__top">
                    <b className="cconv__name">{c.name}</b>
                    <time className="cconv__time">{ago(c.lastAt, Date.now())}</time>
                  </span>
                  <span className="cconv__last">
                    {c.mine ? t("messages.5") : ""}
                    {c.last}
                  </span>
                </span>
                {c.unread > 0 && <span className="cbadge">{c.unread}</span>}
              </a>
            </li>
          ))}
        </ul>
      </aside>

      <section className="cmsg__thread card">
        {friendId ? (
          <Thread key={friendId} friendId={friendId} onChanged={() => { loadConvs(); refresh(); }} />
        ) : (
          <div className="cmsg__empty">
            <Icon name="message" size={32} />
            <p>{t("messages.6")}</p>
          </div>
        )}
      </section>
    </div>
  );
}

function Thread({ friendId, onChanged }) {
  const t = useT();
  const [friend, setFriend] = useState(null);
  const [messages, setMessages] = useState(null);
  const [error, setError] = useState("");
  const [closed, setClosed] = useState(false); // no longer friends
  const [text, setText] = useState("");
  const [busy, setBusy] = useState(false);
  const last = useRef(0);
  const box = useRef(null);
  const stick = useRef(true); // the view follows new messages while it is at the bottom

  const pull = useCallback(
    async (first) => {
      try {
        const r = await fetchMessages(friendId, first ? 0 : last.current);
        setFriend(r.friend);
        if (first) setMessages(r.messages);
        else if (r.messages.length) setMessages((m) => [...(m || []), ...r.messages.filter((x) => !(m || []).some((y) => y.id === x.id))]);
        if (r.messages.length) {
          last.current = r.messages[r.messages.length - 1].id;
          onChanged();
        }
        setError("");
      } catch (e) {
        if (e.status === 403) setClosed(true);
        setError(e.message);
      }
    },
    [friendId, onChanged],
  );
  useEffect(() => {
    pull(true);
    const t = setInterval(() => !document.hidden && pull(false), 4000);
    return () => clearInterval(t);
  }, [pull]);

  useEffect(() => {
    const el = box.current;
    if (el && stick.current) el.scrollTop = el.scrollHeight;
  }, [messages]);

  async function send() {
    const body = text.trim();
    if (!body || busy) return;
    setBusy(true);
    try {
      const m = await sendMessage(friendId, body);
      setText("");
      setMessages((l) => [...(l || []), m]);
      last.current = Math.max(last.current, m.id);
      stick.current = true;
      setError("");
      onChanged();
    } catch (e) {
      setError(e.message);
    } finally {
      setBusy(false);
    }
  }

  return (
    <>
      <header className="cthread__head">
        <a className="btn btn--ghost cthread__back" href={communityHash("messages")} aria-label={t("messages.7")}>
          <Icon name="back" size={16} />
        </a>
        {friend ? <PersonCell p={friend} /> : <span className="cnote">{t("leadersPage.6")}</span>}
        {friend && (
          <span className="cthread__tools">
            <BlockButton person={friend} className="btn btn--ghost" onDone={() => { window.location.hash = communityHash("messages"); onChanged(); }} />
          </span>
        )}
      </header>
      <div
        className="cthread__box"
        ref={box}
        onScroll={(e) => {
          const el = e.currentTarget;
          stick.current = el.scrollHeight - el.scrollTop - el.clientHeight < 40;
        }}
        aria-live="polite"
      >
        {messages && messages.length === 0 && !closed && <p className="cnote">{t("messages.9")}</p>}
        {messages?.map((m) => (
          <div key={m.id} className={`cbubble${m.mine ? " cbubble--mine" : ""}`}>
            {/* plain text: React escapes it, line breaks are kept by CSS */}
            <p className="cbubble__text">{m.text}</p>
            <time className="cbubble__time">
              {new Date(m.at).toLocaleTimeString(locale(), { hour: "2-digit", minute: "2-digit" })}
              {m.mine && m.read ? t("messages.10") : ""}
            </time>
            {!m.mine && <ReportButton kind="message" targetId={m.id} className="cbubble__report" icon={false} />}
          </div>
        ))}
      </div>
      {error && <p className="cnote cnote--error">{error}</p>}
      {closed ? (
        <p className="cnote">{t("messages.11")} <a href={communityHash("friends")}>{t("messages.3")}</a></p>
      ) : (
        <form
          className="ccompose"
          onSubmit={(e) => {
            e.preventDefault();
            send();
          }}
        >
          <textarea
            className="field ccompose__input"
            value={text}
            maxLength={MAX}
            rows={2}
            placeholder={t("messages.12")}
            aria-label={t("chat.6")}
            onChange={(e) => setText(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === "Enter" && !e.shiftKey && !e.nativeEvent.isComposing) {
                e.preventDefault();
                send();
              }
            }}
          />
          <button type="submit" className="btn btn--primary" disabled={busy || !text.trim()} aria-label={t("messages.14")}>
            <Icon name="send" size={18} />
          </button>
        </form>
      )}
    </>
  );
}
