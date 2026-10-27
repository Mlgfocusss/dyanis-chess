import { serverMsg, tr } from "../i18n/index.js";
// Client of the community API (backend/internal/community) and the forum API (backend/internal/forum).

async function readJSON(res) {
  let body = null;
  try {
    body = await res.json();
  } catch {
    // not JSON: handled below
  }
  if (!res.ok) {
    const e = new Error(serverMsg(body?.error) || tr("common.serverReplied", { status: res.status }));
    e.status = res.status;
    throw e;
  }
  return body;
}

const json = (method, body) => ({
  method,
  headers: { "Content-Type": "application/json" },
  body: body === undefined ? undefined : JSON.stringify(body),
});

const call = (path, method = "GET", body) => fetch(path, method === "GET" ? undefined : json(method, body ?? {})).then(readJSON);
const enc = encodeURIComponent;

// ---------------------------------------------------------------- community

// { friendRequests, messages, challenges, yourTurn, accepted: [{id, gameId}], vacation (unix ms or 0) }
export const fetchSummary = () => call("/api/community/summary");

export const searchPeople = (q) => call(`/api/community/search?q=${enc(q)}`).then((r) => r.items);

// { friends: [...], incoming: [...], outgoing: [...] }
export const fetchFriends = () => call("/api/community/friends");
// { relation: "self" | "friend" | "incoming" | "outgoing" | "none", muted?, online? }
export const fetchRelation = (id) => call(`/api/community/friends/${enc(id)}`);
export const requestFriend = (to) => call("/api/community/friends", "POST", { to });
export const acceptFriend = (id) => call(`/api/community/friends/${enc(id)}/accept`, "POST");
export const declineFriend = (id) => call(`/api/community/friends/${enc(id)}/decline`, "POST");
export const removeFriend = (id) => call(`/api/community/friends/${enc(id)}`, "DELETE");
export const muteFriend = (id, muted) => call(`/api/community/friends/${enc(id)}/mute`, "PUT", { muted });

export const fetchConversations = () => call("/api/community/conversations").then((r) => r.items);
// { friend, messages: [{id, mine, text, at, read}] }; with `after` only the newer ones
export const fetchMessages = (id, after = 0) => call(`/api/community/messages/${enc(id)}${after ? `?after=${after}` : ""}`);
export const sendMessage = (id, text) => call(`/api/community/messages/${enc(id)}`, "POST", { text });

// { incoming: [...], outgoing: [...] }
export const fetchChallenges = () => call("/api/community/challenges");
// body: { to, color, baseMs, incMs, days, rated }
export const sendChallenge = (body) => call("/api/community/challenges", "POST", body);
export const acceptChallenge = (id) => call(`/api/community/challenges/${enc(id)}/accept`, "POST").then((r) => r.gameId);
export const declineChallenge = (id) => call(`/api/community/challenges/${enc(id)}/decline`, "POST");
export const cancelChallenge = (id) => call(`/api/community/challenges/${enc(id)}`, "DELETE");

// { items: [{id, name, avatarUrl, country, kind, at}], max }
export const fetchBlocks = () => call("/api/community/blocks");
export const blockPlayer = (id) => call(`/api/community/blocks/${enc(id)}`, "PUT");
export const unblockPlayer = (id) => call(`/api/community/blocks/${enc(id)}`, "DELETE");

// A complaint. body: { kind: "message" | "forum_post" | "forum_topic" | "user", targetId (a string), reason, note }
export const sendReport = (body) => call("/api/reports", "POST", body);
// The moderators' queue (403 for everybody else): items of { id, kind, reason, note, snapshot, reporter, target, targetId, topicId, status, action, created, closedAt }
export const fetchReports = (status = "open") => call(`/api/reports?status=${status}`).then((r) => r.items);
export const closeReport = (id, action) => call(`/api/reports/${id}/close`, "POST", { action });

export const fetchCorrespondence =() => call("/api/community/correspondence").then((r) => r.items);
export const setVacation = (days) => call("/api/community/vacation", "PUT", { days });

// ------------------------------------------------------------------- forum

export const fetchForumCategories = () => call("/api/forum/categories").then((r) => r.categories);
// { categories, moderator?, openReports? }: the last two only for a moderator
export const fetchForumHome = () => call("/api/forum/categories");
export const fetchForumTopics = (category, page = 1) => call(`/api/forum/topics?${category ? `category=${enc(category)}&` : ""}page=${page}`);
export const fetchForumTopic = (id, page = 1) => call(`/api/forum/topics/${enc(id)}?page=${page}`);
export const createForumTopic = (category, title, body) => call("/api/forum/topics", "POST", { category, title, body });
export const replyForumTopic = (id, body) => call(`/api/forum/topics/${enc(id)}/posts`, "POST", { body });
export const editForumPost = (id, body) => call(`/api/forum/posts/${enc(id)}`, "PUT", { body });
export const deleteForumPost = (id) => call(`/api/forum/posts/${enc(id)}`, "DELETE");
export const deleteForumTopic = (id) => call(`/api/forum/topics/${enc(id)}`, "DELETE");
export const pinForumTopic = (id, on) => call(`/api/forum/topics/${enc(id)}/pin`, "POST", { on });
export const lockForumTopic = (id, on) => call(`/api/forum/topics/${enc(id)}/lock`, "POST", { on });
