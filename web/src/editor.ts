// Help for the mdsh attributes in opening fences, and Vim's :w, :q and
// :RunBlock, for the markdown editors.

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
  // run runs the block at the cursor, for :RunBlock.
  run?(): void;
}

// active is the editor Vim's ex commands act on: the last one focused.
let active: ExTarget | null = null;

Vim.defineEx("write", "w", () => void exWrite(false));
Vim.defineEx("wq", "wq", () => void exWrite(true));
Vim.defineEx("xit", "x", () => void exWrite(true));
Vim.defineEx("quit", "q", () => active?.close());
Vim.defineEx("RunBlock", "RunBlock", () => active?.run?.());

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

// clickFocus makes a click in an editor that doesn't have the focus put
// the cursor where it lands without scrolling. The pane may have been
// scrolled away from the cursor, as the split view's panes follow each
// other, and WebKit scrolls an editable to its caret when it takes the
// focus: in a file just opened, to the top. So the cursor moves to the
// click first, the editor takes the focus, and the scroll is put back
// before CodeMirror reads where the click is.
export function clickFocus(): Extension {
  return EditorView.domEventHandlers({
    mousedown(e, view) {
      if (view.hasFocus || e.button !== 0 || e.shiftKey || e.altKey || e.metaKey || e.ctrlKey) return false;
      const pos = view.posAtCoords({ x: e.clientX, y: e.clientY });
      if (pos == null) return false;
      const s = view.scrollDOM;
      const [top, left] = [s.scrollTop, s.scrollLeft];
      view.dispatch({ selection: { anchor: pos }, userEvent: "select.pointer" });
      view.focus();
      s.scrollTop = top;
      s.scrollLeft = left;
      return false;
    },
  });
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
  const vars = new Map<string, number>(); // var= names, and the line binding each
  for (let n = 1; n <= doc.lines; n++) {
    const line = doc.line(n);
    if (!line.text.includes("{")) continue;
    const r = infoRange(view.state, line.from);
    if (!r || !r.text.trim()) continue;
    const problems = (await App.CheckInfo(r.text)) ?? [];
    for (const p of problems) {
      out.push({ from: r.from + p.from, to: r.from + p.to, severity: p.severity === "error" ? "error" : "warning", message: p.message });
    }
    // Only one textbox or choice can bind a variable, or the last would
    // quietly win.
    const v = /^\s*(?:textbox|choice)\b/.test(r.text) && !r.text.includes("<include") ? /[{\s]var="?([A-Za-z0-9_]+)"?/.exec(r.text) : null;
    if (!v) continue;
    const first = vars.get(v[1]);
    if (first === undefined) vars.set(v[1], n);
    else out.push({ from: r.from + v.index + 1, to: r.from + v.index + v[0].length, severity: "error", message: `var=${v[1]} is already bound on line ${first}, so this one is ignored` });
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

// inputAttrs are the attributes, besides their own, that mean something
// on a textbox or a choice.
const inputAttrs = new Set(["#", "show"]);

function attrCompletions(attrs: AttrSpec[], names: () => Names) {
  return (ctx: CompletionContext): CompletionResult | null => {
    const tok = tokenAt(ctx.state, ctx.pos);
    if (!tok) return null;
    const eq = tok.text.indexOf("=");
    if (eq < 0) {
      const present = new Set((infoRange(ctx.state, ctx.pos)?.text.match(/[{\s]([#<\w-]+)/g) ?? []).map((s) => s.slice(1).replace(/^#.*/, "#")));
      const include = present.has("<include");
      const input = include ? "" : (/^\s*(textbox|choice)\b/.exec(infoRange(ctx.state, ctx.pos)?.text ?? "")?.[1] ?? "");
      const options: Completion[] = attrs
        .filter((a) => (input ? !!a.inputs?.includes(input) || inputAttrs.has(a.name) : !a.inputs && (!!a.include === include || a.name === "<include")))
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
    let from = tok.start + eq + 1;
    let values: Completion[] = [];
    // A value typed with an opening quote is completed inside it.
    let value = tok.text.slice(eq + 1);
    const opened = value.startsWith('"');
    if (opened) value = value.slice(1);
    // path is a completion for a file path. A value with spaces or braces
    // needs quotes; one with a quote can't be written at all.
    const path = (label: string, detail: string, boost?: number): Completion[] => {
      if (label.includes('"')) return [];
      const apply = opened ? label + '"' : /[\s{}]/.test(label) ? `"${label}"` : label;
      return [{ label, apply, detail, type: "text", boost }];
    };
    // res://name is name in the document's assets folder; other paths
    // are relative to the document.
    const assetsDir = known.assetsDir ? known.assetsDir + "/" : "the assets folder";
    const res = (prefix: string): Completion[] => (known.assets ?? []).flatMap((f) => path(prefix + "res://" + f, "in " + assetsDir, 1));
    const files = (): Completion[] => (known.files ?? []).flatMap((f) => path(f, "beside the document"));
    switch (spec.kind) {
      case AttrKind.AttrChoice: {
        const kind = key === "out" ? /^(image|screenshot|recording):/.exec(value) : null;
        if (kind) {
          values = res(kind[0]);
          break;
        }
        values = (spec.values ?? []).map((v) => ({ label: v, type: "enum" }));
        break;
      }
      case AttrKind.AttrText:
        if (key === "env") values = known.envs?.map((v) => ({ label: v, type: "variable" })) ?? [];
        if (key === "file") values = value.startsWith("res:") ? res("") : [...res(""), ...files()];
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
        if (key === "run") values.push({ label: "once", detail: "only while the block has no output", type: "constant" });
        break;
      case AttrKind.AttrDuration:
        values = ["30s", "5m", "1h"].map((v) => ({ label: v, type: "constant" }));
        break;
    }
    if (!values.length) return null;
    if (opened) from++;
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
