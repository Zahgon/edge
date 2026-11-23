// record.mjs records what Edge's bundled js-stringify 1.0.2 (js.stringify)
// returns for generated values:
//
//     node testdata/record.mjs   (in internal/jsstringify, in the UTC time zone, the original at /opt/original)
//
// It writes testdata/stringify.json.gz in jsvaltest's format: [src, out] or
// [src, null, "Name: message"], where src is an expression evaluating to the
// value. Inputs come from a fixed mulberry32 generator.
import { writeFileSync } from 'node:fs'
import { gzipSync } from 'node:zlib'
import edge from '/opt/original/build/index.js'

const stringify = edge.globals.js.stringify

let seed = 20261123
function rand(n) {
  seed = (seed + 0x6d2b79f5) | 0
  let t = Math.imul(seed ^ (seed >>> 15), 1 | seed)
  t = (t + Math.imul(t ^ (t >>> 7), 61 | t)) ^ t
  return Math.floor((((t ^ (t >>> 14)) >>> 0) / 4294967296) * n)
}
const pick = (xs) => xs[rand(xs.length)]

function lit(s) {
  let out = "'"
  for (let i = 0; i < s.length; i++) {
    const c = s.charCodeAt(i)
    if (c === 39 || c === 92) out += '\\' + s[i]
    else if (c >= 32 && c < 127) out += s[i]
    else out += '\\u' + c.toString(16).padStart(4, '0')
  }
  return out + "'"
}

const specials = ['<', '>', '/', '</script>', '<!--', '\u2028', '\u2029', '"', "'", '\\', '\n', '\t', '\u0000',
  '\u001f', '\u007f', 'é', '😀', '\ud800', '\udc00', '\ufeff', '&', '`', '${x}']
function str() {
  let s = ''
  for (let n = rand(6), i = 0; i < n; i++) s += rand(2) ? pick(specials) : String.fromCharCode(32 + rand(95))
  return lit(s)
}
const leaves = ['undefined', 'null', 'true', 'false', '0', '-0', '1.5', '-1e21', '1e-7', 'NaN', 'Infinity',
  '2 ** 53', 'new Date(0)', 'new Date(1e12)', 'new Date(-1)', 'new Date(NaN)', 'new Date(8.64e15)',
  'new Date(-62198755200000)', 'Object.create(Date.prototype)', 'function f() {}', '() => 1', '/a\\/b/g',
  'new Map([[1, 2]])', "new Set(['a'])", "new String('s</')", 'new Number(5)', 'new Boolean(false)',
  "new Error('e')", "({ toJSON() { return '</x>' } })", '({ toJSON() { return undefined } })',
  "({ toJSON() { throw new Error('toJSON') } })", "({ get g() { throw new Error('getter') } })",
  "({ get g() { return '<g>' } })", '(function () { var o = {}; o.self = o; return o })()',
  "(function () { var d = new Date(0); d.toISOString = function () { return '<iso>' }; return d })()",
  '(function () { var d = new Date(0); d.toISOString = function () { return new Date(5) }; return d })()',
  '(function () { var d = new Date(0); d.toISOString = 5; return d })()',
  "Object.assign(Object.create(null), { a: '/' })", "Object.create({ inherited: 1 })",
  "Object.defineProperty({ v: 1 }, 'h', { value: 2, enumerable: false })",
  "(function () { return arguments })(1, '<')"]

function value(depth) {
  switch (depth > 3 ? 9 : rand(10)) {
    case 0: case 1: {
      const items = []
      for (let n = rand(5), i = 0; i < n; i++) items.push(value(depth + 1))
      return '[' + items.join(', ') + ']'
    }
    case 2: case 3: {
      const props = []
      for (let n = rand(5), i = 0; i < n; i++) props.push(`${rand(3) ? str() : pick(["'0'", "'1'", "'10'", "'a'"])}: ${value(depth + 1)}`)
      return '{ ' + props.join(', ') + ' }'
    }
    case 4: case 5: return str()
    default: return pick(leaves)
  }
}

const cases = []
const seen = new Set()
for (const leaf of leaves) seen.add(leaf), cases.push(leaf)
for (let attempts = 0; seen.size < 3000 && attempts < 30000; attempts++) {
  const src = value(0)
  if (!seen.has(src)) seen.add(src), cases.push(src)
}
const out = cases.map((src) => {
  try {
    return [src, stringify((0, eval)('(' + src + '\n)'))]
  } catch (e) {
    return [src, null, e instanceof Error ? e.name + ': ' + e.message : 'thrown: ' + String(e)]
  }
})
writeFileSync('testdata/stringify.json.gz', gzipSync(JSON.stringify(out), { level: 9, mtime: 0 }))
console.log(out.length, 'cases')
