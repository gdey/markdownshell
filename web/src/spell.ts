// Spell checking in the markdown editors, by the webview's own checker:
// the prose is checked, and code, links and HTML are not.

import { RangeSetBuilder, type Extension } from "@codemirror/state";
import { Decoration, type DecorationSet, EditorView, ViewPlugin, type ViewUpdate } from "@codemirror/view";
import { syntaxTree } from "@codemirror/language";

// blockNodes and inlineNodes are the markdown nodes whose text isn't
// prose. Blocks are left out a line at a time, the rest as marks.
const blockNodes = new Set(["FencedCode", "CodeBlock", "HTMLBlock", "CommentBlock", "ProcessingInstructionBlock"]);
const inlineNodes = new Set(["InlineCode", "URL", "Autolink", "HTMLTag", "Comment", "LinkLabel", "Entity"]);

const lineOff = Decoration.line({ attributes: { spellcheck: "false" } });
const markOff = Decoration.mark({ attributes: { spellcheck: "false" } });

function decorations(view: EditorView): DecorationSet {
  const marks: { from: number; to: number; line: boolean }[] = [];
  const doc = view.state.doc;
  for (const { from, to } of view.visibleRanges) {
    syntaxTree(view.state).iterate({
      from,
      to,
      enter: (n) => {
        if (blockNodes.has(n.name)) {
          for (let l = doc.lineAt(n.from).number, last = doc.lineAt(n.to).number; l <= last; l++) {
            const line = doc.line(l);
            if (line.from >= from && line.from <= to) marks.push({ from: line.from, to: line.from, line: true });
          }
          return false;
        }
        if (inlineNodes.has(n.name)) {
          if (n.to > n.from) marks.push({ from: n.from, to: n.to, line: false });
          return false;
        }
      },
    });
  }
  marks.sort((a, b) => a.from - b.from || Number(b.line) - Number(a.line));
  const b = new RangeSetBuilder<Decoration>();
  let lastLine = -1;
  for (const m of marks) {
    if (m.line) {
      if (m.from === lastLine) continue;
      lastLine = m.from;
      b.add(m.from, m.from, lineOff);
    } else b.add(m.from, m.to, markOff);
  }
  return b.finish();
}

const proseOnly = ViewPlugin.fromClass(
  class {
    decorations: DecorationSet;
    constructor(view: EditorView) {
      this.decorations = decorations(view);
    }
    update(u: ViewUpdate) {
      if (u.docChanged || u.viewportChanged || syntaxTree(u.startState) !== syntaxTree(u.state)) this.decorations = decorations(u.view);
    }
  },
  { decorations: (v) => v.decorations },
);

// spellCheck turns the webview's spell checker on for the prose, or off.
export function spellCheck(on: boolean): Extension {
  if (!on) return EditorView.contentAttributes.of({ spellcheck: "false" });
  return [EditorView.contentAttributes.of({ spellcheck: "true", autocorrect: "off", autocapitalize: "off" }), proseOnly];
}
