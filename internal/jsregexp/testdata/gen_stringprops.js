// Records V8's behaviour for the properties of strings of the v flag
// (stringprops.json.gz). Run with
//
//     node testdata/gen_stringprops.js emoji-sequences.txt emoji-zwj-sequences.txt testdata/stringprops.json.gz
//
// where the two files are Unicode 17.0.0's emoji data
// (https://www.unicode.org/Public/17.0.0/emoji/).
//
// "cases" has the layout of exec.json: [pattern, flags, input, start, result], where
// result is the flattened match indices (-1 for undefined), null for no match, or
// "E:" and the message of the SyntaxError new RegExp(pattern, flags) throws. Global
// and sticky cases follow each other through the input, start being the lastIndex
// left by the previous match.
//
// "members" lists candidate strings, every emoji character and sequence the data
// files list and near misses of each, and for each property the indexes of the
// candidates s with /^\p{P}$/v.test(s).
"use strict";
const fs = require("fs");
const zlib = require("zlib");

const properties = [
  "Basic_Emoji",
  "Emoji_Keycap_Sequence",
  "RGI_Emoji",
  "RGI_Emoji_Flag_Sequence",
  "RGI_Emoji_Modifier_Sequence",
  "RGI_Emoji_Tag_Sequence",
  "RGI_Emoji_ZWJ_Sequence",
];

const family = "\u{1f468}\u200d\u{1f469}\u200d\u{1f467}\u200d\u{1f466}";
const england = "\u{1f3f4}\u{e0067}\u{e0062}\u{e0065}\u{e006e}\u{e0067}\u{e007f}";
const scotland = "\u{1f3f4}\u{e0067}\u{e0062}\u{e0073}\u{e0063}\u{e0074}\u{e007f}";
const inputs = [
  "",
  "a",
  "\u{1f600}",
  "\u231a",
  "\u00a9",
  "\u00a9\ufe0f",
  "#\ufe0f\u20e3",
  "#\u20e3",
  "1\ufe0f\u20e31",
  "\u{1f1eb}\u{1f1f7}",
  "\u{1f1fa}\u{1f1f8}\u{1f1ec}\u{1f1e7}\u{1f1eb}",
  "\u{1f1e6}\u{1f1e6}",
  "\u{1f44b}\u{1f3fd}",
  "\u{1f44b}\u{1f3fd}\u{1f3fd}",
  "a\u{1f3fb}",
  "\u{1f3fb}",
  england,
  scotland,
  "\u{1f3f4}\u{e0067}\u{e0062}\u{e007f}",
  family,
  "\u{1f468}\u200d\u{1f469}\u200d\u{1f467}",
  "\u{1f468}\u200d\u{1f469}\u200d",
  "\u{1f468}\u200d",
  "\u{1f469}\u{1f3fd}\u200d\u{1f4bb}",
  "\u{1f3f3}\ufe0f\u200d\u{1f308}",
  "\u{1f3f3}\u200d\u{1f308}",
  "\u2764\ufe0f\u200d\u{1f525}",
  "Hi \u{1f44b}\u{1f3fd} there " + family + "!",
  "x" + england + "y\u{1f1ef}\u{1f1f5}z#\ufe0f\u20e3",
  "\u{1f600}\u{1f601}\u{1f602}",
  "\ud83d",
  "\ude00\ud83d\ude00",
  "\u{1f9d1}\u200d\u{1f91d}\u200d\u{1f9d1}\u{1f3fb}",
  "abc" + family + "abc",
  "\u{1f1e8}\u{1f1f6}",
];

const patterns = [];
for (const p of properties) {
  patterns.push(["\\p{" + p + "}", "v"], ["^\\p{" + p + "}$", "v"], ["[\\p{" + p + "}]", "v"], ["\\p{" + p + "}", "gv"]);
}
patterns.push(
  ["\\p{RGI_Emoji}+", "v"],
  ["(\\p{RGI_Emoji})(\\p{RGI_Emoji})?", "dv"],
  ["(?<e>\\p{RGI_Emoji})\\k<e>", "v"],
  ["\\p{RGI_Emoji}{2}", "v"],
  ["(?:\\p{RGI_Emoji}|.)*?!", "v"],
  ["(?<=\\p{RGI_Emoji})[a-z!]", "gv"],
  ["(?<!\\p{RGI_Emoji_ZWJ_Sequence})\\p{Basic_Emoji}", "gv"],
  ["(?<=^\\p{RGI_Emoji_Flag_Sequence})\\p{RGI_Emoji_Flag_Sequence}", "v"],
  ["[\\p{RGI_Emoji}a-z]+", "gv"],
  ["^[\\p{RGI_Emoji}\\q{abc|ab}]+$", "v"],
  ["[\\q{}\\p{Emoji_Keycap_Sequence}]", "gv"],
  ["[\\p{RGI_Emoji}--\\p{Basic_Emoji}]", "gv"],
  ["[\\p{RGI_Emoji}--\\q{" + family + "|\u{1f600}}]", "gv"],
  ["[\\p{RGI_Emoji}--[\\p{Basic_Emoji}\\p{RGI_Emoji_ZWJ_Sequence}]]", "gv"],
  ["[\\p{RGI_Emoji}&&\\p{RGI_Emoji_Flag_Sequence}]", "gv"],
  ["[\\p{RGI_Emoji}&&[\\p{Emoji_Keycap_Sequence}\\q{\u{1f1eb}\u{1f1f7}}]]", "gv"],
  ["[\\p{Basic_Emoji}&&\\p{Emoji}]", "gv"],
  ["[\\p{RGI_Emoji}&&[a-z\\u231a]]", "gv"],
  ["[[\\p{RGI_Emoji_Flag_Sequence}]--\\q{\u{1f1fa}\u{1f1f8}|\u{1f1ec}\u{1f1e7}}]", "gv"],
  ["[^\\p{RGI_Emoji}&&a]", "gv"],
  ["[^[^a]--\\q{ab}]", "v"],
  ["\\p{RGI_Emoji_Modifier_Sequence}|\\p{Basic_Emoji}", "gv"],
  ["\\p{Emoji_Keycap_Sequence}", "giv"],
  ["(?i:\\p{RGI_Emoji_Tag_Sequence})", "gv"],
  ["\\p{RGI_Emoji}", "yv"],
  ["\\p{RGI_Emoji}$", "mv"],
  ["\\p{RGI_Emoji_ZWJ_Sequence}|\\u{1f468}", "gv"],
);

const errors = [];
for (const p of properties) {
  errors.push(
    ["\\P{" + p + "}", "v"],
    ["[\\P{" + p + "}]", "v"],
    ["[^\\p{" + p + "}]", "v"],
    ["\\p{" + p + "}", "u"],
    ["[\\p{" + p + "}]", "u"],
    ["\\P{" + p + "}", "u"],
    ["\\p{" + p + "}", ""],
    ["\\p{" + p + "}", "iv"],
    ["\\p{" + p + "=Yes}", "v"],
    ["\\p{" + p.toLowerCase() + "}", "v"],
  );
}
errors.push(
  ["\\p{RGI_Emoji", "v"],
  ["\\p{Emoji_Presentation_Sequence}", "v"],
  ["\\p{RGI_Emoji_Sequence}", "v"],
  ["\\p{Emoji_Zwj_Sequence}", "v"],
  ["\\p{Basic_Emoji}", "i"],
  ["[^\\p{RGI_Emoji}--\\p{RGI_Emoji}]", "v"],
  ["[^\\p{Basic_Emoji}--a]", "v"],
  ["[^\\p{Basic_Emoji}&&\\p{RGI_Emoji}]", "v"],
  ["[^[\\p{RGI_Emoji}]]", "v"],
  ["[^a\\p{RGI_Emoji}]", "v"],
  ["[^\\p{RGI_Emoji}a]", "v"],
  ["[[^\\p{RGI_Emoji_Flag_Sequence}]]", "v"],
  ["[^\\q{ab}--\\q{ab}]", "v"],
  ["[^[\\q{ab}&&\\q{ab}]]", "v"],
  ["[^[\\q{ab}&&[a]]]", "v"],
  ["[^\\q{}]", "v"],
  ["[^\\q{a}]", "v"],
  ["[\\p{RGI_Emoji}-a]", "v"],
  ["[a-\\p{RGI_Emoji}]", "v"],
  ["[\\p{RGI_Emoji}--]", "v"],
  ["[\\p{RGI_Emoji}&&\\p{Basic_Emoji}--a]", "v"],
);

// indices flattens the d-flag indices of a match.
function indices(m) {
  return m.indices.flatMap((r) => (r === undefined ? [-1, -1] : r));
}

const cases = [];
for (const [p, f] of patterns) {
  const re = new RegExp(p, f.includes("d") ? f : f + "d");
  for (const s of inputs) {
    re.lastIndex = 0;
    if (!re.global && !re.sticky) {
      const m = re.exec(s);
      cases.push([p, f, s, 0, m ? indices(m) : null]);
      continue;
    }
    for (;;) {
      const start = re.lastIndex;
      const m = re.exec(s);
      cases.push([p, f, s, start, m ? indices(m) : null]);
      if (!m) break;
      if (m[0] === "") re.lastIndex += s.codePointAt(re.lastIndex) > 0xffff ? 2 : 1;
    }
  }
  // A global search resumed inside a surrogate pair.
  if (re.global) {
    for (const start of [1, 2, 3]) {
      re.lastIndex = start;
      const m = re.exec(family);
      cases.push([p, f, family, start, m ? indices(m) : null]);
    }
  }
}
for (const [p, f] of errors) {
  let result;
  try {
    new RegExp(p, f);
    result = "compiled";
  } catch (e) {
    result = "E:" + e.message;
  }
  cases.push([p, f, "", 0, result === "compiled" ? null : result]);
}

// Near misses of each member: without its variation selectors, without its last
// code point, and with a skin tone added to its first code point.
const rgi = /^\p{RGI_Emoji}$/v;
const singleRGI = /^[\p{RGI_Emoji}&&\p{Any}]$/v;
const candidates = new Set();
for (let cp = 0; cp <= 0x10ffff; cp++) {
  if (cp >= 0xd800 && cp <= 0xdfff) continue;
  const c = String.fromCodePoint(cp);
  if (singleRGI.test(c) || rgi.test(c + "\ufe0f")) {
    candidates.add(c);
    candidates.add(c + "\ufe0f");
    candidates.add(c + "\u{1f3fd}");
  }
}
const sequences = process.argv.slice(2, 4).flatMap((file) =>
  fs
    .readFileSync(file, "utf8")
    .split("\n")
    .map((line) => line.replace(/#.*/, "").split(";")[0].trim())
    .filter((field) => field.includes(" ")),
);
for (const field of sequences) {
  const cps = field.split(/\s+/).map((h) => String.fromCodePoint(parseInt(h, 16)));
  const s = cps.join("");
  candidates.add(s);
  candidates.add(s.replaceAll("\ufe0f", ""));
  candidates.add(cps.slice(0, -1).join(""));
  candidates.add(cps[0] + "\u{1f3fd}" + cps.slice(1).join(""));
}
const list = [...candidates].sort();
const accepted = {};
for (const p of properties) {
  const re = new RegExp("^\\p{" + p + "}$", "v");
  accepted[p] = [];
  list.forEach((s, i) => {
    if (re.test(s)) accepted[p].push(i);
  });
}

fs.writeFileSync(process.argv[4], zlib.gzipSync(JSON.stringify({ cases, members: { candidates: list, accepted } }), { level: 9 }));
