// zonenames.js records testdata/zonenames.json, from which gen/main.go writes
// zonenames_gen.go: the names V8 prints in parentheses at the end of
// Date.prototype.toString for each IANA time zone (ICU's long specific names, such
// as "Eastern Standard Time" and "Eastern Daylight Time"). Run it in
// internal/jsdate:
//
//     node testdata/zonenames.js /usr/share/zoneinfo > testdata/zonenames.json
//
// It lists every zone file (links included) under the given zoneinfo directory, sets
// process.env.TZ to each in turn (Node resets V8's time zone cache when TZ changes)
// and records the offset and name String(date) shows in mid-January and mid-July of
// 2026. Of two different offsets, the smaller is the zone's standard time and the
// larger its daylight-saving time; a zone with one offset has the same name twice.
// Each entry of "zones" is [zone, standard name, daylight-saving name], in the
// order the directory walk finds the zones.
"use strict";
const fs = require("fs");
const path = require("path");

const root = process.argv[2] || "/usr/share/zoneinfo";

function zones(dir, prefix) {
  const out = [];
  for (const name of fs.readdirSync(dir).sort()) {
    if (name === "posix" || name === "right") continue;
    const full = path.join(dir, name);
    const rel = prefix ? prefix + "/" + name : name;
    const st = fs.statSync(full);
    if (st.isDirectory()) {
      out.push(...zones(full, rel));
      continue;
    }
    const magic = Buffer.alloc(4);
    const fd = fs.openSync(full, "r");
    try {
      fs.readSync(fd, magic, 0, 4, 0);
    } finally {
      fs.closeSync(fd);
    }
    if (magic.toString("latin1") === "TZif") out.push(rel);
  }
  return out;
}

const instants = [Date.UTC(2026, 0, 15, 12), Date.UTC(2026, 6, 15, 12)];
const rows = [];
for (const zone of zones(root, "")) {
  process.env.TZ = zone;
  const seen = new Map();
  for (const t of instants) {
    const d = new Date(t);
    const offset = -d.getTimezoneOffset() * 60;
    const name = /\(([^)]*)\)$/.exec(String(d))[1];
    seen.set(offset, name);
  }
  const entries = [...seen].sort((a, b) => a[0] - b[0]);
  rows.push([zone, entries[0][1], entries[entries.length - 1][1]]);
}

// One zone to a line.
const head = `{"node": ${JSON.stringify(process.version)}, "icu": ${JSON.stringify(process.versions.icu)}, "zones": [\n`;
process.stdout.write(head + rows.map((row) => JSON.stringify(row)).join(",\n") + "\n]}\n");
