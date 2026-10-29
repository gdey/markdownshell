// The block editor: CodeMirror on a block's text, fences included, with
// help for the attributes in its opening fence and an optional Vim mode.

import { Compartment, EditorState, type Extension, Prec } from "@codemirror/state";
import { EditorView, drawSelection, highlightActiveLine, hoverTooltip, keymap, lineNumbers, type Tooltip } from "@codemirror/view";
import { defaultKeymap, history, historyKeymap, indentWithTab } from "@codemirror/commands";
import { StreamLanguage, bracketMatching, defaultHighlightStyle, syntaxHighlighting } from "@codemirror/language";
import { autocompletion, type Completion, type CompletionContext, type CompletionResult } from "@codemirror/autocomplete";
import { type Diagnostic, linter } from "@codemirror/lint";
import { shell } from "@codemirror/legacy-modes/mode/shell";
import { Vim, vim } from "@replit/codemirror-vim";

import { App, AttrKind, type AttrSpec, type Names } from "./api.js";

export interface BlockEditorOptions {
  text: string;
  vim: boolean;
  attrs: AttrSpec[];
  names: Names;
  // save applies the edit to the document; it rejects if the text isn't
  // one block.
  save(text: string): Promise<void>;
  // saveFile writes the file, for :w.
  saveFile(): Promise<void>;
  close(): void;
}

export interface BlockEditor {
  view: EditorView;
  setVim(on: boolean): void;
  // info is the opening fence's info string and setInfo replaces it.
  info(): string;
  setInfo(info: string): void;
  destroy(): void;
}

// fence matches an opening fence line: the fence, then the info string.
const fence = /^(\s*(?:`{3,}|~{3,}))(.*)$/;

// active is the editor Vim's ex commands act on.
let active: { opts: BlockEditorOptions; view: EditorView } | null = null;

Vim.defineEx("write", "w", () => void exWrite(false));
Vim.defineEx("wq", "wq", () => void exWrite(true));
Vim.defineEx("xit", "x", () => void exWrite(true));
Vim.defineEx("quit", "q", () => active?.opts.close());

async function exWrite(close: boolean) {
  if (!active) return;
  const { opts, view } = active;
  await opts.save(view.state.doc.toString());
  await opts.saveFile();
  if (close) opts.close();
}

export function blockEditor(parent: HTMLElement, opts: BlockEditorOptions): BlockEditor {
  const vimMode = new Compartment();
  const escape = new Compartment();
  const escapeKey = (on: boolean): Extension =>
    on ? [] : keymap.of([{ key: "Escape", run: () => (opts.close(), true) }]);
  // Mod-Enter applies the edit and closes the editor, as Done does.
  const save = () => {
    opts.save(view.state.doc.toString()).then(opts.close, () => {});
    return true;
  };
  const view = new EditorView({
    parent,
    state: EditorState.create({
      doc: opts.text,
      extensions: [
        vimMode.of(opts.vim ? vim() : []),
        lineNumbers(),
        history(),
        drawSelection(),
        highlightActiveLine(),
        bracketMatching(),
        StreamLanguage.define(shell),
        syntaxHighlighting(defaultHighlightStyle, { fallback: true }),
        Prec.high(keymap.of([{ key: "Mod-Enter", run: save }])),
        escape.of(escapeKey(opts.vim)),
        keymap.of([...defaultKeymap, ...historyKeymap, indentWithTab]),
        autocompletion({ override: [attrCompletions(opts)], activateOnTyping: true }),
        hoverTooltip(attrHover(opts.attrs)),
        linter(checkInfo, { delay: 300 }),
        EditorView.domEventHandlers({ focus: () => void (active = { opts, view }) }),
        EditorView.theme({ "&": { fontSize: "13px" }, ".cm-content": { fontFamily: "ui-monospace, SFMono-Regular, Menlo, monospace" } }),
      ],
    }),
  });
  active = { opts, view };
  // Start on the first line of the body, where the code is.
  const body = view.state.doc.lines > 1 ? view.state.doc.line(2).from : 0;
  view.dispatch({ selection: { anchor: body } });
  view.focus();

  const firstLine = () => view.state.doc.line(1);
  return {
    view,
    setVim(on) {
      view.dispatch({ effects: [vimMode.reconfigure(on ? vim() : []), escape.reconfigure(escapeKey(on))] });
    },
    info() {
      return fence.exec(firstLine().text)?.[2].trim() ?? "";
    },
    setInfo(info) {
      const line = firstLine();
      const m = fence.exec(line.text);
      if (!m) return;
      view.dispatch({ changes: { from: line.from + m[1].length, to: line.to, insert: info } });
    },
    destroy() {
      if (active?.view === view) active = null;
      view.destroy();
    },
  };
}

// infoRange is where the info string is in the first line, or null.
function infoRange(state: EditorState): { from: number; text: string } | null {
  const line = state.doc.line(1);
  const m = fence.exec(line.text);
  if (!m) return null;
  return { from: line.from + m[1].length, text: m[2] };
}

async function checkInfo(view: EditorView): Promise<Diagnostic[]> {
  const r = infoRange(view.state);
  if (!r || !r.text.trim()) return [];
  const problems = (await App.CheckInfo(r.text)) ?? [];
  return problems.map((p) => ({
    from: r.from + p.from,
    to: r.from + p.to,
    severity: p.severity === "error" ? "error" : "warning",
    message: p.message,
  }));
}

// token finds the attribute token around pos in the info string: its
// start and text so far.
function tokenAt(state: EditorState, pos: number): { start: number; text: string } | null {
  const r = infoRange(state);
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

function attrCompletions(opts: BlockEditorOptions) {
  return (ctx: CompletionContext): CompletionResult | null => {
    const tok = tokenAt(ctx.state, ctx.pos);
    if (!tok) return null;
    const eq = tok.text.indexOf("=");
    if (eq < 0) {
      const present = new Set((infoRange(ctx.state)?.text.match(/[{\s]([#<\w-]+)/g) ?? []).map((s) => s.slice(1).replace(/^#.*/, "#")));
      const include = present.has("<include");
      const options: Completion[] = opts.attrs
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
    const spec = opts.attrs.find((a) => a.name === key);
    if (!spec) return null;
    const from = tok.start + eq + 1;
    let values: Completion[] = [];
    switch (spec.kind) {
      case AttrKind.AttrChoice:
        values = (spec.values ?? []).map((v) => ({ label: v, type: "enum" }));
        break;
      case AttrKind.AttrText:
        if (key === "env") values = opts.names.envs?.map((v) => ({ label: v, type: "variable" })) ?? [];
        break;
      case AttrKind.AttrCond:
        values = [
          { label: "{{$VAR}}", detail: "true unless empty, 0, false, no or off", type: "variable" },
          { label: "{{$?}}", detail: "the last block's exit status", type: "variable" },
          { label: "{{$ran?}}", detail: "whether the last block ran", type: "variable" },
          ...(opts.names.ids ?? []).flatMap((id) => [
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
    const r = infoRange(view.state);
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
