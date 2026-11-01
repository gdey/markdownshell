import "./tab.js";
import { EditorView } from "@codemirror/view";
import { Events, Window } from "/wails/runtime.js";

import { App, type AttrSpec, type Cell, type Doc, type File as Template, type Names, type Settings, type Start, message } from "./api.js";
import { $, el, iconButton, icons, svg } from "./dom.js";
import { foldMarks, restoreFolds, setSourceFolds, sourceFolding, sourceFolds } from "./fold.js";
import { Ansi, codeLines, colorOutput, highlightProse } from "./highlight.js";
import { type MarkdownEditor, insertAction, markdownBar, markdownEditor } from "./markdown.js";
import { initReport, openReport } from "./report.js";
import { runGutter, setRunLines } from "./rungutter.js";
import { closeTerminal, endTerminal, openTerminal, sizeTerminal, writeTerminal } from "./terminal.js";
import { applyTheme } from "./theme.js";

let me = ""; // this window's name; events from other windows are ignored
let doc: Doc = { file: "", dirty: false, changed: false, running: false, fresh: false, run: { args: [], only: "", step: false, stopOnError: false, shell: "" }, args: "" };
let cells: Cell[] = [];
let running = false;
let ranHere = false; // the run going on was started from this page
let selected = -1; // index into the block cards, for j/k
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
}
let view: View = { split: false, rendered: true, outline: false, code: true, output: true, ...loadView() };

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
}

function fail(err: unknown) {
  status(message(err));
}

// refresh reads the document's state and draws whichever screen fits.
async function refresh() {
  doc = await App.Doc();
  const open = doc.file !== "";
  $("toolbar").hidden = !open;
  $("workspace").hidden = !open;
  $("start").hidden = open;
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
  drawOutline();
  sizeTerminal();
  restore();
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
  return e;
}

// drawProse draws one top-level block of prose. Clicking it shows its
// markdown in the source pane, except on a link, which followLinks
// follows.
function drawProse(c: Cell): HTMLElement {
  // goldmark's HTML, with raw HTML in the markdown left out.
  const s = el("section", { className: "prose", innerHTML: c.html ?? "", title: "Click to show it in the source" });
  highlightProse(s);
  s.onclick = (e) => {
    if (window.getSelection()?.toString() || (e.target as HTMLElement).closest("input")) return;
    void showInSource(s, c.line, true);
  };
  enableTasks(s, c.tasks);
  if (c.heading) {
    s.classList.add("heading-cell");
    s.dataset.level = String(c.heading);
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
    ),
    foldPart("code", el("pre", { className: "code numbered" }, el("code", {}, ...codeLines(c.body ?? "", c.lang ?? ""))), () => foldBlock(block, "code")),
    el("div", { className: "ask", hidden: true }),
    foldPart("output", el("div", { className: "output", innerHTML: c.output ?? "" }), () => foldBlock(block, "output")),
  );
  colorOutput(block.querySelector(".output")!);
  block.dataset.foldCode = c.collapsed ? "1" : "";
  block.dataset.foldOutput = c.collapsedOutput ? "1" : "";
  // Checkboxes in output, such as out=markdown's, are edited in place
  // too, until the block runs again.
  enableTasks(block.querySelector<HTMLElement>(".output")!, c.outputTasks);
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
// mdsh:collapse lines say how the file opens; a click lasts while the
// tab is open, and is never written to the file. The source pane keeps
// folds of its own, and the Folding menu copies them either way.

// folds are the clicked folds, by key: the block's id and part, or the
// heading's dataset.fold. The rest are as the file says.
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
}

function foldSection(cell: HTMLElement, on = !sectionFolded(cell)) {
  folds.set(cell.dataset.fold ?? "", on);
  applyFolds();
  drawOutline();
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
function foldAll(what: "blocks" | "sections", on: boolean) {
  for (const e of $("doc").children as HTMLCollectionOf<HTMLElement>) {
    if (what === "blocks" && e.classList.contains("block")) {
      folds.set(partKey(e, "code"), on);
      folds.set(partKey(e, "output"), on);
    }
    if (what === "sections" && e.dataset.fold) folds.set(e.dataset.fold, on);
  }
  applyFolds();
  drawOutline();
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
  for (const e of $("doc").children as HTMLCollectionOf<HTMLElement>) {
    const c = cells.find((c) => String(c.line) === e.dataset.line);
    if (!c) continue;
    if (c.heading && e.dataset.fold) folds.set(e.dataset.fold, lines.has(c.line));
    if (c.block >= 0) {
      folds.set(partKey(e, "code"), lines.has(c.line));
      folds.set(partKey(e, "output"), !!c.outputLine && lines.has(c.outputLine));
    }
  }
  if (!view.rendered) toggle("rendered", true);
  applyFolds();
  drawOutline();
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
  markOutline();
  if (i < 0 || i >= all.length) return;
  all[i].classList.add("selected");
  if (scroll) all[i].scrollIntoView({ block: "nearest" });
}

function runAllButton() {
  const b = $<HTMLButtonElement>("run-all");
  b.replaceChildren(svg(running ? icons.stop : icons.play), el("span", {}, running ? "Stop" : "Run all"));
  b.classList.toggle("stop", running);
  b.disabled = !running && !cells.some((c) => c.block >= 0);
}

function busy(on: boolean) {
  running = on;
  if (!on) for (const b of document.querySelectorAll("#doc .block.now")) b.classList.remove("now");
  runAllButton();
  for (const x of document.querySelectorAll<HTMLButtonElement>(".block .head button, #outline .run")) x.disabled = on;
}

async function start(method: "RunBlock" | "RunFrom", block: number) {
  if (running) return;
  busy(true);
  ranHere = true;
  status("running");
  try {
    await App[method](block);
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

let source: MarkdownEditor | null = null;
let sourceTimer = 0;
let sourcePending = false; // typed but not yet applied

function openSource() {
  if (source) return;
  source = markdownEditor($("source-editor"), {
    text: "",
    vim: settings.vim,
    attrs,
    names: () => known,
    lineNumbers: true,
    extensions: [runGutter((line) => void runLine(line)), sourceFolding()],
    onChange() {
      sourcePending = true;
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
  void applySource();
  source.destroy();
  source = null;
  sourceFoldsFor = "";
  $("source-bar").replaceChildren();
}

async function applySource() {
  clearTimeout(sourceTimer);
  if (!source || !sourcePending) return;
  if (running) {
    sourceTimer = window.setTimeout(() => void applySource(), 1000);
    return;
  }
  sourcePending = false;
  try {
    await App.SetText(source.view.state.doc.toString());
    status("");
  } catch (err) {
    fail(err);
    return;
  }
  $("dirty").hidden = false;
  cells = (await App.Cells()) ?? [];
  drawDoc();
  markRunLines();
  markCurrent(source.view.state.doc.lineAt(source.view.state.selection.main.head).number);
}

// sourceFoldsFor is the file whose folds the source pane opened with.
let sourceFoldsFor = "";

async function loadSource() {
  if (!source || sourcePending || doc.file === "") return;
  const marks = foldMarks(source.view);
  try {
    source.setText(await App.Text());
  } catch (err) {
    fail(err);
  }
  markRunLines();
  // The pane opens with the folds the file asks for, then keeps its own.
  if (sourceFoldsFor !== doc.file) {
    sourceFoldsFor = doc.file;
    setSourceFolds(source.view, renderedFoldLines(true));
  } else {
    restoreFolds(source.view, marks);
  }
}

// markRunLines puts a run button on each runnable block's first line.
function markRunLines() {
  const lines = cells.filter((c) => c.block >= 0).map((c) => c.line);
  source?.view.dispatch({ effects: setRunLines.of(lines) });
}

// runLine runs the block whose fence opens on line of the source pane,
// once what was typed is applied.
async function runLine(line: number) {
  if (running) return;
  await applySource();
  const c = cells.find((c) => c.block >= 0 && c.line === line);
  if (c) await start("RunBlock", c.block);
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
    if (cell) pane.scrollTop = cell.offsetTop - pane.offsetTop - 8;
    return;
  }
  const top = pane.scrollTop + pane.offsetTop;
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
      items.push(outlineItem(name, level + 1, e, "step input" + (taken ? " bad" : ""), taken ? "!" : ""));
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
      const row = el("div", { className: "row" }, item, play);
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
      items.push(item);
    }
  }
  nav.replaceChildren(...(items.length ? items : [el("p", { className: "muted" }, "No headings or blocks")]));
  markOutline();
}

// markOutline shows the play button on the selected block's entry.
function markOutline() {
  const b = blocks()[selected]?.dataset.block;
  for (const row of document.querySelectorAll<HTMLElement>("#outline .row")) row.classList.toggle("active", row.dataset.block === b);
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
  document.body.classList.toggle("hide-code", !view.code);
  document.body.classList.toggle("hide-output", !view.output);
  $("outline-toggle").classList.toggle("on", view.outline);
  $("view-split").classList.toggle("on", view.split);
  $("view-rendered").classList.toggle("on", view.rendered);
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
  report: () => (doc.file && !running ? openReport() : undefined),
  stop: () => App.Stop(),
  runoptions: () => openRunOptions(),
  split: () => toggle("split"),
  rendered: () => toggle("rendered"),
  outline: () => toggle("outline"),
  code: () => toggle("code"),
  output: () => toggle("output"),
  foldblocks: () => foldAll("blocks", true),
  foldsections: () => foldAll("sections", true),
  unfoldall() {
    foldAll("blocks", false);
    foldAll("sections", false);
  },
  foldstosource: () => foldsToSource(),
  foldstorendered: () => foldsToRendered(),
};

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
}

// liveColors is the ANSI state of each stream of a running block's
// output.
const liveColors = new WeakMap<HTMLElement, Record<string, Ansi>>();

function onBlock(e: BlockEvent) {
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

function onConfirm({ block, question }: Question) {
  const card = document.getElementById("block-" + block);
  if (!card) return;
  // The question is asked in the block's card, so show it.
  if (!view.rendered) toggle("rendered", true);
  const ask = card.querySelector<HTMLElement>(".ask")!;
  const info = el("span", { className: "info", title: "This block is marked confirm, so it waits for you before it runs." }, svg(icons.info));
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
});

$("run-all").onclick = () => void actions.runall();
let resizeTimer = 0;
window.addEventListener("resize", () => {
  clearTimeout(resizeTimer);
  resizeTimer = window.setTimeout(() => doc.file && sizeTerminal(), 200);
});
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
// it.
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
      break;
    case "a":
      if (selected >= 0) void start("RunFrom", current());
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
