import test from "node:test";
import assert from "node:assert/strict";
import {
  COMMUNITY_TABS,
  REPORT_NOTE_MAX,
  REPORT_REASONS,
  forumHash,
  parseCommunityHash,
  parseForumHash,
  postShown,
  reportActions,
  reportLink,
  targetOf,
} from "./logic.js";

test("the list of blocked players is a tab of the community with an address", () => {
  assert.ok(COMMUNITY_TABS.includes("blocked"));
  assert.deepEqual(parseCommunityHash("#community/blocked"), { tab: "blocked", id: "" });
  assert.deepEqual(parseCommunityHash("#community/blocked/abc"), { tab: "blocked", id: "" });
});

test("the queue of complaints has an address of its own", () => {
  assert.deepEqual(parseForumHash("#forum/reports"), { view: "reports" });
  assert.equal(forumHash.reports(), "#forum/reports");
  assert.deepEqual(parseForumHash("#forum/reportsx"), { view: "home" });
});

test("the reasons and the note limit are the server's", () => {
  assert.deepEqual(REPORT_REASONS, ["spam", "abuse", "cheating", "other"]);
  assert.equal(REPORT_NOTE_MAX, 300);
  assert.equal(targetOf(42), "42");
});

test("a moderator can delete or lock only on the forum", () => {
  assert.deepEqual(reportActions("forum_post"), ["none", "delete", "lock"]);
  assert.deepEqual(reportActions("forum_topic"), ["none", "delete", "lock"]);
  assert.deepEqual(reportActions("user"), ["none"]);
  assert.deepEqual(reportActions("message"), ["none"]);
});

test("a complaint links to the topic or the player, never to a private message", () => {
  assert.equal(reportLink({ kind: "forum_post", topicId: 12 }), "#forum/t/12");
  assert.equal(reportLink({ kind: "forum_topic", topicId: 5 }), "#forum/t/5");
  assert.equal(reportLink({ kind: "forum_post" }), "");
  assert.equal(reportLink({ kind: "user", target: { id: "abc" } }), "#user/abc");
  assert.equal(reportLink({ kind: "user", target: { name: "gone" } }), "");
  assert.equal(reportLink({ kind: "message", target: { id: "abc" } }), "");
});

test("a message of a blocked author is hidden until asked", () => {
  assert.equal(postShown({ blockedByMe: true }, false), false);
  assert.equal(postShown({ blockedByMe: true }, true), true);
  assert.equal(postShown({ blockedByMe: true, deleted: true }, false), true); // nothing to hide
  assert.equal(postShown({}, false), true);
});
