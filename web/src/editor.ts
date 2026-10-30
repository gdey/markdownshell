// Help for the mdsh attributes in opening fences, and Vim's :w and :q,
// for the markdown editors.

import { type EditorState, type Extension } from "@codemirror/state";
import { EditorView, hoverTooltip, type Tooltip } from "@codemirror/view";
import { autocompletion, type Completion, type CompletionContext, type CompletionResult } from "@codemirror/autocomplete";
import { type Diagnostic, linter } from "@codemirror/lint";
import { Vim } from "@replit/codemirror-vim";

import { App, AttrKind, type AttrSpec, type Names } from "./api.js";

// fence matches an opening fence line: the fence, then the info string.
const fence = /^(\s*(?:`{3,}|~{3,}))(.*)$/;

// ExTarget is what Vim's :w and :q act on in one editor.
export interface ExTarget {
  view: EditorView;
  // write applies the edit and saves the file.
  write(): Promise<void>;
  close(): void;
}

// active is the editor Vim's ex commands act on: the last one focused.
let active: ExTarget | null = null;

Vim.defineEx("write", "w", () => void exWrite(false));
Vim.defineEx("wq", "wq", () => void exWrite(true));
Vim.defineEx("xit", "x", () => void exWrite(true));
Vim.defineEx("quit", "q", () => active?.close());

async function exWrite(close: boolean) {
  const t = active;
  if (!t) return;
  await t.write();
  if (close) t.close();
}

// exTarget makes an editor the one ex commands act on while it has focus.
export function exTarget(t: () => ExTarget | null): Extension {
  return EditorView.domEventHandlers({ focus: () => void (active = t()) });
}

// forgetEx stops ex commands acting on view once it is gone.
export function forgetEx(view: EditorView) {
  if (active?.view === view) active = null;
}

// attrHelp is the help for mdsh attributes in opening fences: completion
// inside {…}, hover help and underlined mistakes.
export function attrHelp(attrs: AttrSpec[], names: () => Names): Extension {
  return [
    autocompletion({ override: [attrCompletions(attrs, names)], activateOnTyping: true }),
    hoverTooltip(attrHover(attrs)),
    linter(checkInfo, { delay: 300 }),
  ];
}

// infoRange is where the info string is on the line holding pos, if
// that line opens a block's fence, or null. Output and tape fences
// carry mdsh's own attributes, not a block's.
function infoRange(state: EditorState, pos: number): { from: number; text: string } | null {
  const line = state.doc.lineAt(pos);
  const m = fence.exec(line.text);
  if (!m || /^\s*(output|tape)\b/.test(m[2])) return null;
  return { from: line.from + m[1].length, text: m[2] };
}

// checkInfo checks the attributes of every fence with some.
async function checkInfo(view: EditorView): Promise<Diagnostic[]> {
  const out: Diagnostic[] = [];
  const doc = view.state.doc;
  for (let n = 1; n <= doc.lines; n++) {
    const line = doc.line(n);
    if (!line.text.includes("{")) continue;
    const r = infoRange(view.state, line.from);
    if (!r || !r.text.trim()) continue;
    const problems = (await App.CheckInfo(r.text)) ?? [];
    for (const p of problems) {
      out.push({ from: r.from + p.from, to: r.from + p.to, severity: p.severity === "error" ? "error" : "warning", message: p.message });
    }
  }
  return out;
}

// token finds the attribute token around pos in the info string: its
// start and text so far.
function tokenAt(state: EditorState, pos: number): { start: number; text: string } | null {
  const r = infoRange(state, pos);
  if (!r || pos < r.from || pos > r.from + r.text.length) return null;
  const before = r.text.slice(0, pos - r.from);
  const open = before.lastIndexOf("{");
  if (open < 0 || before.lastIndexOf("}") > open) return null;
  let start = open + 1;
  let quoted = false;
  for (let i = open + 1; i < before.length; i++) {
    if (before[i] === '"') quoted = !quoted;
    else if (!quoted && /\s/.test(before[i])) start = i + 1;
  }
  return { start: r.from + start, text: before.slice(start) };
}

function attrCompletions(attrs: AttrSpec[], names: () => Names) {
  return (ctx: CompletionContext): CompletionResult | null => {
    const tok = tokenAt(ctx.state, ctx.pos);
    if (!tok) return null;
    const eq = tok.text.indexOf("=");
    if (eq < 0) {
      const present = new Set((infoRange(ctx.state, ctx.pos)?.text.match(/[{\s]([#<\w-]+)/g) ?? []).map((s) => s.slice(1).replace(/^#.*/, "#")));
      const include = present.has("<include");
      const options: Completion[] = attrs
        .filter((a) => !!a.include === include || a.name === "<include")
        .filter((a) => !present.has(a.name) || tok.text.startsWith(a.name))
        .map((a) => ({
          label: a.name === "#" ? "#id" : a.name,
          apply: a.name === "#" ? "#" : a.kind === AttrKind.AttrFlag ? a.name : a.bare ? a.name : a.name + "=",
          detail: a.default ? "default: " + a.default : undefined,
          info: a.help,
          type: a.kind === AttrKind.AttrFlag ? "keyword" : "property",
        }));
      return { from: tok.start, options, validFor: /^[#<\w-]*$/ };
    }
    const key = tok.text.slice(0, eq);
    const spec = attrs.find((a) => a.name === key);
    if (!spec) return null;
    const known = names();
    const from = tok.start + eq + 1;
    let values: Completion[] = [];
    switch (spec.kind) {
      case AttrKind.AttrChoice:
        values = (spec.values ?? []).map((v) => ({ label: v, type: "enum" }));
        break;
      case AttrKind.AttrText:
        if (key === "env") values = known.envs?.map((v) => ({ label: v, type: "variable" })) ?? [];
        break;
      case AttrKind.AttrCond:
        values = [
          { label: "{{$VAR}}", detail: "true unless empty, 0, false, no or off", type: "variable" },
          { label: "{{$?}}", detail: "the last block's exit status", type: "variable" },
          { label: "{{$ran?}}", detail: "whether the last block ran", type: "variable" },
          ...(known.ids ?? []).flatMap((id) => [
            { label: `{{#${id}/?}}`, detail: `#${id}'s exit status`, type: "variable" },
            { label: `{{#${id}/ran?}}`, detail: `whether #${id} ran`, type: "variable" },
          ]),
        ];
        break;
      case AttrKind.AttrDuration:
        values = ["30s", "5m", "1h"].map((v) => ({ label: v, type: "constant" }));
        break;
    }
    if (!values.length) return null;
    return { from, options: values };
  };
}

function attrHover(attrs: AttrSpec[]) {
  return (view: EditorView, pos: number): Tooltip | null => {
    const r = infoRange(view.state, pos);
    if (!r || pos < r.from || pos > r.from + r.text.length) return null;
    const re = /[#<\w-]+(=("[^"]*"|\S*))?/g;
    for (let m; (m = re.exec(r.text)); ) {
      const from = r.from + m.index;
      const to = from + m[0].length;
      if (pos < from || pos > to) continue;
      const key = m[0].startsWith("#") ? "#" : m[0].split("=")[0];
      const spec = attrs.find((a) => a.name === key);
      if (!spec) return null;
      return {
        pos: from,
        end: to,
        above: true,
        create: () => {
          const dom = document.createElement("div");
          dom.className = "attr-help";
          dom.append(Object.assign(document.createElement("b"), { textContent: key === "#" ? "#id" : key }), " " + spec.help);
          if (spec.default) dom.append(Object.assign(document.createElement("div"), { className: "muted", textContent: "Default: " + spec.default }));
          return { dom };
        },
      };
    }
    return null;
  };
}
