// Keeping the source pane and the file in Go in step while both change:
// the user types in the pane, and Go writes into the file, as when a run
// writes its output or an include fills its fence.

import { ChangeSet, type ChangeSpec } from "@codemirror/state";

// Synced is the pane's side of the text: base, the text Go last had as
// far as the pane knows, and local, what was typed since.
export class Synced {
  base: string;
  local: ChangeSet;

  constructor(text: string) {
    this.base = text;
    this.local = ChangeSet.empty(text.length);
  }

  // pending is whether something was typed that Go doesn't have yet.
  get pending(): boolean {
    return !this.local.empty;
  }

  // typed notes changes typed in the pane.
  typed(changes: ChangeSet) {
    this.local = this.local.compose(changes);
  }

  // incoming takes theirs, Go's text now, and returns the changes that
  // bring the pane's text up to it while keeping what was typed: Go's
  // changes since base, moved past the typing.
  incoming(theirs: string): ChangeSet {
    const remote = diff(this.base, theirs);
    const changes = remote.map(this.local);
    this.local = this.local.map(remote, true);
    this.base = theirs;
    return changes;
  }

  // sent notes that Go took text, the pane's text when it was sent,
  // after later typing made the pane's text now.
  sent(text: string, now: string) {
    this.base = text;
    this.local = text === now ? ChangeSet.empty(now.length) : diff(text, now);
  }
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
