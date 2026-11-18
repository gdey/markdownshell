// Text for the Neovim source pane: Neovim counts lines from 1 and columns
// in UTF-8 bytes, where the page counts UTF-16 units.

import { EditorState, Text } from "@codemirror/state";

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
