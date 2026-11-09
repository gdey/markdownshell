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

const runLines = StateField.define<RangeSet<GutterMarker>>({
  create: () => RangeSet.empty,
  update(set, tr) {
    set = set.map(tr.changes);
    for (const e of tr.effects) {
      if (!e.is(setRunLines)) continue;
      const doc = tr.state.doc;
      const sections = new Set(e.value.sections);
      const lines = [...new Set([...e.value.blocks, ...sections])].filter((n) => n >= 1 && n <= doc.lines).sort((a, b) => a - b);
      set = RangeSet.of(lines.map((n) => (sections.has(n) ? sectionMarker : marker).range(doc.line(n).from)));
    }
    return set;
  },
});

function hasButton(view: EditorView, pos: number): boolean {
  let found = false;
  view.state.field(runLines).between(pos, pos, () => {
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
      markers: (view) => view.state.field(runLines),
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
