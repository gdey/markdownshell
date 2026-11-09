// Folding in the source pane: a heading folds its section, down to the
// next heading of the same or a higher level, and a fence or an
// mdsh:output region folds to its closing line. The rendered view keeps
// its own folds; sourceFolds and setSourceFolds copy them across.

import { type EditorState, type Extension, StateField, type Transaction } from "@codemirror/state";
import { type EditorView, keymap } from "@codemirror/view";
import { codeFolding, foldEffect, foldGutter, foldKeymap, foldService, foldable, foldedRanges, unfoldEffect } from "@codemirror/language";

const fenceOpen = /^(\s*(?:>\s*)*(?:[-*+]\s+|\d+[.)]\s+)?)(`{3,}|~{3,})/;
const atxHeading = /^ {0,3}(#{1,6})(?:\s|$)/;
const regionOpen = /^\s*(?:>\s*)*<!-- mdsh:output\b.*-->\s*$/;
const regionClose = /^\s*(?:>\s*)*<!-- \/mdsh:output -->\s*$/;

// An outline is what opens a fold in a document, in line order: for
// each line that opens one, counted from 1, its info is the last line a
// fence or an output region hides, or for a heading minus its level, and
// minus ten more for a setext heading. A region that is never closed is
// kept too, with info 0, as closing it opens a fold far above.
//
// It is worked out once when the text is loaded, and then again only
// around each edit: scanning the whole text on every key took time that
// grew with its length, to the size of the typing itself on a long
// document.
class Outline {
  constructor(
    readonly lines: Int32Array,
    readonly info: Int32Array,
  ) {}

  // ends are the last lines the headings' sections run to, by item,
  // worked out the first time a heading's fold is asked for.
  private headingEnds?: Int32Array;

  // at is the index of the item on line n, or of the first one after.
  at(n: number): number {
    let lo = 0;
    let hi = this.lines.length;
    while (lo < hi) {
      const mid = (lo + hi) >> 1;
      if (this.lines[mid] < n) lo = mid + 1;
      else hi = mid;
    }
    return lo;
  }

  // end is the last line item i's fold hides, before blank lines at the
  // end of a section are taken off, or 0.
  end(i: number, lastLine: number): number {
    const info = this.info[i];
    if (info >= 0) return info;
    if (!this.headingEnds) {
      // A heading's section runs to the next heading of the same or a
      // higher level.
      const ends = new Int32Array(this.lines.length);
      const open: number[] = [];
      for (let k = 0; k <= this.lines.length; k++) {
        const level = k < this.lines.length ? headingOf(this.info[k]) : 0;
        if (k < this.lines.length && !level) continue;
        while (open.length && headingOf(this.info[open[open.length - 1]]) >= level) {
          ends[open.pop()!] = k < this.lines.length ? this.lines[k] - 1 : lastLine;
        }
        open.push(k);
      }
      this.headingEnds = ends;
    }
    return this.headingEnds[i];
  }
}

const headingOf = (info: number) => (info >= 0 ? 0 : -info > 10 ? -info - 10 : -info);

// scan adds to lines and info what opens a fold from line n of doc on,
// where n starts no fence or region already open. With stop, it stops
// at the first item stop says to, and returns that item's line.
function scan(state: EditorState, n: number, lines: number[], info: number[], stop?: (line: number, info: number) => boolean): number {
  const doc = state.doc;
  const push = (line: number, i: number): boolean => {
    if (stop?.(line, i)) return true;
    lines.push(line);
    info.push(i);
    return false;
  };
  for (; n <= doc.lines; n++) {
    const text = doc.line(n).text;
    const f = fenceOpen.exec(text);
    if (f) {
      const fence = f[2];
      const closing = new RegExp("^\\s*(?:>\\s*)*" + (fence[0] === "`" ? "`" : "~") + "{" + fence.length + ",}\\s*$");
      let end = n + 1;
      while (end <= doc.lines && !closing.test(doc.line(end).text)) end++;
      end = Math.min(end, doc.lines);
      if (end > n && push(n, end)) return n;
      n = end;
      continue;
    }
    if (regionOpen.test(text)) {
      let end = n + 1;
      while (end <= doc.lines && !regionClose.test(doc.line(end).text)) end++;
      if (end <= doc.lines) {
        if (push(n, end)) return n;
        // A region holds markdown, so its headings are left alone.
        n = end;
        continue;
      }
      if (push(n, 0)) return n;
    }
    const level = headingLevel(state, n);
    if (level > 0) {
      const atx = !!atxHeading.exec(text);
      if (push(n, -(atx ? level : level + 10))) return n;
      if (!atx) n++; // the setext underline
    }
  }
  return 0;
}

// rescan is the outline of the text after tr, from was, the outline
// before it. It scans from just above the first line tr changed, or from
// the fence or region that line is in, until it comes past the last line
// changed to a line that opens the same fold as before; the rest is as
// it was, moved by the lines added or taken away.
function rescan(was: Outline, tr: Transaction): Outline {
  const before = tr.startState.doc;
  const after = tr.state.doc;
  let fromA = before.length;
  let toB = 0;
  tr.changes.iterChangedRanges((fa: number, _ta: number, _fb: number, tb: number) => {
    fromA = Math.min(fromA, fa);
    toB = Math.max(toB, tb);
  });
  const lastChanged = after.lineAt(toB).number;
  const delta = after.lines - before.lines;
  // The line above the change can be a setext heading's text, whose
  // underline the change touches.
  let from = Math.max(1, before.lineAt(fromA).number - 1);
  let keep = was.at(from);
  const prev = keep - 1;
  if (prev >= 0) {
    const info = was.info[prev];
    const line = was.lines[prev];
    if ((info > 0 && info >= from) || (info < -10 && line + 1 >= from)) from = line;
  }
  // A region left open folds once a line below closes it.
  for (let i = 0; i < keep; i++) {
    if (was.info[i] === 0) {
      from = Math.min(from, was.lines[i]);
      break;
    }
  }
  keep = was.at(from);
  const lines = Array.from(was.lines.subarray(0, keep));
  const info = Array.from(was.info.subarray(0, keep));
  let same = -1;
  scan(tr.state, from, lines, info, (line, i) => {
    if (line <= lastChanged + 1) return false;
    const k = was.at(line - delta);
    if (k >= was.lines.length || was.lines[k] !== line - delta) return false;
    const old = was.info[k];
    if (old > 0 ? old + delta !== i : old !== i) return false;
    same = k;
    return true;
  });
  if (same < 0) return new Outline(Int32Array.from(lines), Int32Array.from(info));
  const n = lines.length + was.lines.length - same;
  const L = new Int32Array(n);
  const I = new Int32Array(n);
  L.set(lines);
  I.set(info);
  for (let k = same, j = lines.length; k < was.lines.length; k++, j++) {
    L[j] = was.lines[k] + delta;
    const v = was.info[k];
    I[j] = v > 0 ? v + delta : v;
  }
  return new Outline(L, I);
}

const outlineField = StateField.define<Outline>({
  create(state: EditorState) {
    const lines: number[] = [];
    const info: number[] = [];
    scan(state, 1, lines, info);
    return new Outline(Int32Array.from(lines), Int32Array.from(info));
  },
  update(was: Outline, tr: Transaction) {
    return tr.docChanged ? rescan(was, tr) : was;
  },
});

// headingLevel is the level of the heading on line n, or 0. A setext
// heading's level comes from the underline on the line after.
function headingLevel(state: EditorState, n: number): number {
  const text = state.doc.line(n).text;
  const m = atxHeading.exec(text);
  if (m) return m[1].length;
  if (n >= state.doc.lines || !text.trim() || /^\s*([-*+]|\d+[.)])\s/.test(text) || /^\s*(>|<!--)/.test(text)) return 0;
  const under = state.doc.line(n + 1).text;
  if (/^ {0,3}=+\s*$/.test(under)) return 1;
  if (/^ {0,3}-+\s*$/.test(under)) return 2;
  return 0;
}

// foldAt is the range that folding line n hides, or null.
function foldAt(state: EditorState, n: number): { from: number; to: number } | null {
  const o = state.field(outlineField, false);
  if (!o) return null;
  const i = o.at(n);
  if (i >= o.lines.length || o.lines[i] !== n) return null;
  let end = o.end(i, state.doc.lines);
  if (o.info[i] < 0) {
    // A section's fold leaves out the blank lines at its end.
    while (end > n && !state.doc.line(end).text.trim()) end--;
  }
  if (end <= n) return null;
  return { from: state.doc.line(n).to, to: state.doc.line(end).to };
}

// sourceFolding folds headings, fences and output regions, with arrows
// in a gutter and CodeMirror's fold keys.
export function sourceFolding(): Extension {
  return [
    outlineField,
    foldService.of((state, from) => foldAt(state, state.doc.lineAt(from).number)),
    codeFolding({ placeholderText: "…" }),
    foldGutter(),
    keymap.of(foldKeymap),
  ];
}

// sourceFolds are the lines, counted from 1, whose folds are closed.
export function sourceFolds(view: EditorView): number[] {
  const lines: number[] = [];
  foldedRanges(view.state).between(0, view.state.doc.length, (from) => {
    lines.push(view.state.doc.lineAt(from).number);
  });
  return lines;
}

// setSourceFolds closes the folds on these lines, counted from 1, and
// opens the rest.
export function setSourceFolds(view: EditorView, lines: number[]) {
  const state = view.state;
  const doc = state.doc;
  const want: { from: number; to: number }[] = [];
  for (const n of [...new Set(lines)].sort((a, b) => a - b)) {
    if (n < 1 || n > doc.lines) continue;
    const line = doc.line(n);
    const range = foldable(state, line.from, line.to);
    if (range) want.push(range);
  }
  const have: { from: number; to: number }[] = [];
  foldedRanges(state).between(0, doc.length, (from, to) => {
    have.push({ from, to });
  });
  if (want.length === have.length && want.every((r, i) => r.from === have[i].from && r.to === have[i].to)) return;
  view.dispatch({ effects: [...have.map((r) => unfoldEffect.of(r)), ...want.map((r) => foldEffect.of(r))] });
}

// foldMarks names the closed folds by their line's text and which of the
// lines alike it is, so restoreFolds can find them again after the text is
// replaced from outside, as when a run writes output.
export function foldMarks(view: EditorView): string[] {
  const doc = view.state.doc;
  const folded = new Set(sourceFolds(view));
  const seen = new Map<string, number>();
  const marks: string[] = [];
  for (let n = 1; n <= doc.lines; n++) {
    const text = doc.line(n).text;
    const nth = seen.get(text) ?? 0;
    seen.set(text, nth + 1);
    if (folded.has(n)) marks.push(nth + ":" + text);
  }
  return marks;
}

// restoreFolds closes the folds foldMarks named, where their lines
// still open one.
export function restoreFolds(view: EditorView, marks: string[]) {
  const doc = view.state.doc;
  const want = new Set(marks);
  const seen = new Map<string, number>();
  const lines: number[] = [];
  for (let n = 1; n <= doc.lines; n++) {
    const text = doc.line(n).text;
    const nth = seen.get(text) ?? 0;
    seen.set(text, nth + 1);
    if (want.has(nth + ":" + text)) lines.push(n);
  }
  setSourceFolds(view, lines);
}
