import test from "node:test";
import assert from "node:assert/strict";
import {
  CORR_DAYS,
  ago,
  badgeCount,
  communityHash,
  daysText,
  forumHash,
  gameText,
  parseCommunityHash,
  parseForumHash,
  plural,
  timeLeft,
  toNotify,
  urgent,
  yourColorText,
} from "./logic.js";

test("plural follows the Russian rules", () => {
  const f = (n) => plural(n, "день", "дня", "дней");
  assert.deepEqual([1, 2, 4, 5, 11, 12, 14, 21, 22, 25, 101, 111].map(f), [
    "день", "дня", "дня", "дней", "дней", "дней", "дней", "день", "дня", "дней", "день", "дней",
  ]);
  assert.equal(daysText(3), "3 дня");
});

test("the days per move are those of the server", () => {
  assert.deepEqual(CORR_DAYS, [1, 2, 3, 5, 7, 10, 14]);
});

test("timeLeft reads like a person would say it", () => {
  const now = 1_000_000_000;
  assert.equal(timeLeft(now - 1, now), "время вышло");
  assert.equal(timeLeft(now + 30_000, now), "меньше минуты");
  assert.equal(timeLeft(now + 12 * 60_000, now), "12 мин");
  assert.equal(timeLeft(now + 5 * 3600_000 + 10 * 60_000, now), "5 ч 10 мин");
  assert.equal(timeLeft(now + 3 * 3600_000, now), "3 ч");
  assert.equal(timeLeft(now + (2 * 24 + 3) * 3600_000, now), "2 д 3 ч");
  assert.equal(timeLeft(now + 24 * 3600_000, now), "1 д");
  assert.equal(timeLeft(now + 5 * 24 * 3600_000 - 20_000, now), "5 д"); // just after a move was made
  assert.equal(timeLeft(now + 90_000, now), "2 мин");
});

test("urgent: a quarter of the time or six hours, whichever is less", () => {
  const now = 0;
  assert.equal(urgent(5 * 3600_000, now, 3), true);
  assert.equal(urgent(7 * 3600_000, now, 3), false);
  assert.equal(urgent(5 * 3600_000, now, 1), true); // one day: six hours is a quarter
  assert.equal(urgent(7 * 3600_000, now, 1), false);
});

test("ago", () => {
  const now = Date.UTC(2026, 9, 22, 12, 0, 0);
  assert.equal(ago(now - 10_000, now), "только что");
  assert.equal(ago(now - 5 * 60_000, now), "5 мин назад");
  assert.equal(ago(now - 3 * 3600_000, now), "3 часа назад");
  assert.equal(ago(now - 26 * 3600_000, now), "вчера");
  assert.equal(ago(now - 3 * 86400_000, now), "3 дня назад");
});

test("gameText", () => {
  assert.equal(gameText({ baseMs: 300000, incMs: 2000 }), "Блиц 5 + 2");
  assert.equal(gameText({ baseMs: 60000, incMs: 0 }), "Пуля 1 мин");
  assert.equal(gameText({ baseMs: 600000, incMs: 0 }), "Рапид 10 мин");
  assert.equal(gameText({ baseMs: 0, incMs: 0, days: 3 }), "Заочная: 3 дня на ход");
  assert.equal(gameText({ baseMs: 30000, incMs: 0 }), "Пуля 30 с");
});

test("yourColorText is the other side of the sender's choice", () => {
  assert.equal(yourColorText("white"), "вы играете чёрными");
  assert.equal(yourColorText("black"), "вы играете белыми");
  assert.equal(yourColorText("random"), "цвет по жребию");
});

test("badgeCount adds up what waits", () => {
  assert.equal(badgeCount(null), 0);
  assert.equal(badgeCount({ friendRequests: 1, messages: 2, challenges: 3, yourTurn: 4 }), 10);
  assert.equal(badgeCount({ messages: 2 }), 2);
});

test("toNotify respects the setting and does not repeat", () => {
  const list = [
    { id: "a", friend: true },
    { id: "b", friend: false },
    { id: "c", friend: true },
  ];
  const shown = new Set(["c"]);
  assert.deepEqual(toNotify(list, shown, "all").map((c) => c.id), ["a", "b"]);
  assert.deepEqual(toNotify(list, shown, "friends").map((c) => c.id), ["a"]);
  assert.deepEqual(toNotify(list, shown, "none"), []);
});

test("community addresses", () => {
  assert.deepEqual(parseCommunityHash("#community"), { tab: "friends", id: "" });
  assert.deepEqual(parseCommunityHash("#community/challenges"), { tab: "challenges", id: "" });
  assert.deepEqual(parseCommunityHash("#community/messages/abc"), { tab: "messages", id: "abc" });
  assert.deepEqual(parseCommunityHash("#community/friends/abc"), { tab: "friends", id: "" });
  assert.deepEqual(parseCommunityHash("#community/nonsense"), { tab: "friends", id: "" });
  assert.equal(communityHash("messages", "abc"), "#community/messages/abc");
  assert.equal(communityHash("friends"), "#community/friends");
});

test("forum addresses", () => {
  assert.deepEqual(parseForumHash("#forum"), { view: "home" });
  assert.deepEqual(parseForumHash("#forum/c/openings"), { view: "category", category: "openings", page: 1 });
  assert.deepEqual(parseForumHash("#forum/c/openings/3"), { view: "category", category: "openings", page: 3 });
  assert.deepEqual(parseForumHash("#forum/t/42/2"), { view: "topic", id: "42", page: 2 });
  assert.deepEqual(parseForumHash("#forum/t/xx"), { view: "home" });
  assert.deepEqual(parseForumHash("#forum/new/games"), { view: "new", category: "games" });
  assert.equal(forumHash.topic(7), "#forum/t/7");
  assert.equal(forumHash.topic(7, 2), "#forum/t/7/2");
  assert.equal(forumHash.category("games", 1), "#forum/c/games");
});
