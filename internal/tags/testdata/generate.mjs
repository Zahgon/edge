// Generates random Edge templates for the recorded compile tests: text, mustaches, comments,
// tags with valid and invalid arguments, nested blocks, components with slots,
// claimed and unknown tags, escaped tags and mustaches, \r\n and \r line endings,
// unicode and lone surrogates. The sequence depends only on the seed.
export function generate(initialSeed, count) {
  let seed = initialSeed >>> 0
  // mulberry32
  function rnd() {
    seed = (seed + 0x6d2b79f5) >>> 0
    let t = seed
    t = Math.imul(t ^ (t >>> 15), t | 1)
    t ^= t + Math.imul(t ^ (t >>> 7), t | 61)
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296
  }
  const pick = (a) => a[Math.floor(rnd() * a.length)]
  const exprs = [
    'a', 'user.name', 'a + b', "'str'", '"x}}y"', '`t ${a} u`', 'a ? b : c', 'a, b', 'fn(a, b)', 'a.b.c()',
    '[1, 2, ...xs]', '{ a: 1, b }', '{ ...o, [k]: v }', 'x => x * 2', '(x, y) => { return x + y }', '({a, b}) => a',
    '([a, b]) => b', 'a?.b?.c', 'a ?? b', 'new Date(a)', '!a', 'typeof a', 'a++', '--b', 'a = 1', 'a += 2',
    'await a', 'this.a', 'Math.max(a, 1)', 'JSON.stringify(a)', 'undefined', 'NaN', 'Infinity', 'null', 'true',
    '/re+/gi', '1e21', '0x1F', '1n', "'日本😀'", 'a["b"]', 'a[b]', 'function foo() {}', 'class A {}', 'if (a) {}',
    'a b', '(', 'a +', '}', "'unterminated", 'let x = 1', ';', '', ' ', 'a\n+ b', '`multi\nline ${a}`',
    'x in list', '(item, i) in list', 'item in [1,2]', 'item in items', '(v, k) in obj', '(v, k, z) in obj', 'x in y.z', 'a = b, c', "'k', a + 1", "a, 'k.l', 1", "1n, 2", "null, 2", "/r/g, 2", "'a b', 1", "true, 1", "user, 'a', 1", '(a, b, c) in list', 'a.b in c', '[a] in c',
    "'name', { a: 1 }", "'main'", "'main', props", "'a', b, c", 'a = 1, b', "'k', 'v'", "obj, 'k', 'v'",
    'constructor', 'toString()', 'hasOwnProperty("a")', 'process.env', 'console.log(a)', 'require("x")',
    'globalThis.x', 'Buffer.from(a)', 'setTimeout', 'Symbol.iterator', '(a = 1) => a', '(...r) => r', '({...r}) => r',
    '([, b]) => b', '({a: {b}}) => b', '[1,,2]', '() => { return }', '() => { if (a) {} }', 'async () => 1',
    'function () {}', 'a.#b', 'super.x', 'import("x")', 'new.target', 'yield a', '\\u0061', "'\\ud800'", "'" + String.fromCharCode(0xd800) + "' + a",
    '"\u2028"', 'a /* c */ + b', 'a // c', '\u00a0a', 'é', '𝒜', 'a\u2028b', '{a:1}.a', '{{ a }}',
    "'a', 'b', 3, 4", "'msg'", "'msg', 'file.edge'", "'msg', 'f', 1, 2", "'msg', 'f', 1, 2, 3",
    "({a} = b)", 'a = b = c', 'x = y', "'nested', { a: { b: [1, { c }] } }", "a ? 'x' : 'y'", "a || 'b'",
    "`${a}`", "'stack-' + a", "1", "{ a: 1 }", "foo, bar", "(a, b)", "'x'", "x", "[x, y] = z", "{ x, y: [z] } = w",
    "x = 1", "[a, ...b] = c", "{...a} = b", "[[a]] = b", "{a: {b}} = c", "[a = 1] = b", "[, a] = b", "1 = 2", "a.b = c",
    "a.b, 'c', d", "1, 2", "/a/g, 2", "1n, 2", "'path', value", "user, 'name', 'virk'", "a, b, c, d",
  ]
  const names = ['a', 'b', 'user', 'item', 'i', 'x', 'props', 'state', '$filename', '$context', 'Math', 'JSON', 'undefined', 'escape', 'toString', 'constructor', 'é', '𝒜', 'await', 'let', 'async', 'of', 'get']
  const ops = ['+', '-', '*', '/', '%', '**', '==', '===', '!=', '<', '<=', '>>>', '&', '|', '^', '&&', '||', '??', 'in', 'instanceof', ',', '=', '+=', '??=']
  function composed(depth) {
    if (depth > 2 || rnd() < 0.25) return rnd() < 0.6 ? pick(names) : pick(exprs)
    const e = () => composed(depth + 1)
    switch (Math.floor(rnd() * 16)) {
      case 0: return `${e()} ${pick(ops)} ${e()}`
      case 1: return `${pick(names)}(${e()}, ${e()})`
      case 2: return `(${pick(names)}, ${pick(names)}) => ${e()}`
      case 3: return `${pick(names)} => { return ${e()} }`
      case 4: return `${e()}.${pick(names)}`
      case 5: return `${e()}[${e()}]`
      case 6: return `[${e()}, ...${e()}]`
      case 7: return `{ ${pick(names)}: ${e()}, ${pick(names)}, [${e()}]: 1 }`
      case 8: return `${e()} ? ${e()} : ${e()}`
      case 9: return `await ${e()}`
      case 10: return '`a${' + e() + '}b${' + e() + '}`'
      case 11: return `(${e()})`
      case 12: return `${e()}?.${pick(names)}?.(${e()})`
      case 13: return `new ${pick(names)}(${e()})`
      case 14: return `({ ${pick(names)}, ...${pick(names)} }) => ${e()}`
      default: return `([${pick(names)}, ${pick(names)}]) => ${e()}`
    }
  }
  const words = ['hello', 'world', '日本', '😀', 'x\u2028y', '\u00a0', '\ufeff', 'tab\there', '<p>', '</div>', '~', '@', '@@', '{', '}', '}}', '{ {', '@ if', 'email@example.com', '(', ')', '\ud800', '\udc00z', '-->', '\\']
  const tagNames = ['if', 'elseif', 'else', 'unless', 'each', 'let', 'assign', 'eval', 'include', 'includeIf', 'component', 'slot', 'inject', 'newError', 'stack', 'pushTo', 'pushOnceTo', 'debugger', 'set', 'section', 'layout', 'super', 'xButton', 'comp.card', 'inlFoo', 'skBar', 'unknown', 'toString', 'constructor', '__proto__', 'hasOwnProperty', 'valueOf', 'end', 'endif']
  const blockNames = ['if', 'unless', 'each', 'each', 'component', 'component', 'slot', 'pushTo', 'pushOnceTo', 'section', 'xButton', 'comp.card']
  function ws() { return pick(['', '', '', ' ', '  ', '\t', '\u00a0', '   ']) }
  function eol() { return pick(['\n', '\n', '\n', '\r\n', '\r']) }
  function expr() { return rnd() < 0.5 ? pick(exprs) : composed(0) }
  function mustache() {
    const e = expr()
    return pick([
      `{{ ${e} }}`, `{{${e}}}`, `{{{ ${e} }}}`, `@{{ ${e} }}`, `@{{{ ${e} }}}`, `{{-- ${pick(words)} --}}`, `{{ ${e}`, `{{ ${e} }`,
      `{{\n  ${e}\n}}`, `{{-- multi\nline --}}`, `{{ ${e} }}${pick(words)}{{ ${pick(exprs)} }}`, `${pick(words)} {{ ${e} }} ${pick(words)}`, `{{ ${e} }}   `,
    ])
  }
  function tagLine() {
    const name = pick(tagNames)
    const e = expr()
    return ws() + pick(['@', '@', '@', '@@', '@!']) + name + pick(['', '', ' ', '  ', '   ']) + pick([`(${e})`, `(${e})`, '', `(${e}) ~`, `(${e})~`, `(${e}) trailing`, `(\n  ${e}\n)`, `(${e}`, `(${e}))`, '()', ` text`, `(${e}) {{ a }}`, '~'])
  }
  const slotArgs = ["'main'", "'a'", "'b', p", "'b', props", '1', '2', "'x'", 'name', "'a', 'b'", "'a', p, q", '', "'a', p.q", '"main"']
  function slot(depth) {
    return `${ws()}@slot(${pick(slotArgs)})${pick(['', '~'])}${eol()}` + body(depth + 1) + pick(['@endslot', '@end', '@endslot~', '']) + eol()
  }
  function block(depth) {
    const name = pick(blockNames)
    const e = rnd() < 0.3 ? expr() : pick(exprs)
    let s = `${ws()}@${pick(['', '', '', '!'])}${name}(${e})${pick(['', '', '~', ' '])}${eol()}`
    if (/component|xButton|comp\.card/.test(name)) for (let n = Math.floor(rnd() * 3); n > 0; n--) s += rnd() < 0.6 ? slot(depth) : body(depth + 1)
    s += body(depth + 1)
    s += pick([`@end${name}`, '@end', `@end${name}~`, '@end~', `  @end${name}  `, '', `@end${pick(blockNames)}`])
    return s
  }
  function body(depth) {
    let s = ''
    const n = Math.floor(rnd() * 4)
    for (let i = 0; i < n; i++) {
      const r = rnd()
      if (r < 0.3) s += mustache()
      else if (r < 0.55) s += tagLine()
      else if (r < 0.7 && depth < 3) s += block(depth)
      else s += pick(words) + ws() + pick(words)
      s += eol()
    }
    return s
  }
  const cases = []
  for (let i = 0; i < count; i++) {
    let src = body(0)
    if (rnd() < 0.3) src += body(0)
    if (rnd() < 0.2) src = src.trimEnd()
    cases.push({ src, claim: rnd() < 0.5, migrate: rnd() < 0.4, async: rnd() < 0.5 })
  }
  return cases
}
