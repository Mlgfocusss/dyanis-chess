import { locale, tr } from "../i18n/index.js";
// Text and small decisions of the community pages. Plain JavaScript with no imports, covered by `npm test`.

// 1 день, 2 дня, 5 дней, 11 дней, 21 день.
export function plural(n, one, few, many) {
  const m10 = n % 10;
  const m100 = n % 100;
  if (m10 === 1 && m100 !== 11) return one;
  if (m10 >= 2 && m10 <= 4 && (m100 < 12 || m100 > 14)) return few;
  return many;
}

export const daysText = (n) => tr("common.days", { n });

// The days per move a correspondence game may have (the server's list, backend/internal/play).
export const CORR_DAYS = [1, 2, 3, 5, 7, 10, 14];

// How much time is left until a moment: "2 д 3 ч", "5 ч 10 мин", "12 мин", "меньше минуты", "время вышло".
export function timeLeft(dueMs, nowMs) {
  const left = dueMs - nowMs;
  if (left <= 0) return tr("commLogic.4");
  if (left < 60000) return tr("commLogic.5");
  const min = Math.ceil(left / 60000); // 4 д 23 ч 59 мин reads as 5 д
  const d = Math.floor(min / 1440);
  const h = Math.floor((min % 1440) / 60);
  const m = min % 60;
  if (d > 0) return h ? tr("commLogic.6", { d, h }) : tr("commLogic.7", { d });
  if (h > 0) return m ? tr("commLogic.8", { h, m }) : tr("commLogic.9", { h });
  if (m > 0) return tr("commLogic.10", { m });
  return tr("commLogic.5");
}

// Is the deadline close (under a quarter of the time, or under six hours)?
export const urgent = (dueMs, nowMs, days) => dueMs - nowMs < Math.min(6 * 3600000, (days || 1) * 86400000 * 0.25);

// "только что", "5 мин назад", "вчера", "3 дня назад", or the date.
export function ago(thenMs, nowMs) {
  const s = Math.max(0, Math.round((nowMs - thenMs) / 1000));
  if (s < 45) return tr("commLogic.11");
  const min = Math.round(s / 60);
  if (min < 60) return tr("commLogic.12", { min });
  const h = Math.round(min / 60);
  if (h < 24) return tr("community.hoursAgo", { n: h });
  const d = Math.round(h / 24);
  if (d === 1) return tr("commLogic.14");
  if (d < 7) return tr("community.daysAgo", { n: d });
  return new Date(thenMs).toLocaleDateString(locale(), { day: "numeric", month: "short" });
}

// "Блиц 5 + 2", "Рапид 10 мин", "Заочная: 3 дня на ход"; the kind of the clock is by the same
// estimate as the server's (the base time plus 40 moves of increment): under 3 minutes bullet, under 8 blitz.
export function gameText(c) {
  if (c.days) return tr("commLogic.16", { p: daysText(c.days) });
  const base = c.baseMs / 60000;
  const label = Number.isInteger(base) ? `${base}` : tr("clock.3", { p: c.baseMs / 1000 });
  const inc = c.incMs ? ` + ${c.incMs / 1000}` : Number.isInteger(base) ? tr("commLogic.18") : "";
  const total = c.baseMs / 1000 + (40 * c.incMs) / 1000;
  const kind = total < 180 ? tr("rating.cat.bullet") : total < 480 ? tr("rating.cat.blitz") : tr("rating.cat.rapid");
  return `${kind} ${label}${inc}`;
}

export const COLOR_TEXT = { white: "commLogic.22", black: "commLogic.23", random: "commLogic.24" };

// What the sender's colour means for the one who answers: the sender plays white, so the receiver plays black.
export function yourColorText(senderColor) {
  if (senderColor === "white") return tr("commLogic.25");
  if (senderColor === "black") return tr("commLogic.26");
  return tr("commLogic.24");
}

// The number on the menu badge: everything that waits for the person.
export function badgeCount(summary) {
  if (!summary) return 0;
  return (summary.friendRequests || 0) + (summary.messages || 0) + (summary.challenges || 0) + (summary.yourTurn || 0);
}

// Which of the incoming challenges to tell the person about: the ones not shown yet, from everybody or only
// from friends (settings → Вызовы → Уведомления). `mode` is "all", "friends" or "none".
export function toNotify(incoming, shown, mode) {
  if (mode === "none") return [];
  return incoming.filter((c) => !shown.has(c.id) && (mode !== "friends" || c.friend));
}

// The address parts of the community page: #community/<tab>[/<friend id>].
export const COMMUNITY_TABS = ["friends", "messages", "challenges", "correspondence", "blocked"];
export function parseCommunityHash(hash) {
  const parts = hash.replace(/^#/, "").split("/");
  if (parts[0] !== "community") return { tab: "friends", id: "" };
  const tab = COMMUNITY_TABS.includes(parts[1]) ? parts[1] : "friends";
  return { tab, id: tab === "messages" ? parts[2] || "" : "" };
}
export const communityHash = (tab, id = "") => `#community/${tab}${id ? `/${id}` : ""}`;

// The forum addresses: #forum, #forum/c/<category>[/<page>], #forum/t/<id>[/<page>], #forum/new[/<category>].
export function parseForumHash(hash) {
  const p = hash.replace(/^#/, "").split("/");
  const page = (s) => Math.max(1, parseInt(s, 10) || 1);
  if (p[0] !== "forum") return { view: "home" };
  if (p[1] === "c" && p[2]) return { view: "category", category: p[2], page: page(p[3]) };
  if (p[1] === "t" && /^\d+$/.test(p[2] || "")) return { view: "topic", id: p[2], page: page(p[3]) };
  if (p[1] === "new") return { view: "new", category: p[2] || "" };
  if (p[1] === "reports") return { view: "reports" }; // the moderators' queue of complaints
  return { view: "home" };
}
export const forumHash = {
  reports: () => "#forum/reports",
  home: () => "#forum",
  category: (c, page = 1) => `#forum/c/${c}${page > 1 ? `/${page}` : ""}`,
  topic: (id, page = 1) => `#forum/t/${id}${page > 1 ? `/${page}` : ""}`,
  newTopic: (c = "") => `#forum/new${c ? `/${c}` : ""}`,
};

// ----------------------------------------------------------- blocks and complaints

// Why a person complains (the server's list); the labels are in the dictionaries as "safety.reason.<id>".
export const REPORT_REASONS = ["spam", "abuse", "cheating", "other"];
export const REPORT_NOTE_MAX = 300;

// What the moderator may do when closing a complaint of a kind (the server refuses the rest).
export function reportActions(kind) {
  return kind === "forum_post" || kind === "forum_topic" ? ["none", "delete", "lock"] : ["none"];
}

// The page a complaint is about, for the moderator's link: the topic of a forum message, the player's page; "" for a private message.
export function reportLink(r) {
  if ((r.kind === "forum_post" || r.kind === "forum_topic") && r.topicId) return forumHash.topic(r.topicId);
  if (r.kind === "user" && r.target?.id) return `#user/${r.target.id}`;
  return "";
}

// Is the text of a forum message shown? Messages of a player the reader has blocked stay behind "Показать" until it is pressed.
export const postShown = (post, revealed) => !post.blockedByMe || post.deleted || !!revealed;

// The id for the complaint request: the server takes it as a string.
export const targetOf = (id) => String(id);
