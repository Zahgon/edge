// sources.mjs records testdata/sources.json, from which gen/main.go writes
// sources_gen.go: the text Function.prototype.toString returns for each function
// and class Edge hands to templates (the globals, the Template, Stacks,
// ComponentProps, Props, SafeValue and EdgeError classes and their methods, the
// macros the bundled tags boot), as the built original defines them. Run it in
// internal/edgert with the built edge.js at /opt/original:
//
//     node testdata/sources.mjs > testdata/sources.json
import { Edge, EdgeError, Template, edgeGlobals } from '/opt/original/build/index.js'
import { migrate } from '/opt/original/build/src/migrate/plugin.js'
import { readFileSync } from 'node:fs'

const sources = new Map()
const add = (key, fn) => sources.set(key, Function.prototype.toString.call(fn))
const addClass = (key, cls) => {
  add(key, cls)
  for (const name of Object.getOwnPropertyNames(cls.prototype)) {
    if (name !== 'constructor') add(`${key}.${name}`, cls.prototype[name])
  }
}

for (const [name, value] of Object.entries(edgeGlobals)) {
  if (typeof value === 'function') add(`globals.${name}`, value)
  else for (const [key, fn] of Object.entries(value)) add(`globals.${name}.${key}`, fn)
}

// The migrate plugin's globals: the values it replaces or adds.
const compat = Edge.create()
const before = { ...compat.globals }
compat.use(migrate)
compat.createRenderer()
for (const [name, value] of Object.entries(compat.globals)) {
  if (value !== before[name]) add(`migrate.${name}`, value)
}

addClass('Template', Template)
const Macroable = Object.getPrototypeOf(Template)
add('Macroable', Macroable)
for (const name of ['macro', 'getter', 'instanceProperty']) add(`Macroable.${name}`, Macroable[name])
// The get function Macroable.getter defines.
class Sample extends Macroable {}
Sample.getter('sample', () => undefined)
add('Macroable.getter.get', Object.getOwnPropertyDescriptor(Sample.prototype, 'sample').get)

// The classes Edge does not export are reached through the objects templates
// receive: a custom tag hands them to a global.
let seen
const grab = {
  tagName: 'grab', block: false, seekable: false,
  compile(parser, buffer, token) {
    buffer.writeExpression('state.grab({ stacks: template.stacks, props: state.$props })', token.filename, 1)
  },
}
for (const edge of [Edge.create(), compat]) {
  edge.registerTag(grab)
  edge.global('grab', (objects) => { seen = objects })
  edge.registerTemplate('grab-component', { template: '@grab' })
  edge.renderRawSync('@!component("grab-component", {})')
  const cls = seen.props.constructor
  addClass(cls.name, cls)
  if (cls.name === 'ComponentProps') add('ComponentProps.create', cls.create)
}
addClass('Stacks', seen.stacks.constructor)
add('SafeValue', edgeGlobals.html.safe('').constructor)
add('EdgeError', EdgeError)

const recording = {
  edge: JSON.parse(readFileSync('/opt/original/package.json', 'utf8')).version,
  node: process.version,
  sources: Object.fromEntries([...sources].sort(([a], [b]) => (a < b ? -1 : a > b ? 1 : 0))),
}
process.stdout.write(JSON.stringify(recording, null, 2) + '\n')
