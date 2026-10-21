import { test } from "node:test";
import assert from "node:assert/strict";
import {
  ABOUT_MAX,
  LOCATION_MAX,
  PERSON_MAX,
  aboutCounter,
  checkAbout,
  checkLocation,
  checkPersonName,
  checkProfile,
  diffProfile,
  fullName,
  normalizeAbout,
  savedValues,
  textLength,
} from "./profileFields.js";

const rep = (ch, n) => ch.repeat(n);

test("limits are the server's", () => {
  assert.equal(PERSON_MAX, 40);
  assert.equal(LOCATION_MAX, 60);
  assert.equal(ABOUT_MAX, 500);
});

test("textLength counts code points", () => {
  assert.equal(textLength("Анна"), 4);
  assert.equal(textLength("a🙂b"), 3);
  assert.equal(textLength(null), 0);
});

test("first and last names", () => {
  const good = {
    Анна: "Анна",
    "  Анна   Мария ": "Анна Мария",
    "Жан-Люк": "Жан-Люк",
    "O'Brien": "O'Brien",
    "D’Artagnan": "D’Artagnan",
    "Дж.": "Дж.",
    "": "",
    "   ": "",
    José: "José",
    "Анна Мария": "Анна Мария",
    [rep("я", 40)]: rep("я", 40),
  };
  for (const [raw, want] of Object.entries(good)) {
    assert.deepEqual(checkPersonName(raw, "Имя"), { value: want, error: "" }, JSON.stringify(raw));
  }
  for (const raw of [rep("я", 41), "Анна1", "Анна<b>", "a\tb", "a\nb", "Аня 🙂", "---", "...", "a/b", "a_b", "a​b", "a‮b"]) {
    const r = checkPersonName(raw, "Фамилия");
    assert.notEqual(r.error, "", JSON.stringify(raw));
    assert.match(r.error, /Фамилия/);
  }
  assert.match(checkPersonName(rep("я", 41), "Имя").error, /не больше 40/);
});

test("location", () => {
  assert.deepEqual(checkLocation("  Гродно,   Беларусь "), { value: "Гродно, Беларусь", error: "" });
  assert.deepEqual(checkLocation("Нью-Йорк (США)"), { value: "Нью-Йорк (США)", error: "" });
  assert.deepEqual(checkLocation(""), { value: "", error: "" });
  assert.deepEqual(checkLocation(rep("я", 60)), { value: rep("я", 60), error: "" });
  assert.match(checkLocation(rep("я", 61)).error, /не больше 60/);
  for (const raw of ["a\nb", "a\tb", "a\x00b", "a​b"]) assert.notEqual(checkLocation(raw).error, "", JSON.stringify(raw));
});

test("about: tidying", () => {
  assert.equal(normalizeAbout("  привет  \r\nмир \r\n"), "привет\nмир");
  assert.equal(normalizeAbout("один\n\n\n\n\nдва"), "один\n\nдва");
  assert.equal(normalizeAbout("строка\rстрока"), "строка\nстрока");
  assert.equal(normalizeAbout(" \n \n"), "");
  assert.equal(normalizeAbout(null), "");
  assert.equal(normalizeAbout("a b"), "a b");
});

test("about: rules", () => {
  assert.deepEqual(checkAbout("Тег <b>жирный</b> & «кавычки»"), { value: "Тег <b>жирный</b> & «кавычки»", error: "" }); // plain text
  assert.deepEqual(checkAbout("два\n\nабзаца"), { value: "два\n\nабзаца", error: "" });
  assert.equal(checkAbout(rep("я", 500)).error, "");
  assert.match(checkAbout(rep("я", 501)).error, /не больше 500/);
  // the empty lines folded by the tidying do not count
  assert.equal(checkAbout(rep("я", 249) + "\n\n\n\n\n" + rep("я", 249)).error, "");
  assert.notEqual(checkAbout("a\tb").error, "");
  assert.notEqual(checkAbout("a\x00b").error, "");
  assert.notEqual(checkAbout("a‮b").error, "");
});

test("about: counter", () => {
  assert.equal(aboutCounter(""), "0/500");
  assert.equal(aboutCounter("Привет"), "6/500");
  assert.equal(aboutCounter("a\r\nb"), "3/500");
  assert.equal(aboutCounter("  a  "), "1/500");
  assert.equal(aboutCounter(rep("я", 501)), "501/500");
});

const USER = { name: "Анна", firstName: "Анна", lastName: "Иванова", location: "Минск", about: "Привет", country: "BY", avatarUrl: "preset:crown" };

test("savedValues", () => {
  assert.deepEqual(savedValues(USER), {
    name: "Анна",
    firstName: "Анна",
    lastName: "Иванова",
    location: "Минск",
    about: "Привет",
    country: "BY",
    avatar: "preset:crown",
  });
  assert.deepEqual(savedValues({ name: "Гость 1234" }), {
    name: "Гость 1234",
    firstName: "",
    lastName: "",
    location: "",
    about: "",
    country: "",
    avatar: "",
  });
  assert.equal(savedValues({ name: "A", avatarUrl: "https://x.test/p.png" }).avatar, "google");
  assert.equal(savedValues(null).name, "");
});

test("checkProfile: tidy values, one message per field", () => {
  const saved = savedValues(USER);
  const ok = checkProfile({ ...saved, firstName: "  Мария  ", about: "Привет\r\nмир " });
  assert.equal(ok.valid, true);
  assert.deepEqual(ok.errors, {});
  assert.equal(ok.values.firstName, "Мария");
  assert.equal(ok.values.about, "Привет\nмир");

  const bad = checkProfile({ ...saved, name: "я", lastName: "Иванова2", location: "a\nb", about: rep("я", 501) });
  assert.equal(bad.valid, false);
  assert.deepEqual(Object.keys(bad.errors).sort(), ["about", "lastName", "location", "name"]);
  assert.match(bad.errors.name, /короткое/);
});

test("diffProfile sends only what changed", () => {
  const saved = savedValues(USER);
  assert.deepEqual(diffProfile(saved, saved), {});
  assert.deepEqual(diffProfile(saved, { ...saved, country: "PL" }), { country: "PL" });
  assert.deepEqual(diffProfile(saved, { ...saved, country: "" }), { country: "" }); // clearing is a change
  assert.deepEqual(diffProfile(saved, { ...saved, avatar: "", about: "" }), { avatar: "", about: "" });
  assert.deepEqual(diffProfile(saved, { ...saved, name: "Борис", location: "Гродно" }), { name: "Борис", location: "Гродно" });
  assert.deepEqual(diffProfile(savedValues({ name: "A" }), savedValues({ name: "A" })), {});
});

test("fullName", () => {
  assert.equal(fullName("Анна", "Иванова"), "Анна Иванова");
  assert.equal(fullName("Анна", ""), "Анна");
  assert.equal(fullName("", "Иванова"), "Иванова");
  assert.equal(fullName(undefined, null), "");
});
