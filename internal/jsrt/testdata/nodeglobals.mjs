// nodeglobals.mjs records testdata/nodeglobals.json, from which gen/main.go writes
// nodeglobals_gen.go: the identifiers that are defined globals in a Node ES
// module, the names edge-parser leaves untransformed because
// `global[name] !== undefined`. That test reads through the global object's
// prototype chain, so the list holds the string keys of globalThis and of every
// object on its chain whose value is not undefined. Run it in internal/jsrt:
//
//     node testdata/nodeglobals.mjs > testdata/nodeglobals.json
//
// Reading a few of Node's lazy globals prints experimental warnings to stderr;
// they do not change the list.
const names = new Set();
for (let o = globalThis; o !== null; o = Object.getPrototypeOf(o)) {
  for (const name of Object.getOwnPropertyNames(o)) {
    if (globalThis[name] !== undefined) names.add(name);
  }
}
const recording = { node: process.version, globals: [...names].sort() };
process.stdout.write(JSON.stringify(recording, null, 2) + "\n");
