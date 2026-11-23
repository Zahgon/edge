// record.mjs records what Edge's string globals return for generated arguments:
//
//     node testdata/record.mjs   (in internal/strutil, in the UTC time zone, the original at /opt/original)
//
// It writes testdata/globals.json.gz in jsvaltest's format: every case is
// [src, out] or [src, null, "Name: message"], where src is an expression
// evaluating to [name, args] and the global edge.globals[name] is called with
// args (this undefined), in order, in one process, as templates call them. The
// result is rendered by describe below. Inputs come from a fixed
// mulberry32 generator, so the output is reproducible.
import { readFileSync, writeFileSync } from 'node:fs'
import { gzipSync } from 'node:zlib'
import edge from '/opt/original/build/index.js'

let seed = 20261123
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

// describe renders a result: strings and numbers by value (-0 kept), objects
// by their tag and, for arrays and plain objects, their JSON.
function describe(v) {
  if (typeof v === 'string') return 'string ' + v
  if (typeof v === 'number') return 'number ' + (Object.is(v, -0) ? '-0' : String(v))
  if (v === null || typeof v !== 'object') return typeof v + ' ' + String(v)
  return 'object ' + Object.prototype.toString.call(v) + ' ' + JSON.stringify(v)
}

const words = [
  '', ' ', 'hello world', 'HelloWorld', 'helloWorld', 'XMLHttpRequest', 'getHTTPResponseCode', 'hello_world-foo.bar',
  '  spaced  out  ', 'ÀÉÎõü', 'élan vital', 'ǅungla', 'İstanbul', 'straße', '😀 emoji', '\ud800 lone \udc00', 'foo123bar',
  '123abc', 'a1B2c3', '__proto__', 'camelCase', 'snake_case', 'kebab-case', 'Title Case', 'dot.case', 'UPPER CASE',
  'mixed-UP_case.string', 'the lord of the rings', 'a tale: of two cities', 'x-men: the last stand', 'one/two\\three',
  'tab\tnew\nline', 'ǈ', 'ﬁne', 'Σίσυφος ΌΣΟΣ', 'iPhone and iPad', 'v1.2.3', 'e.g. this', 'A', 'a', '-', '--x--',
  'Ünïcödé Wörds', 'café-au-lait', 'ŉ', 'ΐ', 'İi', 'snake__double', '$dollar$sign', 'with   many   spaces',
  'trailing ', ' leading', 'emoji😀Inside', 'a.b.c', 'A.B.C', 'ABC', 'AbcDEF', 'already-dashed-Words', 'to be or not to be',
  'an ode to joy', 'Über cool', 'año nuevo', 'ﬀ ligature', 'Ⅻ roman', 'ǲ digraph', 'ᾈ', '1st place', 'the end:',
  'step: one', 'v2: the sequel', 'what—ever', 'en–dash', "it's", "don't stop", 'O\'Neil', 'node.js', 'iOS app', 'eBay',
  'McDonald', 'TCP/IP', 'R2-D2', 'C-3PO', 'mañana', 'naïve', 'résumé', 'Zürich', 'Ærøskøbing', 'Łódź', 'Ångström',
]

const nonStrings = [
  '5', '-0', 'NaN', 'null', 'undefined', 'true', 'false', '[1, 2]', "['a', 'b']", '[]', '{}', '1.5', '-12',
  "({ toString() { return 'From Obj' } })", "new String('  Str Obj  ')", "({ valueOf() { return 42 } })",
  "({ toString() { throw new Error('no string') } })", "({ trim() { return 'trimmed!' } })",
  "({ trim() { return 7 } })", "({ trim() { return { normalize() { return 'normed' } } } })",
  "({ get length() { throw new Error('length getter') } })", "Object('boxed')", "new Date(0)", "/re/g",
  'function f() {}', "({ length: 3, 0: 'x', 1: 'y', 2: 'z' })", "({ length: 2, 0: 'p', 1: 'q' })",
]

function anyString() {
  return lit(rand(4) === 0 ? pick(words) + pick(words) : pick(words))
}
function anyValue() {
  return rand(5) === 0 ? pick(nonStrings) : anyString()
}

const pluralizeSource = readFileSync('/opt/original/node_modules/pluralize/pluralize.js', 'utf8')
const pluralWords = [...new Set([...pluralizeSource.matchAll(/'([a-z]+)'/gi)].map((m) => m[1]))]
  .concat(['bias', 'biases', 'canvas', 'fez', 'fezzes', 'lens', 'veto', 'vetoes', 'quiche', 'cache',
    'person', 'people', 'Person', 'PEOPLE', 'Child', 'CHILDREN', 'goose', 'geese', 'matrix', 'matrices', 'index',
    'indices', 'octopus', 'cactus', 'status', 'bus', 'hero', 'potato', 'photo', 'leaf', 'life', 'knife', 'thief',
    'mouse', 'ox', 'criterion', 'phenomenon', 'analysis', 'thesis', 'crisis', 'fish', 'news', 'series', 'money',
    'pokemon', 'bison', 'quiz', 'quizzes', 'tooth', 'woman', 'men', 'data', 'datum', 'agenda', 'alumnus', 'genus',
    'die', 'dice', 'passerby', 'mother-in-law', 'Hello World', 'API', 'apis', 'user_id', 'boxes', 'buses', 'axis',
    'Toothpaste', 'sheep', 'Sheep', 'SHEEP', 'my', 'I', 'we', 'he', 'she', 'it', 'this', 'that', 'echo', 'echoes', ''])

function pluralWord() {
  const w = pick(pluralWords)
  switch (rand(6)) {
    case 0: return lit(w.toUpperCase())
    case 1: return lit(w.charAt(0).toUpperCase() + w.slice(1))
    case 2: return lit('some ' + w)
    default: return lit(w)
  }
}

const counts = ['0', '1', '2', '-1', '1.5', "'1'", 'NaN', 'undefined', '-0', '100', "({ valueOf() { return 1 } })"]
const options = {
  dash: ['undefined', '{}', '{ capitalize: true }', '{ capitalize: 0 }', '{ capitalize: "yes" }', 'null', 'true',
    "({ get capitalize() { throw new Error('capitalize getter') } })"],
  dot: ['undefined', '{}', '{ lowerCase: true }', '{ lowerCase: 1 }', 'null', "({ get lowerCase() { return true } })"],
  noCase: ['undefined', 'null', '(input, index) => input.toUpperCase() + index', '(input) => "[" + input + "]"', "'x'",
    '(input, index, all) => all.length', '() => { throw new TypeError("transform") }', '5'],
  sentence: ['undefined', '{}', 'null', "{ separator: ' | ' }", "{ lastSeparator: ' or ' }", "{ pairSeparator: ' & ' }",
    "{ separator: '; ', lastSeparator: '; and ', pairSeparator: ' + ' }", "{ pairSeparator: '' }", "{ separator: 0 }",
    "({ get pairSeparator() { return ' then ' } })"],
  bytes: ['undefined', '{}', 'null', '{ decimalPlaces: 0 }', '{ decimalPlaces: 3 }', '{ fixedDecimals: true }',
    "{ thousandsSeparator: ',' }", "{ thousandsSeparator: '.', decimalPlaces: 1 }", "{ unit: 'KB' }", "{ unit: 'mb' }",
    "{ unit: 'xx' }", "{ unitSeparator: ' ' }", "{ unit: 'B', thousandsSeparator: ' ' }", '{ decimalPlaces: -1 }',
    '{ decimalPlaces: 1.5 }', "{ decimalPlaces: '2' }", '{ fixedDecimals: true, decimalPlaces: 4 }', "{ unit: 'PB' }",
    "{ unit: 'gb', fixedDecimals: true, unitSeparator: '_' }", "{ thousandsSeparator: '$&' }", '{ decimalPlaces: 101 }'],
  truncate: ['undefined', '{}', 'null', '{ completeWords: true }', '{ completeWords: false }', '{ strict: true }',
    '{ strict: false }', "{ suffix: '…' }", '{ suffix: 5 }', "{ suffix: '' }", "{ completeWords: true, suffix: ' [more]' }",
    "{ suffix: '$&$1' }", "{ completeWords: 'yes' }", '{ completeWords: undefined, strict: 1 }', "'opts'", '7',
    "({ get suffix() { throw new Error('suffix getter') } })"],
}

const numbers = ['0', '-0', '1', '-1', '999', '1000', '1500', '59999', '60000', '89999', '90000', '3599999', '3600000',
  '86399999', '86400000', '31557599999', '31557600000', '-1500', '1.5', '1e21', '-1e21', 'NaN', 'Infinity', '-Infinity',
  '0.4', '2.5', '1023', '1024', '1536', '1048575', '1048576', '1073741824', '1099511627776', '1125899906842624',
  '1152921504606846976', '-1024', '2 ** 53', '123456789', '987654321.123', '5e-324', "'1000'", "'abc'", 'null',
  'undefined', 'true', '[]', '[1500]', "({ valueOf() { return 2500 } })", "({ toString() { return '3000' } })"]

const durations = ["'1s'", "'1.5h'", "'2 days'", "'1y'", "'-1m'", "'100'", "'abc'", '5', "''", "'1 hour'", "'1hr'",
  "'10 wks'", "'.5s'", "'1.s'", "'1 MS'", "'2 Minutes'", "'3secs'", "'1 week'", "'2yrs'", "'0s'", "'-.5d'", "'1  s'",
  "' 1s'", "'1e3'", "'1,5s'", 'null', 'undefined', "new String('2h')", "({ toLowerCase() { return '3m' } })", 'NaN',
  "'1d2h'", "'1 dAY'", "'99999999999999999999y'", "'0.000001ms'", "'-0'", "'٣s'"]

const byteTexts = ["'1KB'", "'1.5 mb'", "'1024'", "'abc'", '5', 'null', "'1TB'", "'1pb'", "'-1kb'", "' 10 GB '",
  "'1e3kb'", "'1 b'", "'1B'", "''", "'.5kb'", "'1,024kb'", 'NaN', 'undefined', "'0x10'", "'10 kib'", "'1.5'",
  "new String('2kb')", '[1]', "'1 KB'", "'1  KB'", '-0', "'5gb6'", "'Infinity'", "'1PB'", 'true']

const ordinals = ['1', '2', '3', '4', '11', '12', '13', '21', '22', '23', '101', '111', '112', '-1', '-11', '-21',
  '1.5', "'23'", "'abc'", 'NaN', 'Infinity', "'1e3'", '0', '-0', "'12px'", "' 42 '", 'null', 'undefined', 'true',
  '[3]', "({ valueOf() { return 2 } })", "'0x11'", '1e21', '2.9', "'-3'"]

const sentences = ['', 'short', 'Hello world', '  padded text  ',
  'Lorem ipsum dolor sit amet, consectetur adipiscing elit, sed do eiusmod tempor incididunt',
  '<p>Hello <b>world</b> and <i>everyone</i> here</p>', '<p>one para</p>\n\n<p>two para</p>', '<p>a</p>\n<p>b</p>',
  'line one<br/>line two<br>line three<br />four', '<!-- a comment --> visible text after comment',
  'ÇÜ wörds with àccents çà et là', '<div><span>nested <em>tags</em> that stay open', 'one\n\ntwo\n\nthree paragraphs',
  '<img src="x.png"> image then text <hr/> rule', 'emoji 😀😀😀 sequence of 😀 faces', '\ud800 lone surrogate text',
  'a-b-c d_e_f g.h.i', "don't won't can't shan't", '<a href="/x">link text</a> trailing words here',
  'word '.repeat(12), 'x'.repeat(30), 'tabs\there\tand\tthere', '<p>para</p>', '<</>>', '<b>bold</b>',
  '<p>First</p>\r\n\r\n<p>Second</p>', 'multi\nline\ntext with breaks', '<P>Upper</P> <BR> case tags',
  "<br>", 'ends with tag <b>', 'a < b > c', '<x y="1"><z></x>']

const lengths = ['undefined', '0', '1', '5', '10', '11', '20', '25', '40', '100', "'10'", 'NaN', '-1', 'Infinity', '3.5', 'null']

function argsFor(name) {
  switch (name) {
    case 'camelCase': case 'snakeCase': case 'pascalCase': case 'capitalCase': case 'sentenceCase': case 'titleCase':
      return [anyValue()]
    case 'dashCase': return [anyValue(), pick(options.dash)]
    case 'dotCase': return [anyValue(), pick(options.dot)]
    case 'noCase': return rand(3) ? [anyValue()] : [anyValue(), pick(options.noCase)]
    case 'pluralize': {
      const args = [rand(10) ? pluralWord() : pick(nonStrings)]
      if (rand(4)) args.push(pick(counts))
      if (rand(3) === 0) args.push(pick(['true', 'false', '1', 'undefined']))
      return args
    }
    case 'sentence': {
      if (rand(10) === 0) return [pick(nonStrings), pick(options.sentence)]
      const items = []
      for (let n = rand(6), i = 0; i < n; i++) items.push(rand(4) ? anyString() : pick(nonStrings))
      return ['[' + items.join(', ') + ']', pick(options.sentence)]
    }
    case 'prettyMs': return rand(2) ? [pick(numbers)] : [pick(numbers), pick(['true', 'false', '1', "''", 'undefined'])]
    case 'toMs': return [pick(durations)]
    case 'prettyBytes': return [pick(numbers), pick(options.bytes)]
    case 'toBytes': return [pick(byteTexts)]
    case 'ordinal': return [pick(ordinals)]
    case 'truncate': case 'excerpt': {
      const args = [rand(12) ? lit(pick(sentences)) : pick(nonStrings)]
      if (rand(6)) args.push(pick(lengths))
      if (rand(3)) args.push(pick(options.truncate))
      return args
    }
  }
  throw new Error(name)
}

const plan = {
  camelCase: 450, snakeCase: 450, pascalCase: 450, capitalCase: 450, sentenceCase: 450, titleCase: 600,
  dashCase: 450, dotCase: 450, noCase: 500, pluralize: 2500, sentence: 500, prettyMs: 400, toMs: 300,
  prettyBytes: 800, toBytes: 300, ordinal: 250, truncate: 1500, excerpt: 1500,
}

const cases = []
const names = Object.keys(plan)
const remaining = { ...plan }
while (names.some((n) => remaining[n] > 0)) {
  const name = pick(names)
  if (remaining[name] <= 0) continue
  remaining[name]--
  const src = `[${lit(name)}, [${argsFor(name).join(', ')}]]`
  try {
    const [, args] = (0, eval)('(' + src + '\n)')
    cases.push([src, describe(edge.globals[name](...args))])
  } catch (e) {
    cases.push([src, null, e instanceof Error ? e.name + ': ' + e.message : 'thrown: ' + String(e)])
  }
}
writeFileSync('testdata/globals.json.gz', gzipSync(JSON.stringify(cases), { level: 9, mtime: 0 }))
console.log(cases.length, 'cases')
