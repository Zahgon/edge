// props.mjs records testdata/props.json.gz, from which gen/props writes
// props_gen.go: the Unicode property names V8 accepts in \p{...} and the code
// points each property matches. Run it in internal/jsregexp:
//
//     node testdata/props.mjs PropertyAliases.txt PropertyValueAliases.txt | gzip -n -9 > testdata/props.json.gz
//
// The two files are the Unicode Character Database's alias files of the Unicode
// version of the running Node (https://www.unicode.org/Public/17.0.0/ucd/ for
// Node 24). They only propose candidate names; V8 decides which it accepts. A
// General_Category name is accepted when \p{gc=name} compiles and maps to the
// value's short name, a Script name when \p{sc=name} compiles and maps to the
// long name, and a binary property name when \p{name} compiles (and is not a
// General_Category value) and maps to the long name; Any, ASCII and Assigned are
// ECMAScript's own. The script checks that V8 treats \p{name} like \p{gc=name} and
// \p{scx=name} like \p{sc=name}.
//
// "members" holds, for every key "gc=<short name>", "bin=<long name>",
// "sc=<long name>" and "scx=<long name>", the code points \p{...} matches with
// the u flag, as sorted lo, hi pairs.
import { readFileSync } from "node:fs";

if (process.argv.length !== 4) {
  throw new Error("usage: node testdata/props.mjs PropertyAliases.txt PropertyValueAliases.txt");
}

// records returns the semicolon-separated fields of the lines of a UCD file.
function records(file) {
  return readFileSync(file, "utf8")
    .split("\n")
    .map((line) => line.replace(/#.*/, "").trim())
    .filter((line) => line !== "")
    .map((line) => line.split(";").map((f) => f.trim()));
}

function compiles(body) {
  try {
    new RegExp("\\p{" + body + "}", "u");
    return true;
  } catch {
    return false;
  }
}

const gc = {};
const script = {};
for (const [prop, short, long, ...rest] of records(process.argv[3])) {
  if (prop === "gc") {
    for (const name of [short, long, ...rest]) if (compiles("gc=" + name)) gc[name] = short;
  } else if (prop === "sc") {
    for (const name of [short, long, ...rest]) if (compiles("sc=" + name)) script[name] = long;
  }
}
const binary = { Any: "Any", ASCII: "ASCII", Assigned: "Assigned" };
for (const [short, long, ...rest] of records(process.argv[2])) {
  for (const name of [short, long, ...rest]) {
    if (!(name in gc) && compiles(name)) binary[name] = long;
  }
}
for (const name of Object.keys(gc)) {
  if (!compiles(name)) throw new Error("\\p{" + name + "} is rejected but \\p{gc=" + name + "} is not");
}
for (const name of Object.keys(script)) {
  if (!compiles("scx=" + name)) throw new Error("\\p{scx=" + name + "} is rejected but \\p{sc=" + name + "} is not");
}

// The code points in three strings without surrogates (so that no two code
// points pair up) and the surrogates one by one.
const segments = [
  [0, 0xd7ff],
  [0xe000, 0xffff],
  [0x10000, 0x10ffff],
].map(([lo, hi]) => {
  const cps = [];
  for (let c = lo; c <= hi; c++) cps.push(c);
  let text = "";
  for (let i = 0; i < cps.length; i += 4096) text += String.fromCodePoint(...cps.slice(i, i + 4096));
  return { lo, text, astral: lo >= 0x10000 };
});

function members(body) {
  const runs = new RegExp("\\p{" + body + "}+", "gu");
  const one = new RegExp("^\\p{" + body + "}$", "u");
  const ranges = [];
  const add = (lo, hi) => {
    if (ranges.length > 0 && ranges[ranges.length - 1] === lo - 1) ranges[ranges.length - 1] = hi;
    else ranges.push(lo, hi);
  };
  for (const seg of segments) {
    if (seg.lo === 0x10000) {
      for (let c = 0xd800; c <= 0xdfff; c++) if (one.test(String.fromCharCode(c))) add(c, c);
    }
    for (const m of seg.text.matchAll(runs)) {
      const width = seg.astral ? 2 : 1;
      const lo = seg.lo + m.index / width;
      add(lo, lo + m[0].length / width - 1);
    }
  }
  return ranges;
}

const keys = [
  ...new Set(Object.values(gc)).values().map((v) => "gc=" + v),
  ...new Set(Object.values(binary)).values().map((v) => "bin=" + v),
  ...new Set(Object.values(script)).values().flatMap((v) => ["sc=" + v, "scx=" + v]),
].sort();
const recording = {
  node: process.version,
  icu: process.versions.icu,
  unicode: process.versions.unicode,
  gc,
  binary,
  script,
  members: Object.fromEntries(keys.map((key) => [key, members(key.startsWith("bin=") ? key.slice(4) : key)])),
};
process.stdout.write(JSON.stringify(recording) + "\n");
