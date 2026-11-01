import "./tab.js";
import { copyLineDown, copyLineUp, moveLineDown, moveLineUp, redo, undo } from "@codemirror/commands";
import { EditorView, scrollPastEnd } from "@codemirror/view";
import { Events, Window } from "/wails/runtime.js";

import { App, type AttrSpec, type Cell, type Deleted, type Doc, type File as Template, type Fold, type Move, type Names, type Settings, type Start, message } from "./api.js";
import { initDeps, litDeps, setDeps } from "./deps.js";
import { $, el, iconButton, icons, svg } from "./dom.js";
import { foldMarks, restoreFolds, setSourceFolds, sourceFolding, sourceFolds } from "./fold.js";
import { Ansi, codeLines, colorOutput, highlightProse } from "./highlight.js";
import { type MarkdownEditor, insertAction, markdownBar, markdownEditor } from "./markdown.js";
import { initConsole, showConsole } from "./console.js";
import { fitHistory, historyBusy, historyEnd, historyStatus, historyTerminal, historyWrite, initHistory, showBar, showHistory } from "./history.js";
import { initReport, openReport } from "./report.js";
import { runGutter, setRunLines } from "./rungutter.js";
import { initSplitGrip, showSplitGrip } from "./split.js";
import { diff } from "./sync.js";
import { closeTerminal, endTerminal, openTerminal, sizeTerminal, wantTerminalSize, writeTerminal } from "./terminal.js";
import { applyTheme } from "./theme.js";

let me = ""; // this window's name; events from other windows are ignored
let doc: Doc = { file: "", dirty: false, changed: false, running: false, fresh: false, run: { args: [], only: "", step: false, stopOnError: false, shell: "" }, args: "", history: false, termCols: 0, termRows: 0 };
let cells: Cell[] = [];
let running = false;
let ranHere = false; // the run going on was started from this page
let selected = -1; // index into the block cards, for j/k
// selectedSection is the selected heading's dataset.fold, or "". A
// heading and a block aren't selected both at once.
let selectedSection = "";
let settings: Settings;
let attrs: AttrSpec[] = [];
let known: Names = { ids: [], envs: [], assets: [], files: [], assetsDir: "" }; // for completion in the source pane

// The view: what each tab shows. It starts as the last tab left it.
// split shows the source pane and rendered the rendered view; at least
// one is always on.
interface View {
  split: boolean;
  rendered: boolean;
  outline: boolean;
  code: boolean;
  output: boolean;
  history: boolean;
  console: boolean;
}
let view: View = { split: false, rendered: true, outline: false, code: true, output: true, history: false, console: false, ...loadView() };

function loadView(): Partial<View> {
  try {
    return JSON.parse(localStorage.getItem("mdsh-view") ?? "{}");
  } catch {
    return {};
  }
}

function mine<T>(handler: (data: T) => void) {
  return (ev: { sender?: string; data: T }) => {
    if (ev.sender === me) handler(ev.data);
  };
}

function status(text: string) {
  $("status").textContent = text;
  historyStatus(text);
}

function fail(err: unknown) {
  status(message(err));
}

// refresh reads the document's state and draws whichever screen fits.
async function refresh() {
  doc = await App.Doc();
  wantTerminalSize(doc.termCols, doc.termRows);
  fitHistory();
  const open = doc.file !== "";
  $("toolbar").hidden = !open;
  $("workspace").hidden = !open;
  $("start").hidden = open;
  syncHistory();
  if (!open) {
    $("changed").hidden = true;
    return drawStart();
  }
  const name = showFile(doc.file);
  $("dirty").hidden = !doc.dirty;
  $("changed").hidden = !doc.changed || $("changed").dataset.kept === doc.file;
  drawOptions();
  cells = (await App.Cells()) ?? [];
  drawDoc();
  if (doc.fresh) openView();
  if (doc.running && !ranHere) await catchUp();
  known = await names();
  await loadSource();
}

// catchUp shows a run that started before this page did, as when its
// tab moved to another window: what it has sent so far, then the rest as
// it comes. A "ran" event says when it is over.
async function catchUp() {
  busy(true);
  status("running");
  for (const e of (await App.Live()) ?? []) {
    if (e.name === "block") onBlock(e.data as BlockEvent);
    else if (e.name === "confirm") onConfirm(e.data as Question);
  }
}

function drawOptions() {
  const r = doc.run;
  const parts: string[] = [];
  if (doc.args) parts.push(doc.args);
  if (r.only) parts.push("only " + r.only);
  if (r.step) parts.push("step");
  if (r.stopOnError) parts.push("stop on error");
  $("options").hidden = parts.length === 0;
  $("options").textContent = parts.join(" · ");
}

function drawDoc() {
  const main = $("doc");
  const restore = anchor();
  closeTerminal();
  main.replaceChildren();
  const seen = new Map<string, number>();
  for (const c of cells) {
    const e = drawCell(c);
    if (c.heading) {
      // A heading is known by its level and text, and which one it is
      // of those alike, so its fold outlives edits elsewhere.
      const name = c.heading + ":" + (e.querySelector("h1, h2, h3, h4, h5, h6")?.textContent ?? "");
      const nth = seen.get(name) ?? 0;
      seen.set(name, nth + 1);
      e.dataset.fold = "section:" + name + ":" + nth;
    }
    main.append(e);
  }
  applyFolds();
  runAllButton();
  select(Math.min(selected, blocks().length - 1), false);
  markSection();
  drawOutline();
  sizeTerminal();
  restore();
  setDeps(cells);
}

// anchor notes where the block being followed is in the rendered view,
// or else the card at its top, and returns a function that puts the
// card drawn in its place back there. Drawing the document anew replaces
// every card, and the view would otherwise jump, as far as the top.
function anchor(): () => void {
  const pane = $("doc-scroll");
  const cards = [...$("doc").children];
  const top = pane.getBoundingClientRect().top;
  const i = following ? cards.indexOf(following) : cards.findIndex((e) => e.getBoundingClientRect().bottom > top);
  if (i < 0) return () => {};
  const offset = cards[i].getBoundingClientRect().top - top;
  return () => {
    const e = $("doc").children[i] as HTMLElement | undefined;
    if (following) following = e?.classList.contains("block") ? e : null;
    if (e) pane.scrollTop += e.getBoundingClientRect().top - top - offset;
  };
}

// Following a run: as a block's output grows, the rendered view scrolls
// it up, smoothly, so the end of the output stays in sight, until the
// block's top reaches the top of the view. Scrolling by hand stops it.
let following: HTMLElement | null = null;
let followFrame = 0;
const followMargin = 8;

// follow starts following block, scrolling up to it first if its top is
// above the view.
function follow(block: HTMLElement) {
  following = block;
  showEnd(block, true);
}

// followOutput keeps the end of the followed block's output in sight,
// once a frame however often it grows.
function followOutput() {
  if (followFrame || !following) return;
  followFrame = requestAnimationFrame(() => {
    followFrame = 0;
    if (following?.isConnected) showEnd(following, false);
  });
}

// showEnd scrolls the rendered view down until block's end is in sight
// or its top is at the top of the view; with up, a block whose top is
// above the view is scrolled down to.
function showEnd(block: HTMLElement, up: boolean) {
  const pane = $("doc-scroll");
  if (pane.hidden) return;
  const p = pane.getBoundingClientRect();
  const r = block.getBoundingClientRect();
  const toTop = r.top - p.top - followMargin;
  const d = up && toTop < 0 ? toTop : Math.max(0, Math.min(r.bottom - p.bottom + followMargin, toTop));
  if (Math.abs(d) > 1) pane.scrollTo({ top: pane.scrollTop + d, behavior: "smooth" });
}

function stopFollowing() {
  following = null;
}

function drawCell(c: Cell): HTMLElement {
  const e = c.textbox ? drawTextbox(c) : c.choice ? drawChoice(c) : c.block < 0 ? drawProse(c) : drawBlock(c);
  e.dataset.line = String(c.line);
  e.dataset.from = String(c.from);
  if (c.drop) e.dataset.drop = "1";
  if (c.movable) {
    // The grip goes first in a block's head, or before an input's name.
    const g = grip("Drag to move this block (alt+↑/↓)", (ev, handle) => startDrag(ev, handle, e, c.from, 0, "doc"));
    const head = e.querySelector(".head");
    if (head) head.prepend(g);
    else {
      // A fence that doesn't run has no head; its grip sits in the
      // margin beside it.
      e.classList.add("fence");
      e.prepend(g);
    }
  }
  return e;
}

// drawProse draws one top-level block of prose. Clicking it shows its
// markdown in the source pane, except on a link, which followLinks
// follows.
function drawProse(c: Cell): HTMLElement {
  // goldmark's HTML, with raw HTML in the markdown left out.
  const s = el("section", { className: "prose", innerHTML: c.html ?? "", title: "Click to show it in the source" });
  highlightProse(s);
  enableDiagrams(s);
  s.onclick = (e) => {
    if (window.getSelection()?.toString() || (e.target as HTMLElement).closest("input")) return;
    if (c.heading) selectSection(s);
    void showInSource(s, c.line, true);
  };
  enableTasks(s, c.tasks);
  if (c.heading) {
    s.classList.add("heading-cell");
    s.dataset.level = String(c.heading);
    if (c.sectionTo) {
      s.dataset.sectionFrom = String(c.sectionFrom ?? 0);
      s.dataset.sectionTo = String(c.sectionTo);
      const play = iconButton("play", "Run this section (s)", () => void runSection(s));
      play.classList.add("run-section");
      play.disabled = running;
      // Next to the heading's text, which names it in the outline and
      // its fold, and a button's svg adds none.
      (s.querySelector("h1, h2, h3, h4, h5, h6") ?? s).append(play);
    }
    s.dataset.foldDefault = c.collapsed ? "1" : "";
    const note = el("button", { className: "fold-note", title: "Unfold this section" });
    note.onclick = (e) => {
      e.stopPropagation();
      foldSection(s, false);
    };
    s.prepend(foldButton("Fold or unfold this section", () => foldSection(s)));
    s.append(note);
  }
  return s;
}

// shownAsCode holds the source of the diagrams switched to their code,
// so a redraw keeps them that way.
const shownAsCode = new Set<string>();

// enableDiagrams gives each d2 or svg diagram Go drew in e a button that
// switches between the picture and its code. A diagram Go couldn't draw
// shows only its code and a note saying why.
function enableDiagrams(e: HTMLElement) {
  for (const fig of e.querySelectorAll<HTMLElement>("figure.diagram")) {
    if (!fig.querySelector(".diagram-image")) continue;
    const key = fig.dataset.lang + "\n" + (fig.querySelector("pre")?.textContent ?? "");
    let b: HTMLButtonElement;
    const show = (code: boolean) => {
      fig.dataset.show = code ? "code" : "image";
      if (code) shownAsCode.add(key);
      else shownAsCode.delete(key);
      const title = code ? "Show the diagram" : "Show the " + fig.dataset.lang + " source";
      b.title = title;
      b.setAttribute("aria-label", title);
      b.replaceChildren(svg(icons[code ? "image" : "code"]));
    };
    b = iconButton("code", "", () => show(fig.dataset.show !== "code"));
    b.classList.add("diagram-toggle");
    fig.prepend(b);
    show(shownAsCode.has(key));
  }
}

// enableTasks makes the task list checkboxes in e check and uncheck
// their item in the file; tasks are their numbers. If they don't match
// the tasks Go found, they stay as drawn.
function enableTasks(e: HTMLElement, tasks: number[] | null | undefined) {
  const boxes = e.querySelectorAll<HTMLInputElement>("input[type=checkbox]");
  if (!tasks || boxes.length !== tasks.length) return;
  boxes.forEach((box, i) => {
    box.disabled = false;
    box.title = "Check or uncheck it in the file";
    box.onchange = () => void edit(() => App.SetTask(tasks[i], box.checked)).then((ok) => ok || (box.checked = !box.checked));
  });
}

// drawTextbox draws a textbox block as a box to type in. What is typed
// goes into the block's body after a pause, as typing in the source pane
// does. Its lines= is the most lines it takes.
function drawTextbox(c: Cell): HTMLElement {
  const t = c.textbox!;
  const box = el("textarea", { className: "textbox", rows: t.lines, value: t.text, placeholder: t.placeholder ?? "", spellcheck: t.lines > 1 });
  box.setAttribute("aria-label", t.id);
  box.dataset.input = String(t.block);
  let timer = 0;
  const save = async () => {
    clearTimeout(timer);
    if (running) {
      timer = window.setTimeout(() => void save(), 1000);
      return;
    }
    if (await edit(() => App.SetTextbox(t.block, box.value))) t.text = box.value;
  };
  box.onkeydown = (e) => {
    if (e.key === "Enter" && !e.isComposing && box.value.split("\n").length >= t.lines) e.preventDefault();
  };
  box.oninput = () => {
    const lines = box.value.split("\n");
    if (lines.length > t.lines) {
      // A paste with too many lines keeps the first ones.
      const at = box.selectionStart;
      box.value = lines.slice(0, t.lines - 1).join("\n") + (t.lines > 1 ? "\n" : "") + lines.slice(t.lines - 1).join(" ");
      box.selectionStart = box.selectionEnd = Math.min(at, box.value.length);
    }
    clearTimeout(timer);
    timer = window.setTimeout(() => void save(), 500);
  };
  box.onblur = () => {
    if (box.value !== t.text) void save();
  };
  return inputCell("textbox-cell", t.id, t.problem, box);
}

// drawChoice draws a choice block as a dropdown, radio buttons or
// checkboxes. Picking writes the checked options back into its body.
function drawChoice(c: Cell): HTMLElement {
  const ch = c.choice!;
  const set = (checked: boolean[]) => edit(() => App.SetChoice(ch.block, checked));
  const options = ch.options ?? [];
  let control: HTMLElement;
  if (ch.mode === "dropdown") {
    const select = el("select", { className: "choice-select" }, el("option", { value: "-1" }, ch.placeholder || "Choose…"));
    options.forEach((o, i) => select.append(el("option", { value: String(i), selected: o.checked }, o.label)));
    if (!options.some((o) => o.checked)) select.value = "-1";
    select.onchange = () => {
      const pick = Number(select.value);
      void set(options.map((_, i) => i === pick));
    };
    control = select;
  } else {
    const radio = ch.mode === "radio";
    control = el("div", { className: "choice-options", role: radio ? "radiogroup" : "group" });
    const inputs = options.map((o, i) => {
      const input = el("input", { type: radio ? "radio" : "checkbox", name: "choice-" + ch.block, checked: o.checked });
      input.onchange = () => {
        const checked = inputs.map((x) => x.checked);
        void set(checked).then((ok) => ok || inputs.forEach((x, j) => (x.checked = options[j].checked)));
      };
      control.append(el("label", {}, input, " " + o.label));
      return input;
    });
  }
  control.dataset.input = String(ch.block);
  control.setAttribute("aria-label", ch.id);
  return inputCell("choice-cell", ch.id, ch.problem, control);
}

// inputCell is the card around a textbox's or choice's control, with
// its name over it and, when its var= binds nothing, why.
function inputCell(className: string, id: string, problem: string | undefined, control: HTMLElement): HTMLElement {
  const cell = el("section", { className: "input-cell " + className + (problem ? " taken" : ""), title: problem || id }, el("span", { className: "name" }, id), control);
  if (problem) cell.append(el("p", { className: "problem" }, problem));
  return cell;
}

// edit makes an edit in Go from the rendered view, such as checking a
// task, once what was typed in the source pane is applied. The cells are
// read again, without drawing them again, so the view keeps its focus.
// It reports whether the edit was made.
async function edit(call: () => Promise<void>): Promise<boolean> {
  if (running) {
    status("can't change the file while a block runs");
    return false;
  }
  await applySource();
  try {
    await call();
    status("");
  } catch (err) {
    fail(err);
    return false;
  }
  $("dirty").hidden = false;
  // A var= input can change what prose and sections show, so cells
  // whose prose changed are drawn again, and all of them when sections
  // came or went, keeping the focus in the input being typed in.
  const fresh = (await App.Cells()) ?? [];
  const focused = document.activeElement as HTMLTextAreaElement | null;
  const input = focused?.closest<HTMLElement>("[data-input]")?.dataset.input;
  const [from, to] = [focused?.selectionStart, focused?.selectionEnd];
  if (fresh.length !== cells.length) {
    cells = fresh;
    drawDoc();
    const again = input ? document.querySelector<HTMLElement>(`#doc [data-input="${input}"]`) : null;
    if (again) {
      again.focus();
      if (again instanceof HTMLTextAreaElement && from != null) again.setSelectionRange(from, to ?? from);
    }
  } else {
    // Each card's handlers hold its cell, so update them in place.
    fresh.forEach((c, i) => {
      const changed = c.block < 0 && !c.textbox && !c.choice && c.html !== cells[i].html;
      // Go leaves out fields that are false, so clear the fold flags first.
      Object.assign(cells[i], { collapsed: false, collapsedOutput: false }, c);
      const card = $("doc").children[i] as HTMLElement | undefined;
      if (card && changed) {
        const fold = card.dataset.fold;
        const fresh = drawCell(cells[i]);
        if (fold) fresh.dataset.fold = fold;
        card.replaceWith(fresh);
      } else if (card) {
        card.dataset.line = String(c.line);
        // A var= input can change what mdsh:collapse and collapse= say.
        if (c.heading) card.dataset.foldDefault = c.collapsed ? "1" : "";
        if (c.block >= 0) {
          card.dataset.foldCode = c.collapsed ? "1" : "";
          card.dataset.foldOutput = c.collapsedOutput ? "1" : "";
        }
      }
    });
    applyFolds();
    drawOutline();
  }
  await loadSource();
  markRunLines();
  return true;
}

function drawBlock(c: Cell): HTMLElement {
  // The outline names a block by the .name's first child.
  const name = el("span", { className: "name" }, el("span", { className: "id" }, c.name ?? c.id ?? ""), el("span", { className: "lang" }, c.lang ?? ""), c.attrs ? el("span", { className: "attrs" }, c.attrs) : "");
  const exit = el("span", { className: "exit" + (c.failed ? " bad" : "") }, c.running ? backgroundNote : c.ran ? "exit " + c.status : "");
  const block = el(
    "section",
    { className: "block" + (c.running ? " background" : c.failed ? " failed" : c.ran ? " ran" : ""), id: "block-" + c.block },
    el(
      "div",
      { className: "head" },
      name,
      exit,
      iconButton("pencil", "Edit this block in the source (e)", () => void editBlock(block, c)),
      iconButton("play", "Run this block (enter)", () => start("RunBlock", c.block)),
      iconButton("runFrom", "Run from here (a)", () => start("RunFrom", c.block)),
      ...(c.movable ? [trashButton(c)] : []),
    ),
    foldPart("code", el("pre", { className: "code numbered" }, el("code", {}, ...codeLines(c.body ?? "", c.lang ?? ""))), () => foldBlock(block, "code")),
    el("div", { className: "ask", hidden: true }),
    foldPart("output", el("div", { className: "output", innerHTML: c.output ?? "" }), () => foldBlock(block, "output")),
  );
  if (c.movable) block.querySelector(".output-part")!.append(clearOutputButton(c));
  colorOutput(block.querySelector(".output")!);
  block.dataset.foldCode = c.collapsed ? "1" : "";
  block.dataset.foldOutput = c.collapsedOutput ? "1" : "";
  // Checkboxes in output, such as out=markdown's, are edited in place
  // too, until the block runs again.
  enableTasks(block.querySelector<HTMLElement>(".output")!, c.outputTasks);
  enableDiagrams(block.querySelector<HTMLElement>(".output")!);
  block.querySelector<HTMLElement>(".output")!.onclick = (e) => {
    const img = (e.target as HTMLElement).closest("img");
    if (img) viewImage(img);
  };
  block.dataset.block = String(c.block);
  block.onclick = (e) => {
    select(blocks().indexOf(block), false);
    if (!(e.target as HTMLElement).closest("button, input")) void showInSource(block, c.line, false);
  };
  return block;
}

// Folds in the rendered view. A block's code and its output each fold
// to their first line, and a heading folds its section, down to the next
// heading of the same or a higher level. collapse=, collapse-output= and
// mdsh:collapse lines say how the file opens. A click, the Folding menu
// and the outline write the fold to the file, as a bare collapse or
// collapse-output or an mdsh:collapse line under the heading, so it
// opens that way next time (saveFolds). Where the file folds by a
// condition, or while a block runs, the fold lasts only while the tab is
// open. The source pane keeps folds of its own, and the Folding menu
// copies them either way.

// folds are the clicked folds the file doesn't hold, by key: the block's
// id and part, or the heading's dataset.fold. The rest are as the file
// says.
const folds = new Map<string, boolean>();

type Part = "code" | "output";

const partKey = (block: HTMLElement, part: Part) => part + ":" + (block.querySelector(".name")?.firstChild?.textContent ?? block.dataset.block);

function blockFolded(block: HTMLElement, part: Part): boolean {
  return folds.get(partKey(block, part)) ?? !!block.dataset[part === "code" ? "foldCode" : "foldOutput"];
}

function sectionFolded(cell: HTMLElement): boolean {
  return folds.get(cell.dataset.fold ?? "") ?? !!cell.dataset.foldDefault;
}

function foldButton(title: string, onclick: () => void): HTMLButtonElement {
  const b = el("button", { className: "fold", title }, svg(icons.chevron));
  b.setAttribute("aria-label", title);
  b.onclick = (e) => {
    e.stopPropagation();
    onclick();
  };
  return b;
}

// foldPart wraps a block's code or output with its fold button and the
// one line it shows when folded.
function foldPart(part: Part, full: HTMLElement, toggle: () => void): HTMLElement {
  const summary = el("div", { className: "fold-summary", hidden: true, title: "Unfold" });
  summary.onclick = (e) => {
    e.stopPropagation();
    toggle();
  };
  return el("div", { className: "part " + part + "-part" }, foldButton(part === "code" ? "Fold or unfold the code" : "Fold or unfold the output", toggle), full, summary);
}

function foldBlock(block: HTMLElement, part: Part, on = !blockFolded(block, part)) {
  folds.set(partKey(block, part), on);
  drawPart(block, part);
  void saveFolds([blockFold(block, part, on)]);
}

function foldSection(cell: HTMLElement, on = !sectionFolded(cell)) {
  folds.set(cell.dataset.fold ?? "", on);
  applyFolds();
  drawOutline();
  const f = sectionFold(cell, on);
  if (f) void saveFolds([f]);
}

const blockFold = (block: HTMLElement, part: Part, on: boolean): Fold => ({ block: Number(block.dataset.block), output: part === "output", heading: 0, on });

function sectionFold(cell: HTMLElement, on: boolean): Fold | null {
  const c = cells.find((c) => String(c.line) === cell.dataset.line);
  return c?.heading ? { block: -1, output: false, heading: c.from, on } : null;
}

// saveFolds writes folds to the file, then forgets the clicks the file
// now holds, so that Undo and edits to the file show. The file can't
// change while a block runs, so then the folds aren't saved.
async function saveFolds(list: Fold[]) {
  if (!list.length || running) return;
  if (!(await edit(() => App.SetFolds(list)))) return;
  for (const e of $("doc").children as HTMLCollectionOf<HTMLElement>) {
    const held = (key: string, file: boolean) => folds.get(key) === file && folds.delete(key);
    if (e.classList.contains("block")) {
      held(partKey(e, "code"), !!e.dataset.foldCode);
      held(partKey(e, "output"), !!e.dataset.foldOutput);
    } else if (e.dataset.fold) held(e.dataset.fold, !!e.dataset.foldDefault);
  }
  // Where the file folds by a condition, nothing changed.
  $("dirty").hidden = !(await App.Doc()).dirty;
}

// drawPart shows a block's code or output whole, or as its first line.
function drawPart(block: HTMLElement, part: Part) {
  const wrap = block.querySelector<HTMLElement>("." + part + "-part");
  if (!wrap) return;
  const on = blockFolded(block, part);
  wrap.classList.toggle("folded", on);
  const summary = wrap.querySelector<HTMLElement>(".fold-summary")!;
  summary.hidden = !on;
  if (on) summarize(wrap.children[1] as HTMLElement, summary);
}

// summarize puts full's first line in summary, and how much more there is.
function summarize(full: HTMLElement, summary: HTMLElement) {
  // Highlighted code is one element per line, with no newlines between.
  const rows = full.querySelectorAll(".line");
  const text = rows.length ? [...rows].map((r) => r.textContent ?? "") : (full.textContent ?? "").split("\n");
  const lines = text.filter((l) => l.trim());
  const images = full.querySelectorAll("img").length;
  let more = lines.length > 1 ? " … " + (lines.length - 1) + (lines.length === 2 ? " more line" : " more lines") : "";
  if (images) more += " … " + images + (images === 1 ? " image" : " images");
  summary.replaceChildren(el("span", { className: "first" }, lines[0] ?? (images ? "" : "empty")), el("span", { className: "muted" }, more));
}

// applyFolds draws every fold: each block's parts, and the sections,
// hiding the cells under a folded heading.
function applyFolds() {
  const doc = $("doc");
  for (const b of doc.querySelectorAll<HTMLElement>(":scope > .block")) {
    drawPart(b, "code");
    drawPart(b, "output");
  }
  const open: { level: number; folded: boolean; cell: HTMLElement; hidden: number }[] = [];
  const headings: typeof open = [];
  for (const e of doc.children as HTMLCollectionOf<HTMLElement>) {
    const level = Number(e.dataset.level ?? 0);
    if (level) while (open.length && open[open.length - 1].level >= level) open.pop();
    const owner = open.find((h) => h.folded);
    e.classList.toggle("in-fold", !!owner);
    if (owner) owner.hidden++;
    if (level) {
      const h = { level, folded: sectionFolded(e), cell: e, hidden: 0 };
      e.classList.toggle("folded", h.folded);
      open.push(h);
      headings.push(h);
    }
  }
  for (const h of headings) {
    const note = h.cell.querySelector<HTMLElement>(".fold-note");
    if (note) note.textContent = h.folded && h.hidden ? "… " + h.hidden + (h.hidden === 1 ? " part" : " parts") : "";
  }
  // A selected block that a fold hides is selected no longer.
  const current = doc.querySelector<HTMLElement>(".block.selected");
  if (current) select(current.classList.contains("in-fold") ? -1 : blocks().indexOf(current), false);
}

// reveal unfolds the sections that hide cell, so it can be shown, as
// when a block asks before it runs or the toolbar jumps to it.
function reveal(cell: HTMLElement) {
  if (!cell.classList.contains("in-fold")) return;
  let level = Number(cell.dataset.level ?? 0) || 7;
  for (let e = cell.previousElementSibling as HTMLElement | null; e && level > 1; e = e.previousElementSibling as HTMLElement | null) {
    const l = Number(e.dataset.level ?? 0);
    if (!l || l >= level) continue;
    level = l;
    if (sectionFolded(e)) folds.set(e.dataset.fold ?? "", false);
  }
  applyFolds();
  drawOutline();
}

// foldAll folds or unfolds every block's code and output, or every
// section.
function foldAll(what: "blocks" | "sections", on: boolean): Fold[] {
  const list: Fold[] = [];
  for (const e of $("doc").children as HTMLCollectionOf<HTMLElement>) {
    if (what === "blocks" && e.classList.contains("block")) {
      for (const part of ["code", "output"] as const) {
        // Only blocks with output get collapse-output.
        const empty = part === "output" && !e.querySelector(".output")?.innerHTML.trim();
        if (blockFolded(e, part) !== on && !(on && empty)) list.push(blockFold(e, part, on));
        folds.set(partKey(e, part), on);
      }
    }
    if (what === "sections" && e.dataset.fold) {
      const f = sectionFolded(e) !== on && sectionFold(e, on);
      if (f) list.push(f);
      folds.set(e.dataset.fold, on);
    }
  }
  applyFolds();
  drawOutline();
  return list;
}

// renderedFoldLines are the source lines of the rendered view's folds,
// or, with fromFile, of the folds the file asks for.
function renderedFoldLines(fromFile: boolean): number[] {
  const lines: number[] = [];
  for (const e of $("doc").children as HTMLCollectionOf<HTMLElement>) {
    const c = cells.find((c) => String(c.line) === e.dataset.line);
    if (!c) continue;
    if (c.heading && (fromFile ? c.collapsed : sectionFolded(e))) lines.push(c.line);
    if (c.block >= 0) {
      if (fromFile ? c.collapsed : blockFolded(e, "code")) lines.push(c.line);
      if (c.outputLine && (fromFile ? c.collapsedOutput : blockFolded(e, "output"))) lines.push(c.outputLine);
    }
  }
  return lines;
}

async function foldsToSource() {
  if (!view.split) toggle("split", true);
  await loadSource();
  if (source) setSourceFolds(source.view, renderedFoldLines(false));
}

function foldsToRendered() {
  if (!source) return status("open the source view to copy its folds");
  const lines = new Set(sourceFolds(source.view));
  const list: Fold[] = [];
  for (const e of $("doc").children as HTMLCollectionOf<HTMLElement>) {
    const c = cells.find((c) => String(c.line) === e.dataset.line);
    if (!c) continue;
    if (c.heading && e.dataset.fold) {
      const on = lines.has(c.line);
      if (sectionFolded(e) !== on) list.push({ block: -1, output: false, heading: c.from, on });
      folds.set(e.dataset.fold, on);
    }
    if (c.block >= 0) {
      const on = { code: lines.has(c.line), output: !!c.outputLine && lines.has(c.outputLine) };
      for (const part of ["code", "output"] as const) {
        if (blockFolded(e, part) !== on[part]) list.push(blockFold(e, part, on[part]));
        folds.set(partKey(e, part), on[part]);
      }
    }
  }
  if (!view.rendered) toggle("rendered", true);
  applyFolds();
  drawOutline();
  void saveFolds(list);
}

// followLinks makes a click on a link in the rendered view follow it,
// rather than load it over the app's page: a web address opens in the
// browser, a markdown file in a new tab and a heading in this document
// scrolls into view. A right-click gets the link menu (menu.go), with
// the link's href as its data.
function followLinks() {
  const link = (e: Event) => (e.target as HTMLElement).closest<HTMLAnchorElement>("#doc a[href]");
  document.addEventListener(
    "click",
    (e) => {
      const a = link(e);
      if (!a) return;
      e.preventDefault();
      e.stopPropagation();
      const href = a.getAttribute("href")!;
      if (href.startsWith("#")) return void scrollToHeading(href.slice(1));
      App.OpenLink(href).catch(fail);
    },
    true,
  );
  document.addEventListener(
    "auxclick",
    (e) => {
      if (link(e)) e.preventDefault();
    },
    true,
  );
  document.addEventListener(
    "contextmenu",
    (e) => {
      const a = link(e);
      // goldmark escapes hrefs, so they hold no quote to end the string.
      if (a) a.style.setProperty("--custom-contextmenu-data", '"' + a.getAttribute("href") + '"');
    },
    true,
  );
}

// scrollToHeading scrolls the heading with the id fragment names into
// view: a GitHub-style one, made from its text.
function scrollToHeading(fragment: string) {
  const slug = (t: string) =>
    t
      .trim()
      .toLowerCase()
      .replace(/[^\p{L}\p{N}\s_-]/gu, "")
      .replace(/\s/g, "-");
  let id = fragment;
  try {
    id = decodeURIComponent(fragment);
  } catch {}
  const h = [...document.querySelectorAll<HTMLElement>("#doc :is(h1, h2, h3, h4, h5, h6)")].find(
    (h) => h.id === id || slug(h.textContent ?? "") === id.toLowerCase(),
  );
  h?.scrollIntoView({ block: "start" });
}

followLinks();

// backgroundNote is what a background block's card says while it runs.
const backgroundNote = "running in the background…";

// backgroundStatus counts the background blocks still running in the
// status line, unless a run is going.
function backgroundStatus() {
  if (running) return;
  const n = document.querySelectorAll("#doc .block.background").length;
  status(n ? n + " in background" : "");
}

// viewImage shows an output image full size over the window, until a
// click or esc.
function viewImage(img: HTMLImageElement) {
  const viewer = $<HTMLDialogElement>("viewer");
  const big = viewer.querySelector("img")!;
  big.src = img.src;
  big.alt = img.alt;
  big.classList.toggle("vector", /\.svg$/i.test(new URL(img.src).pathname));
  viewer.onclick = () => viewer.close();
  viewer.showModal();
}

// blocks are the blocks j and k move between: those no fold hides.
const blocks = () => [...document.querySelectorAll<HTMLElement>("#doc .block:not(.in-fold)")];

function select(i: number, scroll = true) {
  const all = blocks();
  for (const b of all) b.classList.remove("selected");
  selected = i;
  if (i >= 0 && selectedSection) {
    selectedSection = "";
    markSection();
  }
  markOutline();
  litDeps(i >= 0 && i < all.length ? Number(all[i].dataset.block) : -1);
  if (i < 0 || i >= all.length) return;
  all[i].classList.add("selected");
  if (scroll) all[i].scrollIntoView({ block: "nearest" });
}

// selectSection selects the heading cell, so its play button shows in
// the outline and s and Run Section run its section.
function selectSection(cell: HTMLElement) {
  if (!cell.dataset.fold) return;
  select(-1, false);
  selectedSection = cell.dataset.fold;
  markSection();
}

// markSection marks the selected heading in the view and the outline.
function markSection() {
  for (const e of document.querySelectorAll<HTMLElement>("#doc > .heading-cell")) {
    e.classList.toggle("selected", !!selectedSection && e.dataset.fold === selectedSection);
  }
  markOutline();
}

// sectionCell is the heading whose section runs for Run Section and s:
// the selected heading, or else the innermost section holding line, by
// default the selected block's.
function sectionCell(line?: number): HTMLElement | null {
  const headings = [...document.querySelectorAll<HTMLElement>("#doc > .heading-cell")];
  if (line === undefined) {
    const h = selectedSection ? headings.find((e) => e.dataset.fold === selectedSection) : undefined;
    if (h) return h;
    const b = blocks()[selected];
    if (!b) return null;
    line = Number(b.dataset.line);
  }
  let found: HTMLElement | null = null;
  for (const e of headings) {
    if (Number(e.dataset.line) > line) break;
    found = e;
  }
  return found;
}

// runSection runs the blocks of cell's section and its subsections, as
// Run all runs them: confirm, run=once, depends= and on_error apply.
async function runSection(cell: HTMLElement | null) {
  if (running || !doc.file) return;
  if (!cell) {
    status("no section selected");
    return;
  }
  if (!cell.dataset.sectionTo) {
    status("no block in this section runs");
    return;
  }
  // What was typed is applied first, which can draw the cells anew.
  const key = cell.dataset.fold;
  await applySource();
  const now = [...document.querySelectorAll<HTMLElement>("#doc > .heading-cell")].find((e) => e.dataset.fold === key);
  if (!now?.dataset.sectionTo) return;
  await run(() => App.RunSection(Number(now.dataset.sectionFrom), Number(now.dataset.sectionTo)));
}

function runAllButton() {
  const b = $<HTMLButtonElement>("run-all");
  b.replaceChildren(svg(running ? icons.stop : icons.play), el("span", {}, running ? "Stop" : "Run all"));
  b.classList.toggle("stop", running);
  b.disabled = !running && !cells.some((c) => c.block >= 0);
}

function busy(on: boolean) {
  running = on;
  document.body.classList.toggle("running", on);
  if (!on) for (const b of document.querySelectorAll("#doc .block.now")) b.classList.remove("now");
  runAllButton();
  historyBusy(on);
  for (const x of document.querySelectorAll<HTMLButtonElement>(".block .head button, #outline .run, .heading-cell .run-section")) x.disabled = on;
}

function start(method: "RunBlock" | "RunFrom", block: number) {
  return run(() => App[method](block));
}

async function run(call: () => Promise<void>) {
  if (running) return;
  // The History pane may have set the terminal's size to its own.
  sizeTerminal();
  busy(true);
  ranHere = true;
  status("running");
  try {
    await call();
    status("");
  } catch (err) {
    fail(err);
  }
  ranHere = false;
  busy(false);
  await refresh();
  // The output is drawn in full now, and may be longer than it was.
  if (following) showEnd(following, false);
  following = null;
  backgroundStatus();
}

// Editing happens in the source pane. The pencil and a click on prose
// take you there.

// showInSource lines the source pane up with card, whose markdown starts
// on line: the line sits as far down the pane as the card does in the
// rendered view. With open, the pane is opened if need be and the cursor
// goes there.
async function showInSource(card: HTMLElement, line: number, open: boolean) {
  if (!view.split) {
    if (!open) return;
    toggle("split", true);
    await loadSource();
  }
  if (!source) return;
  const cm = source.view;
  const pos = cm.state.doc.line(Math.max(1, Math.min(line, cm.state.doc.lines))).from;
  const offset = view.rendered ? card.getBoundingClientRect().top - cm.scrollDOM.getBoundingClientRect().top : 40;
  holdScroll();
  cm.dispatch({
    selection: open ? { anchor: pos } : undefined,
    effects: EditorView.scrollIntoView(pos, { y: "start", yMargin: Math.max(0, offset) }),
  });
  if (open) cm.focus();
}

// editBlock puts the cursor on the first line of block c's code.
async function editBlock(card: HTMLElement, c: Cell) {
  await showInSource(card, c.line, true);
  if (!source) return;
  const cm = source.view;
  const n = c.line + 1;
  if (n <= cm.state.doc.lines) cm.dispatch({ selection: { anchor: cm.state.doc.line(n).to } });
}

async function names(): Promise<Names> {
  try {
    return await App.Names();
  } catch {
    return { ids: [], envs: [], assets: [], files: [], assetsDir: "" };
  }
}

// The split view's source pane: the whole file in a markdown editor.
// Typing re-renders after a short pause, and a run's output comes back
// into it.
//
// Both the pane and Go change the file: the user types, and a run
// writes output or an include fills its fence. So the pane hands Go the
// text it started from with what was typed, and Go turns it down if
// the file changed since; the pane then reads the file, folds Go's
// changes in around the typing and tries again. Reading and writing
// take turns, so a read never lands over typing that went to Go after
// it started.

let source: MarkdownEditor | null = null;
let sourceTimer = 0;
let sourceTurn: Promise<unknown> = Promise.resolve();

// inTurn runs f once the source pane's reads and writes before it are
// done.
function inTurn<T>(f: () => Promise<T>): Promise<T> {
  const p = sourceTurn.then(f, f);
  sourceTurn = p.catch(() => {});
  return p;
}

function openSource() {
  if (source) return;
  source = markdownEditor($("source-editor"), {
    text: "",
    vim: settings.vim,
    spellCheck: !settings.noSpellCheck,
    attrs,
    names: () => known,
    lineNumbers: true,
    // The last lines can scroll up from the window's bottom edge, and
    // the cursor keeps clear of it.
    extensions: [runGutter((line) => void runLine(line)), sourceFolding(), scrollPastEnd(), EditorView.scrollMargins.of(() => ({ bottom: 48 }))],
    onChange() {
      clearTimeout(sourceTimer);
      sourceTimer = window.setTimeout(() => void applySource(), 500);
    },
    onCursor: (line) => markCurrent(line),
    async write() {
      await applySource();
      await actions.save();
    },
    close() {},
    run: () => void runCurrent(),
  });
  $("source-bar").replaceChildren(markdownBar(() => source?.view ?? null));
  source.view.scrollDOM.addEventListener("scroll", () => syncScroll("source"));
  void loadSource();
}

function closeSource() {
  if (!source) return;
  clearTimeout(sourceTimer);
  // What was typed still goes to Go, so the editor stays until it has.
  const ed = source;
  source = null;
  ed.view.dom.remove();
  sourceFoldsFor = "";
  $("source-bar").replaceChildren();
  const finish = (): Promise<void> =>
    inTurn(async () => {
      if (running && ed.pending) return void window.setTimeout(() => void finish(), 1000);
      await sendSource(ed).catch(fail);
      ed.destroy();
    });
  void finish();
}

// applySource hands Go what was typed in the source pane, once a run
// is over, and draws the document again.
function applySource(): Promise<void> {
  clearTimeout(sourceTimer);
  return inTurn(async () => {
    const ed = source;
    if (!ed?.pending) return;
    if (running) {
      sourceTimer = window.setTimeout(() => void applySource(), 1000);
      return;
    }
    try {
      await sendSource(ed);
      status("");
    } catch (err) {
      fail(err);
      return;
    }
    $("dirty").hidden = false;
    cells = (await App.Cells()) ?? [];
    drawDoc();
    if (source !== ed) return;
    markRunLines();
    markCurrent(ed.view.state.doc.lineAt(ed.view.state.selection.main.head).number);
  });
}

// sendSource hands Go ed's text, reading Go's changes into it first
// whenever Go has some the pane hasn't seen.
async function sendSource(ed: MarkdownEditor) {
  for (let tries = 0; ed.pending; tries++) {
    const text = ed.view.state.doc.toString();
    if (await App.SetText(ed.base, text)) return ed.sent(text);
    if (tries === 5) throw new Error("the file keeps changing; try again");
    ed.load(await App.Text());
  }
}

// sourceFoldsFor is the file whose folds the source pane opened with.
let sourceFoldsFor = "";

// loadSource reads Go's text into the source pane, keeping what was
// typed and not yet sent.
function loadSource(): Promise<void> {
  return inTurn(loadSourceNow);
}

async function loadSourceNow() {
  if (!source || doc.file === "") return;
  const ed = source;
  const marks = foldMarks(ed.view);
  try {
    const text = await App.Text();
    if (source !== ed) return;
    ed.load(text);
  } catch (err) {
    fail(err);
  }
  if (source !== ed) return;
  markRunLines();
  // The pane opens with the folds the file asks for, then keeps its own.
  if (sourceFoldsFor !== doc.file) {
    sourceFoldsFor = doc.file;
    setSourceFolds(ed.view, renderedFoldLines(true));
  } else {
    restoreFolds(ed.view, marks);
  }
}

// markRunLines puts a run button on each runnable block's first line,
// and on each heading whose section has one.
function markRunLines() {
  const lines = cells.filter((c) => c.block >= 0).map((c) => c.line);
  const sections = cells.filter((c) => c.heading && c.sectionTo).map((c) => c.line);
  source?.view.dispatch({ effects: setRunLines.of({ blocks: lines, sections }) });
}

// runLine runs the block whose fence opens on line of the source pane,
// once what was typed is applied.
async function runLine(line: number) {
  if (running) return;
  await applySource();
  const c = cells.find((c) => c.block >= 0 && c.line === line);
  if (c) return start("RunBlock", c.block);
  if (cells.some((c) => c.heading && c.line === line)) await runSection(sectionCell(line));
}

// runCurrentSection runs the section the source pane's cursor is in, or
// without the source pane the selected heading's or block's.
async function runCurrentSection() {
  if (running || !doc.file) return;
  if (!source) return runSection(sectionCell());
  await applySource();
  if (!source) return;
  const cm = source.view.state;
  await runSection(sectionCell(cm.doc.lineAt(cm.selection.main.head).number));
}

// runCurrent runs the block the source pane's cursor is in: on its
// fence, in its body or in its output. Without the source pane it runs
// the selected card, as enter does. It goes through RunBlock, as the
// card's play button does.
async function runCurrent() {
  if (running || !doc.file) return;
  if (!source) {
    const all = blocks();
    if (selected >= 0 && selected < all.length) await start("RunBlock", Number(all[selected].dataset.block));
    else status("no block selected");
    return;
  }
  await applySource();
  const cm = source.view.state;
  const c = blockAtLine(cm.doc.toString(), cm.doc.lineAt(cm.selection.main.head).number);
  if (c) await start("RunBlock", c.block);
  else status("the cursor isn't in a block that runs");
}

// blockAtLine is the runnable cell of text, the file's text, whose lines
// hold line, counted from 1. Cells count in bytes, so the text is too.
function blockAtLine(text: string, line: number): Cell | undefined {
  const bytes = new TextEncoder().encode(text);
  const newlines = (from: number, to: number) => {
    let n = 0;
    for (let i = from; i < to; i++) if (bytes[i] === 10) n++;
    return n;
  };
  return cells.find((c) => {
    if (c.block < 0 || line < c.line) return false;
    const to = Math.min(c.to, bytes.length);
    const last = c.line + newlines(c.from, to) - (to > c.from && bytes[to - 1] === 10 ? 1 : 0);
    return line <= last;
  });
}

// cellAt is the drawn cell holding line, or the last one before it.
function cellAt(line: number): HTMLElement | null {
  let found: HTMLElement | null = null;
  for (const e of document.querySelectorAll<HTMLElement>("#doc > [data-line]")) {
    if (Number(e.dataset.line) > line) break;
    found = e;
  }
  return found;
}

// markCurrent marks the cell the source pane's cursor is in. A block
// the cursor is in becomes the selected one, so the rendered view, the
// outline and enter all go by the same block.
function markCurrent(line: number) {
  for (const e of document.querySelectorAll("#doc > .current")) e.classList.remove("current");
  const cell = cellAt(line);
  cell?.classList.add("current");
  if (cell?.classList.contains("block")) select(blocks().indexOf(cell), false);
  else if (cell?.classList.contains("heading-cell") && Number(cell.dataset.line) === line) selectSection(cell);
}

// syncScroll scrolls one side of the split view to match the other.
let scrolling: "source" | "doc" | "outline" | null = null;
let scrollingTimer = 0;
// holdScroll stops the panes following each other while both are moved
// to one place.
function holdScroll() {
  scrolling = "outline";
  clearTimeout(scrollingTimer);
  scrollingTimer = window.setTimeout(() => (scrolling = null), 800);
}

function syncScroll(from: "source" | "doc") {
  if (!source || !view.split || (scrolling && scrolling !== from)) return;
  scrolling = from;
  clearTimeout(scrollingTimer);
  scrollingTimer = window.setTimeout(() => (scrolling = null), 150);
  const pane = $("doc-scroll");
  const cm = source.view;
  if (from === "source") {
    const line = cm.state.doc.lineAt(cm.lineBlockAtHeight(cm.scrollDOM.scrollTop).from).number;
    const cell = cellAt(line);
    // The pane is positioned, for the dependency lines, so a cell's
    // offsetTop is from the pane's top.
    if (cell) pane.scrollTop = cell.offsetTop - 8;
    return;
  }
  const top = pane.scrollTop;
  const cell = [...document.querySelectorAll<HTMLElement>("#doc > [data-line]")].find((e) => e.offsetTop + e.offsetHeight > top);
  if (!cell) return;
  const line = Math.min(Number(cell.dataset.line), cm.state.doc.lines);
  cm.scrollDOM.scrollTop = cm.lineBlockAt(cm.state.doc.line(line).from).top;
}

// The outline drawer: the headings and blocks, to jump around long
// runbooks. Each block shows its state: ✓ ran, ? waiting for an answer,
// ✗ failed, ▶ the run's current step, ◐ running in the background. The
// selected block has a play button, which runs it as its card's does.

function drawOutline() {
  drawRunning();
  const nav = $("outline");
  if (!view.outline) return;
  const items: HTMLElement[] = [];
  let level = 0;
  for (const e of document.querySelectorAll<HTMLElement>("#doc > section:not(.in-fold)")) {
    if (e.classList.contains("input-cell")) {
      // An input whose var= binds nothing is marked !.
      const taken = e.classList.contains("taken");
      const name = e.querySelector(".name")?.textContent ?? "";
      const item = outlineItem(name, level + 1, e, "step input" + (taken ? " bad" : ""), taken ? "!" : "");
      items.push(outlineRow(item, e, 0));
      continue;
    }
    if (e.classList.contains("block")) {
      const ask = e.querySelector<HTMLElement>(".ask");
      const mark = blockMark(e);
      const name = e.querySelector(".name")?.firstChild?.textContent ?? "";
      const item = outlineItem(name, level + 1, e, "step" + markClass(mark), mark);
      if (mark === "▶") item.title = name + " (running now)";
      else if (mark === "◐") item.title = name + " (running in the background)";
      const block = Number(e.dataset.block);
      const play = iconButton("play", "Run this block (enter)", () => start("RunBlock", block));
      play.classList.add("run");
      play.disabled = running;
      const row = outlineRow(item, e, 0);
      row.append(play);
      row.dataset.block = String(block);
      items.push(row);
      continue;
    }
    for (const h of e.querySelectorAll<HTMLElement>("h1, h2, h3, h4, h5, h6")) {
      level = Number(h.tagName[1]);
      const item = outlineItem(h.textContent ?? "", level, h, "heading", "");
      if (e.dataset.fold) {
        // The mark folds the section, in the outline and the view.
        const mark = item.querySelector<HTMLElement>(".mark")!;
        mark.replaceChildren(svg(icons.chevron));
        mark.classList.add("fold");
        mark.classList.toggle("closed", sectionFolded(e));
        mark.title = "Fold or unfold this section";
        mark.onclick = (ev) => {
          ev.stopPropagation();
          foldSection(e);
        };
      }
      const row = outlineRow(item, e, level);
      row.dataset.section = "";
      if (e.dataset.sectionTo) {
        const play = iconButton("play", "Run this section (s)", () => void runSection(e));
        play.classList.add("run");
        play.disabled = running;
        row.append(play);
        row.dataset.section = e.dataset.fold ?? "";
      }
      items.push(row);
    }
  }
  nav.replaceChildren(...(items.length ? items : [el("p", { className: "muted" }, "No headings or blocks")]));
  markOutline();
}

// markOutline shows the play button on the selected block's or
// heading's entry.
function markOutline() {
  const b = blocks()[selected]?.dataset.block;
  for (const row of document.querySelectorAll<HTMLElement>("#outline .row")) {
    row.classList.toggle("active", row.dataset.section !== undefined ? !!selectedSection && row.dataset.section === selectedSection : row.dataset.block === b);
  }
}

// blockMark is the mark for a block card's state: ? waiting for an
// answer, ▶ the run's current step, ◐ running in the background, ✗
// failed, ✓ ran.
function blockMark(e: HTMLElement): string {
  const ask = e.querySelector<HTMLElement>(".ask");
  if (ask && !ask.hidden) return "?";
  if (e.classList.contains("now")) return "▶";
  if (e.classList.contains("background")) return "◐";
  if (e.classList.contains("failed")) return "✗";
  return e.classList.contains("ran") ? "✓" : "";
}

function markClass(mark: string): string {
  return { "✗": " bad", "✓": " ok", "▶": " now", "◐": " bg", "?": " now" }[mark] ?? "";
}

// outlineRow holds an outline entry for cell, with a grip to drag the
// block, or with level the heading's whole section, when it can move.
function outlineRow(item: HTMLElement, cell: HTMLElement, level: number): HTMLElement {
  const row = el("div", { className: "row" }, item);
  row.dataset.from = cell.dataset.from ?? "";
  if (cell.dataset.drop) row.dataset.drop = "1";
  if (level) row.dataset.level = String(level);
  const movable = level ? cell.dataset.drop === "1" : !!cell.querySelector(".grip");
  if (movable) {
    const title = level ? "Drag to move this section, with all that is in it" : "Drag to move this block";
    row.prepend(grip(title, (ev, handle) => startDrag(ev, handle, row, Number(row.dataset.from), level, "outline")));
  }
  return row;
}

function outlineItem(text: string, level: number, target: HTMLElement, kind: string, mark: string): HTMLElement {
  const b = el("button", { className: "item " + kind, title: text }, el("span", { className: "mark" }, mark), el("span", { className: "text" }, text));
  b.style.paddingLeft = 8 + (level - 1) * 12 + "px";
  b.onclick = () => jumpTo(target);
  return b;
}

// jumpTo brings target to the top of the rendered view and its markdown
// to the top of the source pane, and selects it if it is a block.
function jumpTo(target: HTMLElement) {
  reveal(target.closest<HTMLElement>("#doc > *") ?? target);
  // Both panes go to the entry, so neither follows the other meanwhile.
  holdScroll();
  target.scrollIntoView({ block: "start", behavior: "smooth" });
  const i = blocks().indexOf(target);
  if (i >= 0) select(i, false);
  const heading = target.closest<HTMLElement>("#doc > .heading-cell");
  if (heading) selectSection(heading);
  const cell = target.closest<HTMLElement>("[data-line]");
  if (source && cell) {
    const cm = source.view;
    const line = cm.state.doc.line(Math.min(Number(cell.dataset.line), cm.state.doc.lines));
    cm.dispatch({ selection: { anchor: line.from }, effects: EditorView.scrollIntoView(line.from, { y: "start" }) });
  }
}

// drawRunning lists the blocks running now in the toolbar: the run's
// current step first (or the block waiting for an answer), then the
// ones in the background. A click on one jumps to it.
function drawRunning() {
  // Blocks in folded sections run too.
  const all = [...document.querySelectorAll<HTMLElement>("#doc .block")];
  const now = all.filter((e) => "?▶".includes(blockMark(e) || "x"));
  const bg = all.filter((e) => blockMark(e) === "◐");
  const chip = (e: HTMLElement) => {
    const mark = blockMark(e);
    const name = e.querySelector(".name")?.firstChild?.textContent || "block " + (all.indexOf(e) + 1);
    const why = mark === "?" ? "waiting for an answer" : mark === "▶" ? "running now" : "running in the background";
    const b = el("button", { className: "chip" + markClass(mark), title: name + ": " + why + ". Click to go to it." }, el("span", { className: "mark" }, mark), name);
    b.onclick = () => jumpTo(e);
    return b;
  };
  const list = $("running");
  list.replaceChildren(...now.map(chip), ...bg.map(chip));
  list.hidden = list.childElementCount === 0;
}

// applyView shows what view says, and keeps it for the next window.
function applyView() {
  $("outline").hidden = !view.outline;
  if (!view.split) view.rendered = true;
  $("source").hidden = !view.split;
  $("doc-scroll").hidden = !view.rendered;
  $("workspace").classList.toggle("split", view.split);
  showSplitGrip(view.split && view.rendered);
  document.body.classList.toggle("hide-code", !view.code);
  document.body.classList.toggle("hide-output", !view.output);
  $("outline-toggle").classList.toggle("on", view.outline);
  $("view-split").classList.toggle("on", view.split);
  $("view-rendered").classList.toggle("on", view.rendered);
  showConsole(view.console);
  syncHistory();
  if (view.split) openSource();
  else closeSource();
  drawOutline();
  try {
    localStorage.setItem("mdsh-view", JSON.stringify(view));
  } catch {
    // Only a convenience: the next tab starts with the default view.
  }
  void App.SetView(view).catch(() => {});
}

// showFile names the file in the header, which Save As can change
// without the document being read again. It returns the file's name.
function showFile(file: string): string {
  const name = file.split(/[\\/]/).pop() ?? "";
  $("file").textContent = name;
  $("file").title = file;
  $("changed-text").textContent = name + " changed on disk.";
  return name;
}

// syncHistory shows the History pane if the view has it, and tells Go
// when that changes, as when the file opens or is read again.
function syncHistory() {
  if (!doc.file) {
    showBar(false);
    return;
  }
  if ($("history").hidden) showBar(true);
  if (view.history === doc.history && $("history").classList.contains("closed") === !view.history) return;
  const had = doc.history;
  doc.history = view.history;
  void showHistory(view.history, had);
}

// openView shows the view the settings open files in, if they name one.
function openView() {
  const v = settings.view;
  if (v !== "rendered" && v !== "source" && v !== "both") return;
  view = { ...view, split: v !== "rendered", rendered: v !== "source" };
  applyView();
}

function toggle(key: keyof View, on = !view[key]) {
  view = { ...view, [key]: on };
  // Turning off the only pane shown shows the other one.
  if (!view.split && !view.rendered) view = { ...view, [key === "split" ? "rendered" : "split"]: true };
  applyView();
}

// Actions run from menus, buttons and keys.

const actions: Record<string, () => Promise<void> | void> = {
  newfile: () => openNewFile(),
  async save() {
    try {
      await App.Save();
      status("saved");
    } catch (err) {
      fail(err);
    }
  },
  async saveas() {
    try {
      if (await App.SaveAs()) status("saved");
    } catch (err) {
      fail(err);
    }
  },
  async reload() {
    try {
      await App.Reload();
    } catch (err) {
      fail(err);
    }
  },
  runall: () => (running ? actions.stop() : start("RunFrom", 0)),
  // clearoutputs takes all output out of the document. In the source
  // pane it is one edit, so undo brings the output back.
  async clearoutputs() {
    if (!doc.file || running) return;
    await applySource();
    try {
      await App.ClearOutputs();
      status("outputs cleared");
    } catch (err) {
      fail(err);
    }
    await refresh();
  },
  runcurrent: () => runCurrent(),
  runsection: () => runCurrentSection(),
  report: () => (doc.file && !running ? openReport() : undefined),
  stop: () => App.Stop(),
  runoptions: () => openRunOptions(),
  split: () => toggle("split"),
  rendered: () => toggle("rendered"),
  outline: () => toggle("outline"),
  code: () => toggle("code"),
  output: () => toggle("output"),
  foldblocks: () => void saveFolds(foldAll("blocks", true)),
  foldsections: () => void saveFolds(foldAll("sections", true)),
  unfoldall() {
    void saveFolds([...foldAll("blocks", false), ...foldAll("sections", false)]);
  },
  foldstosource: () => foldsToSource(),
  foldstorendered: () => foldsToRendered(),
  undo: () => undoEdit(false),
  redo: () => undoEdit(true),
  moveup: () => void moveAction(-1, false),
  movedown: () => void moveAction(1, false),
  moveblockup: () => void moveAction(-1, true),
  moveblockdown: () => void moveAction(1, true),
  history: () => toggle("history"),
  console: () => toggle("console"),
};

// Moving blocks
//
// alt+↑/↓ moves a block past the cell above or below it, and with shift
// past the block above or below, prose and all. It moves the selected
// card, the input being typed in, or in the source view the block the
// cursor is in, with its output and tape. Elsewhere in the source view
// the keys are the editor's own (move or copy lines).
//
// The keys are the Edit menu's too. Whichever sees them first acts: a
// key the page took isn't sent on to the menu, and on some systems the
// menu takes it before the page sees it. keyMoved stops both acting.
let keyMoved = 0;

function moveAction(dir: number, byBlock: boolean, key = false): boolean {
  if (!key && Date.now() - keyMoved < 300) return true;
  if (!doc.file) return false;
  const at = document.activeElement as HTMLElement | null;
  const cm = at?.closest<HTMLElement>(".cm-editor");
  if (cm) {
    if (!source || source.view.dom !== cm) return false;
    const view = source.view;
    const line = view.state.doc.lineAt(view.state.selection.main.head).number;
    const inBlock = cells.some((c) => c.movable && c.line <= line && line <= (c.lastLine ?? c.line));
    if (!inBlock) {
      if (key) return false; // the editor's own keys
      lineCommand(dir, byBlock)(view);
      return true;
    }
    void moveInSource(dir, byBlock);
    return true;
  }
  if (key && at?.tagName === "SELECT") return false; // alt+↓ opens a dropdown
  const input = at?.closest<HTMLElement>("#doc [data-input]")?.dataset.input;
  const i = input != null ? Number(input) : selected >= 0 ? Number(blocks()[selected]?.dataset.block ?? -1) : -1;
  if (i < 0 || !cells.some((c) => c.movable && (c.block === i || c.textbox?.block === i || c.choice?.block === i))) return false;
  void moveRendered(i, dir, byBlock);
  return true;
}

function lineCommand(dir: number, byBlock: boolean) {
  if (byBlock) return dir < 0 ? copyLineUp : copyLineDown;
  return dir < 0 ? moveLineUp : moveLineDown;
}

document.addEventListener(
  "keydown",
  (e) => {
    if (!e.altKey || e.metaKey || e.ctrlKey || (e.key !== "ArrowUp" && e.key !== "ArrowDown")) return;
    if (moveAction(e.key === "ArrowUp" ? -1 : 1, e.shiftKey, true)) {
      keyMoved = Date.now();
      e.preventDefault();
      e.stopPropagation();
    }
  },
  true,
);

// setDepends writes block i's depends= as the blocks deps names, from a
// drag or a click on a dependency line. It is a rendered-view edit, so
// Undo takes it back.
async function setDepends(i: number, deps: number[], what: string) {
  if (running) return status("can't change the file while a block runs");
  await applySource();
  try {
    await App.SetDepends(i, deps);
  } catch (err) {
    return fail(err);
  }
  await refresh();
  status(what);
}

// moveRendered moves block i as a rendered-view edit, which Undo takes
// back.
async function moveRendered(i: number, dir: number, byBlock: boolean) {
  if (running) return status("can't change the file while a block runs");
  await applySource();
  const input = (document.activeElement as HTMLElement | null)?.closest("#doc [data-input]") != null;
  let m: Move;
  try {
    m = await App.MoveBlock(i, dir, byBlock);
  } catch (err) {
    return fail(err);
  }
  await refresh();
  if (input) document.querySelector<HTMLElement>(`#doc [data-input="${m.block}"]`)?.focus();
  else {
    const k = blocks().findIndex((b) => Number(b.dataset.block) === m.block);
    if (k >= 0) select(k);
  }
  showMoved(m, () => undoEdit(false));
}

// moveInSource moves the block at the source view's cursor as an edit of
// the source view's own, which its undo takes back.
async function moveInSource(dir: number, byBlock: boolean) {
  if (running) return status("can't change the file while a block runs");
  await applySource();
  const ed = source;
  if (!ed) return;
  const view = ed.view;
  const head = view.state.selection.main.head;
  const cursor = view.state.doc.lineAt(head);
  let m: Move;
  try {
    const i = await App.BlockAt(cursor.number);
    if (i < 0) return void lineCommand(dir, byBlock)(view);
    m = await App.PlanMove(i, dir, byBlock);
  } catch (err) {
    return fail(err);
  }
  if (source !== ed) return;
  // Giving blocks ids first is Go's change, not one to undo.
  if (view.state.doc.toString() !== m.base) ed.load(m.base);
  if (view.state.doc.toString() !== m.base) return status("the file changed; try again");
  const line = Math.min(cursor.number + m.line - m.fromLine, 1 + m.text.split("\n").length - 1);
  view.dispatch({ changes: diff(m.base, m.text), userEvent: "move.line", scrollIntoView: true });
  const to = view.state.doc.line(Math.max(1, Math.min(line, view.state.doc.lines)));
  view.dispatch({ selection: { anchor: Math.min(to.from + (head - cursor.from), to.to) }, scrollIntoView: true });
  await applySource();
  showMoved(m, () => {
    if (source === ed) undo(ed.view);
    void applySource();
  });
}

// showMoved says what a move changed that depends on block order, with
// an Undo button, or just that it moved.
function showMoved(m: Move, undoIt: () => unknown) {
  const bar = $("moved");
  const warnings = m.warnings ?? [];
  if (!warnings.length) {
    bar.hidden = true;
    status(m.note ? `moved. ${m.note}` : "moved");
    return;
  }
  $("moved-text").textContent = [...warnings, m.note ?? ""].filter(Boolean).join(" ");
  $("moved-undo").onclick = () => {
    bar.hidden = true;
    void undoIt();
  };
  bar.hidden = false;
  status("moved");
}
$("moved-close").onclick = () => ($("moved").hidden = true);

// Deleting blocks
//
// The selected card has a trash button, as delete or backspace does: it
// asks first, then takes the block out of the file with its output and
// tape, as a rendered-view edit that Undo puts back. The bar says so,
// with an Undo button of its own. Its output has a trash button too,
// which clears just the output.

function trashButton(c: Cell): HTMLButtonElement {
  const b = iconButton("trash", "Delete this block (delete)", () => void deleteBlock(c.block));
  b.classList.add("trash");
  return b;
}

// clearOutputButton clears the output on the selected card, without
// asking, since the bar's Undo puts it back.
function clearOutputButton(c: Cell): HTMLButtonElement {
  const b = iconButton("trash", "Clear this block's output", () => void clearOutput(c.block));
  b.classList.add("trash", "clear-output");
  return b;
}

async function clearOutput(i: number) {
  if (running) return status("can't change the file while a block runs");
  await applySource();
  try {
    await App.ClearOutput(i);
  } catch (err) {
    return fail(err);
  }
  await refresh();
  $("moved-text").textContent = "Cleared the output.";
  $("moved-undo").onclick = () => {
    $("moved").hidden = true;
    void undoEdit(false);
  };
  $("moved").hidden = false;
  status("cleared output");
}

// deleteBlock asks whether to delete block i, and does.
async function deleteBlock(i: number) {
  if (running) return status("can't change the file while a block runs");
  const c = cells.find((c) => c.block === i);
  if (!c?.movable) return status("only top-level blocks can be deleted");
  const id = c.name ?? c.id ?? "";
  if (!(await confirmDelete(id ? "#" + id : `the block at line ${c.line}`))) return;
  await applySource();
  const k = blocks().findIndex((b) => Number(b.dataset.block) === i);
  let d: Deleted;
  try {
    d = await App.DeleteBlock(i);
  } catch (err) {
    return fail(err);
  }
  await refresh();
  const all = blocks();
  if (all.length) select(Math.min(Math.max(k, 0), all.length - 1));
  $("moved-text").textContent = [`Deleted ${d.name}.`, ...(d.warnings ?? [])].join(" ");
  $("moved-undo").onclick = () => {
    $("moved").hidden = true;
    void undoEdit(false);
  };
  $("moved").hidden = false;
  status("deleted " + d.name);
}

// confirmDelete asks whether to delete the block named name.
function confirmDelete(name: string): Promise<boolean> {
  const dialog = $<HTMLDialogElement>("delete-dialog");
  $("delete-name").textContent = name;
  dialog.returnValue = "";
  dialog.showModal();
  $("delete-ok").focus();
  return new Promise((resolve) => (dialog.onclose = () => resolve(dialog.returnValue === "delete")));
}

// Dragging
//
// A card's grip drags its block, with its output, to a spot between
// top-level cells; the outline's grips drag a block, or a heading's whole
// section, between its entries. A section goes only before a heading of
// its level or higher, or to the end. A line shows the spot; near the
// top or bottom edge the pane scrolls; Esc cancels.

function grip(title: string, start: (ev: PointerEvent, handle: HTMLElement) => void): HTMLElement {
  const g = el("button", { className: "grip", title, tabIndex: -1 }, "⋮⋮");
  g.onpointerdown = (ev) => {
    if (ev.button === 0) start(ev, g);
  };
  g.onclick = (ev) => ev.stopPropagation();
  return g;
}

interface Spot {
  at: number; // a cell's From, or -1 for the end
  y: () => number; // where the line goes, in the viewport
}

// spots are where a drag in pane can drop: before each row or card
// marked drop, and at the end. For a section (level > 0), only before
// headings of its level or higher.
function spots(pane: "doc" | "outline", level: number): Spot[] {
  const rows = pane === "doc" ? [...$("doc").children] : [...$("outline").querySelectorAll<HTMLElement>(":scope > .row")];
  const shown = (rows as HTMLElement[]).filter((e) => e.offsetParent !== null);
  const out: Spot[] = [];
  for (const e of shown) {
    if (e.dataset.drop !== "1") continue;
    if (level && !(Number(e.dataset.level ?? 0) > 0 && Number(e.dataset.level) <= level)) continue;
    out.push({ at: Number(e.dataset.from), y: () => e.getBoundingClientRect().top });
  }
  const last = shown[shown.length - 1];
  if (last) out.push({ at: -1, y: () => last.getBoundingClientRect().bottom });
  return out;
}

function startDrag(ev: PointerEvent, handle: HTMLElement, item: HTMLElement, from: number, level: number, pane: "doc" | "outline") {
  if (running) return void status("can't change the file while a block runs");
  ev.preventDefault();
  const scroller = pane === "doc" ? $("doc-scroll") : $("outline");
  const all = spots(pane, level);
  if (!all.length) return;
  handle.setPointerCapture(ev.pointerId);
  const line = el("div", { className: "drop-line", hidden: true });
  document.body.append(line);
  const startY = ev.clientY;
  let y = ev.clientY;
  let dragging = false;
  let spot: Spot | null = null;
  let frame = 0;
  const place = () => {
    const box = scroller.getBoundingClientRect();
    spot = null;
    let best = Infinity;
    for (const s of all) {
      const d = Math.abs(s.y() - y);
      if (d < best) [best, spot] = [d, s];
    }
    if (!spot) return;
    const top = Math.max(box.top, Math.min(box.bottom, spot.y()));
    Object.assign(line.style, { top: top - 1 + "px", left: box.left + 8 + "px", width: box.width - 16 + "px" });
    line.hidden = false;
  };
  // Near an edge the pane scrolls, faster nearer it.
  const scroll = () => {
    frame = 0;
    if (!dragging) return;
    const box = scroller.getBoundingClientRect();
    const edge = 40;
    const by = y < box.top + edge ? -(box.top + edge - y) / 2 : y > box.bottom - edge ? (y - box.bottom + edge) / 2 : 0;
    if (by) {
      scroller.scrollTop += by;
      place();
      frame = requestAnimationFrame(scroll);
    }
  };
  const end = (drop: boolean) => {
    handle.removeEventListener("pointermove", move);
    handle.removeEventListener("pointerup", up);
    handle.removeEventListener("pointercancel", cancel);
    document.removeEventListener("keydown", esc, true);
    cancelAnimationFrame(frame);
    line.remove();
    item.classList.remove("dragging");
    if (drop && dragging && spot) void moveTo(from, spot.at);
  };
  const move = (e: PointerEvent) => {
    y = e.clientY;
    if (!dragging && Math.abs(y - startY) < 4) return;
    if (!dragging) {
      dragging = true;
      item.classList.add("dragging");
    }
    place();
    if (!frame) frame = requestAnimationFrame(scroll);
  };
  const up = () => end(true);
  const cancel = () => end(false);
  const esc = (e: KeyboardEvent) => {
    if (e.key !== "Escape") return;
    e.preventDefault();
    e.stopPropagation();
    end(false);
  };
  handle.addEventListener("pointermove", move);
  handle.addEventListener("pointerup", up);
  handle.addEventListener("pointercancel", cancel);
  document.addEventListener("keydown", esc, true);
}

// moveTo moves the block or section at from to at, as a rendered-view
// edit, which Undo takes back.
async function moveTo(from: number, at: number) {
  if (running) return status("can't change the file while a block runs");
  await applySource();
  let m: Move;
  try {
    m = await App.MoveTo(from, at);
  } catch (err) {
    return fail(err);
  }
  if (m.text === m.base) return;
  await refresh();
  if (m.block >= 0) {
    const k = blocks().findIndex((b) => Number(b.dataset.block) === m.block);
    if (k >= 0) select(k);
  }
  [...$("doc").children].find((e) => Number((e as HTMLElement).dataset.line) === m.line)?.scrollIntoView({ block: "nearest" });
  showMoved(m, () => undoEdit(false));
}

// undoEdit is Edit › Undo, or Redo. Where the focus is decides what it
// takes back: the source view keeps its own history, a text field its
// own, and everything else is the rendered view's, whose edits (tasks,
// textboxes and choices) Go keeps.
async function undoEdit(again: boolean) {
  const at = document.activeElement as HTMLElement | null;
  const cm = at?.closest<HTMLElement>(".cm-editor");
  if (cm) {
    const view = EditorView.findFromDOM(cm);
    if (view) (again ? redo : undo)(view);
    return;
  }
  if (at?.closest("textarea, input:not([type=checkbox]):not([type=radio]), [contenteditable]")) {
    document.execCommand(again ? "redo" : "undo");
    return;
  }
  if (!doc.file) return;
  if (running) {
    status("can't change the file while a block runs");
    return;
  }
  await applySource();
  try {
    const what = await (again ? App.Redo() : App.Undo());
    status(what ? `${again ? "redid" : "undid"}: ${what}` : `nothing to ${again ? "redo" : "undo"}`);
  } catch (err) {
    fail(err);
  }
  await refresh();
}

// Run options

function openRunOptions() {
  if (!doc.file || running) return;
  $<HTMLInputElement>("opt-args").value = doc.args;
  const only = $<HTMLSelectElement>("opt-only");
  only.replaceChildren(el("option", { value: "" }, "All blocks"));
  for (const c of cells) if (c.block >= 0 && c.id) only.append(el("option", { value: c.id }, c.name ?? c.id));
  only.value = doc.run.only;
  $<HTMLInputElement>("opt-step").checked = doc.run.step;
  $<HTMLInputElement>("opt-stop").checked = doc.run.stopOnError;
  $("opt-error").hidden = true;
  $<HTMLDialogElement>("run-options").showModal();
  $("opt-args").focus();
}

$("opt-ok").onclick = async (e) => {
  e.preventDefault();
  try {
    await App.SetRunOptions(
      $<HTMLInputElement>("opt-args").value,
      $<HTMLSelectElement>("opt-only").value,
      $<HTMLInputElement>("opt-step").checked,
      $<HTMLInputElement>("opt-stop").checked,
    );
    $<HTMLDialogElement>("run-options").close();
    await refresh();
  } catch (err) {
    $("opt-error").textContent = message(err);
    $("opt-error").hidden = false;
  }
};
$("options").onclick = openRunOptions;

// Start screen

let startScreen: Start = { dir: "", home: "", files: [], full: false, recent: [] };

async function drawStart() {
  try {
    startScreen = await App.StartScreen();
    $("start-dir").textContent = tilde(startScreen.dir) + (startScreen.full ? " (showing the first " + (startScreen.files ?? []).length + " files)" : "");
  } catch (err) {
    startScreen = { dir: "", home: "", files: [], full: false, recent: [] };
    $("start-dir").textContent = message(err);
  }
  filterStart();
  $("filter").focus();
}

function fileRow(path: string, label: string, detail: string, desc = ""): HTMLElement {
  const name = el("span", { className: "fname" }, label);
  const left = desc ? el("span", { className: "fleft" }, name, el("span", { className: "fdesc muted", title: desc }, desc)) : name;
  const b = el("button", { className: "file", title: path }, left, el("span", { className: "muted" }, detail));
  b.onclick = () => openPath(path);
  return el("li", {}, b);
}

function filterStart() {
  const q = $<HTMLInputElement>("filter").value.trim().toLowerCase();
  const match = (s: string) => s.toLowerCase().includes(q);
  const recent = (startScreen.recent ?? []).filter(match);
  $("recent-list").hidden = recent.length === 0;
  $("recent-list")
    .querySelector("ul")!
    .replaceChildren(...recent.map((p) => fileRow(p, p.split(/[\\/]/).pop() ?? p, tilde(p.replace(/[\\/][^\\/]*$/, "")))));
  const groups = new Map<string, NonNullable<Start["files"]>>();
  for (const f of startScreen.files ?? []) {
    if (!match(f.rel) && !match(f.description ?? "")) continue;
    const i = f.rel.lastIndexOf("/");
    const dir = i < 0 ? "" : f.rel.slice(0, i);
    if (!groups.has(dir)) groups.set(dir, []);
    groups.get(dir)!.push(f);
  }
  const list = $("file-list");
  list.replaceChildren();
  for (const [dir, files] of groups) {
    list.append(
      el("h2", {}, dir === "" ? "This folder" : dir + "/"),
      el("ul", { className: "files" }, ...files.map((f) => fileRow(f.path, f.rel.slice(dir === "" ? 0 : dir.length + 1), blocksLabel(f.blocks) + " · " + ago(f.mod), f.description))),
    );
  }
  $("start-empty").hidden = groups.size > 0 || recent.length > 0;
}

// tilde shows a path under home as ~/….
function tilde(path: string): string {
  const home = startScreen.home;
  if (home && (path === home || path.startsWith(home + "/"))) return "~" + path.slice(home.length);
  return path;
}

function blocksLabel(n: number): string {
  if (n < 0) return "can't read";
  return n === 1 ? "1 block" : n + " blocks";
}

function ago(when: string): string {
  const s = (Date.now() - new Date(when).getTime()) / 1000;
  if (s < 90) return "just now";
  if (s < 3600) return Math.round(s / 60) + " min ago";
  if (s < 86400) return Math.round(s / 3600) + " h ago";
  const days = Math.round(s / 86400);
  if (days < 30) return days === 1 ? "1 day ago" : days + " days ago";
  return new Date(when).toLocaleDateString();
}

async function openPath(path: string) {
  try {
    await App.OpenPath(path);
  } catch (err) {
    $("start-dir").textContent = message(err);
  }
}

$("filter").oninput = filterStart;
$("filter").onkeydown = (e) => {
  if (e.key === "Enter") document.querySelector<HTMLElement>("#start .file")?.click();
};
$("new-file").onclick = () => openNewFile();

// New file

// openNewFile asks which template to start from, when the templates
// folder in Settings has any, and then where to save the file.
async function openNewFile() {
  let templates: Template[] = [];
  try {
    templates = (await App.Templates()) ?? [];
  } catch (err) {
    $("new-file-error").textContent = "Templates folder: " + message(err);
    $("new-file-error").hidden = false;
  }
  if (templates.length === 0 && $("new-file-error").hidden) return void App.NewFile("").catch(fail);
  const pick = (path: string, label: string, detail: string, desc = "") => {
    const row = fileRow(path, label, detail, desc);
    row.querySelector("button")!.onclick = (e) => {
      e.preventDefault();
      $<HTMLDialogElement>("new-file-dialog").close();
      App.NewFile(path).catch(fail);
    };
    return row;
  };
  $("templates").replaceChildren(
    pick("", "Blank", "a heading named for the file"),
    ...templates.map((f) => pick(f.path, f.rel.replace(/\.(md|markdown)$/, ""), blocksLabel(f.blocks), f.description)),
  );
  $<HTMLDialogElement>("new-file-dialog").showModal();
}
$<HTMLDialogElement>("new-file-dialog").onclose = () => ($("new-file-error").hidden = true);
$("open-other").onclick = () => App.Open().catch(fail);

// Events from Go

interface BlockEvent {
  kind: string;
  block: number;
  data?: string;
  bytes?: string; // base64
}

interface Question {
  block: number;
  question: string;
  // of, when set, is the block that depends on block, which hasn't run.
  of?: number;
}

// liveColors is the ANSI state of each stream of a running block's
// output.
const liveColors = new WeakMap<HTMLElement, Record<string, Ansi>>();

function onBlock(e: BlockEvent) {
  // A command typed in the History pane runs there, before its card is
  // drawn.
  if (e.kind === "terminal" && historyTerminal(e.block, e.data)) return;
  if (e.kind === "tty" && historyWrite(e.block, e.bytes)) return;
  if (e.kind === "terminal-end" && historyEnd(e.block)) return;
  const block = document.getElementById("block-" + e.block);
  if (!block) return;
  const out = block.querySelector(".output")!;
  switch (e.kind) {
    case "background":
      // The block runs on while others do; done comes when it ends.
      block.querySelector<HTMLElement>(".ask")!.hidden = true;
      block.classList.remove("ran", "failed");
      block.classList.add("background");
      block.querySelector(".exit")!.textContent = backgroundNote;
      out.replaceChildren();
      break;
    case "started":
      for (const b of document.querySelectorAll("#doc .block.now")) b.classList.remove("now");
      block.classList.add("now");
      block.querySelector<HTMLElement>(".ask")!.hidden = true;
      out.replaceChildren(el("pre", { className: "live" }));
      follow(block);
      break;
    case "stdout":
    case "stderr": {
      const live = out.querySelector<HTMLElement>(".live");
      if (!live) break;
      // Each stream keeps its own colors between pieces.
      const streams = (liveColors.get(live) ?? liveColors.set(live, {}).get(live))!;
      const ansi = (streams[e.kind] ??= new Ansi());
      live.append(el("span", { className: e.kind }, ...ansi.write(e.data ?? "")));
      if (blockFolded(block, "output")) drawPart(block, "output");
      if (block === following) followOutput();
      break;
    }
    case "done":
      $("dirty").hidden = false;
      block.classList.remove("now");
      // A background block can end while nothing runs, so nothing
      // else would draw its output.
      if (block.classList.contains("background")) {
        if (!running) void refresh().then(backgroundStatus);
        else {
          // Its result shows when the run ends and the page redraws.
          block.classList.remove("background");
          block.querySelector(".exit")!.textContent = "";
        }
      }
      break;
    case "terminal":
      // The block runs in its card, so show it, as a confirm does.
      if (!view.rendered) toggle("rendered", true);
      openTerminal(e.block);
      follow(block);
      break;
    case "tty":
      return writeTerminal(e.block, e.bytes);
    case "terminal-end":
      endTerminal(e.block);
      break;
  }
  if (e.kind !== "stdout" && e.kind !== "stderr") drawOutline();
}

function onConfirm({ block, question, of }: Question) {
  const card = document.getElementById("block-" + block);
  if (!card) return;
  // The question is asked in the block's card, so show it.
  if (!view.rendered) toggle("rendered", true);
  const ask = card.querySelector<HTMLElement>(".ask")!;
  const why =
    of === undefined
      ? "This block is marked confirm, so it waits for you before it runs."
      : "A block you ran lists this one in depends=, and it hasn't run in this session.";
  const info = el("span", { className: "info", title: why }, svg(icons.info));
  const answer = (reply: string) => async () => {
    ask.hidden = true;
    drawOutline();
    await App.Answer(reply).catch(fail);
  };
  const run = el("button", { className: "primary" }, "Run");
  run.onclick = answer("run");
  const skip = el("button", {}, "Skip");
  skip.onclick = answer("skip");
  const stop = el("button", {}, "Stop");
  stop.onclick = answer("stop");
  ask.replaceChildren(info, el("span", { className: "question" }, question), run, skip, stop);
  ask.hidden = false;
  reveal(card);
  card.scrollIntoView({ block: "center" });
  status("waiting for you");
  drawOutline();
}

Events.On("block", mine(onBlock));
Events.On("confirm", mine(onConfirm));
// A run this page didn't start is over.
Events.On(
  "ran",
  mine(() => {
    if (ranHere || !running) return;
    busy(false);
    status("");
    void refresh().then(() => (following = null));
  }),
);

Events.On("opened", mine(() => ((selected = -1), void refresh())));
Events.On(
  "saved",
  mine<string>((file) => {
    // Save As saves to a new file, so name it.
    doc = { ...doc, file, dirty: false, changed: false };
    showFile(file);
    $("changed").hidden = true;
    $("dirty").hidden = true;
    void loadSource();
  }),
);
Events.On(
  "file-changed",
  mine(() => {
    delete $("changed").dataset.kept;
    $("changed").hidden = false;
  }),
);
Events.On(
  "menu",
  mine<string>((action) => {
    const insert = action.match(/^insert:(.+)$/);
    if (insert) return void (source && insertAction(source.view, insert[1]));
    void actions[action]?.();
  }),
);
// Settings come from the Settings window, so they aren't this window's.
Events.On("settings", (ev: { data: Settings }) => {
  settings = ev.data;
  applyTheme(settings.theme);
  source?.setVim(settings.vim);
  source?.setSpellCheck(!settings.noSpellCheck);
});

$("run-all").onclick = () => void actions.runall();
let resizeTimer = 0;
window.addEventListener("resize", () => {
  clearTimeout(resizeTimer);
  resizeTimer = window.setTimeout(() => {
    if (!doc.file) return;
    sizeTerminal();
    fitHistory();
  }, 200);
});
initSplitGrip();
initDeps($("doc-scroll"), { jump: jumpTo, setDepends });
initHistory({
  canRun: () => !running,
  async enter(call) {
    busy(true);
    ranHere = true;
    status("running");
    await call();
    status("");
    ranHere = false;
    busy(false);
    await refresh();
    // What was added is at the end.
    const scroll = $("doc-scroll");
    scroll.scrollTop = scroll.scrollHeight;
  },
  status: (i) => cells.find((c) => c.block === i)?.status,
  toggle: () => void actions.history(),
  stop: () => void actions.stop(),
  async restart() {
    try {
      await App.RestartShells();
    } catch (err) {
      fail(err);
    }
    await refresh();
  },
});
initConsole({ toggle: () => void actions.console() });
initReport({
  dirty: () => !$("dirty").hidden,
  before: applySource,
  runAll: () => start("RunFrom", 0),
});
$("outline-toggle").onclick = () => toggle("outline");
$("view-split").onclick = () => toggle("split");
$("view-rendered").onclick = () => toggle("rendered");
$("doc-scroll").addEventListener("scroll", () => syncScroll("doc"));
// Scrolling by hand, with the wheel, a touch or the scroll bar, stops
// following a run.
$("doc-scroll").addEventListener("wheel", stopFollowing, { passive: true });
$("doc-scroll").addEventListener("touchstart", stopFollowing, { passive: true });
$("doc-scroll").addEventListener("pointerdown", (e) => e.target === e.currentTarget && stopFollowing());
$("reload").onclick = () => void actions.reload();
$("keep").onclick = () => {
  $("changed").dataset.kept = doc.file;
  $("changed").hidden = true;
};

// Keys follow mdsh -i where they don't clash with the OS: j/k move
// between blocks, enter runs the selected one, a runs from it and e edits
// it. s runs the selected heading's section, or the selected block's.
document.addEventListener("keydown", (e) => {
  if (!doc.file || e.metaKey || e.ctrlKey || e.altKey) return;
  const target = e.target as HTMLElement | null;
  if (target?.closest?.("input, textarea, select, dialog, [contenteditable], .cm-editor, .terminal")) return;
  const all = blocks();
  const current = () => Number(all[selected].dataset.block);
  switch (e.key) {
    case "j":
      if (all.length) select(Math.min(selected + 1, all.length - 1));
      break;
    case "k":
      if (all.length) select(Math.max(selected - 1, 0));
      break;
    case "Enter":
      if (selected >= 0) void start("RunBlock", current());
      else if (selectedSection) void runSection(sectionCell());
      break;
    case "a":
      if (selected >= 0) void start("RunFrom", current());
      break;
    case "s":
      void runSection(sectionCell());
      break;
    case "Delete":
    case "Backspace":
      if (selected >= 0) void deleteBlock(current());
      break;
    case "e":
      if (selected >= 0) {
        const c = cells.find((c) => c.block === current());
        if (c) void editBlock(all[selected], c);
      }
      break;
    default:
      return;
  }
  e.preventDefault();
});

me = await Window.Name();
settings = await App.Settings();
applyTheme(settings.theme);
attrs = (await App.BlockAttrs()) ?? [];
applyView();
await refresh();
