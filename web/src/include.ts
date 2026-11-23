// An include block in the rendered view. A grip above its lines drags
// from= and a grip below drags to=, showing the lines as they are
// revealed and writing the range to the file when the grip is let go.
// A button shows the whole file over the window, with the included
// lines marked. The textbox's grip is the model: the same drag, but
// this one writes to the file, since from= and to= are what it holds.

import { App, message } from "./api.js";
import type { Cell } from "./api.js";
import type { IncludeCell, IncludeFile } from "./bindings/github.com/gdey/markdownshell/notebook/models.js";
import { $, el, iconButton } from "./dom.js";
import { codeLines } from "./highlight.js";

export interface IncludeHooks {
  // commit writes from and to on include block, and says whether it did.
  commit(block: number, from: number, to: number): Promise<boolean>;
  // busy is whether a block runs, when the file can't change.
  busy(): boolean;
  say(text: string): void;
}

type Range = { from: number; to: number };

// rangeText says which lines an include shows: its from= and to=, with
// 0 for one it doesn't have, of lines in the file when that is known.
export function rangeText(from: number, to: number, lines: number): string {
  const first = from || 1;
  const last = to || lines;
  let s = last ? `lines ${first}–${last}` : first > 1 ? `from line ${first}` : "all lines";
  if (lines && (first > 1 || last < lines)) s += ` of ${lines}`;
  return s;
}

// sliceLines is lines from through to of text, counting from 1.
export function sliceLines(text: string, from: number, to: number): string {
  const lines = text.split("\n");
  if (lines[lines.length - 1] === "") lines.pop();
  return lines.slice(Math.max(from, 1) - 1, to || lines.length).join("\n") + "\n";
}

// includeControls adds the grips and the whole-file button to section
// s, which draws include cell c.
export function includeControls(s: HTMLElement, c: Cell, hooks: IncludeHooks) {
  const inc = c.include!;
  const pre = s.querySelector<HTMLElement>("pre");
  const code = pre?.querySelector<HTMLElement>("code");
  if (!pre || !code) return;
  s.classList.add("include-cell");
  const label = el("span", { className: "include-range muted" }, inc.window || rangeText(inc.from, inc.to, 0));
  const whole = iconButton("eye", `Show the whole of ${inc.file}`, () => void showIncludeFile(inc, c.firstLine || 1, hooks));
  whole.classList.add("include-whole");
  const bar = el("div", { className: "include-bar" }, el("span", { className: "include-file" }, inc.file), label);
  // An include whose lines are found by match= or symbol= names them
  // by what they hold, so there are no grips to drag, and a window that
  // found nothing says so.
  if (inc.window && inc.last) label.title = `lines ${c.firstLine || 1}–${inc.last}`;
  if (inc.warning) bar.append(el("span", { className: "stale-badge include-nomatch", title: inc.warning }, "no match"));
  bar.append(whole);
  bar.onclick = (e) => e.stopPropagation();
  if (inc.window) {
    pre.before(bar);
    return;
  }
  // load reads the whole file once per card.
  let loaded: Promise<IncludeFile | null> | null = null;
  const load = () => (loaded ??= App.IncludeFile(inc.block).then(
    (f) => f,
    (err) => {
      hooks.say(message(err));
      return null;
    },
  ));

  // preview shows lines r of file, as the drag has them.
  const preview = (file: IncludeFile, r: Range) => {
    code.replaceChildren(...codeLines(sliceLines(file.text, r.from, r.to), inc.lang));
    code.style.counterReset = `line ${r.from - 1}`;
    label.textContent = rangeText(r.from, r.to, file.lines);
  };

  // commit writes r to the file, and puts the lines back as they were
  // when it couldn't. The card is drawn again when it could, so the
  // grip that had the keyboard gets it back on the new card.
  const commit = async (end: string, r: Range, restore: () => void) => {
    const focused = document.activeElement?.classList.contains("include-grip");
    if (!(await hooks.commit(inc.block, r.from, r.to))) return restore();
    if (focused) document.querySelector<HTMLElement>(`#doc .include-cell[data-from="${c.from}"] .include-grip.${end}`)?.focus();
  };

  // rangeAt is the range the drag of end's grip by d lines asks for:
  // the top grip moves from=, the bottom one to=, and neither crosses
  // the other or leaves the file.
  const rangeAt = (file: IncludeFile, end: string, d: number): Range => {
    const from0 = inc.from || 1;
    const to0 = inc.to || file.lines;
    const clamp = (n: number, lo: number, hi: number) => Math.min(Math.max(n, lo), hi);
    return end === "top" ? { from: clamp(from0 + d, 1, to0), to: to0 } : { from: from0, to: clamp(to0 + d, from0, file.lines) };
  };
  const same = (file: IncludeFile, r: Range) => r.from === (inc.from || 1) && r.to === (inc.to || file.lines);

  const includeGrip = (end: "top" | "bottom"): HTMLElement => {
    const g = el("div", { className: "textbox-grip include-grip " + end, tabIndex: 0, title: end === "top" ? "Drag up to show the lines before these" : "Drag down to show the lines after these" });
    g.setAttribute("role", "separator");
    g.setAttribute("aria-orientation", "horizontal");
    g.setAttribute("aria-label", `${end === "top" ? "First" : "Last"} line of ${inc.file}: drag, or press up and down`);
    g.setAttribute("aria-valuenow", String(end === "top" ? inc.from || 1 : inc.to || 0));
    g.onclick = (e) => e.stopPropagation();
    g.onpointerdown = (e) => {
      if (e.button !== 0) return;
      e.preventDefault();
      if (hooks.busy()) return hooks.say("can't change the file while a block runs");
      g.setPointerCapture(e.pointerId);
      const y0 = e.clientY;
      const line = lineHeight(code);
      const before = [...code.childNodes];
      const reset = code.style.counterReset;
      const was = label.textContent;
      const restore = () => {
        code.replaceChildren(...before);
        code.style.counterReset = reset;
        label.textContent = was;
      };
      let file: IncludeFile | null = null;
      let y = y0;
      let up = false;
      const at = (f: IncludeFile) => rangeAt(f, end, Math.round((y - y0) / line));
      const loading = load().then((f) => {
        file = f;
        if (f && !up) preview(f, at(f));
      });
      document.body.classList.add("resizing-rows");
      g.onpointermove = (m) => {
        y = m.clientY;
        if (file) preview(file, at(file));
      };
      g.onpointerup = g.onpointercancel = async () => {
        up = true;
        g.onpointermove = g.onpointerup = g.onpointercancel = null;
        document.body.classList.remove("resizing-rows");
        await loading;
        if (!file) return;
        const r = at(file);
        if (same(file, r)) return restore();
        preview(file, r);
        await commit(end, r, restore);
      };
    };
    // Up and down move the grip's line by one, as dragging does.
    g.onkeydown = async (e) => {
      const dir = e.key === "ArrowDown" ? 1 : e.key === "ArrowUp" ? -1 : 0;
      if (!dir) return;
      e.preventDefault();
      if (hooks.busy()) return hooks.say("can't change the file while a block runs");
      const file = await load();
      if (!file) return;
      const r = rangeAt(file, end, dir);
      if (same(file, r)) return;
      await commit(end, r, () => {});
    };
    return g;
  };
  pre.before(bar, includeGrip("top"));
  pre.after(includeGrip("bottom"));
}

// lineHeight is the height of one of code's lines, in pixels.
function lineHeight(code: HTMLElement): number {
  const line = code.querySelector<HTMLElement>(".line");
  if (line && line.offsetHeight) return line.offsetHeight;
  const s = getComputedStyle(code);
  return parseFloat(s.lineHeight) || parseFloat(s.fontSize) * 1.5 || 18;
}

// showIncludeFile shows the whole of inc's file over the window, with
// the included lines marked, scrolled to the first of them.
async function showIncludeFile(inc: IncludeCell, firstLine: number, hooks: IncludeHooks) {
  const file = await App.IncludeFile(inc.block).catch((err) => {
    hooks.say(message(err));
    return null;
  });
  if (!file) return;
  const first = inc.window ? (inc.last ? firstLine : 0) : inc.from || 1;
  const last = inc.window ? inc.last || 0 : inc.to || file.lines;
  const shown = inc.window ? (last ? `lines ${first}–${last} (${inc.window})` : `no lines (${inc.window})`) : rangeText(inc.from, inc.to, file.lines);
  showFile(inc.file, file, inc.lang, first, last, `${file.lines} lines; ${shown} included`);
}

// showFile shows the whole of file, named name, in the include dialog,
// with lines first through last marked and scrolled to, and note under
// its name.
function showFile(name: string, file: IncludeFile, lang: string, first: number, last: number, note: string) {
  const dialog = $<HTMLDialogElement>("include-dialog");
  $("include-title").textContent = name;
  $("include-note").textContent = note;
  const code = $("include-text");
  const lines = codeLines(file.text, lang);
  lines.forEach((l, i) => l.classList.toggle("included", i + 1 >= first && i + 1 <= last));
  code.replaceChildren(...lines);
  dialog.showModal();
  const target = lines[Math.max(first - 2, 0)];
  if (target) target.scrollIntoView({ block: "start" });
}

// spillName is the name of an out=file file= as the output bar shows
// it: its last part, as build.log.
function spillName(file: string): string {
  return file.replace(/^res:\/\//, "").split("/").pop() || file;
}

// spillLine is the line on the output bar of block c, which has
// out=file: how long the file is, which opens it over the card, and a
// badge when the window found nothing.
export function spillLine(c: Cell, say: (text: string) => void): HTMLElement {
  const sp = c.spill!;
  const name = spillName(sp.file);
  const text = sp.lines ? `${sp.lines.toLocaleString()} ${sp.lines === 1 ? "line" : "lines"} in ${name}` : name;
  const open = el("button", { className: "spill-open link", type: "button", title: `Show the whole of ${sp.file}` }, text);
  open.onclick = async (e) => {
    e.stopPropagation();
    const file = await App.SpillFile(c.block).catch((err) => {
      say(message(err));
      return null;
    });
    if (!file) return;
    const first = sp.first ?? 0;
    const last = sp.last ?? 0;
    const shown = last ? `lines ${first}–${last}${sp.window ? ` (${sp.window})` : ""} shown` : sp.warning ? "no lines shown" : "";
    showFile(sp.file, file, "", first, last, [`${file.lines} lines`, shown, sp.truncated ? "cut at 16 MB" : ""].filter(Boolean).join("; "));
  };
  const line = el("span", { className: "spill-line muted" }, open);
  if (sp.truncated) line.append(el("span", { className: "stale-badge", title: "The output was longer than 16 MB, so its end was dropped" }, "truncated"));
  if (sp.warning) line.append(el("span", { className: "stale-badge include-nomatch", title: sp.warning }, "no match"));
  return line;
}
