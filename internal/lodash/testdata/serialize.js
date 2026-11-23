// serialize renders a value as a deterministic string that shows object identity
// (every object gets a number the first time it is seen, later sightings print
// "#n"), prototypes, own property attributes and accessors without running
// getters, so a result compared between Node and the Go port also shows which
// objects lodash shared, cloned or mutated. Builtins print by name.
(function (root) {
  var builtins = [
    [Object, "Object"], [Object.prototype, "Object.prototype"], [Array, "Array"],
    [Array.prototype, "Array.prototype"], [Function, "Function"],
    [Function.prototype, "Function.prototype"], [Map, "Map"], [Map.prototype, "Map.prototype"],
    [Set, "Set"], [Set.prototype, "Set.prototype"], [Date, "Date"], [Date.prototype, "Date.prototype"],
    [RegExp, "RegExp"], [RegExp.prototype, "RegExp.prototype"], [Error, "Error"],
    [Error.prototype, "Error.prototype"], [String, "String"], [String.prototype, "String.prototype"],
    [Number, "Number"], [Number.prototype, "Number.prototype"], [Boolean, "Boolean"],
    [Boolean.prototype, "Boolean.prototype"]
  ];
  var defaultProtos = [Object.prototype, Array.prototype, Function.prototype, Map.prototype,
    Set.prototype, Date.prototype, RegExp.prototype, Error.prototype, String.prototype,
    Number.prototype, Boolean.prototype];
  var seen = [];
  function index(list, v) {
    for (var i = 0; i < list.length; i++) if (list[i] === v) return i;
    return -1;
  }
  function str(s) { return JSON.stringify(s); }
  function value(v) {
    if (v === undefined) return "undefined";
    if (v === null) return "null";
    var t = typeof v;
    if (t === "number") return v === 0 && 1 / v < 0 ? "-0" : String(v);
    if (t === "string") return str(v);
    if (t === "boolean") return String(v);
    for (var b = 0; b < builtins.length; b++) if (builtins[b][0] === v) return builtins[b][1];
    var id = index(seen, v);
    if (id >= 0) return "#" + id;
    seen.push(v);
    id = seen.length - 1;
    var tag = Object.prototype.toString.call(v);
    var head = "#" + id + " " + tag.slice(8, -1);
    var proto = Object.getPrototypeOf(v);
    if (index(defaultProtos, proto) < 0) head += " ^" + value(proto);
    var parts = [];
    if (t === "function") {
      head += " " + str(Object.getOwnPropertyDescriptor(v, "name").value);
    } else if (v instanceof Date) {
      head += " " + v.getTime();
    } else if (v instanceof RegExp) {
      head += " " + String(v) + " lastIndex=" + v.lastIndex;
    } else if (v instanceof Map) {
      v.forEach(function (val, k) { parts.push(value(k) + " => " + value(val)); });
    } else if (v instanceof Set) {
      v.forEach(function (val) { parts.push(value(val)); });
    }
    var names = Object.getOwnPropertyNames(v);
    for (var i = 0; i < names.length; i++) {
      var k = names[i];
      var d = Object.getOwnPropertyDescriptor(v, k);
      if (t === "function" && !d.enumerable) continue;
      if (Array.isArray(v) && k === "length") continue;
      if (v instanceof Error && k === "stack") continue;
      var flags = (d.enumerable ? "" : "!") + (d.configurable ? "" : "~");
      if ("value" in d) parts.push(flags + str(k) + ": " + value(d.value));
      else parts.push(flags + str(k) + ": " + (d.get ? "get" : "") + (d.set ? "set" : ""));
    }
    return head + " {" + parts.join(", ") + "}";
  }
  return value(root);
})
