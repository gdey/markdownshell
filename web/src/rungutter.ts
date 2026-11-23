// A gutter in the source pane with a run button on the opening line of
// each runnable block, and on each heading whose section has one, so a
// block or a section can be run without the rendered view.

import { type Extension, RangeSet, StateEffect, StateField } from "@codemirror/state";
import { type EditorView, GutterMarker, gutter } from "@codemirror/view";
import { icons, svg } from "./dom.js";

class RunMarker extends GutterMarker {
  constructor(readonly title: string) {
    super();
  }
  eq(other: RunMarker) {
    return other.title === this.title;
  }
  toDOM() {
    const s = document.createElement("span");
    s.className = "cm-run";
    s.title = this.title;
    s.append(svg(icons.play));
    return s;
  }
}

const marker = new RunMarker("Run this block");
const sectionMarker = new RunMarker("Run this section");

// setRunLines puts the buttons on these lines, counted from 1: blocks'
// and sections'. The buttons move with the text as it is typed, until
// the next set.
export const setRunLines = StateEffect.define<{ blocks: number[]; sections: number[] }>();

// runLines are the buttons, and the lines they were put on, while no
// edit since has added or taken away a line: then the same lines again
// need no new buttons, as the buttons moved with the edit. Making them
// afresh at every keystroke took a long file's source pane 100 ms.
type Runs = { set: RangeSet<GutterMarker>; blocks: number[] | null; sections: number[] };

const runLines = StateField.define<Runs>({
  create: () => ({ set: RangeSet.empty, blocks: null, sections: [] }),
  update(runs, tr) {
    let { set, blocks, sections } = runs;
    if (!tr.changes.empty) {
      set = set.map(tr.changes);
      tr.changes.iterChanges((fromA, toA, _fromB, _toB, inserted) => {
        if (inserted.lines > 1 || tr.startState.doc.sliceString(fromA, toA).includes("\n")) blocks = null;
      });
    }
    for (const e of tr.effects) {
      if (!e.is(setRunLines)) continue;
      if (blocks && same(blocks, e.value.blocks) && same(sections, e.value.sections)) continue;
      const doc = tr.state.doc;
      const marks = new Set(e.value.sections);
      const lines = [...new Set([...e.value.blocks, ...marks])].filter((n) => n >= 1 && n <= doc.lines).sort((a, b) => a - b);
      set = RangeSet.of(lines.map((n) => (marks.has(n) ? sectionMarker : marker).range(doc.line(n).from)));
      blocks = e.value.blocks;
      sections = e.value.sections;
    }
    return set === runs.set && blocks === runs.blocks ? runs : { set, blocks, sections };
  },
});

function same(a: number[], b: number[]): boolean {
  if (a.length !== b.length) return false;
  for (let i = 0; i < a.length; i++) if (a[i] !== b[i]) return false;
  return true;
}

function hasButton(view: EditorView, pos: number): boolean {
  let found = false;
  view.state.field(runLines).set.between(pos, pos, () => {
    found = true;
    return false;
  });
  return found;
}

// runGutter calls run with the line whose button was clicked.
export function runGutter(run: (line: number) => void): Extension {
  return [
    runLines,
    gutter({
      class: "cm-run-gutter",
      markers: (view) => view.state.field(runLines).set,
      initialSpacer: () => marker,
      domEventHandlers: {
        mousedown(view, line) {
          if (!hasButton(view, line.from)) return false;
          run(view.state.doc.lineAt(line.from).number);
          return true;
        },
      },
    }),
  ];
}
