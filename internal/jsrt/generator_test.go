package jsrt

import (
	"strings"
	"testing"
)

// TestGenerators checks generator functions and objects: the resumption
// methods, return and throw through try/finally, yield*, iteration by
// for-of, spread and destructuring (closing the generator when it stops
// early), and what toString, typeof and util.inspect show. The expected
// values, and the console output where there is one, were printed by Node
// v24.22.1.
func TestGenerators(t *testing.T) {
	r := NewRealm(Options{})
	var out strings.Builder
	r.SetConsole(&out, &out)
	tests := []struct {
		src, want, log string
	}{
		{`(() => { const GP = Object.getPrototypeOf(function*(){}).prototype; try { GP.next.call({}) } catch (e) { return e.message } })()`,
			`"Method [Generator].prototype.next called on incompatible receiver #<Object>"`, ""},
		{`(() => { const GP = Object.getPrototypeOf(function*(){}).prototype; try { GP.return.call(1) } catch (e) { return e.message } })()`,
			`"Method [Generator].prototype.return called on incompatible receiver 1"`, ""},
		{`String(function* g(a) { yield a })`, `"function* g(a) { yield a }"`, ""},
		{`String(new (Object.getPrototypeOf(function*(){}).constructor)('a', 'yield a'))`, `"function* anonymous(a\n) {\nyield a\n}"`, ""},
		{`(() => { const G = Object.getPrototypeOf(function*(){}).constructor; return G.name + G.length })()`, `"GeneratorFunction1"`, ""},
		{`Object.prototype.toString.call(function*(){})`, `"[object GeneratorFunction]"`, ""},
		{`Object.prototype.toString.call((function*(){})())`, `"[object Generator]"`, ""},
		{`(() => { function* g() {} return [typeof g, g.length, g.name, Object.getOwnPropertyNames(g).join(), Object.getOwnPropertyNames(g.prototype).length, Object.getPrototypeOf(g.prototype) === Object.getPrototypeOf(function*(){}).prototype] })()`,
			`["function",0,"g","length,name,prototype",0,true]`, ""},
		{`(() => { function* g() {} try { new g() } catch (e) { return e.message } })()`, `"g is not a constructor"`, ""},
		{`(() => { function* g() { const x = yield 1; try { yield x * 2 } finally { console.log('fin') } } const it = g(); return [JSON.stringify(it.next('a')), JSON.stringify(it.next(5)), JSON.stringify(it.return(7)), JSON.stringify(it.next())] })()`,
			`["{\"value\":1,\"done\":false}","{\"value\":10,\"done\":false}","{\"value\":7,\"done\":true}","{\"done\":true}"]`, "fin\n"},
		{`(() => { function* g() { try { yield 1 } catch (e) { yield 'c' + e } } const it = g(); return [JSON.stringify(it.next()), JSON.stringify(it.throw('E')), JSON.stringify(it.next())] })()`,
			`["{\"value\":1,\"done\":false}","{\"value\":\"cE\",\"done\":false}","{\"done\":true}"]`, ""},
		{`(() => { function* g() { yield 1 } const it = g(); try { it.throw(new Error('x')) } catch (e) { return e.message + JSON.stringify(it.next()) } })()`, `"x{\"done\":true}"`, ""},
		{`(() => { function* g() { it.next() } const it = g(); try { it.next() } catch (e) { return e.message } })()`, `"Generator is already running"`, ""},
		{`(() => { function* g() { yield* [1, 2]; const r = yield* h(); yield r } function* h() { yield 'h'; return 'ret' } return [...g()].join() })()`, `"1,2,h,ret"`, ""},
		{`(() => { function* nat() { let i = 0; while (true) yield i++ } const [a, b, c] = nat(); return a + b + c })()`, `3`, ""},
		{`(() => { function* g() { try { yield 1; yield 2 } finally { console.log('closed') } } for (const x of g()) { break } return 'ok' })()`, `"ok"`, "closed\n"},
		{`Array.from((function*(){ yield 1; yield 2 })(), x => x * 10).join()`, `"10,20"`, ""},
		{`new Map((function*(){ yield ['a', 1] })()).get('a')`, `1`, ""},
		{`(() => { function* g() { yield 1 } const it = g(); return [String(it), it.toString()] })()`, `["[object Generator]","[object Generator]"]`, ""},
		{`(() => { const o = { *m(x) { yield x } }; return [String(o.m), [...o.m(3)].join()] })()`, `["*m(x) { yield x }","3"]`, ""},
		{`(() => { function* g() { yield* 'ab' } const it = g(); it.next(); try { it.throw(1) } catch (e) { return e.message } })()`, `"The iterator does not provide a 'throw' method."`, ""},
		{`(() => { function* g() { return 5 } return JSON.stringify([...g()]) + JSON.stringify(g().next()) })()`, `"[]{\"value\":5,\"done\":true}"`, ""},
		{`(() => { function* g(a = undefinedVar) {} try { g() } catch (e) { return e.message } })()`, `"undefinedVar is not defined"`, ""},
		{`(() => { function* g() { yield yield 1 } const it = g(); return [it.next().value, it.next(2).value, it.next(3).done] })()`, `[1,2,true]`, ""},
		{`(() => { function* g() { const x = yield; return x } const it = g(); it.next(); return JSON.stringify(it.next(9)) })()`, `"{\"value\":9,\"done\":true}"`, ""},
	}
	for _, tt := range tests {
		out.Reset()
		if got := evalJSON(t, r, tt.src); got != tt.want || out.String() != tt.log {
			t.Errorf("%s\n got %s, log %q\nwant %s, log %q", tt.src, got, out.String(), tt.want, tt.log)
		}
	}
	inspected := []struct {
		src, want string
	}{
		{`(function* gen() {})()`, "Object [Generator] {}"},
		{`(function* gen() {})`, "[GeneratorFunction: gen]"},
		{`({*m() {}})`, "{ m: [GeneratorFunction: m] }"},
	}
	for _, tt := range inspected {
		f, err := r.NewFunction("", "return "+tt.src, false)
		if err != nil {
			t.Fatal(err)
		}
		v, err := r.Call(f, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		if got := r.Inspect(v); got != tt.want {
			t.Errorf("inspect(%s) = %q; want %q", tt.src, got, tt.want)
		}
	}
}
