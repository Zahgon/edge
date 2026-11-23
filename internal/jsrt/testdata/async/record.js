// record.js records what Node does with the async cases below; the Go test
// (async_test.go) replays them against the interpreter. Run it with the reference
// Node (v24) from the repository root:
//
//   node internal/jsrt/testdata/async/record.js | gzip -9n > internal/jsrt/testdata/async/cases.json.gz
//
// Each case body is compiled as an async template function is, with
// the AsyncFunction constructor (new AsyncFunction(src)), or with Function when
// "sync" is set, and called with no arguments; an async case's promise is awaited.
// The recorder adds to every case:
//
//   "ret"     the result: a string as it is, anything else as
//             "(<typeof>) <String(value)>";
//   "err"     when the compilation, the call or the promise fails:
//             "<name>: <message>" for an Error, "thrown <typeof> <String(value)>"
//             otherwise;
//   "stdout"  what the case printed with console.log.
//
// Cases run one after another, each awaited before the next starts. They keep
// their state in local variables, so jobs a case leaves queued cannot affect the
// next one. Unhandled rejections are ignored, as the interpreter ignores them.
'use strict';

const cases = [
  // Results and rejections.
  { name: 'return value', src: 'return 1' },
  { name: 'return await', src: 'return await 1' },
  { name: 'return string', src: 'return await "s" + await "t"' },
  { name: 'throw error', src: 'throw new TypeError("bad")' },
  { name: 'throw primitive', src: 'throw "boom"' },
  { name: 'await rejected primitive', src: 'await Promise.reject(5)' },
  { name: 'await rejected error', src: 'await Promise.reject(new RangeError("r"))' },
  { name: 'return rejected promise', src: 'return Promise.reject(new Error("inner"))' },
  { name: 'undefined result', src: 'await null' },
  { name: 'object result', src: 'return await {a: 1}' },
  { name: 'error object identity', src: 'var e = new Error("x"); try { await Promise.reject(e) } catch (c) { return String(c === e) }' },

  // Ordering of async functions and reactions.
  {
    name: 'interleaved async functions',
    src: `var log = [];
async function a() { log.push('a1'); await null; log.push('a2'); await null; log.push('a3'); }
async function b() { log.push('b1'); await null; log.push('b2'); await null; log.push('b3'); }
var pa = a(), pb = b(); log.push('sync');
await pa; await pb;
return log.join(',');`,
  },
  {
    name: 'then chain versus awaits',
    src: `var log = [];
Promise.resolve().then(() => log.push('t1')).then(() => log.push('t2')).then(() => log.push('t3'));
(async () => { log.push('a0'); await undefined; log.push('a1'); await undefined; log.push('a2'); })();
log.push('sync');
for (var i = 0; i < 10; i++) await null;
return log.join(',');`,
  },
  {
    name: 'await native, thenable and value',
    src: `var log = [];
(async () => { await {then(r) { log.push('then called'); r(); }}; log.push('thenable'); })();
(async () => { await Promise.resolve(); log.push('native'); })();
(async () => { await 1; log.push('value'); })();
Promise.resolve().then(() => log.push('p1')).then(() => log.push('p2')).then(() => log.push('p3')).then(() => log.push('p4'));
for (var i = 0; i < 10; i++) await null;
return log.join(',');`,
  },
  {
    name: 'returning a promise costs ticks',
    src: `var log = [];
async function f() { return Promise.resolve('promise'); }
async function g() { return 'value'; }
async function h() { return {then(r) { r('thenable'); }}; }
f().then(v => log.push(v)); g().then(v => log.push(v)); h().then(v => log.push(v));
Promise.resolve().then(() => log.push(1)).then(() => log.push(2)).then(() => log.push(3)).then(() => log.push(4)).then(() => log.push(5));
for (var i = 0; i < 10; i++) await null;
return log.join(',');`,
  },
  {
    name: 'resolve with a promise',
    src: `var log = [];
new Promise(r => r(Promise.resolve('adopted'))).then(v => log.push(v));
new Promise(r => r('direct')).then(v => log.push(v));
Promise.resolve().then(() => log.push(1)).then(() => log.push(2)).then(() => log.push(3));
for (var i = 0; i < 10; i++) await null;
return log.join(',');`,
  },
  {
    name: 'executor runs synchronously',
    src: `var log = [];
var p = new Promise((res, rej) => { log.push('executor'); res('first'); res('second'); rej(new Error('late')); log.push('after resolve'); });
log.push('constructed');
log.push(await p);
return log.join(',');`,
  },
  {
    name: 'executor throws',
    src: `var log = [];
try { await new Promise(() => { throw new Error('in executor'); }); } catch (e) { log.push(e.message); }
log.push(await new Promise(res => { res('kept'); throw new Error('ignored'); }));
return log.join(',');`,
  },
  {
    name: 'sync part runs first',
    src: `var log = [];
async function f() { log.push('f start'); await 0; log.push('f end'); return 'f'; }
var p = f();
log.push('caller');
log.push(await p);
return log.join(',');`,
  },
  {
    name: 'nested async calls',
    src: `var log = [];
async function inner(n) { log.push('in' + n); await null; log.push('out' + n); return n * 2; }
async function outer() { var a = await inner(1); var b = await inner(2); return a + b; }
var p = outer(); log.push('sync');
log.push(await p);
return log.join(',');`,
  },
  {
    name: 'three loops interleave',
    src: `var log = [];
async function loop(tag, n) { for (var i = 0; i < n; i++) { log.push(tag + i); await null; } return tag; }
var all = await Promise.all([loop('a', 3), loop('b', 2), loop('c', 4)]);
return log.join(',') + '|' + all.join(',');`,
  },
  {
    name: 'then handlers order',
    src: `var log = [];
var p = Promise.resolve('v');
p.then(v => log.push('1' + v)); p.then(v => log.push('2' + v));
p.then(() => { log.push('3'); return Promise.resolve(); }).then(() => log.push('3b'));
p.then(() => log.push('4')).then(() => log.push('4b')).then(() => log.push('4c')).then(() => log.push('4d'));
for (var i = 0; i < 10; i++) await null;
return log.join(',');`,
  },
  {
    name: 'pending promise resolved later',
    src: `var log = []; var resolve;
var p = new Promise(r => { resolve = r; });
p.then(v => log.push('then ' + v));
(async () => { log.push('await ' + await p); })();
log.push('before');
resolve('x');
log.push('after');
for (var i = 0; i < 5; i++) await null;
return log.join(',');`,
  },

  // Promise combinators.
  {
    name: 'Promise.all order',
    src: `var log = [];
async function late(v, n) { for (var i = 0; i < n; i++) await null; log.push(v); return v; }
var r = await Promise.all([late('a', 3), late('b', 1), 'c', Promise.resolve('d'), late('e', 0)]);
return log.join(',') + '|' + r.join(',');`,
  },
  {
    name: 'Promise.all rejection',
    src: `async function late(v, n, fail) { for (var i = 0; i < n; i++) await null; if (fail) throw new Error(v); return v; }
try { await Promise.all([late('a', 3), late('b', 1, true), late('c', 0, true)]); } catch (e) { return 'rejected ' + e.message; }`,
  },
  {
    name: 'Promise.all values',
    src: `var r = await Promise.all('ab'); var e = await Promise.all([]);
return JSON.stringify(r) + JSON.stringify(e) + Array.isArray(e);`,
  },
  { name: 'Promise.all not iterable', src: 'await Promise.all(1)' },
  { name: 'Promise.all object', src: 'await Promise.all({})' },
  { name: 'Promise.all undefined', src: 'await Promise.all()' },
  { name: 'Promise.race not iterable', src: 'await Promise.race(null)' },
  {
    name: 'Promise.allSettled',
    src: `var r = await Promise.allSettled([1, Promise.reject(new Error('no')), Promise.resolve('yes'), Promise.reject('raw')]);
return r.map(x => x.status + ':' + (x.status === 'fulfilled' ? x.value : String(x.reason))).join(',') + '|' + JSON.stringify(r[0]) + Object.keys(r[1]).join();`,
  },
  {
    name: 'Promise.any fulfils',
    src: 'return await Promise.any([Promise.reject(1), Promise.resolve(2), 3])',
  },
  {
    name: 'Promise.any rejects',
    src: `try { await Promise.any([Promise.reject(1), Promise.reject(new Error('two'))]); }
catch (e) { return e.name + '|' + e.message + '|' + e.errors.length + '|' + String(e.errors[1]) + '|' + (e instanceof AggregateError) + (e instanceof Error); }`,
  },
  {
    name: 'Promise.any empty',
    src: 'try { await Promise.any([]); } catch (e) { return e.name + ": " + e.message + " " + e.errors.length; }',
  },
  {
    name: 'Promise.race',
    src: `async function late(v, n) { for (var i = 0; i < n; i++) await null; return v; }
var a = await Promise.race([late('slow', 3), late('fast', 1)]);
var b; try { await Promise.race([late('slow', 2), Promise.reject(new Error('rej'))]); } catch (e) { b = e.message; }
return a + ',' + b;`,
  },
  {
    name: 'combinator ordering',
    src: `var log = [];
Promise.all([1, 2]).then(() => log.push('all'));
Promise.race([1, 2]).then(() => log.push('race'));
Promise.allSettled([1]).then(() => log.push('settled'));
Promise.any([1]).then(() => log.push('any'));
Promise.resolve().then(() => log.push(1)).then(() => log.push(2)).then(() => log.push(3));
for (var i = 0; i < 10; i++) await null;
return log.join(',');`,
  },

  // Prototype methods.
  {
    name: 'then passes through non-functions',
    src: `var a = await Promise.resolve('v').then(null, 5).then(undefined);
var b = await Promise.reject('r').then(x => 'no').catch(x => 'caught ' + x);
return a + ',' + b;`,
  },
  {
    name: 'catch returns a value',
    src: 'return await Promise.reject(new Error("e")).catch(e => e.message + "!").then(v => v + "?")',
  },
  {
    name: 'finally',
    src: `var log = [];
var a = await Promise.resolve('kept').finally(() => { log.push('f1'); return 'ignored'; });
var b; try { await Promise.reject(new Error('orig')).finally(() => log.push('f2')); } catch (e) { b = e.message; }
var c; try { await Promise.resolve(1).finally(() => { throw new Error('from finally'); }); } catch (e) { c = e.message; }
var d; try { await Promise.resolve(1).finally(() => Promise.reject('rejected finally')); } catch (e) { d = e; }
var e = await Promise.resolve('no handler').finally();
return log.join(',') + '|' + [a, b, c, d, e].join(',');`,
  },
  {
    name: 'finally ordering',
    src: `var log = [];
Promise.resolve().finally(() => log.push('finally')).then(() => log.push('after finally'));
Promise.resolve().then(() => log.push(1)).then(() => log.push(2)).then(() => log.push(3)).then(() => log.push(4)).then(() => log.push(5));
for (var i = 0; i < 10; i++) await null;
return log.join(',');`,
  },
  {
    name: 'handler throws',
    src: `try { await Promise.resolve().then(() => { throw new SyntaxError('in handler'); }); } catch (e) { return e.name + ' ' + e.message; }`,
  },
  {
    name: 'thenable throws',
    src: `var log = [];
try { await {then() { throw new Error('then threw'); }}; } catch (e) { log.push(e.message); }
log.push(await {then(r) { r('first'); throw new Error('ignored'); }});
try { await {then(r, j) { j('rejected'); r('late'); }}; } catch (e) { log.push(e); }
return log.join(',');`,
  },
  {
    name: 'chaining cycle',
    src: 'var p = Promise.resolve().then(() => p); await p;',
  },
  {
    name: 'Promise.resolve identity',
    src: `var p = Promise.resolve(1); var t = {then(r) { r(2); }};
return [Promise.resolve(p) === p, Promise.resolve(t) === t, await Promise.resolve(t), Promise.reject(1) === Promise.reject(1)].join(',');`,
  },

  {
    name: 'then getter',
    src: `var log = [];
function thenGetter(get, o) { return Object.defineProperty(o || {}, 'then', {get: get}); }
var v = await thenGetter(function () { log.push('get then'); return undefined; }, {x: 1});
var w = await thenGetter(function () { log.push('get function'); return function (r) { r('via getter'); }; });
try { await thenGetter(function () { throw new Error('getter threw'); }); } catch (e) { log.push(e.message); }
return log.join(',') + '|' + v.x + '|' + w;`,
  },
  {
    name: 'constructor getter in await',
    src: `var log = [];
var p = Promise.resolve('native');
Object.defineProperty(p, 'constructor', {get() { log.push('constructor read'); return Promise; }});
log.push(await p);
return log.join(',');`,
  },

  // Promise objects.
  {
    name: 'promise as a value',
    src: `var p = Promise.resolve(1);
return [String(p), typeof p, p instanceof Promise, JSON.stringify(p), JSON.stringify({p: p}), Object.keys(p).length,
  Object.prototype.toString.call(Promise.prototype), Promise.name, Promise.length, typeof Promise.prototype.then,
  Promise.prototype.then.length, Promise.all.length, Promise.prototype.constructor === Promise,
  Object.getPrototypeOf(p) === Promise.prototype, p + ''].join(',');`,
  },
  {
    name: 'promise properties',
    src: `var p = Promise.resolve(1); p.tag = 'x';
return JSON.stringify(p) + Object.keys(p) + ('then' in p) + p.hasOwnProperty('then');`,
  },
  { name: 'Promise without new', src: 'Promise(() => {})' },
  { name: 'Promise resolver undefined', src: 'new Promise()' },
  { name: 'Promise resolver number', src: 'new Promise(1)' },
  { name: 'Promise resolver object', src: 'new Promise({})' },
  { name: 'Promise resolver array', src: 'new Promise([1, 2])' },
  { name: 'Promise resolver string', src: 'new Promise("s")' },
  { name: 'then on object', src: 'Promise.prototype.then.call({})' },
  { name: 'then on number', src: 'Promise.prototype.then.call(1)' },
  { name: 'then on array', src: 'Promise.prototype.then.call([])' },
  { name: 'resolve on non-object', src: 'Promise.resolve.call(undefined, 1)' },
  { name: 'reject on non-object', src: 'Promise.reject.call(1)' },
  { name: 'finally on non-object', src: 'Promise.prototype.finally.call(1)' },
  { name: 'all on non-object', src: 'Promise.all.call(undefined, [])' },
  {
    name: 'AggregateError constructor',
    src: `var e = new AggregateError([1, 2], 'msg');
return [e.name, e.message, e.errors.join('+'), String(e), e instanceof Error, AggregateError.length, new AggregateError([]).message === ''].join(',');`,
  },
  { name: 'AggregateError not iterable', src: 'new AggregateError(5)' },

  // Async functions as values.
  {
    name: 'async function values',
    src: `async function f() {} var g = async () => 1; var o = {async m() { return this.v; }, v: 'mv'};
return [typeof f, Object.prototype.toString.call(f), String(f()), f.hasOwnProperty('prototype'), f.name, g.name, o.m.name, f.length,
  String(f), String(g), String(o.m), await o.m(), f() instanceof Promise].join('|');`,
  },
  {
    name: 'async function source',
    src: `return [String(async function  named ( a ) { return a }), String(async x => x), String(async (a, b) => a + b),
  String({async 'q'() {}}.q), String({async [1 + 1]() {}}[2]), String({async m() {}}.m)].join('|');`,
  },
  {
    name: 'AsyncFunction constructor',
    src: `var AF = (async function () {}).constructor;
var f = new AF('a', 'b', 'return await a + b');
var g = AF('return 7');
return [AF.name, AF.length, String(f), f.name, f.length, await f(1, 2), await g(), Object.getPrototypeOf(f) === AF.prototype,
  Object.getPrototypeOf(AF.prototype) === Function.prototype, AF.prototype.constructor === AF, Object.prototype.toString.call(AF.prototype),
  f instanceof Function, typeof AF.prototype].join('|');`,
  },
  { name: 'new async function', src: 'new (async function () {})' },
  { name: 'new async arrow', src: 'var a = async () => 1; new a()' },
  { name: 'new async method', src: 'var o = {async m() {}}; new o.m()' },
  {
    name: 'async as identifier',
    src: `var async = function (x) { return 'called ' + x; };
var o = {async: 1, async() { return 'method named async'; }};
var async2 = async
(2);
return [async(1), o.async(), async2, typeof async].join('|');`,
  },
  {
    name: 'async arrow forms',
    src: `var a = async x => x * 2; var b = async (x, y = 3, ...r) => x + y + r.length; var c = async ({p}, [q]) => p + q;
var d = async () => { return await a(5); };
return [await a(2), await b(1), await b(1, 1, 9, 9), await c({p: 'P'}, ['Q']), await d()].join(',');`,
  },
  {
    name: 'this and arguments',
    src: `var o = {v: 'obj', async m() { var self = this; var arrow = async () => this.v; return self.v + (await arrow()) + arguments.length; }};
async function f() { await null; return arguments[0] + arguments.length; }
return await o.m(1, 2) + '|' + await f('x', 'y');`,
  },
  {
    name: 'default parameter throws',
    src: `var log = [];
async function f(a = (() => { throw new Error('in default'); })()) { log.push('body'); }
var p = f(); log.push('returned ' + (p instanceof Promise));
try { await p; } catch (e) { log.push(e.message); }
return log.join(',');`,
  },
  {
    name: 'stack overflow rejects',
    src: `async function r() { return r(); }
async function deep(n) { return n === 0 ? 0 : 1 + await deep(n - 1); }
var a = await deep(50);
try { function s() { s(); } s(); } catch (e) { return a + ' ' + e.name + ': ' + e.message; }`,
  },
  {
    name: 'async recursion depth',
    src: `async function down(n) { if (n === 0) return 'bottom'; await null; return down(n - 1); }
return await down(300);`,
  },

  // Await in statements and expressions.
  {
    name: 'await in loops',
    src: `var out = [];
for (var i = 0; i < 3; i++) out.push(await Promise.resolve(i));
for (var x of [4, 5]) out.push(await x);
var k = 0; while (await (k < 2)) { out.push('w' + k); k++; }
for (var key in {a: 1, b: 2}) out.push(await key);
do { out.push('do'); } while (await false);
return out.join(',');`,
  },
  {
    name: 'await in expressions',
    src: `var a = [await 1, ...(await [2, 3])];
var o = {k: await 'v', [await 'c']: 4};
var t = \`t\${await 5}\${await 'x'}\`;
var c = (await true) ? await 'yes' : 'no';
var s = (await 1) + (await 2) * (await 3);
var [d, e] = await Promise.all([6, 7]);
var {f} = await {f: 8};
var g = typeof await 9;
var h = !(await 0);
var u = (-(await 2)) ** 2;
var n = await null ?? 'nullish';
var l = (await 0) || await 'or';
return JSON.stringify([a, o, t, c, s, d, e, f, g, h, u, n, l]);`,
  },
  {
    name: 'await in call arguments',
    src: `function f() { return Array.prototype.join.call(arguments, '-'); }
var log = [];
function g(x) { log.push('g' + x); return x; }
var r = f(g(1), await g(2), g(3), await Promise.resolve(g(4)));
return r + '|' + log.join(',');`,
  },
  {
    name: 'try catch finally around await',
    src: `var log = [];
try { await Promise.reject(new Error('e1')); log.push('not here'); } catch (e) { log.push('caught ' + e.message); } finally { log.push('finally1'); }
try { await (async () => { throw 'raw'; })(); } catch (e) { log.push(typeof e + ' ' + e); }
try { try { await 1; throw new Error('inner'); } finally { log.push('inner finally'); await null; log.push('after await in finally'); } } catch (e) { log.push('outer ' + e.message); }
async function f() { try { return await 'try'; } finally { log.push('f finally'); } }
log.push(await f());
async function g() { try { await Promise.reject(1); } catch (e) { await null; return 'from catch ' + e; } }
log.push(await g());
return log.join(',');`,
  },
  {
    name: 'await in switch, with and labels',
    src: `var out = [];
switch (await 'b') { case await 'a': out.push('a'); break; case 'b': out.push('b' + await 1); }
with ({w: 'with'}) { out.push(await w); }
outer: for (var i = 0; i < 3; i++) { for (var j = 0; j < 3; j++) { if (await j === 1) continue outer; if (i === 2) break outer; out.push(i + '' + j); } }
return out.join(',');`,
  },
  {
    name: 'closures capture across awaits',
    src: `var fns = [];
for (let i = 0; i < 3; i++) { await null; fns.push(() => i); }
let x = 1; var p = (async () => { await null; return x; })(); x = 2;
return fns.map(f => f()).join(',') + '|' + await p;`,
  },
  {
    name: 'mutual awaits',
    src: `var log = [];
var resolveA; var a = new Promise(r => { resolveA = r; });
async function waiter(n) { log.push('wait' + n); var v = await a; log.push('got' + n + v); return n; }
var ps = [waiter(1), waiter(2), waiter(3)];
log.push('resolving'); resolveA('!');
var rs = await Promise.all(ps);
return log.join(',') + '|' + rs.join(',');`,
  },
  {
    name: 'array callbacks with async',
    src: `var log = [];
var ps = [1, 2, 3].map(async x => { log.push('start' + x); await null; log.push('end' + x); return x * 10; });
log.push('mapped');
var rs = await Promise.all(ps);
[4, 5].forEach(async x => { await null; log.push('each' + x); });
await null; await null;
return log.join(',') + '|' + rs.join(',');`,
  },
  {
    name: 'unhandled rejection is silent',
    src: `Promise.reject(new Error('nobody handles this'));
(async () => { throw new Error('nor this'); })();
await null;
return 'ok';`,
  },
  {
    name: 'never settled promise',
    src: `var log = [];
(async () => { log.push('waiting'); await new Promise(() => {}); log.push('never'); })();
await null;
return log.join(',');`,
  },
  {
    name: 'console ordering',
    src: `console.log('sync start');
var p = (async () => { console.log('async body'); await null; console.log('after await'); })();
Promise.resolve().then(() => console.log('reaction'));
console.log('sync end');
await p;
return 'done';`,
  },
  {
    name: 'template shaped body',
    src: `var locals = {name: 'World', items: Promise.resolve(['a', '<b>'])};
var escapeFn = function (s) { return String(s).replace(/[<>]/g, function (c) { return c === '<' ? '&lt;' : '&gt;'; }); };
var include = async function (name, data) { await null; return '[' + name + ':' + (data && data.x) + ']'; };
var __line = 1, __lines = "x", __filename = undefined;
try {
  var __output = "";
  function __append(s) { if (s !== undefined && s !== null) __output += s }
  with (locals || {}) {
    ; __append("Hello ")
    ; __append(escapeFn( name ))
    ; for (const it of await items) {
    ; __append(escapeFn( await Promise.resolve(it) ))
    ; }
    ; __append( await include('part', {x: 1}) )
  }
  return __output;
} catch (e) { throw e; }`,
  },
  {
    name: 'template shaped rejection',
    src: `var locals = {};
var rethrow = function (err, lines, filename, lineno) { err.message = 'eval.edge:' + lineno + '\\n' + err.message; throw err; };
var __line = 1, __lines = "x", __filename = undefined;
try {
  var __output = "";
  with (locals || {}) {
    ; __line = 3
    ; __output += await missing()
  }
  return __output;
} catch (e) { rethrow(e, __lines, __filename, __line); }`,
  },

  // Messages that render an await expression.
  { name: 'call of await result', src: 'var x = 1; (await x)()' },
  { name: 'method of await result', src: 'var o = {}; (await o).f()' },
  { name: 'await of a failing call', src: 'var o = {}; await o.f()' },
  { name: 'property of await result', src: 'var o; (await o).f' },
  { name: 'for-of over await result', src: 'var x = 1; for (const a of await x) {}' },
  { name: 'new of await result', src: 'var x = 1; new (await x)' },

  // Syntax errors.
  { name: 'await in plain function', sync: true, src: 'await x' },
  { name: 'await assignment in plain function', sync: true, src: 'var y = await f();' },
  { name: 'await call argument in plain function', sync: true, src: 'f(await x)' },
  { name: 'await parenthesized in plain function', sync: true, src: 'if (await x) {}' },
  { name: 'await as identifier', sync: true, src: 'var await = 3; return await' },
  { name: 'await then newline', sync: true, src: 'var await = 4; return await\n1' },
  { name: 'await in nested plain function', src: 'function g() { await 1; }' },
  { name: 'await in nested arrow', src: 'var f = () => { return await 1; }' },
  { name: 'await without operand', src: 'await' },
  { name: 'await exponent', src: 'return await 2 ** 2' },
  { name: 'async function in if', src: 'if (1) async function f() {}' },
  { name: 'async arrow line break', src: 'var f = async x\n=> x' },
  { name: 'async arrow await parameter', src: 'var f = async await => 1' },
  { name: 'async arrow duplicate parameters', src: 'var f = async (a, a) => 1' },
  { name: 'async method duplicate parameters', src: 'var o = {async m(a, a) {}}' },
  { name: 'async function duplicate parameters', src: 'async function f(a, a) { return a } return await f(1, 2)' },
];

const AsyncFunction = (async function () {}).constructor;

function describe(v) {
  try {
    return String(v);
  } catch (e) {
    return '<String failed>';
  }
}

function errorText(e) {
  if (e instanceof Error) return e.name + ': ' + e.message;
  return 'thrown ' + typeof e + ' ' + describe(e);
}

async function main() {
  process.on('unhandledRejection', () => {});
  const { Console } = console;
  const { Writable } = require('stream');
  for (const c of cases) {
    const out = [];
    const stdout = new Writable({ write(chunk, _, done) { out.push(String(chunk)); done(); } });
    globalThis.console = new Console({ stdout });
    try {
      const fn = c.sync ? new Function(c.src) : new AsyncFunction(c.src);
      const ret = c.sync ? fn() : await fn();
      c.ret = typeof ret === 'string' ? ret : '(' + typeof ret + ') ' + describe(ret);
    } catch (e) {
      c.err = errorText(e);
    }
    if (out.length) c.stdout = out.join('');
  }
  process.stdout.write(JSON.stringify({ cases }));
}

main();
