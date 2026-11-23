// record.mjs records what acorn 8.19.0 makes of each input of inputs.json.gz
// (a JSON list of {src, ecma, await}) into acorn.json.gz, which acorn's
// TestRecorded replays. Run it with Node 24 from this directory, with the
// original's node_modules under /opt/original:
//
//   node record.mjs
//
// Each case holds src as UTF-16 code units, the onToken stream and either the
// SyntaxError or the AST. Values are encoded so that JSON keeps them exactly:
// numbers as {$num: String(n)}, BigInts as {$big}, undefined as {$undef}, a
// RegExp as {$re: true}, strings with unpaired surrogates as {$wtf: code units},
// and every Position as {line, column, ref}, where ref names the token whose
// start ("s3") or end ("e3") Position object it is, or "-" for none.
import { parse } from '/opt/original/node_modules/acorn/dist/acorn.mjs';
import { readFileSync, writeFileSync } from 'node:fs';
import { gunzipSync, gzipSync } from 'node:zlib';

const inputs = JSON.parse(gunzipSync(readFileSync('inputs.json.gz')));
const units = (s) => Array.from({ length: s.length }, (_, i) => s.charCodeAt(i));
const encStr = (s) => (s.isWellFormed() ? s : { $wtf: units(s) });

function enc(v, refs) {
  if (v === null) return null;
  switch (typeof v) {
    case 'undefined': return { $undef: 1 };
    case 'string': return encStr(v);
    case 'number': return { $num: String(v) };
    case 'bigint': return { $big: v.toString() };
    case 'boolean': return v;
  }
  if (v instanceof RegExp) return { $re: true };
  if (Array.isArray(v)) return v.map((x) => enc(x, refs));
  if (v.constructor.name === 'Position') return { line: v.line, column: v.column, ref: refs.get(v) ?? '-' };
  const out = {};
  for (const k of Object.keys(v).sort()) {
    out[k] = (k === 'start' || k === 'end') && typeof v[k] === 'number' ? v[k] : enc(v[k], refs);
  }
  return out;
}

const results = inputs.map((c) => {
  const tokens = [];
  const r = { src: units(c.src), ecma: c.ecma, await: c.await };
  let ast;
  try {
    ast = parse(c.src, { ecmaVersion: c.ecma, locations: true, allowAwaitOutsideFunction: c.await, onToken: tokens });
  } catch (e) {
    if (!(e instanceof SyntaxError)) throw e;
    r.error = { message: encStr(e.message), pos: e.pos, loc: { line: e.loc.line, column: e.loc.column }, raisedAt: e.raisedAt };
  }
  const refs = new Map();
  tokens.forEach((t, i) => {
    if (!refs.has(t.loc.start)) refs.set(t.loc.start, `s${i}`);
    if (!refs.has(t.loc.end)) refs.set(t.loc.end, `e${i}`);
  });
  r.tokens = tokens.map((t) => ({
    type: t.type.label,
    value: t.type.label === 'regexp'
      ? { pattern: encStr(t.value.pattern), flags: t.value.flags, value: t.value.value instanceof RegExp }
      : enc(t.value, refs),
    start: t.start,
    end: t.end,
    loc: enc(t.loc, refs),
  }));
  if (ast) r.ast = enc(ast, refs);
  return r;
});

writeFileSync('acorn.json.gz', gzipSync(JSON.stringify(results)));
console.log(`${results.length} cases, ${results.filter((r) => r.error).length} syntax errors`);
