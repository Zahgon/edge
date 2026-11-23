// record.mjs records what he 1.2.0's escape returns for generated strings:
//
//     node testdata/record.mjs   (in internal/htmlent, the original at /opt/original)
//
// It writes testdata/escape.json.gz in jsvaltest's format: [src, out], where
// src is a string literal. Every printable ASCII character and the special
// characters come first, then strings from a fixed mulberry32 generator.
import { writeFileSync } from 'node:fs'
import { gzipSync } from 'node:zlib'
import he from '/opt/original/node_modules/he/he.js'

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

const specials = ['&', '"', "'", '<', '>', '`', '&amp;', '&#x27;', '&lt;', 'é', '😀', '\ud800', '\udc00', '\u2028', '\u0000', '\n']
const inputs = ['']
for (let c = 0; c < 128; c++) inputs.push(String.fromCharCode(c))
inputs.push(...specials)
for (let i = 0; i < 3000; i++) {
  let s = ''
  for (let n = 1 + rand(10), k = 0; k < n; k++) s += rand(2) ? pick(specials) : String.fromCharCode(32 + rand(95))
  inputs.push(s)
}
const cases = inputs.map((s) => [lit(s), he.escape(s)])
writeFileSync('testdata/escape.json.gz', gzipSync(JSON.stringify(cases), { level: 9 }))
console.log(cases.length, 'cases')
