// Text for the Neovim source pane: Neovim counts lines from 1 and columns
// in UTF-8 bytes, where the page counts UTF-16 units, and a run's output
// goes into Neovim as edits to the lines it changed.

import { EditorState, Text } from "@codemirror/state";

import { diff } from "./sync.js";

import type { Edit } from "./bindings/github.com/gdey/markdownshell/cmd/mdsh-gui/nvim/models.js";

const utf8 = new TextEncoder();

// byteCol is the UTF-8 byte column of ch, a UTF-16 column in text.
export function byteCol(text: string, ch: number): number {
  return utf8.encode(text.slice(0, ch)).length;
}

// charCol is the UTF-16 column of col, a UTF-8 byte column in text.
export function charCol(text: string, col: number): number {
  let bytes = 0;
  let i = 0;
  for (const c of text) {
    if (bytes >= col) break;
    bytes += utf8.encode(c).length;
    i += c.length;
  }
  return i;
}

// stateOf is a CodeMirror state holding text, for the checks and
// commands written for the built-in editor.
export function stateOf(text: string, anchor = 0, head = anchor): EditorState {
  return EditorState.create({ doc: text, selection: { anchor, head } });
}

// offsetOf is the offset in doc of line (from 1) and col (UTF-8 bytes).
export function offsetOf(doc: Text, line: number, col: number): number {
  const l = doc.line(Math.max(1, Math.min(line, doc.lines)));
  return l.from + charCol(l.text, col);
}

// posOf is where offset is in doc, as Neovim counts: the line from 1 and
// the column in UTF-8 bytes.
export function posOf(doc: Text, offset: number): { line: number; col: number } {
  const l = doc.lineAt(offset);
  return { line: l.number, col: byteCol(l.text, offset - l.from) };
}

// bufferLines are text's lines as a Neovim buffer holds them: a last
// newline ends the last line rather than starting another.
function bufferLines(text: string): string[] {
  return text.replace(/\n$/, "").split("\n");
}

// lineEdits are the edits to a buffer's lines that turn before into
// after, or null if they can't be: Neovim then gets the whole text. They
// cover only the lines that differ, so typing elsewhere in the moment
// since Neovim last sent its text isn't touched.
export function lineEdits(before: string, after: string): Edit[] | null {
  const al = bufferLines(before);
  const bl = bufferLines(after);
  const a = Text.of(al);
  const b = Text.of(bl);
  const hunks: { from: number; to: number; nfrom: number; nto: number }[] = [];
  diff(a.toString(), b.toString()).iterChanges((fromA, toA, fromB, toB) => {
    const h = { from: a.lineAt(fromA).number, to: a.lineAt(toA).number, nfrom: b.lineAt(fromB).number, nto: b.lineAt(toB).number };
    const last = hunks[hunks.length - 1];
    if (last && h.from <= last.to) {
      last.to = Math.max(last.to, h.to);
      last.nto = Math.max(last.nto, h.nto);
    } else hunks.push(h);
  });
  const edits = hunks.map((h) => ({ start: h.from - 1, end: h.to, old: al.slice(h.from - 1, h.to), new: bl.slice(h.nfrom - 1, h.nto) }));
  // Check the edits make after of before, as Neovim holds them.
  const got = [...al];
  for (let i = edits.length - 1; i >= 0; i--) {
    const e = edits[i];
    got.splice(e.start, e.end - e.start, ...e.new);
  }
  return got.join("\n") === bl.join("\n") ? edits : null;
}
