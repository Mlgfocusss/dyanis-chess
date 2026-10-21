import test from "node:test";
import assert from "node:assert/strict";
import {
  CATEGORIES,
  countryCell,
  countryFilterLine,
  countryFromHash,
  emptyCountryLine,
  isCountryCode,
  botsLine,
  categoryFromHash,
  categoryName,
  emptyLine,
  isMine,
  joinedText,
  kindLabel,
  leadersHash,
  medal,
  placeLine,
  placeText,
  playHash,
  profileRatings,
  rankRows,
  recordText,
  userFromHash,
  userHash,
} from "./leaders.js";

test("the tabs are the four categories, named as everywhere else", () => {
  assert.deepEqual(CATEGORIES.map(categoryName), ["Пуля", "Блиц", "Рапид", "Дуэль"]);
  assert.equal(categoryName("chess"), "Рейтинг");
});

test("addresses of the table and of a player", () => {
  assert.equal(categoryFromHash("#leaders/rapid"), "rapid");
  assert.equal(categoryFromHash("#leaders"), "blitz");
  assert.equal(categoryFromHash("#leaders/chess"), "blitz");
  assert.equal(categoryFromHash("#user/abc"), "blitz");
  assert.equal(leadersHash("duel"), "#leaders/duel");
  assert.equal(leadersHash(""), "#leaders");
  assert.equal(userHash("abc123"), "#user/abc123");
  assert.equal(userHash("abc123", "bullet"), "#user/abc123/bullet");
  assert.equal(userHash("a b", "x"), "#user/a%20b");
  assert.deepEqual(userFromHash("#user/abc123"), { id: "abc123", from: "", country: "" });
  assert.deepEqual(userFromHash("#user/abc123/rapid"), { id: "abc123", from: "rapid", country: "" });
  assert.deepEqual(userFromHash("#user/a%20b/nope"), { id: "a b", from: "", country: "" });
  assert.deepEqual(userFromHash("#user"), { id: "", from: "", country: "" });
  assert.deepEqual(userFromHash("#leaders/blitz"), { id: "", from: "", country: "" });
  assert.deepEqual(userFromHash("#user/%E0%A4%A"), { id: "%E0%A4%A", from: "", country: "" });
});

test("rows are ranked by rating, then games, then name, and numbered from 1", () => {
  const rows = rankRows([
    { id: "a", name: "Борис", rating: 1500, games: 10 },
    { id: "b", name: "Анна", rating: 1700, games: 6 },
    { id: "c", name: "Вера", rating: 1500, games: 30 },
    { id: "d", name: "Алла", rating: 1500, games: 10 },
  ]);
  assert.deepEqual(rows.map((r) => [r.id, r.place]), [["b", 1], ["c", 2], ["d", 3], ["a", 4]]);
  assert.deepEqual(rankRows(null), []);
  const input = [{ id: "x", name: "X", rating: 1, games: 5 }];
  rankRows(input);
  assert.equal(input[0].place, undefined, "the input is not changed");
});

test("the row of the signed-in person is theirs", () => {
  assert.equal(isMine({ id: "a" }, { id: "a" }), true);
  assert.equal(isMine({ id: "a" }, { id: "b" }), false);
  assert.equal(isMine({ id: "a" }, null), false);
});

test("places are written for the table and for a profile", () => {
  assert.equal(placeText(7), "7");
  assert.equal(placeText(0), "—");
  assert.equal(placeLine(3), "3-е место");
  assert.equal(placeLine(0), "");
  assert.deepEqual([1, 2, 3, 4].map(medal), ["gold", "silver", "bronze", ""]);
});

test("who a person is", () => {
  assert.equal(kindLabel("guest"), "Гость");
  assert.equal(kindLabel("google"), "Google");
  assert.match(joinedText("2026-10-12T10:00:00Z"), /2026/);
  assert.equal(joinedText(""), "");
  assert.equal(joinedText("0001-01-01T00:00:00Z"), "");
  assert.equal(recordText({ wins: 12, draws: 3, losses: 5 }), "12 / 3 / 5");
  assert.equal(recordText({}), "0 / 0 / 0");
});

test("a profile lists every category in the order of the tabs", () => {
  const list = profileRatings({
    ratings: [
      { category: "duel", rating: 1310, provisional: true, games: 2, wins: 1, draws: 0, losses: 1 },
      { category: "blitz", rating: 1650, provisional: false, games: 40, wins: 20, draws: 5, losses: 15, place: 4 },
    ],
  });
  assert.deepEqual(list.map((r) => r.category), ["bullet", "blitz", "rapid", "duel"]);
  assert.equal(list[0].value, "—");
  assert.equal(list[0].record, "ещё не играл");
  assert.equal(list[1].value, "1650");
  assert.equal(list[1].record, "партий 40: 20 / 5 / 15");
  assert.equal(list[1].placeLine, "4-е место");
  assert.equal(list[3].value, "1310?");
  assert.equal(list[3].place, 0);
  assert.equal(profileRatings(null).length, 4);
});

test("words for bots and for an empty table", () => {
  assert.equal(botsLine({ points: 30, beaten: 3, total: 10 }), "побеждено 3 из 10");
  assert.equal(botsLine({ points: 0, total: 10 }), "побеждено 0 из 10");
  assert.equal(botsLine(null), "");
  assert.match(emptyLine("blitz"), /Пока никто не набрал рейтинг/);
  assert.match(emptyLine("duel"), /дуэль/);
  assert.equal(playHash("duel"), "#puzzles/duel");
  assert.equal(playHash("rapid"), "#online");
});

test("the country in the addresses", () => {
  assert.equal(leadersHash("blitz", "BY"), "#leaders/blitz/BY");
  assert.equal(leadersHash("", "BY"), "#leaders/blitz/BY");
  assert.equal(leadersHash("duel", ""), "#leaders/duel");
  assert.equal(leadersHash("duel", "by"), "#leaders/duel"); // not a code: ignored
  assert.equal(leadersHash("duel", "<b>"), "#leaders/duel");
  assert.equal(countryFromHash("#leaders/rapid/PL"), "PL");
  assert.equal(countryFromHash("#leaders/rapid"), "");
  assert.equal(countryFromHash("#leaders/rapid/pl"), "");
  assert.equal(countryFromHash("#leaders/rapid/POL"), "");
  assert.equal(countryFromHash("#user/abc/rapid/PL"), "");
  assert.equal(categoryFromHash("#leaders/rapid/PL"), "rapid");
  assert.equal(userHash("abc", "bullet", "BY"), "#user/abc/bullet/BY");
  assert.equal(userHash("abc", "", "BY"), "#user/abc");
  assert.deepEqual(userFromHash("#user/abc/bullet/BY"), { id: "abc", from: "bullet", country: "BY" });
  assert.deepEqual(userFromHash("#user/abc/bullet/xx"), { id: "abc", from: "bullet", country: "" });
  assert.equal(isCountryCode("BY"), true);
  assert.equal(isCountryCode(""), false);
  assert.equal(isCountryCode(undefined), false);
});

test("the country column and the filter line", () => {
  const list = [{ code: "BY", ru: "Беларусь", en: "Belarus" }];
  assert.equal(countryCell(list, "BY"), "Беларусь");
  assert.equal(countryCell(list, ""), "—");
  assert.equal(countryCell(list, undefined), "—");
  assert.equal(countryCell([], "BY"), "BY");
  assert.equal(countryFilterLine(list, ""), "Все страны");
  assert.equal(countryFilterLine(list, "BY"), "Страна: Беларусь");
  assert.match(emptyCountryLine(list, "BY"), /Беларусь/);
});
