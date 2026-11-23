// record.mjs records the behaviour of @poppinss/utils' lodash build (merge, set,
// get, has, pick, omit, size) for generated inputs:
//
//     node testdata/record.mjs   (run in internal/lodash, with the original at /opt/original)
//
// Each case source is an expression evaluating to {args, log}: the call's
// arguments, built from objects with getters (which append to log), prototype
// chains, null-prototype objects, arrays, Dates, RegExps, Maps, Sets, functions,
// arguments objects and circular references. The output is [args result, args, log]
// rendered by serialize.js after the call, so identity and mutation are compared.
// Inputs come from a fixed mulberry32 generator; the files written are
// testdata/<function>.json.gz in jsvaltest's format.
import { createRequire } from "node:module";
import { readFileSync, writeFileSync } from "node:fs";
import { gzipSync } from "node:zlib";

const require = createRequire(import.meta.url);
const _ = require("/opt/original/node_modules/@poppinss/utils/build/lodash/main.cjs");
const serialize = (0, eval)(readFileSync(new URL("./serialize.js", import.meta.url), "utf8"));

let seed = 20261123;
function rnd(n) {
  seed = (seed + 0x6d2b79f5) | 0;
  let t = Math.imul(seed ^ (seed >>> 15), 1 | seed);
  t = (t + Math.imul(t ^ (t >>> 7), 61 | t)) ^ t;
  return Math.floor((((t ^ (t >>> 14)) >>> 0) / 4294967296) * n);
}
function pick(list) {
  return list[rnd(list.length)];
}

const keys = ["a", "b", "c", "0", "1", "2", "x.y", "length", "k k", "-1", "1.5", "__proto__", "$", "_"];
const primitives = ["1", "0", "-0", "NaN", "2.5", "-3", "'a'", "''", "'0'", "null", "undefined", "true", "false", "'x.y'", "Infinity"];

// Specials are objects that are not plain: they test what lodash keeps by reference.
const specials = [
  "new Date(5)", "/re/g", "function fn() {}", "() => 1", "new Map([['m', {z: 1}]])", "new Set([1, 'a'])",
  "new Error('boom')", "(function () { return arguments })(1, {q: 2})", "Object(1)", "Object('str')",
  "Object(false)", "new (function Ctor() { this.own = 1 })()",
  "(function () { function P() { this.v = 1 } P.prototype.inherited = {deep: 1}; return new P() })()",
  "Object.create({protoKey: 'pv'})", "Object.assign(Object.create(null), {np: {d: 1}})",
  "{get g() { L.push('get g'); return {gv: 1} }, plain: 1}",
  "Object.defineProperty({vis: 1}, 'hidden', {value: 2, enumerable: false})",
  "Object.freeze({frozen: {f: 1}})", "{length: 2, 0: 'l0', 1: 'l1'}", "{length: -1, a: 1}",
];

function genValue(depth) {
  const r = rnd(10);
  if (depth > 2 || r < 3) return pick(primitives);
  if (r < 5) return pick(specials);
  if (r < 7) {
    const n = rnd(4);
    const items = [];
    for (let i = 0; i < n; i++) items.push(genValue(depth + 1));
    return "[" + items.join(", ") + "]";
  }
  return genObject(depth);
}

function genObject(depth) {
  const n = rnd(4);
  const props = [];
  for (let i = 0; i < n; i++) {
    const k = pick(keys);
    // An object under "constructor" would be merged into Object itself; keep
    // such values primitive.
    props.push("[" + JSON.stringify(k) + "]: " + (k === "__proto__" && rnd(2) ? pick(primitives) : genValue(depth + 1)));
  }
  if (rnd(6) === 0) props.push("get gk() { L.push('gk'); return " + genValue(depth + 1) + " }");
  if (rnd(8) === 0) props.push("['constructor']: " + pick(primitives));
  return "{" + props.join(", ") + "}";
}

// A case declares a few shared objects (s0, s1) the arguments may reference
// several times, and a circular one.
function prelude() {
  return "var L = []; var s0 = " + genObject(1) + "; var s1 = " + genValue(1) +
    "; var cyc = {c: 1}; cyc.self = cyc; cyc.arr = [cyc];";
}
const shared = ["s0", "s1", "cyc", "s0", "[s0, s1]", "{inner: s0}"];
function genArg() {
  return rnd(4) === 0 ? pick(shared) : genValue(0);
}
function wrap(args) {
  return "(function () { " + prelude() + " return {args: [" + args.join(", ") + "], log: L} })()";
}

const pathStrings = ["a", "b", "a.b", "a[0]", "a[0].b", "a['x.y']", 'a["b"]', "[0]", ".a", "a..b", "a[]",
  "a.b.c", "0", "-1", "1.5", "x.y", "length", "", "[-1]", "a[1.5]", "a['b\\\\'c']", "a[\"q\\\\\"\"]",
  "a.0.b", "c[1][0]", "k k", "a[b]", "[a]", "a.", "a[", "a]", "a[0]b", "['x.y']", "b.a", "c.length",
  "self.self.c", "arr[0].c", "inner.a", "g.gv", "gk", "np.d", "frozen.f", "own", "inherited.deep", "protoKey"];
const pathOthers = ["0", "-0", "1", "NaN", "true", "null", "undefined", "1.5", "-1",
  "['a', 'b']", "['a', 0]", "[0, 'a']", "[-0]", "[['a']]", "[]", "['x.y']", "['a.b']", "['__proto__']",
  "({toString() { L.push('toString'); return 'a.b' }})", "({toString() { return 'a' }, valueOf() { L.push('valueOf'); return 0 }})",
  "[{toString() { L.push('key'); return 'b' }}]", "(function () { return arguments })('a', 'b')"];
function genPath(mutating) {
  let p;
  if (rnd(3) === 0) p = pick(pathOthers);
  else p = JSON.stringify(pick(pathStrings));
  // Paths that walk through __proto__ or constructor would change the realm's
  // builtins for the following cases.
  if (mutating && /__proto__|constructor/.test(p)) return "'a'";
  return p;
}

const targets = () => pick(["{}", "{}", genObject(0), "s0", "[]", "cyc", genValue(0), "null", "undefined", "'str'", "5"]);

const generators = {
  merge() {
    const n = rnd(4);
    const args = [targets()];
    for (let i = 0; i < n; i++) args.push(genArg());
    if (rnd(10) === 0) args.push("'k'", "0", "{k: 'v'}");
    return wrap(args);
  },
  set: () => wrap([targets(), genPath(true), genArg()]),
  get: () => wrap([genArg(), genPath(false), pick(["undefined", "'dflt'", "s0"])]),
  has: () => wrap([genArg(), genPath(false)]),
  pick() {
    const args = [genArg()];
    for (let n = rnd(4), i = 0; i < n; i++) args.push(genPath(false));
    return wrap(args);
  },
  omit() {
    const args = [genArg()];
    for (let n = rnd(4), i = 0; i < n; i++) args.push(genPath(true));
    return wrap(args);
  },
  size() {
    const strings = ["'abc'", "''", "'a\\u0301'", "'\\ud83d\\ude00'", "'\\ud83c\\uddfa\\ud83c\\uddf8'", "'\\ud83d\\udc68\\u200d\\ud83d\\udc69'",
      "'\\ud83d\\udc4d\\ud83c\\udffd'", "'\\ud800x'", "'x\\udc00'", "'\\u2764\\ufe0f'", "'e\\u20d0\\ufe20'", "'\\u200d'",
      "new String('a\\ud83d\\ude00b')", "new String('plain')", "Object.create(Map.prototype)", "Object.create(Set.prototype)",
      "Object.prototype", "Array.prototype", "(function () { function F() {} F.prototype.x = 1; return F.prototype })()",
      "Object.create(Array.prototype)", "{length: 3}", "{length: '3', a: 1}", "{length: 1.5}", "{length: 2, constructor: 1}",
      "(function () { return arguments })(1, 2, 3)", "function (a, b) {}", "Object.assign(function () {}, {p: 1})"];
    return wrap([rnd(3) === 0 ? pick(strings) : genArg()]);
  },
};

// hasHoles reports whether an array reachable from v through own data
// properties has holes. The interpreter's arrays have none (an index written
// past the end fills the gap with undefined), so such cases are not recorded.
function hasHoles(v, seen = new Set()) {
  if (v === null || (typeof v !== "object" && typeof v !== "function") || seen.has(v)) return false;
  seen.add(v);
  if (Array.isArray(v)) {
    for (let i = 0; i < v.length; i++) if (!(i in v)) return true;
  }
  for (const k of Object.getOwnPropertyNames(v)) {
    const d = Object.getOwnPropertyDescriptor(v, k);
    if ("value" in d && hasHoles(d.value, seen)) return true;
  }
  return false;
}

const calls = {
  merge: (a) => _.merge(...a),
  set: (a) => _.set(a[0], a[1], a[2]),
  get: (a) => _.get(a[0], a[1], a[2]),
  has: (a) => _.has(a[0], a[1]),
  pick: (a) => _.pick(...a),
  omit: (a) => _.omit(...a),
  size: (a) => _.size(a[0]),
};

const counts = { merge: 3000, set: 2500, get: 2500, has: 2500, pick: 2000, omit: 2000, size: 1500 };
for (const name of Object.keys(generators)) {
  const cases = [];
  const done = new Set();
  for (let attempts = 0; cases.length < counts[name] && attempts < counts[name] * 20; attempts++) {
    const src = generators[name]();
    if (done.has(src)) continue;
    done.add(src);
    const input = (0, eval)("(" + src + "\n)");
    try {
      const result = calls[name](input.args);
      if (hasHoles([result, input.args])) continue;
      cases.push([src, serialize([result, input.args, input.log])]);
    } catch (e) {
      cases.push([src, null, e instanceof Error ? e.name + ": " + e.message : "thrown: " + String(e)]);
    }
  }
  writeFileSync(new URL("./" + name + ".json.gz", import.meta.url), gzipSync(JSON.stringify(cases), { level: 9 }));
  console.log(name, cases.length);
}
