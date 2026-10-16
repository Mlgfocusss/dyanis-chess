import test from "node:test";
import assert from "node:assert/strict";
import { BEEP_AT, LOW_MS, beepDue } from "./lowtime.js";

test("the clock beeps at ten seconds and in each of the last three, once, and only as it falls", () => {
  assert.deepEqual(BEEP_AT, [10, 3, 2, 1]);
  assert.equal(beepDue(11, 10), true);
  assert.equal(beepDue(10, 10), false); // the same second again
  assert.equal(beepDue(4, 3), true);
  assert.equal(beepDue(2, 1), true);
  assert.equal(beepDue(9, 8), false);
  assert.equal(beepDue(5, 7), false); // a clock that went up (an increment)
  assert.equal(beepDue(1, 0), false);
  assert.ok(LOW_MS > 10000);
});
