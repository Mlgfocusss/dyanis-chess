import test from "node:test";
import assert from "node:assert/strict";
import { TIME_CONTROLS, describeTC, formatClock, tcToRequest } from "./clock.js";

test("formatClock", () => {
  assert.equal(formatClock(605000), "10:05");
  assert.equal(formatClock(60000), "1:00");
  assert.equal(formatClock(59999), "1:00"); // rounds up, never shows 0:59 for 59.999 s... as 1:00
  assert.equal(formatClock(42000), "0:42");
  assert.equal(formatClock(9400), "0:09.4");
  assert.equal(formatClock(9401), "0:09.5");
  assert.equal(formatClock(100), "0:00.1");
  assert.equal(formatClock(0), "0:00.0");
  assert.equal(formatClock(-500), "0:00.0");
  assert.equal(formatClock(3723000), "1:02:03");
});

test("describeTC", () => {
  assert.equal(describeTC({ baseMs: 300000, incMs: 2000 }), "5 + 2");
  assert.equal(describeTC({ baseMs: 600000, incMs: 0 }), "10 мин");
  assert.equal(describeTC({ baseMs: 30000, incMs: 0 }), "30 с");
  assert.equal(describeTC({ baseMs: 0, incMs: 0 }), "без часов");
  assert.equal(describeTC(null), "без часов");
});

test("every ready time control is one the server accepts", () => {
  for (const g of TIME_CONTROLS) {
    for (const o of g.options) {
      const { timeMs, incMs } = tcToRequest(o);
      assert.ok(timeMs >= 10_000 && timeMs <= 3 * 3600_000, `${o}`);
      assert.ok(incMs >= 0 && incMs <= 60_000, `${o}`);
    }
  }
});
