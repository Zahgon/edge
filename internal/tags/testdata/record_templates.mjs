// Records templates.json.gz: random templates (generate.mjs) tokenized and compiled by
// edge.js 6.5.1 the way Compiler.compileRaw does it, on Node 24:
//
//	node record_templates.mjs <seed> <count>   (with the original at /opt/original)
//
// For every template: the tokens as JSON.stringify writes them (partial when the
// tokenizer throws), and the compiled body (stack ids replaced by stack_ID) or the
// error compileRaw throws. Strings that are not well-formed UTF-16 are stored as
// { units: [...] }. Also records the names globalThis defines, which edge-parser
// leaves unresolved.
import { writeFileSync } from 'node:fs'
import { gzipSync } from 'node:zlib'
import { Edge, EdgeError } from '/opt/original/build/index.js'
import { migrate } from '/opt/original/build/src/migrate/plugin.js'
import { Tokenizer } from '/opt/original/node_modules/edge-lexer/build/index.js'
import { generate } from './generate.mjs'

const FILE = 'views\\users/list.edge'

// The claimTag function of the recorded templates: x<Upper> and comp.<...> names are block tags
// with an argument, inl<...> inline tags without one that drop their line break,
// sk<...> inline tags with an argument.
function claim(name) {
  if (/^x[A-Z]/.test(name) || name.startsWith('comp.')) return { seekable: true, block: true }
  if (name.startsWith('inl')) return { seekable: false, block: false, noNewLine: true }
  if (name.startsWith('sk')) return { seekable: true, block: false }
  return null
}

const str = (s) => (s.isWellFormed() ? s : { units: Array.from({ length: s.length }, (_, i) => s.charCodeAt(i)) })

const globals = new Set()
for (let o = globalThis; o; o = Object.getPrototypeOf(o)) {
  for (const k of Object.getOwnPropertyNames(o)) if (globalThis[k] !== undefined) globals.add(k)
}

const records = []
for (const c of generate(Number(process.argv[2]), Number(process.argv[3]))) {
  const edge = Edge.create()
  if (c.migrate) {
    // The plugin registers the tags; layouts are merged by the compiler, not by
    // the tags, so compat mode is turned off again.
    migrate(edge)
    edge.compiler.compat = false
    edge.asyncCompiler.compat = false
  }
  const compiler = c.async ? edge.asyncCompiler : edge.compiler
  if (c.claim) compiler.claimTag(claim)
  const record = { ...c, src: str(c.src) }

  const tokenizer = new Tokenizer(c.src, edge.tags, { filename: FILE, claimTag: c.claim ? claim : undefined })
  try { tokenizer.parse() } catch {}
  record.tokens = JSON.stringify(tokenizer.tokens)

  let compiled = null
  edge.processor.process('compiled', ({ compiled: text }) => { compiled = text })
  try {
    compiler.compileRaw(c.src, FILE)
  } catch (error) {
    // A body V8 cannot turn into a function has been compiled all the same.
    if (compiled === null) {
      record.error = error instanceof EdgeError
        ? { code: error.code, message: str(error.message), filename: error.filename, line: error.line, col: error.col }
        : { name: error.name, message: str(error.message) }
    }
  }
  if (compiled !== null) record.compiled = str(compiled.replace(/stack_[A-Za-z0-9_]+/g, 'stack_ID'))
  records.push(record)
}
writeFileSync('templates.json.gz', gzipSync(JSON.stringify({ file: FILE, globals: [...globals].sort(), cases: records }), { level: 9 }))
