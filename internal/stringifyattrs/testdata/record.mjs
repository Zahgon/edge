// record.mjs records what stringify-attributes 4.0.0 returns for generated
// attribute objects:
//
//     node testdata/record.mjs   (in internal/stringifyattrs, the original at /opt/original)
//
// It writes testdata/stringify.json.gz in jsvaltest's format: [src, out] or
// [src, null, "Name: message"], where src is an expression evaluating to the
// attributes. Inputs come from a fixed mulberry32 generator.
import { writeFileSync } from 'node:fs'
import { gzipSync } from 'node:zlib'
import stringifyAttributes from '/opt/original/node_modules/stringify-attributes/index.js'

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

const specials = ['&', '"', "'", '<', '>', '&amp;', '`', ' ', '=', 'é', '😀', '\ud800', '\n']
function str() {
  let s = ''
  for (let n = rand(5), i = 0; i < n; i++) s += rand(2) ? pick(specials) : String.fromCharCode(97 + rand(26))
  return s
}
const keys = ['class', 'id', 'data-x', 'aria-label', 'disabled', 'x-on:click', '@click', ':value', '0', '1', '10', 'a b', '']
const values = ['true', 'false', 'null', 'undefined', '0', '-0', 'NaN', '1.5', "''", "'text'", '[]', "['a', 'b']",
  "['a', null, undefined, 1, ['x', 'y']]", '[true]', '{}', "({ toString() { return '<obj>' } })",
  "({ toString() { throw new Error('toString') } })", 'new Date(0)', 'function f() {}', "new String('s')",
  "Object.assign(['p', 'q'], { join() { return 'joined' } })", "Object.assign(['p'], { join: 5 })",
  "Object.assign(['p'], { join() { return true } })", 'Object.create(null)']

function attributes() {
  if (rand(25) === 0) return pick(['null', 'undefined', "'str'", '5', '[1, 2]', 'true', 'Object.create(null)'])
  const props = []
  for (let n = rand(6), i = 0; i < n; i++) {
    const key = lit(rand(3) ? pick(keys) : str())
    const value = rand(4) ? pick(values) : lit(str())
    props.push(`${key}: ${value}`)
  }
  if (rand(8) === 0) props.push("get g() { return 'got' }")
  if (rand(12) === 0) props.push("get boom() { throw new TypeError('getter') }")
  return '({ ' + props.join(', ') + ' })'
}

const cases = []
const seen = new Set()
for (let attempts = 0; cases.length < 3000 && attempts < 30000; attempts++) {
  const src = attributes()
  if (seen.has(src)) continue
  seen.add(src)
  try {
    cases.push([src, stringifyAttributes((0, eval)('(' + src + '\n)'))])
  } catch (e) {
    cases.push([src, null, e instanceof Error ? e.name + ': ' + e.message : 'thrown: ' + String(e)])
  }
}
writeFileSync('testdata/stringify.json.gz', gzipSync(JSON.stringify(cases), { level: 9 }))
console.log(cases.length, 'cases')
