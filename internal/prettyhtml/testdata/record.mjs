// record.mjs records what @poppinss/inspect 1.0.1 prints for generated values.
//
//     node record.mjs                   writes html.json.gz and inspect.json.gz (time zone UTC)
//     node record.mjs                   writes dates-Asia_Kolkata.json.gz (time zone Asia/Kolkata)
//
// Run it in node:24 with the original mounted at /opt/original and this directory
// as the working directory. Every case is [src, out] or [src, null, "Name: message"],
// where src is a JavaScript expression building the input (for inspect.json.gz, an
// array [value, options]); the Go tests evaluate the same expression in the
// interpreter. The values come from a fixed mulberry32 generator, so the
// output is reproducible.
import { createRequire } from 'node:module'
import { writeFileSync } from 'node:fs'
import { gzipSync } from 'node:zlib'

const require = createRequire(import.meta.url)
const inspect = require('/opt/original/node_modules/@poppinss/inspect/index.js')

let seed = 20251123
function rand(n) {
  seed = (seed + 0x6d2b79f5) | 0
  let t = Math.imul(seed ^ (seed >>> 15), 1 | seed)
  t = (t + Math.imul(t ^ (t >>> 7), 61 | t)) ^ t
  return Math.floor((((t ^ (t >>> 14)) >>> 0) / 4294967296) * n)
}
const pick = (xs) => xs[rand(xs.length)]

// lit writes a string literal with every character outside printable ASCII escaped.
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

const specials = [' ', ' ', '😀', '\ud800', '\udc00', 'é', 'Ω', '<', '>', '&', '"', "'", '\\', '\\"', ') [', ': ', '"a": ', ' => ', 'n', ' ']
function randomString() {
  const n = rand(8)
  let s = ''
  for (let i = 0; i < n; i++) {
    s += rand(3) === 0 ? pick(specials) : String.fromCharCode(rand(128))
  }
  return s
}

const numbers = ['0', '-0', '1', '-1', '1.5', '-2.25', '1e21', '1e-7', '123456789012', 'NaN', 'Infinity', '-Infinity', '0.1 + 0.2', '2 ** 53', '-1e-300', '5e-324']
const keys = ['a', 'b', 'key_1', '$x', 'a b', 'a-b', '0', '1', '10', '-1', '1.5', 'é', '', '"q"', "it's", '<k>', 'line\nbreak', 'x y', 'Infinity', 'NaN', '01', 'length', 'n"']
const dates = ['0', '1e12', '-1e12', '1700000000123', 'NaN', '8.64e15', '-62198755200000', '951782400000']

function primitive() {
  switch (rand(6)) {
    case 0: return pick(['undefined', 'null', 'true', 'false'])
    case 1: case 2: return pick(numbers)
    default: return lit(randomString())
  }
}

function key() {
  return rand(4) === 0 ? lit(randomString()) : lit(pick(keys))
}

// arrayKey is a key to add to a two-item array without leaving holes in it or
// changing its length, which the interpreter's arrays (they have no holes) cannot
// represent.
function arrayKey() {
  return lit(pick(keys.filter((k) => k !== '10' && k !== 'length')))
}

function leaf() {
  switch (rand(16)) {
    case 0: return `new Date(${pick(dates)})`
    case 1: return pick(['/a+b/gi', '/[/]\\//', 'new RegExp("x\\"y", "m")', '/(?:)/'])
    case 2: return pick(['new Error("boom")', 'new TypeError("t\\"y<pe>")', 'new RangeError()', '(function () { var e = new Error("m"); e.code = "E_X"; return e })()', '(function () { var e = new Error("n"); e.message = "changed <m>"; return e })()', '(function () { var e = new SyntaxError("s"); e["a b"] = [1]; e.nested = { x: 1 }; return e })()'])
    case 3: return pick(['function foo() {}', 'function () {}', '() => 1', 'async function bar() {}', '(function () { var f = function () {}; return f })()', 'Object.defineProperty(function () {}, "name", { value: "" })', 'Object.defineProperty(function named() {}, "name", { value: "renamed" })', 'Object.defineProperty(function () {}, "name", { value: "a<b>" })', 'Math.max', 'Object.assign(function z() {}, { prop: 1 })'])
    case 4: return pick(['Object(1)', 'Object(-0)', 'Object("s\'q")', 'Object(true)', 'new Boolean(false)', 'new Number(NaN)', 'new String("a\\nb")', 'Object("")'])
    case 5: return pick(['new WeakMap()', 'new WeakSet()', 'new Map()', 'new Set()'])
    default: return primitive()
  }
}

function value(depth) {
  if (depth <= 0) return leaf()
  switch (rand(14)) {
    case 0: case 1: {
      const n = rand(5)
      const items = []
      for (let i = 0; i < n; i++) items.push(value(depth - 1))
      return '[' + items.join(', ') + ']'
    }
    case 2: case 3: case 4: {
      const n = rand(5)
      const props = []
      for (let i = 0; i < n; i++) props.push(key() + ': ' + value(depth - 1))
      return '{ ' + props.join(', ') + ' }'
    }
    case 5: {
      const n = rand(4)
      const entries = []
      for (let i = 0; i < n; i++) entries.push('[' + (rand(2) ? key() : value(depth - 1)) + ', ' + value(depth - 1) + ']')
      return 'new Map([' + entries.join(', ') + '])'
    }
    case 6: {
      const n = rand(4)
      const items = []
      for (let i = 0; i < n; i++) items.push(value(depth - 1))
      return 'new Set([' + items.join(', ') + '])'
    }
    case 7: return `Object.assign([${value(depth - 1)}, ${value(depth - 1)}], { ${arrayKey()}: ${value(depth - 1)} })`
    case 8: return special(depth)
    default: return leaf()
  }
}

function special(depth) {
  const v = () => value(depth - 1)
  switch (rand(16)) {
    case 0: return `(function () { var o = { a: ${v()} }; o.self = o; return o })()`
    case 1: return `(function () { var a = [${v()}]; a.push(a); return a })()`
    case 2: return `(function () { var a = { n: 1 }, b = { a: a }; a.b = b; return [a, b] })()`
    case 3: return `(function () { var m = new Map(); m.set("me", m); m.set(m, ${v()}); return m })()`
    case 4: return `(function () { var s = new Set(); s.add(s); s.add(${v()}); return s })()`
    case 5: return `({ get g() { return ${v()} }, h: 1 })`
    case 6: return `({ before: 1, get bad() { throw new Error("getter " + ${lit(randomString())}) } })`
    case 7: return `({ toString: function () { return ${lit(randomString())} }, valueOf: function () { return 42 } })`
    case 8: return `Object.assign(Object.create(null), { ${key()}: ${v()} })`
    case 9: return `Object.assign(Object.create({ inherited: ${v()}, shared: 2 }), { own: ${v()} })`
    case 10: return `(function () { function Point(x) { this.x = x } Point.prototype.norm = function () {}; Point.prototype.tag = "p"; return new Point(${v()}) })()`
    case 11: return `({ nodeName: ${lit(pick(['DIV', 'Span', 'İX', 'svg:Path', randomString()]))}, getAttribute: function () {}, attributes: [{ name: "id", value: ${lit(randomString())} }, { name: "data-x", value: ${v()} }], childNodes: ${pick(['[]', '[1]', 'null', '{ length: 0 }', '{ length: "2" }'])} })`
    case 12: return `({ nodeName: "P", getAttribute: function () {}, attributes: ${pick(['null', '[]', '{ length: 1, 0: { name: "a", value: "\\"q\\"" } }', '{ length: "1", 0: { name: 5, value: null } }'])} })`
    case 13: return `(function () { return arguments })(${v()}, ${v()})`
    case 14: return `Object.defineProperty({ visible: ${v()} }, "hidden", { value: 1, enumerable: false })`
    default: return `({ get nodeName() { throw new TypeError("no node") } })`
  }
}

function describe(e) {
  return e instanceof Error ? e.name + ': ' + e.message : 'thrown: ' + String(e)
}

function run(src, fn) {
  try {
    return [src, fn((0, eval)('(' + src + '\n)'))]
  } catch (e) {
    return [src, null, describe(e)]
  }
}

function write(file, cases) {
  writeFileSync(file, gzipSync(JSON.stringify(cases), { level: 9, mtime: 0 }))
}

const tz = process.env.TZ
if (tz === 'UTC') {
  const html = []
  for (let i = 0; i < 128; i++) html.push(run(lit(String.fromCharCode(i)), inspect.string.html))
  for (const s of specials) html.push(run(lit(s), inspect.string.html))
  for (const n of numbers) html.push(run(n, inspect.string.html))
  for (let i = 0; i < 400; i++) html.push(run(leaf(), inspect.string.html))
  for (let i = 0; i < 300; i++) html.push(run(special(3), inspect.string.html))
  for (let i = 0; i < 1600; i++) html.push(run(value(1 + rand(8)), inspect.string.html))
  for (let d = 1; d <= 9; d++) {
    let src = '1'
    for (let i = 0; i < d; i++) src = rand(2) ? `{ k${i}: ${src}, s: "x" }` : `[${src}, "y"]`
    html.push(run(src, inspect.string.html))
  }
  write('html.json.gz', html)

  const options = ['{}', '{ quoteStyle: "single" }', '{ quoteStyle: "double" }', '{ quoteStyle: "bad" }', '{ noWrap: true }', '{ noWrap: true, quoteStyle: "double" }', '{ depth: 0 }', '{ depth: 1 }', '{ depth: 3 }', '{ depth: 2, noWrap: true }']
  const insp = []
  for (let i = 0; i < 1500; i++) {
    insp.push(run(`[${value(rand(7))}, ${pick(options)}]`, ([v, o]) => inspect.inspect(v, o)))
  }
  write('inspect.json.gz', insp)
} else {
  const cases = []
  for (const d of dates) {
    cases.push(run(`new Date(${d})`, inspect.string.html))
    cases.push(run(`{ at: new Date(${d}), list: [new Date(${d})] }`, inspect.string.html))
    cases.push(run(`new Map([[new Date(${d}), new Date(${d})]])`, inspect.string.html))
  }
  write('dates-' + tz.replace('/', '_') + '.json.gz', cases)
}
