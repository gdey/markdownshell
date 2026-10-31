// Markdown editors, for the split view's source pane and the inline prose
// editor, and the markdown bar over them.

import { Compartment, EditorSelection, EditorState, type Extension, Prec, type StateCommand } from "@codemirror/state";
import { Decoration, type DecorationSet, EditorView, MatchDecorator, ViewPlugin, type ViewUpdate, drawSelection, highlightActiveLine, keymap, lineNumbers } from "@codemirror/view";
import { defaultKeymap, history, historyKeymap } from "@codemirror/commands";
import { LanguageDescription, LanguageSupport, StreamLanguage } from "@codemirror/language";
import { markdown, markdownKeymap } from "@codemirror/lang-markdown";
import { shell } from "@codemirror/legacy-modes/mode/shell";
import { vim } from "@replit/codemirror-vim";

import type { AttrSpec, Names } from "./api.js";
import { el, iconButton, type icons } from "./dom.js";
import { type ExTarget, attrHelp, exTarget, forgetEx } from "./editor.js";
import { editorLook } from "./theme.js";

export interface MarkdownEditorOptions {
  text: string;
  vim: boolean;
  attrs: AttrSpec[];
  names: () => Names;
  // lineNumbers is on for the source pane.
  lineNumbers?: boolean;
  // extensions are added to the editor's own, such as the source pane's
  // run buttons.
  extensions?: Extension[];
  onChange?(text: string): void;
  // onCursor reports the line the cursor moved to.
  onCursor?(line: number): void;
  // write applies the edit and saves the file, for :w.
  write(): Promise<void>;
  // close is :q, and esc when Vim is off. Mod-Enter is done.
  close(): void;
  done?(): void;
}

export interface MarkdownEditor {
  view: EditorView;
  setVim(on: boolean): void;
  // setText replaces the text without calling onChange.
  setText(text: string): void;
  destroy(): void;
}

const sh = new LanguageSupport(StreamLanguage.define(shell));
const codeLanguages = [LanguageDescription.of({ name: "sh", alias: ["zsh", "bash", "shell"], support: sh })];

// attrMarks highlights the {…} of mdsh attributes on opening fences.
const attrMarks = ViewPlugin.fromClass(
  class {
    decorations: DecorationSet;
    matcher = new MatchDecorator({
      regexp: /^(\s*(?:>\s*)*(?:[-*+]\s+|\d+[.)]\s+)?(?:`{3,}|~{3,})[^{\n]*)(\{[^}\n]*\})/g,
      decorate: (add, from, _to, m) => {
        const start = from + m[1].length;
        add(start, start + m[2].length, Decoration.mark({ class: "cm-mdsh-attrs" }));
      },
    });
    constructor(view: EditorView) {
      this.decorations = this.matcher.createDeco(view);
    }
    update(u: ViewUpdate) {
      this.decorations = this.matcher.updateDeco(u, this.decorations);
    }
  },
  { decorations: (v) => v.decorations },
);

export function markdownEditor(parent: HTMLElement, opts: MarkdownEditorOptions): MarkdownEditor {
  const vimMode = new Compartment();
  const escape = new Compartment();
  const escapeKey = (on: boolean): Extension => (on ? [] : keymap.of([{ key: "Escape", run: () => (opts.close(), true) }]));
  let quiet = false; // setText is changing the text
  const target: ExTarget = { view: null as unknown as EditorView, write: opts.write, close: opts.close };
  const view = new EditorView({
    parent,
    state: EditorState.create({
      doc: opts.text,
      extensions: [
        vimMode.of(opts.vim ? vim() : []),
        opts.lineNumbers ? lineNumbers() : [],
        opts.extensions ?? [],
        history(),
        drawSelection(),
        highlightActiveLine(),
        markdown({ codeLanguages }),
        editorLook,
        attrMarks,
        Prec.high(keymap.of([{ key: "Mod-Enter", run: () => (opts.done?.(), !!opts.done) }, ...barKeys, { key: "Tab", run: tableTab(1) }, { key: "Shift-Tab", run: tableTab(-1) }])),
        escape.of(escapeKey(opts.vim)),
        keymap.of([...markdownKeymap, ...defaultKeymap, ...historyKeymap]),
        attrHelp(opts.attrs, opts.names),
        exTarget(() => target),
        EditorView.lineWrapping,
        EditorView.updateListener.of((u) => {
          if (u.docChanged && !quiet) opts.onChange?.(u.state.doc.toString());
          if (u.selectionSet || u.docChanged) opts.onCursor?.(u.state.doc.lineAt(u.state.selection.main.head).number);
        }),
        EditorView.theme({ "&": { fontSize: "13px" }, ".cm-content": { fontFamily: "ui-monospace, SFMono-Regular, Menlo, monospace" } }),
      ],
    }),
  });
  target.view = view;
  return {
    view,
    setVim(on) {
      view.dispatch({ effects: [vimMode.reconfigure(on ? vim() : []), escape.reconfigure(escapeKey(on))] });
    },
    setText(text) {
      const old = view.state.doc.toString();
      if (text === old) return;
      // Change only the part that differs, so the cursor and the scroll
      // position move with the text around them instead of jumping, as
      // when a save writes output in above them.
      let from = 0;
      while (from < old.length && from < text.length && old[from] === text[from]) from++;
      let end = 0;
      while (end < old.length - from && end < text.length - from && old[old.length - 1 - end] === text[text.length - 1 - end]) end++;
      quiet = true;
      view.dispatch({ changes: { from, to: old.length - end, insert: text.slice(from, text.length - end) } });
      quiet = false;
    },
    destroy() {
      forgetEx(view);
      view.destroy();
    },
  };
}

// Markdown bar commands. Each inserts or wraps markdown at the cursor or
// selection, so the text stays plain markdown.

// wrap puts before and after around the selection, or around placeholder
// text it selects when nothing is selected.
const wrap =
  (before: string, after: string, placeholder: string): StateCommand =>
  ({ state, dispatch }) => {
    dispatch(
      state.update(
        state.changeByRange((r) => {
          const text = state.sliceDoc(r.from, r.to);
          if (text.startsWith(before) && text.endsWith(after) && text.length >= before.length + after.length) {
            const inner = text.slice(before.length, text.length - after.length);
            return { changes: { from: r.from, to: r.to, insert: inner }, range: EditorSelection.range(r.from, r.from + inner.length) };
          }
          const inner = text || placeholder;
          return {
            changes: { from: r.from, to: r.to, insert: before + inner + after },
            range: EditorSelection.range(r.from + before.length, r.from + before.length + inner.length),
          };
        }),
        { scrollIntoView: true, userEvent: "input" },
      ),
    );
    return true;
  };

// link makes the selection a link's text and selects the URL to type.
const link =
  (image: boolean): StateCommand =>
  ({ state, dispatch }) => {
    dispatch(
      state.update(
        state.changeByRange((r) => {
          const text = state.sliceDoc(r.from, r.to) || (image ? "alt text" : "text");
          const head = (image ? "![" : "[") + text + "](";
          return { changes: { from: r.from, to: r.to, insert: head + "url)" }, range: EditorSelection.range(r.from + head.length, r.from + head.length + 3) };
        }),
        { scrollIntoView: true, userEvent: "input" },
      ),
    );
    return true;
  };

// prefixLines toggles a prefix on every line the selection touches. A
// numbered list counts up.
const prefixLines =
  (prefix: (i: number) => string, match: RegExp): StateCommand =>
  ({ state, dispatch }) => {
    const changes: { from: number; to?: number; insert?: string }[] = [];
    const seen = new Set<number>();
    const lines = [];
    for (const r of state.selection.ranges) {
      for (let n = state.doc.lineAt(r.from).number; n <= state.doc.lineAt(r.to).number; n++) {
        if (!seen.has(n)) lines.push(state.doc.line(n)), seen.add(n);
      }
    }
    const all = lines.every((l) => match.test(l.text));
    lines.forEach((l, i) => {
      const m = match.exec(l.text);
      if (all && m) changes.push({ from: l.from, to: l.from + m[0].length });
      else changes.push({ from: l.from, insert: prefix(i) });
    });
    dispatch(state.update({ changes, scrollIntoView: true, userEvent: "input" }));
    return true;
  };

// heading steps the line through #, ## and ### and back to none.
const heading: StateCommand = ({ state, dispatch }) => {
  const line = state.doc.lineAt(state.selection.main.head);
  const m = /^(#{1,6}) /.exec(line.text);
  const level = m ? m[1].length : 0;
  const next = level >= 3 ? "" : "#".repeat(level + 1) + " ";
  dispatch(state.update({ changes: { from: line.from, to: line.from + (m ? m[0].length : 0), insert: next }, scrollIntoView: true, userEvent: "input" }));
  return true;
};

// insertBlock puts text on lines of its own, with a blank line around
// it: in place of the selected lines, $SEL in text standing for them, or
// on a blank line, or after the cursor's line. It then selects pick.
const insertBlock =
  (text: string, pick: string): StateCommand =>
  ({ state, dispatch }) => {
    const r = state.selection.main;
    const first = state.doc.lineAt(r.from);
    const last = state.doc.lineAt(r.to);
    let from = first.from;
    let to = last.to;
    let before = "";
    if (r.empty && first.text.trim() !== "") {
      from = to = first.to;
      before = "\n\n";
    } else if (first.number > 1 && state.doc.line(first.number - 1).text.trim() !== "") {
      before = "\n";
    }
    const sel = r.empty ? "" : state.sliceDoc(from, to);
    const body = text.replace("$SEL", sel);
    const next = last.number < state.doc.lines ? state.doc.line(last.number + 1) : null;
    const after = next && next.text.trim() !== "" ? "\n" : "";
    const base = from + before.length;
    const at = pick === "$SEL" ? text.indexOf("$SEL") : body.indexOf(pick);
    const len = pick === "$SEL" ? sel.length : pick.length;
    dispatch(
      state.update({
        changes: { from, to, insert: before + body + after },
        selection: { anchor: base + at, head: base + at + len },
        scrollIntoView: true,
        userEvent: "input",
      }),
    );
    return true;
  };

// tableTab moves between the cells of a table row, and from the end of
// a row to the next, selecting the cell's text. Outside a table it does
// nothing, so tab keeps its usual meaning.
const tableTab =
  (dir: 1 | -1): StateCommand =>
  ({ state, dispatch }) => {
    const pos = state.selection.main.head;
    const line = state.doc.lineAt(pos);
    const bars = (l: { from: number; text: string }) => [...l.text.matchAll(/(?<!\\)\|/g)].map((m) => l.from + m.index!);
    if (!/^\s*\|/.test(line.text)) return false;
    const here = bars(line);
    const k = here.filter((b) => b < pos).length - 1; // the bar before the cursor
    let cell: [number, number] | null = null;
    if (dir > 0 && k + 2 < here.length) cell = [here[k + 1] + 1, here[k + 2]];
    else if (dir < 0 && k >= 1) cell = [here[k - 1] + 1, here[k]];
    else if (dir > 0) {
      // The next row, past the row of dashes under the header.
      let n = line.number + 1;
      while (n <= state.doc.lines && delimiterRow.test(state.doc.line(n).text)) n++;
      const next = n <= state.doc.lines ? state.doc.line(n) : null;
      const nb = next ? bars(next) : [];
      if (next && /^\s*\|/.test(next.text) && nb.length >= 2) cell = [nb[0] + 1, nb[1]];
    }
    if (!cell) return false;
    let [from, to] = cell;
    while (from < to && state.sliceDoc(from, from + 1) === " ") from++;
    while (to > from && state.sliceDoc(to - 1, to) === " ") to--;
    dispatch(state.update({ selection: { anchor: from, head: to }, scrollIntoView: true }));
    return true;
  };

const delimiterRow = /^\s*\|?(\s*:?-+:?\s*\|)+\s*(:?-+:?\s*)?$/;

const table = "| Column | Column |\n| --- | --- |\n| $SEL |  |";

interface Action {
  // name is the action's name in the source view's Insert menu.
  name: string;
  icon: keyof typeof icons;
  title: string;
  run: StateCommand;
  key?: string;
}

const actions: Action[] = [
  { name: "heading", icon: "heading", title: "Heading", run: heading },
  { name: "bold", icon: "bold", title: "Bold (cmd/ctrl+B)", run: wrap("**", "**", "bold text"), key: "Mod-b" },
  { name: "italic", icon: "italic", title: "Italic (cmd/ctrl+I)", run: wrap("_", "_", "italic text"), key: "Mod-i" },
  { name: "code", icon: "code", title: "Code (cmd/ctrl+E)", run: wrap("`", "`", "code"), key: "Mod-e" },
  { name: "link", icon: "link", title: "Link (cmd/ctrl+K)", run: link(false), key: "Mod-k" },
  { name: "image", icon: "image", title: "Image", run: link(true) },
  { name: "ul", icon: "ul", title: "Bulleted list", run: prefixLines(() => "- ", /^\s*[-*+] (?!\[[ xX]\] )/) },
  { name: "ol", icon: "ol", title: "Numbered list", run: prefixLines((i) => i + 1 + ". ", /^\s*\d+[.)] /) },
  { name: "task", icon: "task", title: "Task list", run: prefixLines(() => "- [ ] ", /^\s*[-*+] \[[ xX]\] /) },
  { name: "quote", icon: "quote", title: "Quote", run: prefixLines(() => "> ", /^\s*> ?/) },
  { name: "table", icon: "table", title: "Table", run: insertBlock(table, "Column") },
  { name: "fence", icon: "fence", title: "Code block", run: insertBlock("```\n$SEL\n```", "$SEL") },
];

// The mdsh block button offers a plain block and one marked confirm.
const mdshBlocks: { name: string; label: string; text: string }[] = [
  { name: "sh", label: "sh block", text: "```sh {#id}\n$SEL\n```" },
  { name: "sh-confirm", label: "sh block, confirm first", text: "```sh {#id confirm}\n$SEL\n```" },
  { name: "zsh", label: "zsh block", text: "```zsh {#id}\n$SEL\n```" },
];

// insertAction runs the markdown bar's action called name, as the source
// view's Insert menu asks, and reports whether there is one.
export function insertAction(view: EditorView, name: string): boolean {
  const a = actions.find((a) => a.name === name);
  const b = mdshBlocks.find((b) => b.name === name);
  const run = a?.run ?? (b && insertBlock(b.text, "id"));
  if (!run) return false;
  run(view);
  view.focus();
  return true;
}

const barKeys = actions.filter((a) => a.key).map((a) => ({ key: a.key!, run: (v: EditorView) => a.run(v) }));

// markdownBar is the bar of common markdown actions over an editor.
export function markdownBar(editor: () => EditorView | null): HTMLElement {
  const bar = el("div", { className: "md-bar", role: "toolbar" });
  const act = (run: StateCommand) => () => {
    const view = editor();
    if (!view) return;
    run(view);
    view.focus();
  };
  for (const a of actions) {
    const b = iconButton(a.icon, a.title, act(a.run));
    // Keep the editor's focus and selection when a button is pressed.
    b.onmousedown = (e) => e.preventDefault();
    bar.append(b);
  }
  const menu = el("div", { className: "md-menu", hidden: true });
  for (const m of mdshBlocks) {
    const item = el("button", {}, m.label);
    item.onmousedown = (e) => e.preventDefault();
    item.onclick = () => {
      menu.hidden = true;
      act(insertBlock(m.text, "id"))();
    };
    menu.append(item);
  }
  const mdsh = iconButton("mdsh", "mdsh block", () => (menu.hidden = !menu.hidden));
  mdsh.onmousedown = (e) => e.preventDefault();
  bar.append(el("span", { className: "md-mdsh" }, mdsh, menu));
  return bar;
}
