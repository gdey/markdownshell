// The source pane and Go trade line edits (sync.ts, and Go's
// notebook/lines.go). This plays both sides at random: the pane types,
// Go changes the file as a run would, the pane sends what was typed and
// is turned down when Go moved on, and catches up from the events since.
// After every step the pane's text must be Go's text with what was typed
// and not yet sent, and once all is sent the two must be the same.
// internal/build's tests bundle and run it with node.

import { ChangeSet, Text } from "@codemirror/state";
import { type LineEdit, Synced, type Version, eventChanges, lineCount, lineEditsOf, textOf } from "../src/sync.js";

let seed = 7;
const rand = (n: number) => {
  seed = (seed * 1103515245 + 12345) & 0x7fffffff;
  return seed % n;
};
const pieces = ["a", "bb", "\n", "\n\n", "# H", "```", "x y", ""];
const piece = () => pieces[rand(pieces.length)];
const words = (n: number) => Array.from({ length: n }, piece).join("");

function fail(msg: string): never {
  console.error(msg);
  (globalThis as unknown as { process: { exit(n: number): void } }).process.exit(1);
  throw new Error(msg);
}

// goLines splits text as Go counts lines: after each newline.
const goLines = (text: string) => (text === "" ? [] : text.split(/(?<=\n)/));

// applyGo makes line edits on text as Go's applyLines does, in order,
// each against the text as it was before any of them.
function applyGo(text: string, edits: LineEdit[]): string {
  const l = goLines(text);
  let out = "";
  let at = 0;
  for (const e of edits) {
    if (e.first < at || e.last < e.first || e.last > l.length) fail(`bad edit ${JSON.stringify(e)} on ${JSON.stringify(text)}`);
    out += l.slice(at, e.first).join("") + e.text;
    at = e.last;
  }
  return out + l.slice(at).join("");
}

// A random edit at a random place in text.
function randomChange(text: Text): ChangeSet {
  const from = rand(text.length + 1);
  const to = Math.min(text.length, from + (rand(3) ? 0 : rand(12)));
  return ChangeSet.of({ from, to, insert: rand(4) === 0 ? "" : words(1 + rand(3)) }, text.length);
}

// Go's side: the text, the version and the events since the start.
class Go {
  version: Version = { doc: 1, tick: 0 };
  events: (LineEdit & { tick: number })[] = [];
  constructor(public text: string) {}
  since(v: Version) {
    return { version: this.version, events: this.events.filter((e) => e.tick > v.tick), reset: false };
  }
  add(e: LineEdit) {
    this.version = { doc: 1, tick: this.version.tick + 1 };
    this.events.push({ ...e, tick: this.version.tick });
  }
  edit(v: Version, edits: LineEdit[]) {
    if (v.tick !== this.version.tick) return { applied: false, ...this.since(v) };
    this.text = applyGo(this.text, edits);
    for (let i = edits.length - 1; i >= 0; i--) this.add(edits[i]);
    return { applied: true, version: this.version, events: [], reset: false };
  }
  // write changes a run of lines, as a run writing output does.
  write() {
    const n = goLines(this.text).length;
    const first = rand(n + 1);
    const last = Math.min(n, first + rand(3));
    let text = words(rand(4)).replace(/\n/g, "") + "\n";
    if (rand(3) === 0) text = "";
    // Lines put at the end of a file with no last newline join it.
    if (first === n && n > 0 && !this.text.endsWith("\n")) return;
    if (last === n && !this.text.endsWith("\n") && text !== "") text = text.slice(0, -1);
    const e = { first, last, text };
    this.text = applyGo(this.text, [e]);
    this.add(e);
  }
}

// lineEditsOf must say what the changes do, as Go applies it.
for (let i = 0; i < 20000; i++) {
  const base = textOf(words(rand(14)));
  let c = randomChange(base);
  if (rand(2)) c = c.compose(randomChange(c.apply(base)));
  const want = c.apply(base).toString();
  const got = applyGo(base.toString(), lineEditsOf(base, c));
  if (got !== want) fail(`lineEditsOf ${JSON.stringify(lineEditsOf(base, c))} on ${JSON.stringify(base.toString())} makes ${JSON.stringify(got)}, want ${JSON.stringify(want)}`);
  if (lineCount(base) !== goLines(base.toString()).length) fail(`lineCount of ${JSON.stringify(base.toString())} = ${lineCount(base)}`);
}

// eventChanges must make the event as Go does.
for (let i = 0; i < 5000; i++) {
  const text = words(rand(14));
  const n = goLines(text).length;
  const first = rand(n + 1);
  const last = first + rand(n - first + 1);
  const e = { first, last, text: last === n && !text.endsWith("\n") ? "z" : "z\n" };
  if (first === n && n > 0 && !text.endsWith("\n")) continue;
  const want = applyGo(text, [e]);
  const got = eventChanges(textOf(text), e).apply(textOf(text)).toString();
  if (got !== want) fail(`event ${JSON.stringify(e)} on ${JSON.stringify(text)} makes ${JSON.stringify(got)}, want ${JSON.stringify(want)}`);
}

// Both sides at once.
for (let round = 0; round < 1500; round++) {
  const start = words(rand(20));
  const go = new Go(start);
  const sync = new Synced(start, go.version);
  let pane = textOf(start);
  let typed = 0;
  // inFlight is an edit sent and not yet answered: typing and Go's
  // writes can come in between.
  let inFlight: { version: Version; edits: LineEdit[] } | null = null;
  for (let step = 0; step < 40; step++) {
    const what = rand(5);
    if (what <= 1) {
      const c = randomChange(pane);
      pane = c.apply(pane);
      sync.typed(c);
      typed++;
    } else if (what === 2) {
      go.write();
    } else if (what === 3 && !inFlight && sync.pending) {
      inFlight = { version: sync.version, edits: sync.take() };
    } else if (inFlight) {
      const r = go.edit(inFlight.version, inFlight.edits);
      inFlight = null;
      if (r.applied) sync.sent(r.version);
      else {
        sync.refused();
        const c = sync.incoming(r.events, r.version);
        pane = c.apply(pane);
      }
    } else {
      const l = go.since(sync.version);
      pane = sync.incoming(l.events, l.version).apply(pane);
    }
    if (sync.text().toString() !== pane.toString()) fail(`round ${round} step ${step}: sync's text ${JSON.stringify(sync.text().toString())} isn't the pane's ${JSON.stringify(pane.toString())}`);
  }
  if (inFlight) {
    const r = go.edit(inFlight.version, inFlight.edits);
    if (r.applied) sync.sent(r.version);
    else {
      sync.refused();
      pane = sync.incoming(r.events, r.version).apply(pane);
    }
  }
  for (let tries = 0; sync.pending; tries++) {
    if (tries > 3) fail(`round ${round}: the pane never caught up`);
    const v = sync.version;
    const r = go.edit(v, sync.take());
    if (r.applied) sync.sent(r.version);
    else {
      sync.refused();
      pane = sync.incoming(r.events, r.version).apply(pane);
    }
  }
  const l = go.since(sync.version);
  pane = sync.incoming(l.events, l.version).apply(pane);
  if (pane.toString() !== go.text) fail(`round ${round} (${typed} typed): the pane has ${JSON.stringify(pane.toString())}, Go ${JSON.stringify(go.text)}`);
  if (sync.base.toString() !== go.text) fail(`round ${round}: sync's base ${JSON.stringify(sync.base.toString())}, Go ${JSON.stringify(go.text)}`);
}
