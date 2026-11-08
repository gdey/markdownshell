// The cast player refuses a recording past termrec.ReadCast's limits,
// so a file made to be huge can't make xterm.js allocate without bound.
// internal/build's tests bundle and run it with node.

import { maxCols, maxEvents, maxOutput, maxRows, parseCast } from "../src/castparse.js";

function fail(msg: string): never {
  console.error(msg);
  (globalThis as unknown as { process: { exit(n: number): void } }).process.exit(1);
  throw new Error(msg);
}

const header = (h: Record<string, unknown>) => JSON.stringify({ version: 2, ...h }) + "\n";
const ev = (t: number, s: string) => JSON.stringify([t, "o", s]) + "\n";

function refuses(name: string, text: string, want: string) {
  try {
    parseCast(text);
  } catch (err) {
    const msg = err instanceof Error ? err.message : String(err);
    if (!msg.includes(want)) fail(`${name}: error ${JSON.stringify(msg)}, want it to say ${JSON.stringify(want)}`);
    return;
  }
  fail(`${name}: read, want it refused`);
}

// What fits is read as before.
let c = parseCast(header({ width: maxCols, height: maxRows }) + ev(0, "hi") + JSON.stringify([0.1, "i", "x"]) + "\n");
if (c.cols !== maxCols || c.rows !== maxRows || c.events.length !== 1) fail(`at the limits: got ${c.cols}x${c.rows}, ${c.events.length} events`);
c = parseCast(header({}));
if (c.cols !== 80 || c.rows !== 24) fail(`no size: got ${c.cols}x${c.rows}, want 80x24`);

refuses("huge", header({ width: 100000, height: 100000 }), "too large");
refuses("wide", header({ width: maxCols + 1, height: 24 }), "too large");
refuses("tall", header({ width: 80, height: maxRows + 1 }), "too large");
refuses("negative", header({ width: -1, height: 24 }), "isn't a size");
refuses("string", header({ width: "100000", height: 24 }), "isn't a size");
refuses("fraction", header({ width: 80.5, height: 24 }), "isn't a size");

let many = header({ width: 80, height: 24 });
for (let i = 0; i <= maxEvents; i++) many += ev(i / 1000, "x");
refuses("events", many, "events");
// Input events don't count toward the limit.
let inputs = header({ width: 80, height: 24 });
for (let i = 0; i <= maxEvents; i++) inputs += JSON.stringify([i / 1000, "i", "x"]) + "\n";
if (parseCast(inputs).events.length !== 0) fail("input events kept");

// Output is counted in UTF-8 bytes, as Go counts it: half the limit in
// two-byte runes is the limit exactly, and one more byte is past it.
const half = "é".repeat(maxOutput / 4);
c = parseCast(header({ width: 80, height: 24 }) + ev(0, half) + ev(1, half));
if (c.events.length !== 2) fail("output at the limit refused");
refuses("output", header({ width: 80, height: 24 }) + ev(0, half) + ev(1, half) + ev(2, "x"), "MB of output");
