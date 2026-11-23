// record.mjs records property-information 7.2.0's find(html, name) after Edge's
// module has added its Alpine.js entries to the html schema:
//
//     node testdata/record.mjs   (in internal/propinfo, the original at /opt/original)
//
// It writes testdata/find.json.gz in jsvaltest's format, [src, out] with src a
// string literal naming the property and out the info rendered by describe, in
// the order the finds ran (find keeps state between calls). The names are every
// property, attribute and normal name of the schema in several spellings, data
// and aria names, Alpine names and names from a fixed mulberry32
// generator.
import { writeFileSync } from 'node:fs'
import { gzipSync } from 'node:zlib'
import '/opt/original/build/index.js'
import { find, html } from '/opt/original/node_modules/property-information/index.js'

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

const flags = ['boolean', 'booleanish', 'overloadedBoolean', 'number', 'spaceSeparated', 'commaSeparated',
  'commaOrSpaceSeparated', 'defined', 'mustUseProperty']

function describe(info) {
  if (info === undefined) return 'undefined'
  return [info.property, info.attribute, String(info.space)].concat(flags.filter((f) => info[f])).join('|')
}

const names = new Set()
for (const [property, info] of Object.entries(html.property)) {
  for (const n of [property, info.attribute, property.toLowerCase(), property.toUpperCase(), info.attribute.toUpperCase()]) names.add(n)
}
for (const n of Object.keys(html.normal)) names.add(n)
const extra = ['', 'constructor', 'CONSTRUCTOR', '__proto__', '__PROTO__', 'toString', 'tostring', 'hasOwnProperty',
  'valueOf', 'prototype', 'data', 'DATA', 'data-', 'data-a', 'data-foo-bar', 'data-foo--bar', 'data-Foo-bar',
  'data-foo_bar', 'data-foo.bar', 'data-foo:bar', 'dataFooBar', 'dataFoo', 'datafoo', 'DATA-FOO', 'Data-Foo-Bar',
  'data-1', 'data1', 'data-', 'data--x', 'data-x-', 'dataX', 'dataXY', 'dataXmlHttp', 'data-é', 'dataé', 'data foo',
  'data-foo bar', 'aria-label', 'ARIA-LABEL', 'ariaLabel', 'aria-unknown', 'ariaUnknown', 'role', 'ROLE', 'x-data',
  'x-cloak', 'X-CLOAK', 'x-ignore', 'x-transition:enterstart', 'x-transition:enterStart', 'x-transition:enter-start',
  'x-transition:leaveend', 'x-transition:LeaveEnd', 'x-on:click', 'x-bind:class', 'x-transition', '@click',
  ':class', 'v-if', 'İd', 'ı', 'K', 'ſrc', 'class', 'className', 'CLASSNAME', 'for', 'htmlFor', 'acceptCharset',
  'accept-charset', 'http-equiv', 'xlink:href', 'xLinkHref', 'xml:lang', 'xmlLang', 'xmlns:xlink', 'xmlnsXLink',
  'xmlns', 'XMLNS', 'unknownAttr', 'unknown-attr', 'onclick', 'onClick', 'ON-CLICK', 'tabindex', 'tab-index',
  'readonly', 'read-only', '😀', '\ud800', 'a b', 'ΣΑΣ', 'dataΣ']
for (const n of extra) names.add(n)

// Data names in sequences: dash.test keeps its lastIndex between finds.
const dataParts = ['foo', 'Foo', 'FOO', 'x', 'X', 'a-b', 'a-B', 'aB', 'Ab', '-', '--', '-x', 'x-', 'b-c-d', '1', '_', '.', ':', 'é']
const generated = []
for (let i = 0; i < 3000; i++) {
  let n = pick(['data', 'data-', 'DATA', 'Data-', 'dAtA'])
  for (let k = 1 + rand(3); k > 0; k--) n += pick(dataParts)
  generated.push(n)
}
const all = [...names]
for (let i = 0; i < 2000; i++) generated.push(pick(all))

const cases = []
for (const n of [...all, ...generated]) cases.push([lit(n), describe(find(html, n))])
writeFileSync('testdata/find.json.gz', gzipSync(JSON.stringify(cases), { level: 9, mtime: 0 }))
console.log(cases.length, 'cases')
