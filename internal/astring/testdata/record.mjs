// record.mjs records what astring 1.9.0 writes for the programs of inputs.json.gz
// (a JSON list of {src, ecma, await} that acorn 8.19.0 accepts) into
// astring.json.gz, which astring's TestRecorded replays. Run it with Node 24 from
// this directory, with the original's node_modules under /opt/original:
//
//   node record.mjs
//
// For each program, parsed as edge-parser parses (locations, the case's
// ecmaVersion and allowAwaitOutsideFunction), it records generate(ast) as gen,
// generate(parse(gen)) as regen, and as bare the output for the AST with the raw
// text of every Literal removed and every Property made non-shorthand, the way
// astring prints nodes built by hand. Strings are UTF-16 code units; what a step
// throws is recorded under the step's name with an Error suffix.
import { parse } from '/opt/original/node_modules/acorn/dist/acorn.mjs';
import { generate } from '/opt/original/node_modules/astring/dist/astring.mjs';
import { readFileSync, writeFileSync } from 'node:fs';
import { gunzipSync, gzipSync } from 'node:zlib';

const inputs = JSON.parse(gunzipSync(readFileSync('inputs.json.gz')));
const units = (s) => Array.from({ length: s.length }, (_, i) => s.charCodeAt(i));

function strip(node) {
  if (node === null || typeof node !== 'object' || node instanceof RegExp) return node;
  if (Array.isArray(node)) return node.map(strip);
  const out = {};
  for (const k of Object.keys(node)) {
    if (k === 'loc' || k === 'start' || k === 'end' || (k === 'raw' && node.type === 'Literal')) continue;
    out[k] = strip(node[k]);
  }
  if (out.type === 'Property') out.shorthand = false;
  return out;
}

function attempt(r, key, f) {
  try {
    const s = f();
    r[key] = units(s);
    return s;
  } catch (e) {
    r[key + 'Error'] = units(e instanceof SyntaxError ? e.message : `${e.name}: ${e.message}`);
  }
}

const results = inputs.map((c) => {
  const options = { ecmaVersion: c.ecma, locations: true, allowAwaitOutsideFunction: c.await };
  const ast = parse(c.src, options);
  const r = { src: units(c.src), ecma: c.ecma, await: c.await };
  const gen = attempt(r, 'gen', () => generate(ast));
  if (gen !== undefined) attempt(r, 'regen', () => generate(parse(gen, options)));
  attempt(r, 'bare', () => generate(strip(ast)));
  return r;
});

writeFileSync('astring.json.gz', gzipSync(JSON.stringify(results)));
console.log(`${results.length} programs`);
