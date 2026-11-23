// record.js runs the cases of a fixture in Node and records what Node does with
// them; the Go tests (node_test.go) replay the recorded results against the
// interpreter. Run it with the reference Node (v24) from the repository root:
//
//   gzip -dc internal/jsrt/testdata/F.json.gz | node internal/jsrt/testdata/record.js | gzip -9n > F.new.gz
//
// Input and output are one JSON document:
//
//   {"helpers": {"escapeFn": "<function expression>", ...}, "locals": <value>,
//    "cases": [{"name": "...", "src": "<function body>", "locals": <value>, "this": <value>}, ...]}
//
// Each case body is compiled as a template function,
// new Function('locals, escapeFn, include, rethrow', src), and called with this
// set to the decoded "this" (undefined when absent), locals a null-prototype
// object holding the keys of the decoded "locals" (the case's, or else the
// fixture's), and the helpers, each compiled from its source text. The
// recorder adds to every case:
//
//   "ret"    the return value: a string as it is, anything else as
//            "(<typeof>) <String(value)>";
//   "err"    when the compilation or the call throws: "<name>: <message>" for an
//            Error, "thrown <typeof> <String(value)>" otherwise;
//   "stdout", "stderr"  what the case printed through console.
//
// Values in "locals" and "this" are JSON, with tagged objects for what JSON
// cannot hold: {"$undef":1}, {"$nan":1}, {"$inf":1|-1}, {"$negzero":1},
// {"$date":<ms or tagged NaN>}, {"$fn":"<body>","params":[...]} and
// {"$re":"<source>","flags":"<flags>"}. All cases run in one realm, in order, so
// globals a case creates are visible to the cases after it. The fixtures are
// recorded in the UTC time zone, the container default.
//
// The case lists themselves (battery, strict, props, collections, console, ...)
// are the fixtures: re-recording one replays its cases in Node.
'use strict';

const fs = require('fs');

function decode(v) {
  if (Array.isArray(v)) return v.map(decode);
  if (v === null || typeof v !== 'object') return v;
  if ('$undef' in v) return undefined;
  if ('$nan' in v) return NaN;
  if ('$inf' in v) return v.$inf < 0 ? -Infinity : Infinity;
  if ('$negzero' in v) return -0;
  if ('$date' in v) return new Date(decode(v.$date));
  if ('$fn' in v) return new Function(...v.params, v.$fn);
  if ('$re' in v) return new RegExp(v.$re, v.flags);
  const o = {};
  for (const k of Object.keys(v)) o[k] = decode(v[k]);
  return o;
}

function describe(v) {
  try {
    return String(v);
  } catch (e) {
    return '<String failed>';
  }
}

function errorText(e) {
  if (e instanceof Error) return e.name + ': ' + e.message;
  return 'thrown ' + typeof e + ' ' + describe(e);
}

const { Console } = console;
const input = JSON.parse(fs.readFileSync(0, 'utf8'));
const helpers = {};
for (const [name, src] of Object.entries(input.helpers || {})) {
  helpers[name] = new Function('return (' + src + ')')();
}

for (const c of input.cases) {
  delete c.ret;
  delete c.err;
  delete c.stdout;
  delete c.stderr;
  const out = [];
  const err = [];
  // With ignoreErrors false, Console only calls the streams' write.
  const sink = (chunks) => ({ write(s) { chunks.push(String(s)); return true; } });
  globalThis.console = new Console({ stdout: sink(out), stderr: sink(err), ignoreErrors: false });
  try {
    const locals = Object.create(null);
    const data = c.locals !== undefined ? c.locals : input.locals;
    if (data !== undefined) Object.assign(locals, decode(data));
    const self = c.this === undefined ? undefined : decode(c.this);
    const fn = new Function('locals, escapeFn, include, rethrow', c.src);
    const ret = fn.call(self, locals, helpers.escapeFn, helpers.include, helpers.rethrow);
    c.ret = typeof ret === 'string' ? ret : '(' + typeof ret + ') ' + describe(ret);
  } catch (e) {
    c.err = errorText(e);
  }
  if (out.length) c.stdout = out.join('');
  if (err.length) c.stderr = err.join('');
}

process.stdout.write(JSON.stringify(input));
