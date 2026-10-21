import test from "node:test";
import assert from "node:assert/strict";
import fs from "node:fs";
import path from "node:path";
import vm from "node:vm";
import { fileURLToPath } from "node:url";
import { SETTINGS_KEY } from "./model.js";

const here = path.dirname(fileURLToPath(import.meta.url));
const srcDir = path.join(here, "..");
const css = fs.readFileSync(path.join(srcDir, "index.css"), "utf8");
const html = fs.readFileSync(path.join(srcDir, "..", "index.html"), "utf8");

// Runs the script of index.html against a stand-in page whose localStorage holds `stored`
// (a string, or undefined for nothing saved); returns the data-theme it set (or null).
function runBootScript(stored, { throws = false } = {}) {
  const m = html.match(/<script>([\s\S]*?)<\/script>/);
  assert.ok(m, "index.html has no inline script");
  const attrs = new Map();
  const ctx = {
    document: { documentElement: { setAttribute: (k, v) => attrs.set(k, v) } },
    localStorage: {
      getItem: (k) => {
        if (throws) throw new Error("blocked");
        return k === SETTINGS_KEY && stored !== undefined ? stored : null;
      },
    },
  };
  vm.runInNewContext(m[1], ctx);
  return attrs.has("data-theme") ? attrs.get("data-theme") : null;
}

test("the script of index.html puts the saved theme on <html> before the page is drawn", () => {
  assert.equal(runBootScript(JSON.stringify({ theme: "light" })), "light");
  assert.equal(runBootScript(JSON.stringify({ theme: "system", textSize: "large" })), "system");
});

test("the script of index.html leaves the dark page alone, whatever is in storage", () => {
  assert.equal(runBootScript(undefined), null); // nothing saved: dark
  assert.equal(runBootScript(JSON.stringify({ theme: "dark" })), null);
  assert.equal(runBootScript(JSON.stringify({ boardTheme: "green" })), null); // saved before themes existed
  assert.equal(runBootScript(JSON.stringify({ theme: "neon" })), null);
  assert.equal(runBootScript(JSON.stringify({ theme: 1 })), null);
  assert.equal(runBootScript("not json"), null);
  assert.equal(runBootScript("null"), null);
  assert.equal(runBootScript(undefined, { throws: true }), null); // storage blocked
});

// The declarations of a block of index.css whose selector line starts with `selector`.
function declarations(source, selector) {
  const at = source.indexOf(selector + " {");
  assert.ok(at >= 0, selector + " not found");
  let depth = 0;
  let i = source.indexOf("{", at);
  const start = i + 1;
  for (; i < source.length; i++) {
    if (source[i] === "{") depth++;
    if (source[i] === "}" && --depth === 0) break;
  }
  const body = source.slice(start, i);
  const out = {};
  for (const line of body.replace(/\/\*[\s\S]*?\*\//g, "").split(";")) {
    const mm = line.match(/^\s*(--[\w-]+|color-scheme)\s*:\s*([\s\S]+?)\s*$/);
    if (mm) out[mm[1]] = mm[2].replace(/\s+/g, " ");
  }
  return out;
}

test('the light theme of data-theme="system" is the same palette as data-theme="light"', () => {
  const light = declarations(css, 'html[data-theme="light"]');
  const media = css.slice(css.indexOf("@media (prefers-color-scheme: light)"));
  const system = declarations(media, 'html[data-theme="system"]');
  assert.ok(Object.keys(light).length > 30);
  assert.deepEqual(system, light);
  assert.equal(light["color-scheme"], "light");
  const lightHigh = declarations(css, 'html[data-theme="light"][data-contrast="high"]');
  const systemHigh = declarations(media, 'html[data-theme="system"][data-contrast="high"]');
  assert.deepEqual(systemHigh, lightHigh);
});

test("the light theme only redefines tokens the dark one has", () => {
  const dark = declarations(css, ":root");
  const light = declarations(css, 'html[data-theme="light"]');
  for (const k of Object.keys(light)) assert.ok(k in dark || k === "color-scheme", k + " is not a token of :root");
  // the tokens that carry text and surfaces are all redefined
  for (const k of ["--bg", "--surface", "--surface-2", "--surface-3", "--border", "--border-strong", "--text", "--text-2", "--muted", "--primary", "--on-primary", "--info", "--warn", "--danger"]) {
    assert.ok(k in light, k + " is not redefined for the light theme");
  }
});

// WCAG contrast of two #rrggbb colours.
function lum(hex) {
  const c = [1, 3, 5].map((i) => parseInt(hex.slice(i, i + 2), 16) / 255).map((v) => (v <= 0.03928 ? v / 12.92 : ((v + 0.055) / 1.055) ** 2.4));
  return 0.2126 * c[0] + 0.7152 * c[1] + 0.0722 * c[2];
}
const ratio = (a, b) => (Math.max(lum(a), lum(b)) + 0.05) / (Math.min(lum(a), lum(b)) + 0.05);

test("light theme: the text, the secondary text and the accent are readable (4.5:1) on the page and the cards", () => {
  const t = declarations(css, 'html[data-theme="light"]');
  for (const bg of ["--bg", "--surface", "--surface-2"]) {
    for (const fg of ["--text", "--text-2", "--muted", "--primary", "--info", "--danger"]) {
      assert.ok(ratio(t[fg], t[bg]) >= 4.5, `${fg} on ${bg}: ${ratio(t[fg], t[bg]).toFixed(2)}`);
    }
  }
  assert.ok(ratio(t["--on-primary"], t["--primary"]) >= 4.5, "text on the green button");
  assert.ok(ratio(t["--on-primary"], t["--primary-strong"]) >= 4.5, "text on the pressed green button");
  assert.ok(ratio(t["--warn"], t["--surface"]) >= 4.5, "--warn on --surface");
});

function* cssFiles(dir) {
  for (const e of fs.readdirSync(dir, { withFileTypes: true })) {
    if (e.name === "node_modules") continue;
    const p = path.join(dir, e.name);
    if (e.isDirectory()) yield* cssFiles(p);
    else if (e.name.endsWith(".css")) yield p;
  }
}

test("component stylesheets have no hex colours: they use the tokens of index.css", () => {
  for (const file of cssFiles(srcDir)) {
    if (path.basename(file) === "index.css") continue;
    const text = fs.readFileSync(file, "utf8").replace(/\/\*[\s\S]*?\*\//g, "");
    const hit = text.match(/#[0-9a-fA-F]{3,8}\b/);
    assert.equal(hit, null, `${path.relative(srcDir, file)} has a hex colour ${hit && hit[0]}`);
  }
});
