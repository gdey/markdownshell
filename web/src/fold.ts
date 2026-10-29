// Folding in the source pane: a heading folds its section, down to the
// next heading of the same or a higher level, and a fence or an
// mdsh:output region folds to its closing line. The rendered view keeps
// its own folds; sourceFolds and setSourceFolds copy them across.

import { type EditorState, type Extension, type Text } from "@codemirror/state";
import { type EditorView, keymap } from "@codemirror/view";
import { codeFolding, foldEffect, foldGutter, foldKeymap, foldService, foldable, foldedRanges, unfoldEffect } from "@codemirror/language";

const fenceOpen = /^(\s*(?:>\s*)*(?:[-*+]\s+|\d+[.)]\s+)?)(`{3,}|~{3,})/;
const atx = /^ {0,3}(#{1,6})(?:\s|$)/;
const regionOpen = /^\s*(?:>\s*)*<!-- mdsh:output\b.*-->\s*$/;
const regionClose = /^\s*(?:>\s*)*<!-- \/mdsh:output -->\s*$/;

// outline is where the folds in a document are: for each line that
// opens one, counted from 1, the last line it hides. It is worked out
// once for each version of the text.
const outlines = new WeakMap<Text, Map<number, number>>();

function outline(state: EditorState): Map<number, number> {
  const doc = state.doc;
  let folds = outlines.get(doc);
  if (folds) return folds;
  folds = new Map();
  const headings: { line: number; level: number }[] = []; // open sections
  const close = (line: number, level: number) => {
    while (headings.length && headings[headings.length - 1].level >= level) {
      const h = headings.pop()!;
      let end = line - 1;
      while (end > h.line && !doc.line(end).text.trim()) end--;
      if (end > h.line) folds.set(h.line, end);
    }
  };
  for (let n = 1; n <= doc.lines; n++) {
    const text = doc.line(n).text;
    const f = fenceOpen.exec(text);
    if (f) {
      const fence = f[2];
      const closing = new RegExp("^\\s*(?:>\\s*)*" + (fence[0] === "`" ? "`" : "~") + "{" + fence.length + ",}\\s*$");
      let end = n + 1;
      while (end <= doc.lines && !closing.test(doc.line(end).text)) end++;
      end = Math.min(end, doc.lines);
      if (end > n) folds.set(n, end);
      n = end;
      continue;
    }
    if (regionOpen.test(text)) {
      let end = n + 1;
      while (end <= doc.lines && !regionClose.test(doc.line(end).text)) end++;
      if (end <= doc.lines) {
        folds.set(n, end);
        // A region holds markdown, so its headings are left alone.
        n = end;
        continue;
      }
    }
    const level = headingLevel(state, n);
    if (level > 0) {
      close(n, level);
      headings.push({ line: n, level });
      if (!atx.test(text)) n++; // the setext underline
    }
  }
  close(doc.lines + 1, 0);
  outlines.set(doc, folds);
  return folds;
}

// headingLevel is the level of the heading on line n, or 0. A setext
// heading's level comes from the underline on the line after.
function headingLevel(state: EditorState, n: number): number {
  const text = state.doc.line(n).text;
  const m = atx.exec(text);
  if (m) return m[1].length;
  if (n >= state.doc.lines || !text.trim() || /^\s*([-*+]|\d+[.)])\s/.test(text) || /^\s*(>|<!--)/.test(text)) return 0;
  const under = state.doc.line(n + 1).text;
  if (/^ {0,3}=+\s*$/.test(under)) return 1;
  if (/^ {0,3}-+\s*$/.test(under)) return 2;
  return 0;
}

// foldAt is the range that folding line n hides, or null.
function foldAt(state: EditorState, n: number): { from: number; to: number } | null {
  const end = outline(state).get(n);
  if (!end) return null;
  return { from: state.doc.line(n).to, to: state.doc.line(end).to };
}

// sourceFolding folds headings, fences and output regions, with arrows
// in a gutter and CodeMirror's fold keys.
export function sourceFolding(): Extension {
  return [
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
