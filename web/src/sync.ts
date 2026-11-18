// Keeping the source pane and the file in Go in step while both change:
// the user types in the pane, and Go writes into the file, as when a run
// writes its output or an include fills its fence.

import { ChangeSet, Text, type ChangeSpec } from "@codemirror/state";

// The pane and Go trade line edits, not whole texts (Go's
// notebook/lines.go and docs/design/ui-protocol.md): lines [first, last)
// became text, counting from 0, a line being the text up to and with a
// newline, or what follows the last newline. Each change moves Go's
// version on a tick; an edit names the version it was made against, and
// one against an older text is turned down with the events since.

// Version is a state of Go's text: which document, and how many changes
// it has seen.
export type Version = { doc: number; tick: number };

// LineEdit says lines [first, last) became text.
export type LineEdit = { first: number; last: number; text: string };

// Synced is the pane's side of the text: base, the text Go has at
// version as far as the pane knows; local, what was typed since and not
// yet sent; and sending, what was sent and not yet answered.
export class Synced {
  version: Version;
  base: Text;
  local: ChangeSet;
  sending: ChangeSet | null = null;

  constructor(text: string | Text, version: Version) {
    this.base = typeof text === "string" ? textOf(text) : text;
    this.version = version;
    this.local = ChangeSet.empty(this.base.length);
  }

  // pending is whether something was typed that Go doesn't have yet.
  get pending(): boolean {
    return !this.local.empty || this.sending !== null;
  }

  // typed notes changes typed in the pane.
  typed(changes: ChangeSet) {
    this.local = this.local.compose(changes);
  }

  // incoming takes Go's line events since version, which bring it to
  // to, and returns the changes that bring the pane's text up to Go's
  // while keeping what was typed: Go's changes, moved past the typing.
  incoming(events: LineEdit[], to: Version): ChangeSet {
    let text = this.base;
    let remote = ChangeSet.empty(text.length);
    for (const e of events) {
      const c = eventChanges(text, e);
      text = c.apply(text);
      remote = remote.compose(c);
    }
    return this.merge(remote, text, to);
  }

  // reset is incoming for when the events since version aren't kept:
  // theirs is Go's whole text, at to.
  reset(theirs: string, to: Version): ChangeSet {
    const remote = diff(this.base.toString(), theirs);
    return this.merge(remote, remote.apply(this.base), to);
  }

  private merge(remote: ChangeSet, text: Text, to: Version): ChangeSet {
    const typed = this.sending ? this.sending.compose(this.local) : this.local;
    this.sending = null;
    const changes = remote.map(typed);
    this.local = typed.map(remote, true);
    this.base = text;
    this.version = to;
    return changes;
  }

  // take is what was typed as line edits against base, to send; sent or
  // refused says how Go answered. Typing meanwhile is kept apart.
  take(): LineEdit[] {
    const edits = lineEditsOf(this.base, this.local);
    this.sending = this.local;
    this.local = ChangeSet.empty(this.local.newLength);
    return edits;
  }

  // sent notes that Go took what take gave, bringing it to version to.
  sent(to: Version) {
    if (this.sending) this.base = this.sending.apply(this.base);
    this.sending = null;
    this.version = to;
  }

  // refused notes that Go didn't take it; it is sent again later.
  refused() {
    if (this.sending) this.local = this.sending.compose(this.local);
    this.sending = null;
  }

  // text is the pane's text: Go's with what was typed.
  text(): Text {
    const t = this.sending ? this.sending.apply(this.base) : this.base;
    return this.local.apply(t);
  }
}

// textOf is text as a CodeMirror Text.
export function textOf(text: string): Text {
  return Text.of(text.split("\n"));
}

// lineOffset is where line i, from 0, starts in text, or its end for
// the line past the last.
export function lineOffset(text: Text, i: number): number {
  return i + 1 <= text.lines ? text.line(i + 1).from : text.length;
}

// lineCount is how many lines text has as Go counts them: a last line
// with no newline counts, the empty one after a last newline doesn't.
export function lineCount(text: Text): number {
  return text.length === 0 ? 0 : text.line(text.lines).length === 0 ? text.lines - 1 : text.lines;
}

// eventChanges is the changes on text that a line event makes.
export function eventChanges(text: Text, e: LineEdit): ChangeSet {
  return ChangeSet.of({ from: lineOffset(text, e.first), to: lineOffset(text, e.last), insert: e.text }, text.length);
}

// lineEditsOf is changes, on base, as line edits: each changed run of
// base's lines and the lines that took their place, in order.
export function lineEditsOf(base: Text, changes: ChangeSet): LineEdit[] {
  if (changes.empty) return [];
  const now = changes.apply(base);
  const count = lineCount(base);
  const hunks: { first: number; last: number; from: number; to: number }[] = [];
  changes.iterChanges((fromA, toA, fromB, toB) => {
    const first = Math.min(base.lineAt(fromA).number - 1, count);
    const last = Math.min(base.lineAt(toA).number, count);
    const from = now.lineAt(fromB).from;
    const lb = now.lineAt(toB);
    const to = lb.number < now.lines ? lb.to + 1 : lb.to;
    const h = hunks[hunks.length - 1];
    if (h && first < h.last) {
      h.last = Math.max(h.last, last);
      h.to = Math.max(h.to, to);
    } else hunks.push({ first, last, from, to });
  });
  return hunks.map((h) => ({ first: h.first, last: h.last, text: now.sliceString(h.from, h.to) }));
}

// diff is the changes that turn a into b, line by line, so that changes
// in two places don't become one change over all that lies between,
// which would carry typing in there off with it.
export function diff(a: string, b: string): ChangeSet {
  const changes: ChangeSpec[] = [];
  if (a !== b) {
    const al = lines(a);
    const bl = lines(b);
    let [ai, bi, ao, bo] = [0, 0, 0, 0]; // line indices and offsets
    const hunk = (aTo: number, bTo: number) => {
      let [aEnd, bEnd] = [ao, bo];
      for (; ai < aTo; ai++) aEnd += al[ai].length;
      for (; bi < bTo; bi++) bEnd += bl[bi].length;
      if (aEnd > ao || bEnd > bo) changes.push(trim(a, ao, aEnd, b, bo, bEnd));
      [ao, bo] = [aEnd, bEnd];
    };
    for (const [x, y] of same(al, bl)) {
      hunk(x, y);
      ao += al[ai++].length;
      bo += bl[bi++].length;
    }
    hunk(al.length, bl.length);
  }
  return ChangeSet.of(changes, a.length);
}

// lines splits text after each newline.
function lines(text: string): string[] {
  return text === "" ? [] : text.split(/(?<=\n)/);
}

// trim narrows the change of a[aFrom:aTo] to b[bFrom:bTo] to what
// differs.
function trim(a: string, aFrom: number, aTo: number, b: string, bFrom: number, bTo: number): ChangeSpec {
  while (aFrom < aTo && bFrom < bTo && a[aFrom] === b[bFrom]) aFrom++, bFrom++;
  while (aTo > aFrom && bTo > bFrom && a[aTo - 1] === b[bTo - 1]) aTo--, bTo--;
  return { from: aFrom, to: aTo, insert: b.slice(bFrom, bTo) };
}

// maxEdits bounds the work of matching lines; two texts further apart
// than this are changed in one piece.
const maxEdits = 2000;

// same is the pairs of line indices, a's then b's, that the two keep,
// in order: a longest common subsequence, by Myers' algorithm, after
// the lines both start and end with.
function same(a: string[], b: string[]): [number, number][] {
  let start = 0;
  while (start < a.length && start < b.length && a[start] === b[start]) start++;
  let end = 0;
  while (end < a.length - start && end < b.length - start && a[a.length - 1 - end] === b[b.length - 1 - end]) end++;
  const head: [number, number][] = [];
  for (let i = 0; i < start; i++) head.push([i, i]);
  const tail: [number, number][] = [];
  for (let i = end; i > 0; i--) tail.push([a.length - i, b.length - i]);
  const mid = myers(a.slice(start, a.length - end), b.slice(start, b.length - end));
  return [...head, ...mid.map(([x, y]): [number, number] => [x + start, y + start]), ...tail];
}

function myers(a: string[], b: string[]): [number, number][] {
  const [n, m] = [a.length, b.length];
  if (!n || !m) return [];
  const max = Math.min(n + m, maxEdits);
  const off = max + 1;
  const v = new Int32Array(2 * off + 1);
  const trace: Int32Array[] = []; // v for k in [-d+1, d-1] before step d
  for (let d = 0; d <= max; d++) {
    trace.push(v.slice(off - d + 1, off + d));
    for (let k = -d; k <= d; k += 2) {
      let x = k === -d || (k !== d && v[off + k - 1] < v[off + k + 1]) ? v[off + k + 1] : v[off + k - 1] + 1;
      let y = x - k;
      while (x < n && y < m && a[x] === b[y]) x++, y++;
      v[off + k] = x;
      if (x >= n && y >= m) return back(trace, n, m);
    }
  }
  return [];
}

// back walks the trace from the end to the start, collecting the
// diagonal steps: the lines kept.
function back(trace: Int32Array[], n: number, m: number): [number, number][] {
  const out: [number, number][] = [];
  let [x, y] = [n, m];
  for (let d = trace.length - 1; d > 0; d--) {
    const vd = trace[d];
    const at = (k: number) => vd[k + d - 1];
    const k = x - y;
    const pk = k === -d || (k !== d && at(k - 1) < at(k + 1)) ? k + 1 : k - 1;
    const px = at(pk);
    const py = px - pk;
    while (x > px && y > py) out.push([--x, --y]);
    [x, y] = [px, py];
  }
  while (x > 0 && y > 0) out.push([--x, --y]);
  return out.reverse();
}
