import { useCallback, useEffect, useMemo, useState } from "react";
import { useAuth } from "../auth/AuthContext.jsx";
import Avatar from "../auth/Avatar.jsx";
import {
  createForumTopic,
  deleteForumPost,
  deleteForumTopic,
  editForumPost,
  fetchForumHome,
  fetchForumTopic,
  fetchForumTopics,
  lockForumTopic,
  pinForumTopic,
  replyForumTopic,
} from "../community/api.js";
import ReportButton from "../community/ReportButton.jsx";
import { ago, forumHash, parseForumHash, postShown } from "../community/logic.js";
import "../community/community.css";
import { useT, locale, tOr } from "../i18n/index.js";
import Icon from "../ui/Icon.jsx";
import ReportsQueue from "./ReportsQueue.jsx";

const MAX_BODY = 5000;

// The forum: sections, the topics of a section, a topic with its messages, a new topic. The address
// carries the place (#forum/c/<section>, #forum/t/<id>/<page>, #forum/new/<section>).
export default function ForumPage() {
  const t = useT();
  const [where, setWhere] = useState(() => parseForumHash(window.location.hash));
  useEffect(() => {
    const onHash = () => {
      setWhere(parseForumHash(window.location.hash));
      window.scrollTo?.(0, 0);
    };
    window.addEventListener("hashchange", onHash);
    return () => window.removeEventListener("hashchange", onHash);
  }, []);

  return (
    <div className="page forum">
      <header className="page__head">
        <h1 className="page__title">{t("menu.forum")}</h1>
        <p className="page__sub">{t("forumPage.2")}</p>
      </header>
      {where.view === "home" && <Home />}
      {where.view === "category" && <Category key={where.category + where.page} category={where.category} page={where.page} />}
      {where.view === "topic" && <Topic key={where.id + "/" + where.page} id={where.id} page={where.page} />}
      {where.view === "new" && <NewTopic category={where.category} />}
      {where.view === "reports" && <ReportsQueue />}
    </div>
  );
}

function useCategories() {
  const t = useT();
  const [cats, setCats] = useState(null);
  const [meta, setMeta] = useState({ moderator: false, openReports: 0 }); // told to moderators only
  const [error, setError] = useState("");
  useEffect(() => {
    fetchForumHome().then(
      (r) => {
        setCats(r.categories);
        setMeta({ moderator: !!r.moderator, openReports: r.openReports || 0 });
      },
      (e) => setError(e.message),
    );
  }, []);
  const shown = useMemo(
    () =>
      cats &&
      cats.map((c) => ({ ...c, name: tOr(t, `forum.cat.${c.id}.name`, c.name), description: tOr(t, `forum.cat.${c.id}.desc`, c.description) })),
    [cats, t],
  );
  return { cats: shown, meta, error };
}

function Crumbs({ items }) {
  const t = useT();
  return (
  <nav className="fcrumbs" aria-label={t("forumPage.3")}>
    <a href={forumHash.home()}>{t("menu.forum")}</a>
    {items.map((it) => (
      <span key={it.label}>
        <span aria-hidden="true"> › </span>
        {it.href ? <a href={it.href}>{it.label}</a> : <span>{it.label}</span>}
      </span>
    ))}
  </nav>
  );
}

function Home() {
  const t = useT();
  const { cats, meta, error } = useCategories();
  const [latest, setLatest] = useState(null);
  useEffect(() => {
    fetchForumTopics("", 1).then((r) => setLatest(r.topics.slice(0, 8)), () => {});
  }, []);
  return (
    <>
      {error && <div className="banner banner--error">{error}</div>}
      <div className="fhome__actions fhome__actions--mod">
        {meta.moderator && (
          <a className="btn" href={forumHash.reports()}>
            <Icon name="alert" size={16} /> {meta.openReports > 0 ? t("safety.modLinkCount", { n: meta.openReports }) : t("safety.modLink")}
          </a>
        )}
        <a className="btn btn--primary" href={forumHash.newTopic()}>
          <Icon name="forum" size={16} /> {t("forumPage.4")}
        </a>
      </div>
      <section className="card csection">
        <header className="csection__head">
          <h2>{t("forumPage.5")}</h2>
        </header>
        <ul className="fcats">
          {cats?.map((c) => (
            <li key={c.id}>
              <a className="fcat" href={forumHash.category(c.id)}>
                <Icon name="forum" size={22} />
                <span className="fcat__body">
                  <b>{c.name}</b>
                  <span>{c.description}</span>
                </span>
                <span className="fcat__nums">
                  <b>{c.topics}</b> {t("forumPage.6")} <b>{c.posts}</b> {t("forumPage.7")}
                  {c.lastPost > 0 && <span className="fcat__last">{t("forumPage.8")} {ago(c.lastPost, Date.now())}</span>}
                </span>
              </a>
            </li>
          ))}
        </ul>
      </section>
      {latest && latest.length > 0 && (
        <section className="card csection">
          <header className="csection__head">
            <h2>{t("forumPage.9")}</h2>
          </header>
          <TopicTable topics={latest} showCategory cats={cats} />
        </section>
      )}
    </>
  );
}

function TopicTable({ topics, showCategory, cats }) {
  const t = useT();
  const name = (id) => cats?.find((c) => c.id === id)?.name || id;
  const now = Date.now();
  return (
    <ul className="ftopics">
      {topics.map((tp) => (
        <li key={tp.id} className="ftopic">
          <a className="ftopic__title" href={forumHash.topic(tp.id)}>
            {tp.pinned && <Icon name="flag" size={14} />} {tp.locked && <Icon name="lock" size={14} />} {tp.title}
          </a>
          <span className="ftopic__meta">
            {tp.author.id ? <a href={`#user/${tp.author.id}`}>{tp.author.name}</a> : <span>{tp.author.name}</span>}
            {showCategory && (
              <>
                {" · "}
                <a href={forumHash.category(tp.category)}>{name(tp.category)}</a>
              </>
            )}
          </span>
          <span className="ftopic__nums">
            <b>{Math.max(0, tp.posts - 1)}</b> {t("forumPage.10")}
            <span>{ago(tp.lastPost, now)}</span>
          </span>
        </li>
      ))}
    </ul>
  );
}

function Pager({ page, pages, hashOf }) {
  const t = useT();
  if (pages <= 1) return null;
  const nums = [];
  for (let p = 1; p <= pages; p++) if (p === 1 || p === pages || Math.abs(p - page) <= 2) nums.push(p);
  return (
    <nav className="fpager" aria-label={t("forumPage.11")}>
      {page > 1 && (
        <a className="btn btn--ghost" href={hashOf(page - 1)} rel="prev">
          {t("forumPage.12")}
        </a>
      )}
      {nums.map((p, i) => (
        <span key={p}>
          {i > 0 && p - nums[i - 1] > 1 && <span aria-hidden="true">…</span>}
          <a className={`btn${p === page ? " btn--primary" : " btn--ghost"}`} href={hashOf(p)} aria-current={p === page ? "page" : undefined}>
            {p}
          </a>
        </span>
      ))}
      {page < pages && (
        <a className="btn btn--ghost" href={hashOf(page + 1)} rel="next">
          {t("forumPage.13")}
        </a>
      )}
    </nav>
  );
}

function Category({ category, page }) {
  const t = useT();
  const { cats, error: catsError } = useCategories();
  const [data, setData] = useState(null);
  const [error, setError] = useState("");
  useEffect(() => {
    fetchForumTopics(category, page).then(setData, (e) => setError(e.message));
  }, [category, page]);
  const cat = cats?.find((c) => c.id === category);
  return (
    <>
      <Crumbs items={[{ label: cat?.name || category }]} />
      {(error || catsError) && <div className="banner banner--error">{error || catsError}</div>}
      <div className="fhome__actions">
        <a className="btn btn--primary" href={forumHash.newTopic(category)}>
          <Icon name="forum" size={16} /> {t("forumPage.4")}
        </a>
      </div>
      <section className="card csection">
        <header className="csection__head">
          <h2>{cat?.name || t("forumPage.14")}</h2>
          {cat && <span className="cnote">{cat.description}</span>}
        </header>
        {data && data.topics.length === 0 && <p className="cnote">{t("forumPage.15")}</p>}
        {data && <TopicTable topics={data.topics} />}
      </section>
      {data && <Pager page={page} pages={data.pages} hashOf={(p) => forumHash.category(category, p)} />}
    </>
  );
}

function Topic({ id, page }) {
  const t = useT();
  const { user } = useAuth();
  const { cats } = useCategories();
  const [data, setData] = useState(null);
  const [error, setError] = useState("");
  const load = useCallback(() => {
    fetchForumTopic(id, page).then(
      (d) => {
        setData(d);
        setError("");
      },
      (e) => setError(e.status === 404 ? t("forumPage.16") : e.message),
    );
  }, [id, page]);
  useEffect(load, [load]);

  if (error && !data) {
    return (
      <>
        <Crumbs items={[]} />
        <div className="banner banner--error">{error}</div>
      </>
    );
  }
  if (!data) return <p className="cnote">{t("leadersPage.6")}</p>;
  const topic = data.topic;
  const cat = cats?.find((c) => c.id === topic.category);

  async function run(fn, ...args) {
    setError("");
    try {
      await fn(...args);
      load();
    } catch (e) {
      setError(e.message);
    }
  }
  async function removeTopic() {
    if (!window.confirm(t("forumPage.18"))) return;
    try {
      await deleteForumTopic(id);
      window.location.hash = forumHash.category(topic.category);
    } catch (e) {
      setError(e.message);
    }
  }

  return (
    <>
      <Crumbs items={[{ label: cat?.name || topic.category, href: forumHash.category(topic.category) }, { label: topic.title }]} />
      {error && <div className="banner banner--error">{error}</div>}
      <header className="ftopichead">
        <h2>
          {topic.pinned && <span className="ftag">{t("forumPage.19")}</span>}
          {topic.locked && <span className="ftag">{t("forumPage.20")}</span>}
          {topic.title}
        </h2>
        <span className="ftopichead__tools">
          {data.moderator && (
            <>
              <button type="button" className="btn btn--ghost" onClick={() => run(pinForumTopic, id, !topic.pinned)}>
                {topic.pinned ? t("forumPage.21") : t("forumPage.22")}
              </button>
              <button type="button" className="btn btn--ghost" onClick={() => run(lockForumTopic, id, !topic.locked)}>
                {topic.locked ? t("corrTab.15") : t("gamesReview.close")}
              </button>
            </>
          )}
          {user && topic.author.id && topic.author.id !== user.id && <ReportButton kind="forum_topic" targetId={topic.id} />}
          {data.canDeleteTopic && (
            <button type="button" className="btn btn--danger-ghost" onClick={removeTopic}>
              <Icon name="trash" size={16} /> {t("forumPage.25")}
            </button>
          )}
        </span>
      </header>

      <ol className="fposts">
        {data.posts.map((p) => (
          <Post key={p.id} post={p} onChanged={load} onError={setError} />
        ))}
      </ol>
      <Pager page={page} pages={data.pages} hashOf={(p) => forumHash.topic(id, p)} />

      {data.canPost ? (
        <Compose
          label={t("forumPage.26")}
          onSend={async (body) => {
            const r = await replyForumTopic(id, body);
            // go to the last page, where the new message is
            const last = Math.ceil((topic.posts + 1) / 20);
            if (last !== page) window.location.hash = forumHash.topic(id, last);
            else load();
            return r;
          }}
        />
      ) : topic.locked ? (
        <p className="cnote">{t("forumPage.27")}</p>
      ) : (
        <SignInNote user={user} />
      )}
    </>
  );
}

function SignInNote({ user }) {
  const t = useT();
  const { guest } = useAuth();
  const [error, setError] = useState("");
  if (user) return null;
  return (
    <section className="card csection">
      <p className="cnote">{t("forumPage.28")}</p>
      {error && <p className="cnote cnote--error">{error}</p>}
      <button type="button" className="btn btn--primary" onClick={() => guest().catch((e) => setError(e.message))}>
        {t("auth.asGuest")}
      </button>
    </section>
  );
}

function Post({ post, onChanged, onError }) {
  const t = useT();
  const { user } = useAuth();
  const [editing, setEditing] = useState(false);
  const [confirming, setConfirming] = useState(false);
  const [revealed, setRevealed] = useState(false); // a message of a blocked author, shown on request
  const a = post.author;
  const canReport = !!user && !post.deleted && !!a.id && a.id !== user.id;
  return (
    <li className="fpost" id={`post-${post.id}`}>
      <aside className="fpost__who">
        {a.id ? (
          <a href={`#user/${a.id}`} className="fpost__link">
            <Avatar user={a} size={40} />
            <b>{a.name}</b>
          </a>
        ) : (
          <span className="fpost__link">
            <Avatar user={a} size={40} />
            <b>{a.name}</b>
            <span className="cnote">{t("forumPage.30")}</span>
          </span>
        )}
      </aside>
      <div className="fpost__main">
        <div className="fpost__meta">
          <span>№{post.n}</span>
          <time>{new Date(post.created).toLocaleString(locale(), { day: "numeric", month: "long", hour: "2-digit", minute: "2-digit" })}</time>
          {post.edited > 0 && <span>{t("forumPage.31")}</span>}
        </div>
        {post.deleted ? (
          <p className="fpost__deleted">{t("forumPage.32")}</p>
        ) : !postShown(post, revealed) ? (
          <p className="fpost__hidden">
            {t("safety.hiddenPost")}
            <button type="button" className="btn btn--ghost" onClick={() => setRevealed(true)}>
              {t("safety.show")}
            </button>
          </p>
        ) : editing ? (
          <Compose
            label={t("common.save")}
            initial={post.body}
            onCancel={() => setEditing(false)}
            onSend={async (body) => {
              await editForumPost(post.id, body);
              setEditing(false);
              onChanged();
            }}
          />
        ) : (
          /* plain text: React escapes it, the line breaks are kept by CSS */
          <p className="fpost__body">{post.body}</p>
        )}
        {!post.deleted && !editing && (post.canEdit || post.canDelete || canReport || (post.blockedByMe && revealed)) && (
          <div className="fpost__tools">
            {post.blockedByMe && revealed && (
              <button type="button" className="btn btn--ghost" onClick={() => setRevealed(false)}>
                {t("safety.hide")}
              </button>
            )}
            {canReport && <ReportButton kind="forum_post" targetId={post.id} />}
            {post.canEdit && (
              <button type="button" className="btn btn--ghost" onClick={() => setEditing(true)}>
                {t("forumPage.34")}
              </button>
            )}
            {post.canDelete &&
              (confirming ? (
                <>
                  <button
                    type="button"
                    className="btn btn--danger"
                    onClick={async () => {
                      try {
                        await deleteForumPost(post.id);
                        onChanged();
                      } catch (e) {
                        onError(e.message);
                      }
                      setConfirming(false);
                    }}
                  >
                    {t("forumPage.35")}
                  </button>
                  <button type="button" className="btn btn--ghost" onClick={() => setConfirming(false)}>
                    {t("gameRoom.67")}
                  </button>
                </>
              ) : (
                <button type="button" className="btn btn--danger-ghost" onClick={() => setConfirming(true)}>
                  {t("forumPage.37")}
                </button>
              ))}
          </div>
        )}
      </div>
    </li>
  );
}

// A box for a message; onSend(body) rejects with the reason to show.
function Compose({ label, initial = "", onSend, onCancel }) {
  const t = useT();
  const [text, setText] = useState(initial);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  async function go() {
    if (!text.trim() || busy) return;
    setBusy(true);
    setError("");
    try {
      await onSend(text);
      if (!initial) setText("");
    } catch (e) {
      setError(e.message);
    } finally {
      setBusy(false);
    }
  }
  return (
    <section className={initial ? "fcompose" : "card csection fcompose"}>
      <textarea
        className="field fcompose__input"
        value={text}
        maxLength={MAX_BODY}
        rows={initial ? 5 : 4}
        aria-label={label}
        placeholder={t("forumPage.38")}
        onChange={(e) => setText(e.target.value)}
      />
      {error && <p className="cnote cnote--error">{error}</p>}
      <div className="fcompose__bar">
        <span className="cnote">{text.length} / {MAX_BODY}</span>
        <span>
          {onCancel && (
            <button type="button" className="btn btn--ghost" onClick={onCancel}>
              {t("common.cancel")}
            </button>
          )}{" "}
          <button type="button" className="btn btn--primary" disabled={busy || !text.trim()} onClick={go}>
            {busy ? t("forumPage.40") : label}
          </button>
        </span>
      </div>
    </section>
  );
}

function NewTopic({ category: initial }) {
  const t = useT();
  const { user, guest } = useAuth();
  const { cats } = useCategories();
  const [category, setCategory] = useState(initial || "");
  const [title, setTitle] = useState("");
  const [body, setBody] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");

  async function create() {
    setBusy(true);
    setError("");
    try {
      if (!user) await guest();
      const r = await createForumTopic(category, title, body);
      window.location.hash = forumHash.topic(r.id);
    } catch (e) {
      setError(e.message);
      setBusy(false);
    }
  }
  const ready = category && title.trim().length >= 3 && body.trim().length >= 2;
  return (
    <>
      <Crumbs items={[{ label: t("forumPage.4") }]} />
      <section className="card csection fnew">
        <header className="csection__head">
          <h2>{t("forumPage.4")}</h2>
        </header>
        <label className="fnew__label" htmlFor="forum-cat">
          {t("forumPage.14")}
        </label>
        <select id="forum-cat" className="field" value={category} onChange={(e) => setCategory(e.target.value)}>
          <option value="">{t("forumPage.41")}</option>
          {cats?.map((c) => (
            <option key={c.id} value={c.id}>
              {c.name}
            </option>
          ))}
        </select>
        <label className="fnew__label" htmlFor="forum-title">
          {t("forumPage.42")}
        </label>
        <input id="forum-title" className="field" value={title} maxLength={120} onChange={(e) => setTitle(e.target.value)} placeholder={t("forumPage.43")} />
        <label className="fnew__label" htmlFor="forum-body">
          {t("chat.6")}
        </label>
        <textarea id="forum-body" className="field fcompose__input" rows={8} maxLength={MAX_BODY} value={body} onChange={(e) => setBody(e.target.value)} />
        {error && <div className="banner banner--error">{error}</div>}
        {!user && <p className="cnote">{t("forumPage.45")}</p>}
        <div className="fcompose__bar">
          <span className="cnote">{body.length} / {MAX_BODY}</span>
          <button type="button" className="btn btn--primary" disabled={busy || !ready} onClick={create}>
            {busy ? t("lobby.24") : t("forumPage.47")}
          </button>
        </div>
      </section>
    </>
  );
}
