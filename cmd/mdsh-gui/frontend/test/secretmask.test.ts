// The source pane masks the lines inside a secret block's fences, and
// the masks follow the text as it is typed, until the next set.

import { EditorState } from "@codemirror/state";
import { EditorView } from "@codemirror/view";
import { secretMask, setSecretLines } from "../src/secretmask.js";

function fail(msg: string): never {
  console.error(msg);
  (globalThis as unknown as { process: { exit(n: number): void } }).process.exit(1);
  throw new Error(msg);
}

// masked is the lines, counted from 1, with the cm-secret class.
const masked = (s: EditorState) => {
  const out: number[] = [];
  for (const d of s.facet(EditorView.decorations)) {
    if (typeof d === "function") continue;
    d.between(0, s.doc.length, (from) => void out.push(s.doc.lineAt(from).number));
  }
  return out.join(" ");
};

const text = ["# Doc", "", "```sh {#tok secret}", "echo one", "echo two", "```", "", "````output {for=tok}", "s3cr3t", "```", "````", "", "~~~sh", "never closed"].join("\n");
let s = EditorState.create({ doc: text, extensions: secretMask() });
if (masked(s) !== "") fail(`masked before a set: ${masked(s)}`);

s = s.update({ effects: setSecretLines.of({ code: [3], output: [8] }) }).state;
if (masked(s) !== "4 5 9 10") fail(`masked ${masked(s)}, want 4 5 9 10, the code and the output, not the fences, and a shorter fence doesn't close a longer one`);

// A line typed above moves the masks down with the text.
s = s.update({ changes: { from: 0, insert: "new\n" } }).state;
if (masked(s) !== "5 6 10 11") fail(`after typing a line above: ${masked(s)}, want 5 6 10 11`);

// A fence that is never closed masks to the end; one that isn't a fence
// masks nothing.
s = s.update({ effects: setSecretLines.of({ code: [14, 2], output: [] }) }).state;
if (masked(s) !== "15") fail(`masked ${masked(s)}, want 15`);

s = s.update({ effects: setSecretLines.of({ code: [], output: [] }) }).state;
if (masked(s) !== "") fail(`masked after clearing: ${masked(s)}`);
