// The source pane's folds, worked out again only around each edit, must
// be the folds of the text worked out afresh. This makes random
// documents of the lines that open and close folds, edits them at
// random, and compares the two after every edit. internal/build's tests
// bundle and run it with node.

import { EditorState } from "@codemirror/state";
import { foldable } from "@codemirror/language";
import { sourceFolding } from "../src/fold.js";

const pieces = ["# H1", "## H2", "### H3", "text", "", "  ", "```sh", "```", "~~~", "~~~~", "Title", "===", "---", "- item", "> ```", "<!-- mdsh:output -->", "<!-- /mdsh:output -->", "<!-- note -->", "1. x", "#nohead", "    code"];

let seed = 1;
const rand = (n: number) => {
  seed = (seed * 1103515245 + 12345) & 0x7fffffff;
  return seed % n;
};
const piece = () => pieces[rand(pieces.length)];

const folding = sourceFolding();
const folds = (s: EditorState) => {
  const out: string[] = [];
  for (let n = 1; n <= s.doc.lines; n++) {
    const line = s.doc.line(n);
    const r = foldable(s, line.from, line.to);
    if (r) out.push(`${n}:${r.from}-${r.to}`);
  }
  return out.join(" ");
};

for (let doc = 0; doc < 2000; doc++) {
  let s = EditorState.create({ doc: Array.from({ length: 5 + rand(60) }, piece).join("\n"), extensions: folding });
  for (let edit = 0; edit < 30; edit++) {
    const from = rand(s.doc.length + 1);
    const to = Math.min(s.doc.length, from + (rand(3) ? 0 : rand(40)));
    const insert = rand(4) === 0 ? "" : rand(2) ? "\n" + piece() : rand(2) ? piece() : "x";
    const before = s.doc.toString();
    s = s.update({ changes: { from, to, insert } }).state;
    const got = folds(s);
    const want = folds(EditorState.create({ doc: s.doc, extensions: folding }));
    if (got !== want) {
      console.log(`after replacing ${from}-${to} with ${JSON.stringify(insert)} in\n${before}\n---\nthe folds are\n  ${got}\nnot\n  ${want}`);
      process.exit(1);
    }
  }
}
