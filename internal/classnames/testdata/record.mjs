// record.mjs records what Edge's bundled classnames 2.5.1 (html.classNames)
// returns for generated arguments:
//
//     node testdata/record.mjs   (in internal/classnames, the original at /opt/original)
//
// It writes testdata/classnames.json.gz in jsvaltest's format: [src, out] or
// [src, null, "Name: message"], where src is an expression evaluating to the
// argument list. Inputs come from a fixed mulberry32 generator.
import { writeFileSync } from 'node:fs'
import { gzipSync } from 'node:zlib'
import edge from '/opt/original/build/index.js'

const classNames = edge.globals.html.classNames

let seed = 20261123
function rand(n) {
  seed = (seed + 0x6d2b79f5) | 0
  let t = Math.imul(seed ^ (seed >>> 15), 1 | seed)
  t = (t + Math.imul(t ^ (t >>> 7), 61 | t)) ^ t
  return Math.floor((((t ^ (t >>> 14)) >>> 0) / 4294967296) * n)
}
const pick = (xs) => xs[rand(xs.length)]

const leaves = ["'a'", "'btn'", "'btn-primary'", "''", "' spaced '", "'é'", "'\\ud83d\\ude00'", '0', '1', '-0', 'NaN',
  '1.5', '-2', 'Infinity', 'true', 'false', 'null', 'undefined', 'function f() {}', '() => 1', "new String('str')",
  "new String('')", 'new Number(3)', 'new Boolean(false)', 'new Date(0)', '/re/', "new Map([['k', 1]])",
  "new Set(['s'])", "({ toString() { return 'custom' } })", "({ toString() { return '' } })",
  "({ toString() { return 5 } })", "({ toString() { return { valueOf() { return 'vo' } } } })",
  "({ toString: function () { return 'fn' }, a: true })", "({ toString: Object.prototype.toString, a: 1 })",
  "({ toString: Math.max, b: 1 })", "({ toString: 7 })", "({ toString: undefined, c: 1 })",
  "({ get toString() { throw new Error('toString getter') } })",
  "({ toString() { throw new TypeError('toString throws') } })",
  "Object.assign(Object.create(null), { a: 1 })", "Object.create({ inherited: true })",
  "Object.assign(Object.create({ inherited: true }), { own: 1 })",
  "Object.defineProperty({ vis: 1 }, 'hidden', { value: 1, enumerable: false })",
  "({ get g() { return 1 }, get h() { return 0 } })", "({ get boom() { throw new Error('getter') } })",
  "({ 2: 1, 1: 1, b: 1, a: 1 })", "({ 'a b': 1, '': 1, 'x': '' })",
  "(function () { function C() { this.own = 1 } C.prototype.proto = 1; return new C() })()",
  "(function () { function C() {} C.prototype.toString = function () { return 'proto-custom' }; return new C() })()",
  "(function () { return arguments })('arg', 0, 'x')"]

function value(depth) {
  switch (depth > 2 ? 9 : rand(10)) {
    case 0: case 1: {
      const items = []
      for (let n = rand(5), i = 0; i < n; i++) items.push(value(depth + 1))
      return '[' + items.join(', ') + ']'
    }
    case 2: {
      const props = []
      for (let n = rand(5), i = 0; i < n; i++) props.push(`${pick(["'a'", "'b'", "'c d'", "'0'", "'é'", "'x-y'"])}: ${pick(leaves)}`)
      return '{ ' + props.join(', ') + ' }'
    }
    default: return pick(leaves)
  }
}

const cases = []
const seen = new Set()
for (let attempts = 0; cases.length < 4000 && attempts < 40000; attempts++) {
  const args = []
  for (let n = rand(5), i = 0; i < n; i++) args.push(value(0))
  const src = '[' + args.join(', ') + ']'
  if (seen.has(src)) continue
  seen.add(src)
  try {
    cases.push([src, classNames(...(0, eval)('(' + src + '\n)'))])
  } catch (e) {
    cases.push([src, null, e instanceof Error ? e.name + ': ' + e.message : 'thrown: ' + String(e)])
  }
}
writeFileSync('testdata/classnames.json.gz', gzipSync(JSON.stringify(cases), { level: 9, mtime: 0 }))
console.log(cases.length, 'cases')
