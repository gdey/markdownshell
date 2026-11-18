// Masking in the source pane: a secret block's code, and the output of a
// block whose output may hold a secret, are blurred there as on its card
// in the rendered view, until the card's eye button shows them (finding
// 9 of the v1.4.0 security review). The fences stay clear, so it is
// plain which block is masked.

import { type EditorState, type Extension, RangeSet, StateEffect, StateField, type Text } from "@codemirror/state";
import { Decoration, type DecorationSet, EditorView } from "@codemirror/view";

// SecretLines names the fences, counted from 1, whose blocks' lines are
// masked: code for a block whose code holds a secret, output for one
// whose saved output starts there.
export interface SecretLines {
  code: number[];
  output: number[];
}

// setSecretLines masks these blocks. The masks move with the text as it
// is typed, until the next set.
export const setSecretLines = StateEffect.define<SecretLines>();

const masked = Decoration.line({ class: "cm-secret" });

const opening = /^\s*(`{3,}|~{3,})/;

// inside is the lines between the fence opening on line n and the one
// closing it, or the end of the text when nothing does.
export function inside(doc: Text, n: number): number[] {
  if (n < 1 || n > doc.lines) return [];
  const m = opening.exec(doc.line(n).text);
  if (!m) return [];
  const fence = m[1];
  const close = new RegExp("^\\s*" + fence[0] + "{" + fence.length + ",}\\s*$");
  const lines: number[] = [];
  for (let i = n + 1; i <= doc.lines; i++) {
    if (close.test(doc.line(i).text)) break;
    lines.push(i);
  }
  return lines;
}

function decorations(state: EditorState, s: SecretLines): DecorationSet {
  const doc = state.doc;
  const lines = new Set<number>();
  for (const n of [...s.code, ...s.output]) for (const l of inside(doc, n)) lines.add(l);
  return RangeSet.of(
    [...lines].sort((a, b) => a - b).map((l) => masked.range(doc.line(l).from)),
    true,
  );
}

const secretLines = StateField.define<DecorationSet>({
  create: () => Decoration.none,
  update(set, tr) {
    set = set.map(tr.changes);
    for (const e of tr.effects) if (e.is(setSecretLines)) set = decorations(tr.state, e.value);
    return set;
  },
  provide: (f) => EditorView.decorations.from(f),
});

// secretMask is the source pane's masking.
export function secretMask(): Extension {
  return secretLines;
}
