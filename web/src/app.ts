import "./startup.js";
import { tabID } from "./tab.js";
import { copyLineDown, copyLineUp, moveLineDown, moveLineUp, redo, undo } from "@codemirror/commands";
import { ChangeSet, type EditorState, type StateCommand, type Text } from "@codemirror/state";
import { EditorView, scrollPastEnd } from "@codemirror/view";
import { Clipboard, Events, Window } from "/wails/runtime.js";

import { App, type AttrSpec, type Cell, type Deleted, type Heads, type Doc, type File as Template, type Fold, type Move, type Names, type OutsideFiles, type Settings, type SourceBlock, type Start, message } from "./api.js";
import { enableCasts } from "./cast.js";
import { drawDeps, initDeps, litDeps, markDeps, setDeps } from "./deps.js";
import { $, announce, el, iconButton, icons, keyActivates, markOutput, scrollBehavior, setOutput, svg } from "./dom.js";
import { folderGear, initFolders } from "./folders.js";
import { foldMarks, restoreFolds, setSourceFolds, sourceFolding, sourceFolds } from "./fold.js";
import { Ansi, codeLines, colorOutput, highlightProse } from "./highlight.js";
import { busyFrontMatter, frontMatterOpen, initFrontMatter, loadFrontMatter, showFrontMatter } from "./frontmatter.js";
import { linkify } from "./links.js";
import { forceLinting } from "@codemirror/lint";
import { type GoBlocks } from "./editor.js";
import { type MarkdownEditor, insertAction, insertCommand, markdownBar, markdownEditor } from "./markdown.js";
import { confirmShells, dismissShells, lostWarning } from "./confirm.js";
import { addEvent, consoleFile, initConsole, loadEvents, showConsole } from "./console.js";
import { bugReportPage, copyBugReport } from "./bugreport.js";
import { copyHistory, fitHistory, focusHistory, historyBusy, historyEnd, historyLive, historyStatus, historyTerminal, historyWrite, initHistory, jobLabel, selectAllHistory, shellJobs, showBar, showHistory } from "./history.js";
import { NvimPane, nvimColors } from "./nvim.js";
import { offsetOf, posOf, stateOf } from "./nvimtext.js";
import { initReport, openReport } from "./report.js";
import { runGutter, setRunLines } from "./rungutter.js";
import { secretMask, setSecretLines } from "./secretmask.js";
import { textboxGrip } from "./textbox.js";
import { includeControls } from "./include.js";
import { drawHTTP, statusBadge } from "./http.js";
import { drawEarlier, fullTime, timeAgo } from "./earlier.js";
import { initSplitGrip, showSplitGrip } from "./split.js";
import { type LineEdit, Synced, type Version, diff, eventChanges, lineEditsOf, textOf } from "./sync.js";
import { adoptBackground, closeBackgrounds, closeTerminal, endBackground, endTerminal, openBackground, openTerminal, sizeTerminal, wantTerminalSize, writeBackground, writeTerminal } from "./terminal.js";
import { applyTheme } from "./theme.js";
import { applyKeyHints, hinted, isMac } from "./keys.js";
import { drawn as startupDrawn, mark as startupMark } from "./startup.js";
import { type HeadRow, decodeHeads, headOf, moveHead, partial, sameHead } from "./heads.js";
import { Heights } from "./heights.js";

let me = ""; // this window's name; events from other windows are ignored
let doc: Doc = { file: "", dirty: false, changed: false, running: false, fresh: false, openHistory: false, run: { args: [], only: "", step: false, stopOnError: false, shell: "" }, args: "", history: false, termCols: 0, termRows: 0, envBanner: "", frontMatterError: "", frontMatterFix: { label: "", detail: "" }, lastView: "", lastHistory: false };
let cells: Cell[] = [];
let running = false;
// noRun is why no block can run, such as front matter that doesn't
// parse, or "".
let noRun = "";
let ranHere = false; // the run going on was started from this page
let selected = -1; // index into blocks(), for j/k
// selectedSection is the selected heading's fold's name (foldKeys), or "". A
// heading and a block aren't selected both at once.
let selectedSection = "";
let settings: Settings;
let attrs: AttrSpec[] = [];
let known: Names = { ids: [], envs: [], assets: [], files: [], assetsDir: "" }; // for completion in the source pane

// The view: what each tab shows. split shows the source pane and
// rendered the rendered view; at least one is always on. Those two and
// history are each file's own: a file opens as it was last shown, and
// with the History pane closed the first time (fileView). The rest
// start as the last tab left them.
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
    const { split: _s, rendered: _r, history: _h, ...rest } = JSON.parse(localStorage.getItem("mdsh-view") ?? "{}") as Partial<View>;
    return rest;
  } catch {
    return {};
  }
}

// viewFile is the file whose own view the page shows, "" until fileView
// has shown one.
let viewFile = "";

// fileView takes the file's own view: the view the settings open files
// in when the file has just opened, else how the file was last shown,
// else the rendered view; and the History pane only if it was open when
// the file was last shown, or a session just opened.
function fileView() {
  viewFile = doc.file;
  const pick = (doc.fresh && settings.view) || doc.lastView || "rendered";
  const v = pick === "source" || pick === "both" ? pick : "rendered";
  view = { ...view, split: v !== "rendered", rendered: v !== "source", history: doc.lastHistory || doc.openHistory };
}

// rememberView keeps the file's own view for the next time it opens.
function rememberView() {
  if (!doc.file || viewFile !== doc.file) return;
  const v = view.split && view.rendered ? "both" : view.split ? "source" : "rendered";
  void App.RememberView(doc.file, v, view.history).catch(() => {});
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

// lostNote says what the running run lost of what blocks set in the
// shells, as the Events tab tells it, for the status line once it ends.
let lostNote = "";

// stopWait is how long a stopped block has to stop before the page asks
// whether to kill its shell.
const stopWait = 3000;
let killTimer = 0;
let askingKill = false;

// stopRun stops the running block: it is interrupted, as ctrl+c would,
// and its shell lives on. A block that hasn't stopped stopWait later,
// such as a loop that ignores ctrl+c, can have its shell killed, if the
// user says so.
async function stopRun() {
  await App.Stop();
  clearTimeout(killTimer);
  killTimer = window.setTimeout(async () => {
    if (!running) return;
    askingKill = true;
    const kill = await confirmShells("The block hasn't stopped", "It was interrupted, as ctrl+c would, but it is still running. Kill its shell to end it? The variables, functions and aliases that blocks set in that shell will be gone.", "Kill Shell");
    askingKill = false;
    if (kill && running) await App.KillShell();
  }, stopWait);
}

// eventsFile is the file whose event log the Events tab shows.
let eventsFile: string | null = null;

// refresh reads the document's state and draws whichever screen fits.
async function refresh() {
  doc = await App.Doc();
  // A file opened in the tab, or a page taking over a tab, shows the
  // file's own view. A session opens with the History pane open, to
  // type in.
  const opened = doc.file !== "" && doc.file !== viewFile;
  // Another file's background terminals aren't this one's.
  if (opened || !doc.file) closeBackgrounds();
  if (!doc.file) viewFile = "";
  else if (opened) fileView();
  else if (doc.openHistory) view = { ...view, history: true };
  wantTerminalSize(doc.termCols, doc.termRows);
  fitHistory();
  const open = doc.file !== "";
  // The Events tab shows this file's log: read it when the file is new
  // to the page.
  if (doc.file !== eventsFile) {
    eventsFile = doc.file;
    void loadEvents();
  }
  // Whether it may reach outside its folder is asked when a file
  // opens or is read again, not on every redraw.
  if (open && doc.file !== outsideFile) {
    outsideFile = doc.file;
    void checkOutside();
  }
  consoleFile(doc.file);
  $("toolbar").hidden = !open;
  $("workspace").hidden = !open;
  $("start").hidden = open;
  syncHistory();
  $("fullenv-text").textContent = doc.envBanner;
  $("fullenv").hidden = !open || doc.envBanner === "";
  if (!open) $("outside").hidden = true;
  showFrontMatterError(open);
  if (!open) {
    $("changed").hidden = true;
    return drawStart();
  }
  const name = showFile(doc.file);
  $("dirty").hidden = !doc.dirty;
  $("changed").hidden = !doc.changed || $("changed").dataset.kept === doc.file;
  drawOptions();
  await openCells();
  await loadFrontMatter();
  if (opened) applyView();
  if (doc.running && !ranHere) await catchUp();
  known = await names();
  await loadSource();
  if (doc.openHistory) focusHistory();
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
    else if (e.name === "sources") onSources(e.data as SourcesQuestion);
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
  $("options").setAttribute("aria-label", "Run options: " + parts.join(", "));
}

// firstHeads is how many cells' heads a file opens with, and
// moreHeads how many each later chunk brings.
const firstHeads = 300;
const moreHeads = 2000;

// layout is the layout of Go's that the cells came from: the bodies of
// the partial ones are asked for by its gen. An edit has Go lay the
// document out again, as opening it does.
let layout: { gen: number; done: boolean } | null = null;
// lastGen is the gen of the layout Go started last. Bodies of an
// earlier one, asked for before it, are of a document since changed.
let lastGen = 0;
// headIndex is each cell's index in the layout.
const headIndex = new WeakMap<Cell, number>();

// openCells draws the document from the heads of its first cells and
// the first screen's bodies, and then takes in the rest as Go lays them
// out, chunk by chunk. A short document comes whole, every card drawn.
async function openCells() {
  const h = await App.Attach(firstHeads);
  const fresh = decodeHeads(h.heads as HeadRow[]);
  fresh.forEach((c, i) => headIndex.set(c, i));
  layout = { gen: h.gen, done: h.done };
  lastGen = h.gen;
  const want = h.done && fresh.length <= lazyCells ? fresh : fresh.slice(0, firstBodies);
  if (!(await takeBodies(want))) return;
  cells = fresh;
  drawDoc();
  if (!h.done) void streamHeads(h.gen, h.step);
}

// firstBodies is how many cells from the top have bodies as a file
// opens: more than a screen holds.
const firstBodies = 60;

// streamHeads takes in the heads of layout gen after those the page
// has, a chunk at a time, while the page goes on, and the fixes to
// heads it has, from step since.
async function streamHeads(gen: number, since: number) {
  while (layout?.gen === gen && !layout.done) {
    let h: Heads;
    try {
      // Each chunk is as long as the document so far: each one has the
      // page index every cell again, so chunks that double keep that to
      // a few times in all.
      h = await App.MoreHeads(gen, cells.length, since, Math.max(moreHeads, cells.length));
    } catch {
      // The document changed; whatever changed it draws it again.
      return;
    }
    if (layout?.gen !== gen) return;
    since = h.step;
    const more = decodeHeads(h.heads as HeadRow[]);
    more.forEach((c, k) => headIndex.set(c, h.at + k));
    fixHeads(h.fixAt ?? [], decodeHeads(h.fixes as HeadRow[]));
    layout.done = h.done;
    appendCells(more);
    // The outline, the Run all button and the dependency lines go
    // through every cell: they are drawn once the cells are all in,
    // rather than after every chunk.
    if (h.done) afterCells();
    // A chunk at a time, so typing and scrolling go on between.
    await new Promise((r) => setTimeout(r));
  }
}

// appendCells adds more cells at the end of the document. Their cards
// are drawn as the view comes to them.
function appendCells(more: Cell[]) {
  for (const c of more) cells.push(c);
  // Once they are all in, a short document has every card drawn.
  if (layout?.done) {
    virtual = cells.length > lazyCells;
    $("doc").classList.toggle("virtual", virtual);
  }
  indexCells();
  placeSoon();
}

// fixHeads takes in the heads of cells Go laid out before and since
// changed, as a heading's section does once the next heading comes.
// A card drawn from a fixed cell is drawn again, from its new body.
function fixHeads(at: number[], heads: Cell[]) {
  const redraw: HTMLElement[] = [];
  at.forEach((i, k) => {
    const c = cells[i];
    if (!c) return;
    takeHead(c, heads[k]);
    headIndex.set(c, i);
    const card = cardOf.get(c);
    if (!card) return;
    if (c.heading) {
      card.dataset.sectionFrom = String(c.sectionFrom ?? 0);
      card.dataset.sectionTo = String(c.sectionTo ?? 0);
    }
    bare.add(card);
    redraw.push(card);
  });
  if (redraw.length) void bodiesFor(redraw);
}

// takeHead has cell c take head h, which Go laid out since c was: c
// keeps what it has, but for what the head says now, and its body is
// asked for again.
function takeHead(c: Cell, h: Cell) {
  for (const key of ["collapsed", "collapsedOutput", "ran", "failed", "stale", "running", "secret", "terminal", "nosave", "sectionFrom", "sectionTo", "movable", "lastLine"] as const) delete c[key];
  Object.assign(c, h);
  partial.add(c);
}

// takeBodies asks Go for the bodies of the partial ones of these cells,
// of layout l, and puts them in, saying whether l is still the latest.
async function takeBodies(list: Cell[], l = layout): Promise<boolean> {
  const want = list.filter((c) => partial.has(c));
  if (!want.length) return !!l && l.gen === lastGen;
  if (!l || l.gen !== lastGen) return false;
  let got: Cell[];
  try {
    got = (await App.Bodies(l.gen, want.map((c) => headIndex.get(c)!))) ?? [];
  } catch {
    return false;
  }
  if (l.gen !== lastGen) return false;
  want.forEach((c, k) => {
    // Go leaves out fields that are false.
    for (const key of ["collapsed", "collapsedOutput", "hidden"] as const) delete c[key];
    Object.assign(c, got[k]);
    partial.delete(c);
  });
  return true;
}

// A Relayout is the cells Go laid out after an edit, and their layout,
// for the page to make its own as it takes them in: every one, fresh,
// or, for a long document, how they changed from those the page has.
interface Relayout {
  fresh?: Cell[];
  splice?: SpliceIn;
  l: { gen: number; done: boolean };
  // moving is the step to ask Go for the move limits from, when they
  // came as the cells had them before.
  moving?: number;
}

// A SpliceIn is a Splice as the page takes it in: cells [at, at+remove)
// are replaced by heads, those after are moved as move says, and fixes
// are the heads of the others that changed, by their new index. drawn
// are the cells Go laid out in the place of the kept cells whose cards
// are drawn, by those cells, for them to take their bodies from.
interface SpliceIn {
  at: number;
  remove: number;
  heads: Cell[];
  move: { line: number; pos: number; block: number };
  fixes: Map<number, Cell>;
  drawn: Map<Cell, Cell>;
}

// relayout has Go lay the document out again, after an edit, and is
// the heads of its cells.
async function relayout(): Promise<Relayout> {
  // Go says how the cells changed from those the page has, when it has
  // them all: a keystroke changes one or two of a long file's cells,
  // and every head of it was megabytes to send and to read.
  const was = layout?.done ? layout.gen : 0;
  const s = await App.Relayout(was);
  lastGen = s.gen;
  const heads = decodeHeads(s.heads as HeadRow[]);
  const l = { gen: s.gen, done: true };
  const moving = s.moving ? s.step : undefined;
  if (s.reset || !was) {
    heads.forEach((c, i) => headIndex.set(c, i));
    return { fresh: heads, l, moving };
  }
  const fixes = new Map<number, Cell>();
  const fixed = decodeHeads(s.fixes as HeadRow[]);
  (s.fixAt ?? []).forEach((i, k) => fixes.set(i, fixed[k]));
  heads.forEach((c, k) => headIndex.set(c, s.at + k));
  const splice: SpliceIn = { at: s.at, remove: s.remove, heads, move: s, fixes, drawn: new Map() };
  // A short document is drawn whole, its cells compared one by one.
  if (cells.length - s.remove + heads.length <= lazyCells) return { fresh: spliced(splice), l, moving };
  return { splice, l, moving };
}

// takeMoves asks Go for the move limits of r's cells, once they are
// known, and takes in the cells whose limits changed, as a block that
// can be dragged now, after r was drawn.
async function takeMoves(r: Relayout) {
  if (r.moving === undefined) return;
  let h: Heads;
  try {
    h = await App.MoreHeads(r.l.gen, cells.length, r.moving, 0);
  } catch {
    // The document changed again; its next layout brings them.
    return;
  }
  if (layout?.gen !== r.l.gen || !h.fixAt?.length) return;
  fixHeads(h.fixAt, decodeHeads(h.fixes as HeadRow[]));
  drawOutline();
}

// changedCells is how many cells r lays out anew.
function changedCells(r: Relayout): number {
  if (r.splice) return Math.max(r.splice.heads.length, r.splice.remove);
  const fresh = r.fresh ?? [];
  const { head, tail } = unchanged(fresh);
  return fresh.length - head - tail;
}

// newIndex is where cell i of the page's goes in sp, or -1 if it goes.
function newIndex(sp: SpliceIn, i: number): number {
  if (i < sp.at) return i;
  if (i < sp.at + sp.remove) return -1;
  return i - sp.remove + sp.heads.length;
}

// headAfter is the head of the cell Go laid out in the place of c, cell
// i of the page's, kept, at j in sp.
function headAfter(sp: SpliceIn, c: Cell, i: number, j: number): Cell {
  const fix = sp.fixes.get(j);
  if (fix) return fix;
  const h = headOf(c);
  if (i >= sp.at) moveHead(h, sp.move);
  return h;
}

// spliced is every cell of sp, the page's cells as heads, spliced.
function spliced(sp: SpliceIn): Cell[] {
  const fresh = new Array<Cell>(cells.length - sp.remove + sp.heads.length);
  for (let i = 0; i < cells.length; i++) {
    const j = newIndex(sp, i);
    if (j >= 0) fresh[j] = headAfter(sp, cells[i], i, j);
  }
  sp.heads.forEach((c, k) => (fresh[sp.at + k] = c));
  fresh.forEach((c, i) => headIndex.set(c, i));
  return fresh;
}

// drawnBodies takes the bodies of the cells of r that are drawn as it
// is taken in: all of them, in a document drawn whole; else those of
// the cells kept whose cards are drawn, and of the cells new in place
// of others, when there are a few, as typing makes.
async function drawnBodies(r: Relayout): Promise<Relayout> {
  const { fresh, splice: sp, l } = r;
  if (sp) {
    const want = sp.heads.length <= manyCards ? [...sp.heads] : [];
    for (const c of cardOf.keys()) {
      const i = cellIndex.get(c);
      const j = i === undefined ? -1 : newIndex(sp, i);
      if (j < 0) continue;
      const f = headAfter(sp, c, i!, j);
      headIndex.set(f, j);
      sp.drawn.set(c, f);
      want.push(f);
    }
    await takeBodies(want, l);
    return r;
  }
  if (!fresh) return r;
  if (fresh.length <= lazyCells) {
    await takeBodies(fresh, l);
    return r;
  }
  const { head, tail } = unchanged(fresh);
  const few = fresh.length - head - tail <= manyCards;
  const want = fresh.filter((_, j) => {
    const inMiddle = j >= head && j < fresh.length - tail;
    if (inMiddle && few) return true;
    // The cell drawn now in j's place: the same one, kept, or at the
    // same index where as many cells came as went.
    const i = !inMiddle ? (j < head ? j : j - fresh.length + cells.length) : fresh.length === cells.length ? j : -1;
    return i >= 0 && cardOf.has(cells[i]);
  });
  await takeBodies(want, l);
  return r;
}

// bodiesFor draws again, from their bodies, cards drawn before their
// bodies came: these cards, or the cards of their cells drawn since.
async function bodiesFor(cards: HTMLElement[]) {
  const list = cards.map((e) => cellOf.get(e)).filter((c): c is Cell => !!c);
  if (!(await takeBodies(list))) return;
  for (const c of list) {
    const card = cardOf.get(c);
    if (card && bare.has(card) && !partial.has(c)) redrawCard(card, c);
  }
  // A body can be taller or shorter than its head guessed.
  placeSoon();
}

// afterCells brings what goes through every cell up to date with them.
function afterCells() {
  runAllButton();
  markSection();
  drawOutline();
  setDeps(cells);
}

function drawDoc() {
  const main = $("doc");
  const at = anchor();
  measureCards();
  closeTerminal();
  behind = false;
  virtual = cells.length > lazyCells || !!(layout && !layout.done);
  main.classList.toggle("virtual", virtual);
  cardOf.clear();
  main.replaceChildren();
  following = null;
  drawn++;
  indexCells();
  place(at);
  runAllButton();
  select(Math.min(selected, blocks().length - 1), false);
  markSection();
  drawOutline();
  sizeTerminal();
  setDeps(cells);
  // Every card is new, which can leave the view blank as a long jump
  // does.
  repaint($("doc-scroll"));
}

// drawn counts the times the document's cards were drawn, all of them
// or some.
let drawn = 0;

// placeFields are the fields of a cell that say where in the file it
// is, which typing above it changes without changing how it is drawn.
const placeFields = ["line", "from", "to", "lastLine", "outputLine", "drop", "dropAfter", "dropBefore"] as const;

// sameDrawn says cells a and b draw the same card: they are alike but
// for their places in the file. A code block's index, and the name and
// id of one not named, which come from its line and index, change with
// every block or line added above it, and so do which blocks a
// heading's section runs and a textbox's or a choice's index. keep
// writes those into the card, rather than every card below an edit
// being drawn again. The cells are compared field by field: turning
// each into JSON, as this once did, took most of the time a pause in
// typing spent on a long document.
function sameDrawn(a: Cell, b: Cell): boolean {
  // A cell made from its head is known by its hash.
  if (partial.has(a) || partial.has(b)) return sameHead(a, b);
  if (renumbered(a) !== renumbered(b) || !!a.heading !== !!b.heading || !!a.sectionTo !== !!b.sectionTo) return false;
  const skip = new Set<string>(placeFields);
  if (renumbered(a)) {
    skip.add("block").add("name");
    // An unnamed block's id is made from its index.
    if ((a.name !== a.id) !== (b.name !== b.id)) return false;
    if (a.name !== a.id) skip.add("id");
  }
  if (a.heading) skip.add("sectionFrom").add("sectionTo");
  if (a.textbox || a.choice) skip.add("textbox").add("choice");
  if (!sameJSON(a, b, skip)) return false;
  if (a.textbox || b.textbox) return !!a.textbox && !!b.textbox && sameJSON(a.textbox, b.textbox, inputSkip);
  if (a.choice || b.choice) return !!a.choice && !!b.choice && sameJSON(a.choice, b.choice, inputSkip);
  return true;
}

const inputSkip = new Set(["block"]);

// sameJSON says a and b, as Go sent them, are alike, but for the keys
// in skip at the top. A key that is missing is alike one that is
// undefined, as JSON leaves both out.
function sameJSON(a: unknown, b: unknown, skip?: Set<string>): boolean {
  if (a === b) return true;
  if (typeof a !== "object" || typeof b !== "object" || a === null || b === null) return false;
  if (Array.isArray(a) || Array.isArray(b)) {
    if (!Array.isArray(a) || !Array.isArray(b) || a.length !== b.length) return false;
    return a.every((x, i) => sameJSON(x, b[i]));
  }
  const x = a as Record<string, unknown>;
  const y = b as Record<string, unknown>;
  for (const k in x) if (!skip?.has(k) && !sameJSON(x[k], y[k])) return false;
  for (const k in y) if (!skip?.has(k) && !(k in x) && y[k] !== undefined) return false;
  return true;
}

// renumbered says c's card is a code block's, whose index and name keep
// can change.
const renumbered = (c: Cell) => c.block >= 0 && !c.textbox && !c.choice;

// updateDoc draws the cells Go laid out after an edit in the source
// pane, r, drawing again only the cards whose cells changed. Typing
// changes a cell or two, and drawing every card of a long document
// again, as drawDoc does, took long enough to stall the typing. The
// cards before and after the cells that changed are kept, with their
// places in the file brought up to date. Their handlers hold their
// cells, so those are updated in place.
function updateDoc(r: Relayout) {
  layout = r.l;
  behind = false;
  queueMicrotask(() => void takeMoves(r));
  const at = anchor();
  const { next, middle, gone } = r.splice ? takeSplice(r.splice) : takeFresh(r.fresh ?? []);
  drawn++;
  if (!gone.length && !middle.length) {
    cells = next;
    bareBodies();
    return;
  }
  measureCards();
  for (const c of gone) {
    const card = cardOf.get(c);
    if (!card) continue;
    if (card === following) following = null;
    card.remove();
    cardOf.delete(c);
  }
  cells = next;
  virtual = cells.length > lazyCells;
  $("doc").classList.toggle("virtual", virtual);
  indexCells();
  bareBodies();
  // A few new cards, as typing makes, are drawn as typing's; many, as
  // a fence opened above them makes, are drawn as the view's, like
  // those that scrolling brings into it.
  place(at, middle.length <= manyCards ? new Set(middle) : undefined);
  runAllButton();
  select(Math.min(selected, blocks().length - 1), false);
  markSection();
  drawOutline();
  setDeps(cells);
  // Many new cards can leave the view blank, as drawing every one can.
  if (middle.length > 20) repaint($("doc-scroll"));
}

// takeFresh lines fresh, every cell Go laid out, up with the cells the
// page has, keeping those alike at the head and the tail. It is the
// cells as they are now, those new among them, and those gone.
function takeFresh(fresh: Cell[]): { next: Cell[]; middle: Cell[]; gone: Cell[] } {
  const { head, tail } = unchanged(fresh);
  const middle = fresh.slice(head, fresh.length - tail);
  const next: Cell[] = [];
  for (let i = 0; i < head; i++) next.push(keepCell(cells[i], fresh[i], i));
  for (const c of middle) next.push(c);
  for (let k = tail; k > 0; k--) next.push(keepCell(cells[cells.length - k], fresh[fresh.length - k], fresh.length - k));
  return { next, middle, gone: cells.slice(head, cells.length - tail) };
}

// takeSplice is takeFresh for a splice: the cells kept are moved where
// they are, rather than each compared with a copy, which for a long
// document took most of the time a keystroke spent on the page. Those
// out of sight take their bodies again when drawn, as what a block's
// body says of where it can be dragged can change with any edit.
function takeSplice(sp: SpliceIn): { next: Cell[]; middle: Cell[]; gone: Cell[] } {
  const next = new Array<Cell>(cells.length - sp.remove + sp.heads.length);
  for (let i = 0; i < cells.length; i++) {
    const j = newIndex(sp, i);
    if (j < 0) continue;
    const c = cells[i];
    next[j] = c;
    if (cardOf.has(c)) {
      keepCell(c, sp.drawn.get(c) ?? headAfter(sp, c, i, j), j);
      continue;
    }
    if (i >= sp.at) {
      moveHead(c, sp.move);
      if (c.outputLine) c.outputLine += sp.move.line;
      headIndex.set(c, j);
    }
    const fix = sp.fixes.get(j);
    if (fix) takeHead(c, fix);
    else partial.add(c);
  }
  sp.heads.forEach((c, k) => (next[sp.at + k] = c));
  return { next, middle: sp.heads, gone: cells.slice(sp.at, sp.at + sp.remove) };
}

// keepCell has cell c, drawn now, take f, the cell Go laid out in its
// place, j, after an edit, writing what changed into its card.
function keepCell(c: Cell, f: Cell, j: number): Cell {
  const card = cardOf.get(c);
  headIndex.set(c, j);
  if (partial.has(c) && !partial.has(f)) {
    // Its head was all it had: it takes the rest from f. What says
    // where it is, and its numbers, stay as drawn, for the lines
    // below to bring up to date.
    const input = (c.textbox ?? c.choice)?.block;
    const was = { block: c.block, name: c.name, id: c.id, sectionFrom: c.sectionFrom, sectionTo: c.sectionTo, ...Object.fromEntries(placeFields.map((k) => [k, c[k]])) };
    for (const key of ["collapsed", "collapsedOutput", "hidden"] as const) delete c[key];
    Object.assign(c, f, was, { textbox: f.textbox && { ...f.textbox }, choice: f.choice && { ...f.choice } });
    const box = c.textbox ?? c.choice;
    if (box && input !== undefined) box.block = input;
    partial.delete(c);
  }
  if (renumbered(c) && (c.block !== f.block || c.name !== f.name || c.id !== f.id)) {
    if (card) {
      card.id = "block-" + f.block;
      card.dataset.block = String(f.block);
      if (c.secret) card.classList.toggle("masked", !revealed.has(f.block));
      const id = card.querySelector(":scope > .head .id");
      if (id) id.textContent = f.name ?? f.id ?? "";
    }
    c.block = f.block;
    c.name = f.name;
    c.id = f.id;
    if (card) labelBlock(card);
  }
  const input = c.textbox ?? c.choice;
  const to = (f.textbox ?? f.choice)?.block;
  if (input && to !== undefined && input.block !== to) {
    if (card) renumberInput(card, input.block, to);
    input.block = to;
  }
  if (c.heading && f.sectionTo && (c.sectionFrom !== f.sectionFrom || c.sectionTo !== f.sectionTo)) {
    if (card) {
      card.dataset.sectionFrom = String(f.sectionFrom ?? 0);
      card.dataset.sectionTo = String(f.sectionTo);
    }
    c.sectionFrom = f.sectionFrom;
    c.sectionTo = f.sectionTo;
  }
  if (placeFields.every((k) => c[k] === f[k])) return keepHead(c, f, card);
  // Only what changed is written: writing every card's place, though
  // the same, made the page work out every card's style again. It is
  // told from the cell, as reading the card's took longer.
  const set = (key: string, was: string | undefined, v: string | undefined) => {
    if (was === v || !card) return;
    if (v === undefined) delete card.dataset[key];
    else card.dataset[key] = v;
  };
  set("line", String(c.line), String(f.line));
  set("from", String(c.from), String(f.from));
  set("drop", c.drop ? "1" : undefined, f.drop ? "1" : undefined);
  set("dropAfter", c.dropAfter >= 0 ? String(c.dropAfter) : undefined, f.dropAfter >= 0 ? String(f.dropAfter) : undefined);
  set("dropBefore", c.dropBefore >= 0 ? String(c.dropBefore) : undefined, f.dropBefore >= 0 ? String(f.dropBefore) : undefined);
  Object.assign(c, Object.fromEntries(placeFields.map((k) => [k, f[k]])));
  return keepHead(c, f, card);
}

// keepHead has a cell kept whose body didn't come, as one out of sight,
// take its head, and its body when it is drawn: what is drawn of a
// block, as whether it can be dragged, can change with cells far from
// it.
function keepHead(c: Cell, f: Cell, card: HTMLElement | undefined): Cell {
  if (!partial.has(f)) return c;
  takeHead(c, f);
  if (card) bare.add(card);
  return c;
}

// bareBodies draws again the cards drawn from heads, from their cells'
// bodies: those that came with the cells, or else asked for now.
function bareBodies() {
  const want: HTMLElement[] = [];
  for (const card of [...cardOf.values()].filter((e) => bare.has(e))) {
    const c = cellOf.get(card);
    if (c && partial.has(c)) want.push(card);
    else if (c) redrawCard(card, c);
  }
  if (want.length) void bodiesFor(want);
}

// renumberInput writes a textbox's or a choice's new block index, to,
// into its card, wherever drawTextbox and drawChoice wrote it.
function renumberInput(card: HTMLElement, from: number, to: number) {
  for (const e of card.querySelectorAll<HTMLElement>("[data-input]")) e.dataset.input = String(to);
  for (const e of card.querySelectorAll<HTMLInputElement>(`input[name="choice-${from}"]`)) e.name = "choice-" + to;
  const problem = card.querySelector<HTMLElement>(`#problem-${from}`);
  if (problem) {
    problem.id = "problem-" + to;
    for (const e of card.querySelectorAll(`[aria-describedby="problem-${from}"]`)) e.setAttribute("aria-describedby", problem.id);
  }
}

// unchanged is how many cells at the head and the tail of fresh are
// drawn as the cells drawn now are.
function unchanged(fresh: Cell[]): { head: number; tail: number } {
  const was = cells;
  let head = 0;
  while (head < was.length && head < fresh.length && sameDrawn(was[head], fresh[head])) head++;
  let tail = 0;
  while (tail < was.length - head && tail < fresh.length - head && sameDrawn(was[was.length - 1 - tail], fresh[fresh.length - 1 - tail])) tail++;
  return { head, tail };
}

// A Place is where a cell is in the rendered view: its index, and how
// far its top is below the top of the view. follow marks the block being
// followed.
interface Place {
  i: number;
  offset: number;
  follow?: boolean;
}

// anchor is where the block being followed is in the rendered view, or
// else the first cell in sight, for place to put back where it was once
// the cards around it are drawn anew. Drawing the document again
// replaces every card, and the view would otherwise jump, as far as the
// top.
function anchor(): Place | null {
  const pane = $("doc-scroll");
  if (pane.hidden || !cells.length) return null;
  const f = following?.isConnected ? cellOf.get(following) : undefined;
  const fi = f && cellIndex.get(f);
  if (following && fi !== undefined) return { i: fi, offset: following.getBoundingClientRect().top - pane.getBoundingClientRect().top, follow: true };
  const i = indexAt(pane.scrollTop);
  return i < 0 ? null : { i, offset: topOf(i) - pane.scrollTop };
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
  if (Math.abs(d) > 1) jump(d, scrollBehavior(), (behavior) => pane.scrollTo({ top: pane.scrollTop + d, behavior }));
}

// jump scrolls the rendered view by about distance with move. WebKit on
// macOS can leave the view blank after a long jump, until it is scrolled
// again by hand, so a jump further than the view is tall goes at once,
// not smoothly, and the view is then painted again.
function jump(distance: number, behavior: ScrollBehavior, move: (behavior: ScrollBehavior) => void) {
  const pane = $("doc-scroll");
  const far = Math.abs(distance) > pane.clientHeight;
  move(far ? "auto" : behavior);
  if (far) repaint(pane);
}

// repaint has WebKit paint the rendered view afresh, as scrolling it a
// little by hand does: once the jump has been drawn, the view scrolls a
// pixel and back, and its content gets a layer of its own for that
// frame, which draws all of it again. nudging keeps the source pane
// from following the pixel.
let repaintFrame = 0;
let nudging = false;
function repaint(pane: HTMLElement) {
  cancelAnimationFrame(repaintFrame);
  repaintFrame = requestAnimationFrame(() => {
    const top = pane.scrollTop;
    const by = top > 0 ? -1 : 1;
    nudging = true;
    pane.classList.add("repaint");
    pane.scrollTop = top + by;
    repaintFrame = requestAnimationFrame(() => {
      pane.classList.remove("repaint");
      if (Math.abs(pane.scrollTop - (top + by)) < 1) pane.scrollTop = top;
      repaintFrame = requestAnimationFrame(() => {
        repaintFrame = 0;
        nudging = false;
      });
    });
  });
}

// scrolledByHand says the rendered view was scrolled by hand since the
// run started, so the run doesn't scroll back when it's over.
let scrolledByHand = false;

function stopFollowing() {
  following = null;
  scrolledByHand = true;
  handScrollAt = Date.now();
}

// handScrollAt is when the rendered view was last scrolled by hand.
let handScrollAt = 0;
// handScrollQuiet is how long after scrolling by hand the user counts as
// still scrolling, in milliseconds.
const handScrollQuiet = 1000;
let askTimer = 0;

// centerAsk puts a block's question in the middle of the rendered view.
// The question, not its card, is centered: the card holds the output of
// the block's last run, which can be far taller than the view, and the
// card's middle would leave the question out of sight. While the view is
// being scrolled by hand it waits for the scrolling to stop, so the view
// isn't pulled from under the user.
function centerAsk(ask: HTMLElement, behavior: ScrollBehavior = "auto") {
  clearTimeout(askTimer);
  const wait = handScrollAt + handScrollQuiet - Date.now();
  if (wait > 0) {
    askTimer = window.setTimeout(() => centerAsk(ask, behavior), wait);
    return;
  }
  const pane = $("doc-scroll");
  if (!ask.isConnected || ask.hidden || pane.hidden) return;
  const p = pane.getBoundingClientRect();
  const r = ask.getBoundingClientRect();
  jump(r.top + r.height / 2 - (p.top + p.height / 2), behavior, (behavior) => ask.scrollIntoView({ block: "center", behavior }));
}

function drawCell(c: Cell): HTMLElement {
  const e = c.textbox ? drawTextbox(c) : c.choice ? drawChoice(c) : c.block < 0 ? drawProse(c) : drawBlock(c);
  cellOf.set(e, c);
  e.dataset.line = String(c.line);
  e.dataset.from = String(c.from);
  if (c.drop) e.dataset.drop = "1";
  if (c.dropAfter >= 0) e.dataset.dropAfter = String(c.dropAfter);
  if (c.dropBefore >= 0) e.dataset.dropBefore = String(c.dropBefore);
  if (c.block >= 0 && !c.textbox && !c.choice) e.style.setProperty("--guess", guessHeight(c) + "px");
  if (c.movable) {
    // The grip goes first in a block's head, or on the line of an
    // input's name, before it.
    const g = grip("Drag to move this block (alt+↑/↓)", (ev, handle) => startDrag(ev, handle, e, c.from, 0, "doc"));
    const head = e.querySelector(".head");
    const name = e.querySelector(":scope.input-cell > .name");
    if (head) head.prepend(g);
    else if (name) {
      const line = el("div", { className: "name-line" });
      name.before(line);
      line.append(g, name);
    } else {
      // A fence that doesn't run has no head; its grip sits in the
      // margin beside it.
      e.classList.add("fence");
      e.prepend(g);
    }
  }
  return e;
}

// The rendered view of a long document draws only the cards near what is
// in sight: a screen and a half above it and below it, between spacers
// as tall as the cards they stand for, so the page holds a few hundred
// cards however long the document is. Every card drawn, a long
// document made a million or more elements, and WebKit then took
// seconds over each key typed and ran short of memory. As the view
// scrolls, place draws the cards coming near it and takes away those
// that went far from it, but for cards in use: selected, followed,
// focused, running, asking or being dragged.
//
// Each cell has a height in heights: a card's as last drawn, kept by the
// hash of its cell so a card drawn again, or one alike, starts at its
// size, or else a guess from its head. Selecting, folding, the outline,
// the dependency lines and the source pane's scrolling go by the cells
// and their heights, not the cards.

// lazyCells is the most cells a document has with every card drawn.
// ?lazy= in the page's address changes it, for the stress test.
const lazyCells = Number(new URLSearchParams(location.search).get("lazy") ?? 400);

// virtual is set while the view draws only the cards near the view.
let virtual = false;

// maxCards is the most cards place draws around the view.
const maxCards = 300;

// cellOf is each card's cell, and cardOf each drawn cell's card; keep and
// edit update the cell in place.
const cellOf = new WeakMap<HTMLElement, Cell>();
const cardOf = new Map<Cell, HTMLElement>();

// cellIndex is each cell's index in cells, blockCells each block's cell
// by its index, and foldKeys each heading's fold's name.
let cellIndex = new Map<Cell, number>();
let blockCells = new Map<number, Cell>();
const foldKeys = new WeakMap<Cell, string>();

// hidden marks the cells a folded section hides, by index, hiddenBy is
// the index of the heading whose fold hides each, and hides is how many
// a folded heading hides.
let hidden = new Uint8Array(0);
let hiddenBy = new Int32Array(0);
const hides = new WeakMap<Cell, number>();

// heights is how tall each cell's card is, by index, and 0 for a hidden
// one; measured is the height of cards drawn, by their cells' hashes.
let heights = new Heights();
const measured = new Map<string, number>();

// spacerRange is the cells each spacer stands for, from up to to.
const spacerRange = new WeakMap<HTMLElement, { from: number; to: number }>();

// placed are the cards place drew for the view, rather than an edit,
// which the stress test leaves out of the cards typing drew.
const placed = new WeakSet<HTMLElement>();
// The stress test (gui.cjs) reads these: the cards placed, the line each
// cell starts on, and whether cells or bodies are still coming.
Object.assign(window, { mdshPlaced: placed, mdshLines: () => cells.map((c) => c.line), mdshLoading: () => !!(layout && !layout.done) || [...cardOf.values()].some((e) => bare.has(e)) });

// indexCells works out, for cells, what goes through all of them: their
// indices, the folds' names and what they hide, and the heights.
function indexCells() {
  cellIndex = new Map();
  blockCells = new Map();
  headingByKey = new Map();
  const seen = new Map<string, number>();
  cells.forEach((c, i) => {
    cellIndex.set(c, i);
    if (renumbered(c)) blockCells.set(c.block, c);
    if (!c.heading) return;
    // A heading is known by its level and text, and which one it is of
    // those alike, so its fold outlives edits elsewhere.
    const name = c.heading + ":" + (c.title ?? "");
    const nth = seen.get(name) ?? 0;
    seen.set(name, nth + 1);
    const key = "section:" + name + ":" + nth;
    foldKeys.set(c, key);
    headingByKey.set(key, c);
  });
  foldCells();
}

// foldCells works out which cells the folded sections hide, and the
// cells' heights, and marks the cards drawn.
function foldCells() {
  hidden = new Uint8Array(cells.length);
  hiddenBy = new Int32Array(cells.length);
  const open: { level: number; folded: boolean; cell: Cell; at: number; hidden: number }[] = [];
  const headings: typeof open = [];
  cells.forEach((c, i) => {
    const level = c.heading ?? 0;
    if (level) while (open.length && open[open.length - 1].level >= level) open.pop();
    const owner = open.find((h) => h.folded);
    if (owner) {
      hidden[i] = 1;
      hiddenBy[i] = owner.at;
      owner.hidden++;
    }
    if (level) {
      const h = { level, folded: shownFolded(c), cell: c, at: i, hidden: 0 };
      open.push(h);
      headings.push(h);
    }
  });
  for (const h of headings) hides.set(h.cell, h.folded ? h.hidden : 0);
  heights = new Heights(cells.map((c, i) => (hidden[i] ? 0 : (measured.get(c.hash ?? "") ?? guessPitch(c)))));
  shownBlocks = null;
  for (const [c, card] of cardOf) decorate(card, c);
}

// guessPitch is about how far down the page cell c's card takes the
// next one, before it is drawn.
function guessPitch(c: Cell): number {
  if (c.textbox || c.choice) return 96;
  if (c.heading) return [0, 64, 56, 48, 44, 40, 40][c.heading] ?? 40;
  if (c.block >= 0) return guessHeight(c) + 18;
  // About a line of prose for every 90 bytes of markdown.
  return 16 + 24 * Math.max(1, Math.ceil((c.to - c.from) / 90));
}

// guessHeight is about how tall block c's card is, in pixels, inside its
// border, which content-visibility takes it to be until it is first in
// sight.
function guessHeight(c: Cell): number {
  // A body or output ends with a newline, which starts no line.
  const lines = (s?: string) => (s ? s.replace(/\r?\n$/, "").split("\n").length : 0);
  // A cell whose body hasn't come has its lines counted by Go.
  const out = partial.has(c) ? (c.outputLines ?? 0) : lines(c.output);
  const code = partial.has(c) ? (c.codeLines ?? 0) : lines(c.body);
  return 60 + 19 * code + (out ? 30 + 19 * out : 0);
}

// decorate marks card, c's, with what the page knows of c beyond its
// cell: whether a fold hides it, a heading's fold, a block's folded code
// and output, and the selection and the source pane's cursor.
function decorate(card: HTMLElement, c: Cell) {
  const i = cellIndex.get(c);
  card.classList.toggle("in-fold", i !== undefined && !!hidden[i]);
  card.classList.toggle("current", c === currentCell);
  if (c.heading) {
    const key = foldKeys.get(c) ?? "";
    const folded = shownFolded(c);
    card.dataset.fold = key;
    card.classList.toggle("folded", folded);
    card.classList.toggle("selected", !!key && key === selectedSection);
    card.querySelector(":scope > button.fold")?.setAttribute("aria-expanded", String(!folded));
    const note = card.querySelector<HTMLElement>(".fold-note");
    if (note) {
      const n = folded ? (hides.get(c) ?? 0) : 0;
      note.textContent = n ? "… " + n + (n === 1 ? " part" : " parts") : "";
      note.setAttribute("aria-label", note.textContent ? note.textContent.slice(2) + " folded away: unfold this section" : "");
    }
  }
  if (renumbered(c)) {
    card.classList.toggle("selected", c === blocks()[selected]);
    drawPart(card, "code");
    drawPart(card, "output");
  }
}

let placeFrame = 0;
function placeSoon() {
  if (!placeFrame) placeFrame = requestAnimationFrame(() => place());
}

// place draws the cards near the view, at, or where the view is, and
// takes away those far from it, then puts at back where it was. With
// eager, the cards of those cells are drawn as an edit's.
function place(at = anchor(), eager?: Set<Cell>) {
  cancelAnimationFrame(placeFrame);
  placeFrame = 0;
  const pane = $("doc-scroll");
  measureCards();
  const want: number[] = [];
  if (!virtual) for (let i = 0; i < cells.length; i++) want.push(i);
  else if (cells.length) {
    const view = pane.clientHeight || window.innerHeight;
    const i0 = at ? Math.min(at.i, cells.length - 1) : 0;
    // The view's top, as the heights have it.
    const y0 = heights.sum(i0) - (at?.offset ?? 0);
    const lo = heights.find(y0 - 1.5 * view);
    const hi = heights.find(y0 + 2.5 * view);
    // Down from the cell in sight first, so a run of cells too short to
    // count leaves the view drawn.
    for (let i = i0; i <= hi && want.length < maxCards; i++) if (!hidden[i]) want.push(i);
    for (let i = i0 - 1; i >= lo && want.length < maxCards; i--) if (!hidden[i]) want.push(i);
    const keep = new Set(want);
    for (const [c, card] of cardOf) {
      const i = cellIndex.get(c);
      if (i !== undefined && !keep.has(i) && inUse(card)) want.push(i);
    }
    want.sort((a, b) => a - b);
  }
  // The dependency lines go to the cards drawn.
  if (reconcile(want, eager)) drawDeps();
  if (!at || pane.hidden) return;
  if (at.follow) {
    const c = cells[at.i];
    following = (c && renumbered(c) && cardOf.get(c)) || null;
  }
  const top = topOf(at.i) - at.offset;
  if (Math.abs(pane.scrollTop - top) >= 1) pane.scrollTop = top;
}

// reconcile makes the cards in the page those of the cells at want, in
// order, between spacers for the rest. The cards kept stay where they
// are in the page, keeping the focus and what is drawn in them.
function reconcile(want: number[], eager?: Set<Cell>): boolean {
  const main = $("doc");
  const keep = new Set(want);
  let changed = false;
  for (const [c, card] of cardOf) {
    const i = cellIndex.get(c);
    if (i !== undefined && keep.has(i)) continue;
    card.remove();
    cardOf.delete(c);
    changed = true;
  }
  let next = main.firstElementChild as HTMLElement | null;
  const skip = () => {
    const e = next!;
    next = e.nextElementSibling as HTMLElement | null;
    return e;
  };
  const spacer = (from: number, to: number) => {
    const s = next && spacerRange.has(next) ? skip() : main.insertBefore(el("div", { className: "spacer" }), next);
    if (setSpacer(s, from, to)) changed = true;
  };
  let from = 0;
  for (const i of want) {
    const c = cells[i];
    const card = cardOf.get(c);
    if (i > from) spacer(from, i);
    // Spacers no longer needed go.
    while (next && next !== card && spacerRange.has(next)) skip().remove();
    if (card && card === next) skip();
    else {
      main.insertBefore(card ?? drawCard(c, !eager?.has(c)), next);
      changed = true;
    }
    from = i + 1;
  }
  if (from < cells.length) spacer(from, cells.length);
  while (next) {
    skip().remove();
    changed = true;
  }
  return changed;
}

// setSpacer has spacer s stand for the cells from up to to, saying
// whether its height changed.
function setSpacer(s: HTMLElement, from: number, to: number): boolean {
  spacerRange.set(s, { from, to });
  const h = Math.round(heights.sum(from, to)) + "px";
  if (s.style.height === h) return false;
  s.style.height = h;
  return true;
}

// measureCards notes the heights of the cards drawn: how far down each
// takes the next, which counts the margin between them.
function measureCards() {
  if (!virtual) return;
  const kids = [...($("doc").children as HTMLCollectionOf<HTMLElement>)].filter((e) => e.offsetParent !== null);
  kids.forEach((e, k) => {
    const c = cellOf.get(e);
    if (!c || partial.has(c)) return;
    const i = cellIndex.get(c);
    if (i === undefined || hidden[i]) return;
    const n = kids[k + 1];
    const h = n && cellOf.has(n) ? n.offsetTop - e.offsetTop : e.offsetHeight + 16;
    if (h <= 0) return;
    heights.set(i, h);
    if (c.hash) measured.set(c.hash, h);
  });
}

// indexAt is the index of the cell shown at y, in the rendered view's
// content: the first whose bottom is below it, or the last.
function indexAt(y: number): number {
  let last = -1;
  for (const e of $("doc").children as HTMLCollectionOf<HTMLElement>) {
    if (e.offsetParent === null) continue;
    const r = spacerRange.get(e);
    if (r) {
      if (r.to <= r.from) continue;
      const top = e.offsetTop;
      if (top + e.offsetHeight <= y) {
        last = r.to - 1;
        continue;
      }
      const i = heights.find(heights.sum(r.from) + Math.max(0, y - top));
      return Math.max(r.from, Math.min(r.to - 1, i));
    }
    const i = cellIndex.get(cellOf.get(e)!) ?? -1;
    if (e.offsetTop + e.offsetHeight > y) return i;
    last = i;
  }
  return last;
}

// topOf is where cell i's top is, in the rendered view's content; a
// hidden cell's is the top of what follows it.
function topOf(i: number): number {
  const main = $("doc");
  for (const e of main.children as HTMLCollectionOf<HTMLElement>) {
    if (e.offsetParent === null) continue;
    const r = spacerRange.get(e);
    if (r) {
      if (r.to > i) return e.offsetTop + heights.sum(r.from, Math.max(i, r.from));
      continue;
    }
    if ((cellIndex.get(cellOf.get(e)!) ?? -1) >= i) return e.offsetTop;
  }
  return main.offsetTop + main.offsetHeight;
}

// scrollToCell brings cell i to offset below the top of the rendered
// view, drawing the cards around it.
function scrollToCell(i: number, offset = 8) {
  if (i < 0 || i >= cells.length) return;
  place({ i, offset });
}

// inUse says card can't be taken away: it is selected, followed,
// focused, running, asking or being dragged.
function inUse(card: HTMLElement): boolean {
  return card === following || card.matches(".selected, .current, .now, .background, .timing, .dep-target, .dragging") || card.contains(document.activeElement) || !!card.querySelector(".ask:not([hidden]), .terminal, .cast-player");
}

// bare are the cards drawn from their cells' heads alone, before their
// bodies came.
const bare = new WeakSet<HTMLElement>();

// drawCard draws c's card, as the view's when mark is set.
function drawCard(c: Cell, mark = true): HTMLElement {
  const e = drawCell(c);
  cardOf.set(c, e);
  if (partial.has(c)) bare.add(e);
  if (running) for (const b of e.querySelectorAll<HTMLButtonElement>(".head button:not(.stop-bg)")) b.disabled = true;
  decorate(e, c);
  if (mark) placed.add(e);
  // A cell whose body hasn't come is drawn without it, and again when
  // it comes.
  if (partial.has(c)) wantBody(e);
  return e;
}

// cardFor is c's card, drawn where it goes if it isn't, as a block that
// starts running or asks is. It stays while it is in use.
function cardFor(c: Cell): HTMLElement | null {
  const have = cardOf.get(c);
  if (have) return have;
  const i = cellIndex.get(c);
  if (i === undefined) return null;
  const card = drawCard(c);
  const main = $("doc");
  for (const e of main.children as HTMLCollectionOf<HTMLElement>) {
    const r = spacerRange.get(e);
    if (r && r.from <= i && i < r.to) {
      const after = el("div", { className: "spacer" });
      e.after(card, after);
      setSpacer(e, r.from, i);
      setSpacer(after, i + 1, r.to);
      return card;
    }
    const at = r ? r.from : (cellIndex.get(cellOf.get(e)!) ?? -1);
    if (at > i) {
      e.before(card);
      return card;
    }
  }
  main.append(card);
  return card;
}

// redrawCard draws c's card again in card's place, keeping the marks
// put on it since.
function redrawCard(card: HTMLElement, c: Cell): HTMLElement {
  const e = drawCell(c);
  for (const k in card.dataset) e.dataset[k] = card.dataset[k];
  for (const k of card.classList) e.classList.add(k);
  const rings = card.style.getPropertyValue("--dep-rings");
  if (rings) e.style.setProperty("--dep-rings", rings);
  if (running) for (const b of e.querySelectorAll<HTMLButtonElement>(".head button:not(.stop-bg)")) b.disabled = true;
  card.replaceWith(e);
  cardOf.set(c, e);
  if (partial.has(c)) bare.add(e);
  if (following === card) following = e;
  if (placed.has(card)) placed.add(e);
  decorate(e, c);
  return e;
}

// wanted are cards drawn without their cells' bodies, which are asked
// for together, once the page is done drawing.
let wanted: HTMLElement[] = [];
function wantBody(card: HTMLElement) {
  if (!wanted.length) {
    queueMicrotask(() => {
      const cards = wanted;
      wanted = [];
      void bodiesFor(cards);
    });
  }
  wanted.push(card);
}

// blockCard is block i's card, drawn.
function blockCard(i: number): HTMLElement | null {
  const c = blockCells.get(i);
  return c ? cardFor(c) : null;
}

// cardName is a block card's name, as its head shows it.
function cardName(e: HTMLElement): string | undefined {
  return e.querySelector(".name")?.firstChild?.textContent ?? undefined;
}

// shownCard is block's card, or the card of the heading whose fold
// hides it, where it is drawn, for the dependency lines.
function shownCard(block: number): HTMLElement | null {
  const c = blockCells.get(block);
  const i = c ? cellIndex.get(c) : undefined;
  if (!c || i === undefined) return null;
  return cardOf.get(hidden[i] ? cells[hiddenBy[i]] : c) ?? null;
}

// placesNow is where each block's card is, or would be, in the rendered
// view's content, as it is laid out now: the top and the bottom of the
// card, or of the heading whose fold hides it. The dependency lines run
// to blocks whose cards aren't drawn.
function placesNow(): (block: number) => { top: number; bottom: number; at: number } | null {
  const segs: { from: number; to: number; top: number }[] = [];
  for (const e of $("doc").children as HTMLCollectionOf<HTMLElement>) {
    if (e.offsetParent === null) continue;
    const r = spacerRange.get(e);
    if (r) segs.push({ ...r, top: e.offsetTop });
    else {
      const i = cellIndex.get(cellOf.get(e)!);
      if (i !== undefined) segs.push({ from: i, to: i + 1, top: e.offsetTop });
    }
  }
  const topAt = (i: number) => {
    // The last stretch starting at or before i.
    let lo = 0;
    let hi = segs.length;
    while (lo < hi) {
      const mid = (lo + hi) >> 1;
      if (segs[mid].from > i) hi = mid;
      else lo = mid + 1;
    }
    const s = segs[lo - 1];
    if (s && i < s.to) return s.top + heights.sum(s.from, i);
    return segs[lo]?.top ?? (s ? s.top + heights.sum(s.from, s.to) : 0);
  };
  return (block) => {
    const c = blockCells.get(block);
    let i = c ? cellIndex.get(c) : undefined;
    if (i === undefined) return null;
    if (hidden[i]) i = hiddenBy[i];
    const top = topAt(i);
    return { top, bottom: top + Math.max(20, heights.get(i) - 16), at: i };
  };
}

// cellName is block c's name, as its card's head shows it.
const cellName = (c: Cell) => c.name ?? c.id ?? "";

// drawProse draws one top-level block of prose. Clicking it shows its
// markdown in the source pane, except on a link, which followLinks
// follows.
function drawProse(c: Cell): HTMLElement {
  // goldmark's HTML, with raw HTML in the markdown cleaned (rawhtml).
  const s = el("section", { className: "prose" + (c.hidden ? " hidden-block" : ""), innerHTML: c.html ?? "", title: "Click to show it in the source" });
  if (c.hidden) s.prepend(hiddenBadge(c.hidden));
  highlightProse(s);
  // An include's lines are numbered as they are in its file.
  const code = c.firstLine ? s.querySelector<HTMLElement>("pre.numbered > code") : null;
  if (code) code.style.counterReset = `line ${c.firstLine! - 1}`;
  enableDiagrams(s);
  enableRemoteImages(s);
  enableCasts(s, { save: (n, body) => void saveFrame(-1, body, n) });
  enableCopy(s);
  if (c.include) includeControls(s, c, { commit: (i, from, to) => edit(() => App.SetIncludeRange(i, from, to)), busy: () => running, say: status });
  s.onclick = (e) => {
    if (window.getSelection()?.toString() || (e.target as HTMLElement).closest("input, .include-grip, .include-bar")) return;
    if (c.heading) selectSection(c);
    void showInSource(s, c.line, true);
  };
  enableTasks(s, c.tasks);
  if (c.heading) {
    s.classList.add("heading-cell");
    s.dataset.level = String(c.heading);
    if (c.sectionTo) {
      s.dataset.sectionFrom = String(c.sectionFrom ?? 0);
      s.dataset.sectionTo = String(c.sectionTo);
      const play = iconButton("play", "Run this section (s)", () => void runSection(c));
      play.classList.add("run-section");
      play.disabled = running || !!noRun;
      // Next to the heading's text, which names it in the outline and
      // its fold, and a button's svg adds none. The heading keeps its
      // text as its name, without the button's.
      const h = s.querySelector("h1, h2, h3, h4, h5, h6");
      if (h) h.setAttribute("aria-label", h.textContent ?? "");
      (h ?? s).append(play);
    }
    const note = el("button", { className: "fold-note", title: "Unfold this section" });
    note.onclick = (e) => {
      e.stopPropagation();
      foldSection(c, false);
    };
    s.prepend(foldButton("Fold or unfold this section", () => foldSection(c)));
    s.append(note);
  }
  return s;
}

// enableCopy gives each fenced code block in prose a button that copies
// its code to the clipboard. The button sits in a wrapper, so it stays
// put while a wide block scrolls. A diagram's corner holds its own button.
function enableCopy(e: HTMLElement) {
  for (const pre of e.querySelectorAll<HTMLElement>("pre")) {
    if (pre.closest("figure.diagram")) continue;
    const b = iconButton("copy", "Copy this code", () => void copyCode(preText(pre), ""));
    b.classList.add("copy-code");
    const wrap = el("div", { className: "copyable" });
    pre.replaceWith(wrap);
    wrap.append(pre, b);
  }
}

// preText is the code in pre, a line per .line where it is numbered.
function preText(pre: HTMLElement): string {
  const lines = pre.querySelectorAll(".line");
  if (lines.length) return [...lines].map((l) => l.textContent ?? "").join("\n");
  return pre.querySelector("code")?.textContent ?? "";
}

// copyCode puts a fenced block's code on the clipboard, as written in the
// file, and says so in the status line.
async function copyCode(code: string, name: string, secret = false) {
  const text = code.endsWith("\n") ? code.slice(0, -1) : code;
  try {
    await setClipboard(text, secret);
  } catch (err) {
    return status("couldn't copy: " + message(err));
  }
  const n = text ? text.split("\n").length : 0;
  status("Copied " + (name ? name + ", " : "") + n + (n === 1 ? " line" : " lines") + (secret ? secretNote() : ""));
}

// copyCurl puts http block c's request on the clipboard as a curl
// command line, with its placeholders filled in. It may hold a secret,
// so it is copied as one.
async function copyCurl(c: Cell) {
  let text: string;
  try {
    text = await App.Curl(c.block);
    await setClipboard(text, true);
  } catch (err) {
    return status("couldn't copy as curl: " + message(err));
  }
  status("Copied " + (c.name ?? c.id ?? "the request") + " as curl" + secretNote());
}

// copyOutputButton copies the block's output as text.
function copyOutputButton(block: HTMLElement, c: Cell): HTMLButtonElement {
  const b = iconButton("copy", "Copy this block's output", () => void copyOutput(block, c));
  b.classList.add("copy-output");
  return b;
}

// copyOutput puts a block's output on the clipboard as text: the text
// the block printed, without the colors, or, for out=markdown, the text
// as shown. The button says Copied for a moment, and the status line
// says how much.
async function copyOutput(block: HTMLElement, c: Cell) {
  const out = block.querySelector<HTMLElement>(".output")!;
  const text = outputText(out).replace(/\n$/, "");
  const name = c.name ?? c.id ?? "";
  if (!text) return status("no text output to copy");
  try {
    await setClipboard(text, !!c.secret);
  } catch (err) {
    return status("couldn't copy: " + message(err));
  }
  const n = text.split("\n").length;
  status("Copied the output of " + (name ? name + ", " : "") + n + (n === 1 ? " line" : " lines") + (c.secret ? secretNote() : ""));
  const b = block.querySelector<HTMLElement>(".copy-output");
  if (!b) return;
  b.classList.add("copied");
  clearTimeout(copiedTimers.get(b));
  copiedTimers.set(b, setTimeout(() => b.classList.remove("copied"), 1500));
}

const copiedTimers = new WeakMap<HTMLElement, ReturnType<typeof setTimeout>>();

// outputText is the text of a block's output, out. Output a block
// printed is in pre elements, one for each stream, whose text is what it
// printed; other output, such as out=markdown's, is taken as shown.
function outputText(out: HTMLElement): string {
  const kids = [...out.children];
  if (kids.length && kids.every((k) => k.tagName === "PRE")) return kids.map((k) => k.textContent ?? "").join("");
  return out.innerText;
}

// secretNote ends the status line after copying a secret. Only macOS
// can mark it concealed, so elsewhere a clipboard manager may keep it
// after it comes off (finding 9 of the v1.4.0 security review).
const secretNote = () => "; it comes off the clipboard in " + aWhile(revealFor()) + (isMac ? "" : ", but a clipboard manager may keep a copy");

// aWhile says how long ms is, in whole seconds or minutes.
function aWhile(ms: number): string {
  const s = Math.round(ms / 1000);
  if (s < 60 || s % 60) return s + "s";
  return s / 60 + (s === 60 ? " minute" : " minutes");
}

// setClipboard puts text on the clipboard. A secret goes on marked
// concealed, so clipboard managers and history leave it out, and comes
// off again after a while if it is still there.
async function setClipboard(text: string, secret: boolean) {
  if (secret) await App.CopySecret(text);
  else await Clipboard.SetText(text);
}

// revealed holds the blocks whose secret code and output are shown. A
// block marked secret, or whose code names a secret variable, is masked
// until its eye button shows it, and masked again when the window loses
// focus or after revealFor, which the settings set.
const revealed = new Set<number>();
const revealFor = () => (settings.secretShowSeconds > 0 ? settings.secretShowSeconds : 60) * 1000;
let revealTimer = 0;

function revealButton(c: Cell): HTMLButtonElement {
  const shown = revealed.has(c.block);
  const b = iconButton(shown ? "eyeOff" : "eye", shown ? "Hide the secret" : "Show the secret", () => showSecret(c.block, !revealed.has(c.block)));
  b.classList.add("reveal");
  b.setAttribute("aria-pressed", String(shown));
  return b;
}

// showSecret shows or masks block i's secret, redrawing just its eye
// button.
function showSecret(i: number, on: boolean) {
  if (on) revealed.add(i);
  else revealed.delete(i);
  const block = document.getElementById("block-" + i);
  const c = cells.find((c) => c.block === i);
  if (block && c) {
    block.classList.toggle("masked", !on);
    block.querySelector(".head .reveal")?.replaceWith(revealButton(c));
  }
  maskSource();
  clearTimeout(revealTimer);
  if (revealed.size) revealTimer = window.setTimeout(maskAll, revealFor());
}

function maskAll() {
  for (const i of [...revealed]) showSecret(i, false);
}
window.addEventListener("blur", maskAll);

// shownAsCode holds the source of the diagrams switched to their code,
// so a redraw keeps them that way.
const shownAsCode = new Set<string>();

// enableRemoteImages draws the images from other sites in e that the
// user loaded in this document before, and gives the note for each of
// the others a Load button. Go fetches the image (App.RemoteImage), as
// the window's CSP loads none from elsewhere, and remembers the answer
// for that image in this document, so it loads by itself from then on.
function enableRemoteImages(e: HTMLElement) {
  for (const note of e.querySelectorAll<HTMLElement>("em.remote-image[data-src]")) {
    const src = note.dataset.src!;
    const show = (uri: string) => {
      if (!uri || !note.isConnected) return;
      note.replaceWith(el("img", { src: uri, alt: note.dataset.alt ?? "", title: src }));
    };
    const b = el("button", { className: "load-image", title: "Load " + src + " and show it here, now and whenever this document opens" }, "Load image");
    b.onclick = (ev) => {
      ev.stopPropagation();
      b.disabled = true;
      b.textContent = "Loading…";
      App.RemoteImage(src, true)
        .then(show)
        .catch((err) => {
          b.disabled = false;
          b.textContent = "Load image";
          fail(err);
        });
    };
    note.append(" ", b);
    App.RemoteImage(src, false).then(show).catch(() => {});
  }
}

// enableDiagrams gives each d2 or svg diagram, or cast recording, Go drew
// in e a button that switches between the picture and its code. A diagram Go couldn't draw
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
      const title = code ? (fig.dataset.lang === "cast" ? "Show the recording" : "Show the diagram") : "Show the " + fig.dataset.lang + " source";
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
    box.setAttribute("aria-label", (box.closest("li")?.textContent ?? "").trim() || "Task");
    box.onchange = () => void edit(() => App.SetTask(tasks[i], box.checked)).then((ok) => ok || (box.checked = !box.checked));
  });
}

// drawTextbox draws a textbox block as a box to type in. What is typed
// goes into the block's body after a pause, as typing in the source pane
// does. Its lines= is the most lines it takes; lines=fit takes any
// number, in a box as tall as the text. A grip under it makes it taller.
function drawTextbox(c: Cell): HTMLElement {
  const t = c.textbox!;
  const most = t.fit ? Infinity : t.lines;
  const box = el("textarea", { className: "textbox", rows: t.lines, value: t.text, placeholder: t.placeholder ?? "", spellcheck: most > 1 });
  box.setAttribute("aria-label", t.id);
  box.dataset.input = String(t.block);
  const grip = textboxGrip(box, doc.file, t.id, !!t.fit);
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
    if (e.key === "Enter" && !e.isComposing && box.value.split("\n").length >= most) e.preventDefault();
  };
  box.oninput = () => {
    const lines = box.value.split("\n");
    if (lines.length > most) {
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
  const cell = inputCell("textbox-cell", t.id, t.problem, box, t.nosave ? "textbox" : "");
  box.after(grip);
  return cell;
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
  return inputCell("choice-cell", ch.id, ch.problem, control, ch.nosave ? "choice" : "");
}

// inputCell is the card around a textbox's or choice's control, with
// its name over it, a badge when it is nosave (kind is then its kind),
// and, when its var= binds nothing, why.
function inputCell(className: string, id: string, problem: string | undefined, control: HTMLElement, nosave = ""): HTMLElement {
  const cell = el("section", { className: "input-cell " + className + (problem ? " taken" : "") + (nosave ? " nosave" : ""), title: problem || id }, el("span", { className: "name" }, id), nosave ? nosaveBadge(nosave) : "", control);
  if (problem) {
    const p = el("p", { className: "problem", id: "problem-" + control.dataset.input }, problem);
    control.setAttribute("aria-describedby", p.id);
    cell.append(p);
  }
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
  let r: Relayout;
  try {
    // An input's edit is drawn cell by cell, from every cell.
    const got = await relayout();
    r = await drawnBodies(got.splice ? { fresh: spliced(got.splice), l: got.l, moving: got.moving } : got);
  } catch (err) {
    fail(err);
    return false;
  }
  const fresh = r.fresh ?? [];
  const focused = document.activeElement as HTMLTextAreaElement | null;
  const input = focused?.closest<HTMLElement>("[data-input]")?.dataset.input;
  const [from, to] = [focused?.selectionStart, focused?.selectionEnd];
  layout = r.l;
  queueMicrotask(() => void takeMoves(r));
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
      const cell = cells[i];
      headIndex.set(cell, i);
      const card = cardOf.get(cell);
      // A cell whose body didn't come, as one out of sight, takes its
      // head, and its body when it is drawn.
      if (partial.has(c)) {
        takeHead(cell, c);
        if (card) bare.add(card);
        return;
      }
      // An input is drawn again when its value changed, as a copy of the
      // one being typed in does, but not the one that has the focus.
      const inputChanged = (c.textbox && c.textbox.text !== cell.textbox?.text) || (c.choice && JSON.stringify(c.choice.options) !== JSON.stringify(cell.choice?.options));
      const holdsFocus = !!focused && !!card?.contains(focused);
      const changed = c.block < 0 && (c.textbox || c.choice ? inputChanged && !holdsFocus : c.html !== cell.html);
      // Go leaves out fields that are false, so clear the fold flags first.
      Object.assign(cell, { collapsed: false, collapsedOutput: false }, c);
      partial.delete(cell);
      if (card && changed) {
        const e = drawCard(cell, placed.has(card));
        card.replaceWith(e);
        if (following === card) following = e;
      } else if (card) {
        card.dataset.line = String(c.line);
        if (c.block >= 0) markTrash(card, cell);
      }
    });
    // A var= input can change what mdsh:collapse and collapse= say, and
    // a heading's text.
    const was = blocks()[selected];
    indexCells();
    if (was) select(blocks().indexOf(was), false);
    place();
    bareBodies();
    drawOutline();
  }
  await loadSource();
  markRunLines();
  return true;
}

// nosaveBadge marks a block whose value stays in the session: saving
// leaves what the file holds.
const nosaveNote: Record<string, string> = {
  block: "Marked nosave: its output lives in this session. Saving keeps what the file holds after it.",
  textbox: "Marked nosave: what you type lives in this session. Saving keeps the file's text.",
  choice: "Marked nosave: what you pick lives in this session. Saving keeps the file's marks.",
};
function nosaveBadge(kind: string): HTMLElement {
  return el("span", { className: "nosave-badge", title: nosaveNote[kind] }, "session only");
}

// hiddenBadge marks a block that printing leaves out but Run all still
// runs, so a document can't run code its reader never saw.
function hiddenBadge(why: string): HTMLElement {
  return el("span", { className: "hidden-badge", title: why }, "hidden when printed");
}

// staleBadge marks a block whose run is out of date: a block it depends
// on ran again, or had its run cleared, since it ran.
const staleNote = "A block this depends on ran again or was cleared since this ran. Running a block that depends on this one runs it again first.";

// blockClass is the classes of block c's card.
function blockClass(c: Cell): string {
  return "block" + (c.history ? " history" : "") + (c.secret && !revealed.has(c.block) ? " masked" : "") + (c.secretCode ? " secret-code" : "") + (c.hidden ? " hidden-block" : "") + (c.running ? " background" : c.failed ? " failed" : c.ran ? " ran" : "") + (c.stale ? " stale" : "");
}

// runs marks b as a button that starts a run, off while none can.
function runs(b: HTMLButtonElement): HTMLButtonElement {
  b.classList.add("runs");
  b.disabled = !!noRun;
  return b;
}

// playHint is a play button's title. With alt (option on a Mac), the
// block runs in a side shell, started from the History shell's state,
// leaving what runs there, such as an editor, as it is.
const playHint = "Run this block (alt-click: in a side shell, leaving the History pane's command running)";

// runBlockButton is a block's play button.
function runBlockButton(block: number): HTMLButtonElement {
  const b = runs(iconButton("play", hinted(playHint, "Enter"), (e) => start(e.altKey ? "RunBlockSide" : "RunBlock", block)));
  b.classList.add("run-block");
  return b;
}

// stopBackgroundButton stands in for a block's play button while it
// runs in the background, which a run's Stop can't reach once that run
// is over.
function stopBackgroundButton(block: number): HTMLButtonElement {
  const b = iconButton("stop", "Stop this background block", () => void stopBackground(block));
  b.classList.add("stop-bg");
  return b;
}

async function stopBackground(block: number) {
  if (!(await App.StopBackground(block))) status("that block already ended");
}

function drawBlock(c: Cell): HTMLElement {
  // The outline names a block by the .name's first child.
  const name = el("span", { className: "name" }, el("span", { className: "id" }, c.name ?? c.id ?? ""), el("span", { className: "lang" }, c.lang ?? ""), c.attrs ? el("span", { className: "attrs" }, c.attrs) : "");
  const exit = el("span", { className: "exit" + (c.failed ? " bad" : "") }, c.running ? backgroundNote : c.ran ? "exit " + c.status : "");
  const timer = el("span", { className: "timer" });
  const block = el(
    "section",
    { className: blockClass(c), id: "block-" + c.block },
    el(
      "div",
      { className: "head" },
      name,
      c.hidden ? hiddenBadge(c.hidden) : "",
      c.stale ? el("span", { className: "stale-badge", title: staleNote }, "stale") : "",
      c.nosave ? nosaveBadge("block") : "",
      c.http?.status ? statusBadge(c.http) : "",
      jobBadge(c.block),
      exit,
      timer,
      c.secret ? revealButton(c) : "",
      iconButton("copy", "Copy this block's code (c)", () => void copyCode(c.body ?? "", c.name ?? c.id ?? "", !!c.secretCode)),
      c.lang === "http" ? iconButton("curl", "Copy as curl, with the placeholders filled in", () => void copyCurl(c)) : "",
      iconButton("pencil", "Edit this block in the source (e)", () => void editBlock(block, c)),
      runBlockButton(c.block),
      stopBackgroundButton(c.block),
      runs(iconButton("runFrom", "Run from here (a)", () => start("RunFrom", c.block))),
      ...(c.movable ? [trashButton(c)] : []),
    ),
    foldPart("code", el("pre", { className: "code numbered" }, el("code", {}, ...codeLines(c.body ?? "", c.lang ?? ""))), () => foldBlock(c, "code")),
    el("div", { className: "ask", hidden: true }),
    foldPart("output", el("div", { className: "output", innerHTML: c.output ?? "" }), () => foldBlock(c, "output")),
  );
  // The output's buttons sit in its top right corner: copy, and on the
  // selected card the trash.
  // A background tty block that is running shows its terminal.
  if (c.terminal) adoptBackground(c.block, block.querySelector<HTMLElement>(".output")!);
  const tools = el("div", { className: "output-tools" }, el("span", { className: "output-at" }), copyOutputButton(block, c));
  if (c.movable && c.earlier?.length) tools.append(clearEarlierButton(c));
  if (c.movable) tools.append(clearOutputButton(c));
  block.querySelector(".output-part")!.append(tools);
  if (c.earlier?.length) {
    // The earlier runs history=N keeps, folded under the output.
    block.querySelector(".output-part")!.after(
      drawEarlier(c.earlier, {
        key: partKey(c, "output"),
        copy: (r, out, b) => void copyEarlier(c, r.k, out, b),
        remove: (k) => void deleteEarlier(c.block, k),
        view: viewImage,
      }),
    );
  }
  if (c.http) {
    // The response's view takes the output's place; the output stays,
    // hidden, so copying it copies the response as the file saves it.
    block.querySelector(".output")!.before(drawHTTP(c.http, partKey(c, "output")));
    block.classList.add("has-http");
  }
  drawAt(block, c, new Date());
  startAtTicker();
  colorOutput(block.querySelector(".output")!);
  if (c.links) linkify(block.querySelector<HTMLElement>(".output")!);
  markOutput(block.querySelector<HTMLElement>(".output")!);
  // The card is a tab stop, named for the block and its state, so the
  // keyboard and a screen reader reach it; focusing it selects it, as a
  // click does.
  block.setAttribute("role", "group");
  block.tabIndex = 0;
  labelBlock(block);
  block.onfocus = () => {
    if (!block.classList.contains("selected")) select(blocks().indexOf(c), false);
  };
  // Checkboxes in output, such as out=markdown's, are edited in place
  // too, until the block runs again.
  enableTasks(block.querySelector<HTMLElement>(".output")!, c.outputTasks);
  enableDiagrams(block.querySelector<HTMLElement>(".output")!);
  enableRemoteImages(block.querySelector<HTMLElement>(".output")!);
  enableCasts(block.querySelector<HTMLElement>(".output")!, { frame: Number(/(?:^|\s)frame=(\d+)/.exec(c.attrs ?? "")?.[1] ?? 0) || undefined, save: (n) => void saveFrame(c.block, "", n), replace: (as, n) => void replaceCast(c.block, as, n) });
  block.querySelector<HTMLElement>(".output")!.onclick = (e) => {
    const img = (e.target as HTMLElement).closest("img");
    if (img) viewImage(img);
  };
  block.dataset.block = String(c.block);
  if (c.timeout) block.dataset.timeout = String(c.timeout);
  if (timerStarts.has(c.block)) {
    // Drawn again while it runs, as a background block is when the
    // run ends: its timer goes on.
    block.classList.add("timing");
    drawTimer(block, performance.now() - timerStarts.get(c.block)!, true);
  } else if (c.ran && c.ms) {
    restTimer(timer, c.ms, !!c.timedOut, c.timeout ?? 0);
  }
  block.onclick = (e) => {
    select(blocks().indexOf(c), false);
    if (!(e.target as HTMLElement).closest("button, input")) void showInSource(block, c.line, false);
  };
  return block;
}

// labelBlock names block's card for a screen reader: its name, its
// language, its attributes and its state, as the card shows them.
function labelBlock(block: HTMLElement) {
  const text = (sel: string) => block.querySelector(sel)?.textContent?.trim() ?? "";
  const name = text(".head .id") || "block " + (Number(block.dataset.block) + 1);
  const states: Record<string, string> = { "?": "waiting for an answer", "▶": "running now", "◐": "running in the background", "✗": "failed", "✓": "ran" };
  const state = states[blockMark(block)] ?? "";
  const exit = text(".head .exit");
  const took = block.classList.contains("timing") ? "" : block.querySelector<HTMLElement>(".head .timer")?.title ?? "";
  const parts = [name, text(".head .lang") + " block", text(".head .attrs"), state, state && exit && !exit.startsWith("running") ? exit : "", state ? took.toLowerCase() : ""];
  block.setAttribute("aria-label", parts.filter(Boolean).join(", "));
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
// name and part, or the heading's fold's name (foldKeys). The rest are
// as the file says.
const folds = new Map<string, boolean>();

type Part = "code" | "output";

const partKey = (c: Cell, part: Part) => part + ":" + (cellName(c) || String(c.block));

function blockFolded(c: Cell, part: Part): boolean {
  return folds.get(partKey(c, part)) ?? !!(part === "code" ? c.collapsed : c.collapsedOutput);
}

function sectionFolded(c: Cell): boolean {
  return folds.get(foldKeys.get(c) ?? "") ?? !!c.collapsed;
}

// peeks are the folded sections a dependency line being dragged has
// opened for the moment, by heading key, so the blocks in them can be
// dropped on. They change neither folds nor the file.
const peeks = new Set<string>();
let peekTimer = 0;
let peekCell: Cell | null = null;
// How long the line is held over a folded heading before it opens.
const peekDelay = 400;

// askPeeks are the folded sections a block's question has opened, by
// heading key, so the question can be seen. Like peeks they change
// neither folds nor the file: each closes again once the run moves on
// to a block outside it, or ends.
const askPeeks = new Set<string>();

// shownFolded is whether a section is drawn folded: folded, and not
// opened by a drag or a question.
function shownFolded(c: Cell): boolean {
  const key = foldKeys.get(c) ?? "";
  return !peeks.has(key) && !askPeeks.has(key) && sectionFolded(c);
}

// peekAsk opens the folded sections that hide c, for its question.
function peekAsk(c: Cell) {
  const i = cellIndex.get(c);
  if (i === undefined || !hidden[i]) return;
  for (const key of hidingSections(i)) askPeeks.add(key);
  applyFolds();
}

// closeAsks closes the sections a question opened, except those that
// hold c, the block the run is at now; with no c, all of them.
function closeAsks(c?: Cell | null) {
  let closed = false;
  for (const key of [...askPeeks]) {
    const h = headingByKey.get(key);
    if (h && c && holds(h, c)) continue;
    askPeeks.delete(key);
    closed = true;
  }
  if (closed) {
    applyFolds();
    drawOutline();
  }
}

// holds is whether c is in the section under heading, at any depth.
function holds(heading: Cell, c: Cell): boolean {
  const from = cellIndex.get(heading);
  const to = cellIndex.get(c);
  if (from === undefined || to === undefined || to <= from) return false;
  const level = heading.heading ?? 0;
  for (let i = from + 1; i <= to; i++) {
    const l = cells[i].heading ?? 0;
    if (l && l <= level) return false;
  }
  return true;
}

// headingByKey is each heading's cell by its fold's name.
let headingByKey = new Map<string, Cell>();

// peek follows a dependency drag over cell, the top-level cell under
// the pointer (null between cells changes nothing). The sections it
// opened close once the pointer is out of them, and a folded heading
// held under the pointer opens.
function peek(card: HTMLElement | null) {
  const cell = card && cellOf.get(card);
  if (!cell) return;
  let closed = false;
  for (const key of [...peeks]) {
    const h = headingByKey.get(key);
    if (h && (h === cell || holds(h, cell))) continue;
    peeks.delete(key);
    closed = true;
  }
  if (closed) {
    applyFolds();
    drawOutline();
  }
  if (cell === peekCell) return;
  clearTimeout(peekTimer);
  peekCell = null;
  const key = foldKeys.get(cell);
  if (!key || !shownFolded(cell)) return;
  peekCell = cell;
  peekTimer = window.setTimeout(() => {
    peekCell = null;
    peeks.add(key);
    applyFolds();
    drawOutline();
  }, peekDelay);
}

// endPeek closes what a dependency drag opened, when it ends. Sections
// holding the block it was dropped on stay open while the tab is, as
// reveal leaves them, so the new line is seen arriving there; the file
// keeps them folded.
function endPeek(card: HTMLElement | null) {
  const dropped = card && cellOf.get(card);
  clearTimeout(peekTimer);
  peekCell = null;
  if (!peeks.size) return;
  for (const key of peeks) {
    const h = headingByKey.get(key);
    if (dropped && h && holds(h, dropped)) folds.set(key, false);
  }
  peeks.clear();
  applyFolds();
  drawOutline();
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
  const summary = el("div", { className: "fold-summary", hidden: true, title: "Unfold", role: "button", tabIndex: 0 });
  summary.setAttribute("aria-label", "Unfold the " + part);
  summary.onclick = (e) => {
    e.stopPropagation();
    toggle();
  };
  summary.onkeydown = (e) => {
    if (keyActivates(e)) toggle();
  };
  return el("div", { className: "part " + part + "-part" }, foldButton(part === "code" ? "Fold or unfold the code" : "Fold or unfold the output", toggle), full, summary);
}

function foldBlock(c: Cell, part: Part, on = !blockFolded(c, part)) {
  folds.set(partKey(c, part), on);
  const card = cardOf.get(c);
  if (card) drawPart(card, part);
  placeSoon();
  void saveFolds([blockFold(c, part, on)]);
}

function foldSection(c: Cell, on = !shownFolded(c)) {
  const key = foldKeys.get(c) ?? "";
  // Folding by hand takes over from a question that opened it.
  askPeeks.delete(key);
  folds.set(key, on);
  applyFolds();
  drawOutline();
  const f = sectionFold(c, on);
  if (f) void saveFolds([f]);
}

const blockFold = (c: Cell, part: Part, on: boolean): Fold => ({ block: c.block, output: part === "output", heading: 0, on });

function sectionFold(c: Cell, on: boolean): Fold | null {
  return c.heading ? { block: -1, output: false, heading: c.from, on } : null;
}

// saveFolds writes folds to the file, then forgets the clicks the file
// now holds, so that Undo and edits to the file show. The file can't
// change while a block runs, so then the folds aren't saved.
async function saveFolds(list: Fold[]) {
  if (!list.length || running) return;
  if (!(await edit(() => App.SetFolds(list)))) return;
  const held = (key: string, file: boolean) => folds.get(key) === file && folds.delete(key);
  for (const c of cells) {
    if (renumbered(c)) {
      held(partKey(c, "code"), !!c.collapsed);
      held(partKey(c, "output"), !!c.collapsedOutput);
    } else if (c.heading) held(foldKeys.get(c) ?? "", !!c.collapsed);
  }
  // Where the file folds by a condition, nothing changed.
  $("dirty").hidden = !(await App.Doc()).dirty;
}

// replaceCast swaps tty block i's recording, in the file, for the text
// of frame n or an animated GIF of it, without asking, since the bar's
// Undo puts it back.
async function replaceCast(i: number, as: "text" | "gif", n: number) {
  if (running) return status("can't change the file while a block runs");
  await applySource();
  try {
    await App.ReplaceCast(i, as, n);
  } catch (err) {
    return fail(err);
  }
  await refresh();
  const what = as === "text" ? `the text of frame ${n}` : "an animated GIF";
  $("moved-text").textContent = `Replaced the recording with ${what}.`;
  $("moved-undo").onclick = () => {
    $("moved").hidden = true;
    void undoEdit(false);
  };
  $("moved").hidden = false;
  status("replaced the recording");
}

// saveFrame writes frame=n, where a recording's player was left, to tty
// block i, or with i < 0 to the cast fence holding body, so the file
// opens at that frame. n is 0 for the last frame. While a block runs the
// file can't change, so it is left as it is.
async function saveFrame(i: number, body: string, n: number) {
  if (running) return;
  await edit(() => App.SetCastFrame(i, body, n));
}

// drawPart shows a block's code or output whole, or as its first line.
function drawPart(block: HTMLElement, part: Part) {
  const wrap = block.querySelector<HTMLElement>("." + part + "-part");
  const c = cellOf.get(block);
  if (!wrap || !c) return;
  const on = blockFolded(c, part);
  wrap.classList.toggle("folded", on);
  wrap.querySelector(":scope > button.fold")?.setAttribute("aria-expanded", String(!on));
  const summary = wrap.querySelector<HTMLElement>(".fold-summary")!;
  summary.hidden = !on;
  // An http block's response view sits before its output, which still
  // holds the response as text: the summary is that, from its status line.
  if (on) summarize(wrap.querySelector<HTMLElement>(":scope > :is(pre.code, .output)")!, summary);
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

// applyFolds works out what the sections' folds hide again, marks the
// cards drawn, and draws the cards that come into sight.
function applyFolds() {
  const was = blocks()[selected];
  foldCells();
  // A selected block that a fold hides is selected no longer.
  if (was) select(blocks().indexOf(was), false);
  place();
}

// reveal unfolds the sections that hide c, so it can be shown, as when
// a block asks before it runs or the toolbar jumps to it.
function reveal(c: Cell) {
  const i = cellIndex.get(c);
  if (i === undefined || !hidden[i]) return;
  for (const key of hidingSections(i)) {
    folds.set(key, false);
    askPeeks.delete(key);
  }
  applyFolds();
  drawOutline();
}

// hidingSections are the keys of the folded sections that hold cell i.
function hidingSections(i: number): string[] {
  const keys: string[] = [];
  let level = cells[i]?.heading || 7;
  for (let j = i - 1; j >= 0 && level > 1; j--) {
    const l = cells[j].heading ?? 0;
    if (!l || l >= level) continue;
    level = l;
    if (sectionFolded(cells[j])) keys.push(foldKeys.get(cells[j]) ?? "");
  }
  return keys;
}

// foldAll folds or unfolds every block's code and output, or every
// section.
function foldAll(what: "blocks" | "sections", on: boolean): Fold[] {
  const list: Fold[] = [];
  for (const c of cells) {
    if (what === "blocks" && renumbered(c)) {
      for (const part of ["code", "output"] as const) {
        // Only blocks with output get collapse-output.
        const empty = part === "output" && !(partial.has(c) ? c.outputLines : c.output?.trim());
        if (blockFolded(c, part) !== on && !(on && empty)) list.push(blockFold(c, part, on));
        folds.set(partKey(c, part), on);
      }
    }
    if (what === "sections" && c.heading) {
      const f = sectionFolded(c) !== on && sectionFold(c, on);
      if (f) list.push(f);
      folds.set(foldKeys.get(c) ?? "", on);
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
  for (const c of cells) {
    if (c.heading && (fromFile ? c.collapsed : sectionFolded(c))) lines.push(c.line);
    if (c.block >= 0) {
      if (fromFile ? c.collapsed : blockFolded(c, "code")) lines.push(c.line);
      if (c.outputLine && (fromFile ? c.collapsedOutput : blockFolded(c, "output"))) lines.push(c.outputLine);
    }
  }
  return lines;
}

async function foldsToSource() {
  if (!view.split) toggle("split", true);
  await loadSource();
  if (source) setSourceFolds(source.view, renderedFoldLines(false));
  if (nvimDoc) await nvimCall("set_folds", renderedFoldLines(false)).catch(fail);
}

async function foldsToRendered() {
  const lines = new Set(nvimDoc ? ((await nvimCall<number[]>("folds").catch(fail)) ?? []) : source ? sourceFolds(source.view) : []);
  if (!source && !nvimDoc) return status("open the source view to copy its folds");
  const list: Fold[] = [];
  for (const c of cells) {
    if (c.heading) {
      const on = lines.has(c.line);
      if (sectionFolded(c) !== on) list.push({ block: -1, output: false, heading: c.from, on });
      folds.set(foldKeys.get(c) ?? "", on);
    }
    if (c.block >= 0) {
      const on = { code: lines.has(c.line), output: !!c.outputLine && lines.has(c.outputLine) };
      for (const part of ["code", "output"] as const) {
        if (blockFolded(c, part) !== on[part]) list.push(blockFold(c, part, on[part]));
        folds.set(partKey(c, part), on[part]);
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
// scrolls into view. An image map's areas, from HTML in the document,
// are links too. A right-click gets the link menu (menu.go), with the
// link's href as its data.
function followLinks() {
  const link = (e: Event) => (e.target as HTMLElement).closest<HTMLAnchorElement | HTMLAreaElement>("#doc a[href], #doc area[href]");
  document.addEventListener(
    "click",
    (e) => {
      const a = link(e);
      if (!a) return;
      e.preventDefault();
      e.stopPropagation();
      const href = a.getAttribute("href")!;
      if (href.startsWith("#")) return void scrollToHeading(href.slice(1));
      App.OpenLink(href)
        .then((src) => src && viewSrc(src, a instanceof HTMLAreaElement ? a.alt : (a.textContent ?? "")))
        .catch(fail);
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
  // A heading drawn has the id goldmark gave it; any heading, the one
  // GitHub would, from its text.
  const drawnHeading = [...document.querySelectorAll<HTMLElement>("#doc :is(h1, h2, h3, h4, h5, h6)")].find((h) => h.id === id);
  const c = (drawnHeading && cellOf.get(drawnHeading.closest<HTMLElement>("#doc > *")!)) ?? cells.find((c) => c.heading && slug(c.title ?? "") === id.toLowerCase());
  const i = c ? cellIndex.get(c) : undefined;
  if (c && i !== undefined) {
    reveal(c);
    scrollToCell(i, 0);
  }
}

followLinks();

// backgroundNote is what a background block's card says while it runs.
const backgroundNote = "running in the background…";

// backgroundStatus counts the background blocks still running in the
// status line, unless a run is going.
function backgroundStatus() {
  if (running) return;
  const n = document.querySelectorAll("#doc .block.background").length;
  status(n ? n + " in background" : lostNote);
}

// viewImage shows an output image full size over the window, until a
// click or esc.
function viewImage(img: HTMLImageElement) {
  viewSrc(img.src, img.alt);
}

// viewSrc shows the image at src full size over the window, as
// viewImage does: a link to an image beside the document opens here.
function viewSrc(src: string, alt: string) {
  const viewer = $<HTMLDialogElement>("viewer");
  const big = viewer.querySelector("img")!;
  big.src = src;
  big.alt = alt;
  big.classList.toggle("vector", /\.svg$/i.test(new URL(src, location.href).pathname));
  viewer.onclick = () => viewer.close();
  viewer.showModal();
}

// blocks are the blocks j and k move between: those no fold hides. They
// are kept until the cells or the folds change: selecting, marking and
// the outline each ask.
function blocks(): Cell[] {
  shownBlocks ??= cells.filter((c, i) => renumbered(c) && !hidden[i]);
  return shownBlocks;
}

let shownBlocks: Cell[] | null = null;

function select(i: number, scroll = true) {
  const all = blocks();
  for (const b of document.querySelectorAll("#doc .block.selected")) b.classList.remove("selected");
  selected = i;
  if (i >= 0 && selectedSection) {
    selectedSection = "";
    markSection();
  }
  markOutline();
  litDeps(i >= 0 && i < all.length ? all[i].block : -1);
  if (i < 0 || i >= all.length) return;
  const card = cardFor(all[i]);
  if (!card) return;
  card.classList.add("selected");
  if (scroll) card.scrollIntoView({ block: "nearest" });
  // j and k move the focus with the selection, so a screen reader
  // follows and enter runs the block the focus is on.
  const at = document.activeElement;
  if (scroll && (at === document.body || at?.closest("#doc > .block"))) card.focus({ preventScroll: true });
}

// selectSection selects the heading c, so its play button shows in the
// outline and s and Run Section run its section.
function selectSection(c: Cell) {
  const key = foldKeys.get(c);
  if (!key) return;
  select(-1, false);
  selectedSection = key;
  markSection();
}

// markSection marks the selected heading in the view and the outline.
function markSection() {
  for (const e of document.querySelectorAll<HTMLElement>("#doc > .heading-cell.selected")) e.classList.remove("selected");
  if (selectedSection) document.querySelector(`#doc > .heading-cell[data-fold="${CSS.escape(selectedSection)}"]`)?.classList.add("selected");
  markOutline();
}

// sectionCell is the heading whose section runs for Run Section and s:
// the selected heading, or else the innermost section holding line, by
// default the selected block's.
function sectionCell(line?: number): Cell | null {
  if (line === undefined) {
    const h = selectedSection ? headingByKey.get(selectedSection) : undefined;
    if (h) return h;
    const b = blocks()[selected];
    if (!b) return null;
    line = b.line;
  }
  let found: Cell | null = null;
  for (const c of cells) {
    if (c.line > line) break;
    if (c.heading) found = c;
  }
  return found;
}

// runSection runs the blocks of cell's section and its subsections, as
// Run all runs them: confirm, run=once, depends= and on_error apply.
async function runSection(cell: Cell | null) {
  if (running || !doc.file) return;
  if (!cell) {
    status("no section selected");
    return;
  }
  if (!cell.sectionTo) {
    status("no block in this section runs");
    return;
  }
  // What was typed is applied first, which can draw the cells anew.
  const key = foldKeys.get(cell) ?? "";
  await applySource();
  const now = headingByKey.get(key);
  if (!now?.sectionTo) return;
  // The view comes back to the heading, as Run this block comes back to
  // its block.
  const home = () => {
    const h = headingByKey.get(key);
    return h ? cardFor(h) : null;
  };
  await run(() => App.RunSection(now.sectionFrom ?? 0, now.sectionTo!), home);
}

function runAllButton() {
  const b = $<HTMLButtonElement>("run-all");
  b.replaceChildren(svg(running ? icons.stop : icons.play), el("span", {}, running ? "Stop" : "Run all"));
  b.classList.toggle("stop", running);
  b.disabled = !running && (noRun !== "" || !cells.some((c) => c.block >= 0));
  b.title = !running && noRun ? noRun : "";
  void clearOutputsButton();
}

// runButtons are the buttons that start a run: a block's, a section's
// and the outline's.
const runButtons = ".block .head button.runs, #outline .run, .heading-cell .run-section";

// showFrontMatterError shows, while the file is open, why its front
// matter doesn't parse, with the fix mdsh offers, if any, and turns
// running off until it does.
function showFrontMatterError(open: boolean) {
  const err = open ? doc.frontMatterError : "";
  noRun = err ? "Fix the front matter to run blocks" : "";
  $("fmerror").hidden = !err;
  document.body.classList.toggle("no-run", !!err);
  $("fmerror-text").textContent = err.charAt(0).toUpperCase() + err.slice(1) + ".";
  const fix = $<HTMLButtonElement>("fmerror-fix");
  fix.hidden = !err || !doc.frontMatterFix.label;
  fix.textContent = doc.frontMatterFix.label;
  fix.title = doc.frontMatterFix.detail;
  fix.onclick = () => void fixWarning(err, "Fixed the front matter. Blocks can run.");
  // The front matter's first line, after any #! line.
  $("fmerror-source").onclick = () => void goToLine(2);
  for (const x of document.querySelectorAll<HTMLButtonElement>(runButtons)) x.disabled = running || !!noRun;
  runAllButton();
}

// clearOutputsButton offers Clear All Outputs only when there is output
// to clear, so once the outputs are cleared it stays off until a block's
// output comes in.
async function clearOutputsButton() {
  const b = $<HTMLButtonElement>("clear-outputs");
  if (running || !doc.file) {
    b.disabled = true;
    return;
  }
  let has = false;
  try {
    has = await App.HasOutput();
  } catch {
    // No notebook, as while a tab closes: nothing to clear.
  }
  // A run may have started while Go was asked.
  b.disabled = running || !has;
}

// saveAfterRun is set by a save asked for while a block ran, such as
// :w with a command running in the History pane: the save is made once
// the run ends, with what was typed meanwhile.
let saveAfterRun = false;

function busy(on: boolean) {
  running = on;
  if (!on && saveAfterRun) void actions.save();
  document.body.classList.toggle("running", on);
  if (!on) for (const b of document.querySelectorAll("#doc .block.now")) b.classList.remove("now");
  if (!on) stopTimers();
  if (!on) {
    // The block stopped on its own: no need to ask about its shell.
    clearTimeout(killTimer);
    if (askingKill) dismissShells();
  }
  runAllButton();
  historyBusy(on);
  busyFrontMatter(on);
  // A background block's Stop stays, as it runs on beside the run.
  for (const x of document.querySelectorAll<HTMLButtonElement>(".block .head button:not(.stop-bg), #outline .run, .heading-cell .run-section")) x.disabled = on;
  if (!on && noRun) for (const x of document.querySelectorAll<HTMLButtonElement>(runButtons)) x.disabled = true;
}

function start(method: "RunBlock" | "RunBlockSide" | "RunFrom", block: number) {
  // Run this block comes back to the block when it's done, though its
  // depends= ran first above it. Run from here ends where the run does.
  const home = method !== "RunFrom" ? () => blockCard(block) : undefined;
  return run(() => App[method](block), home);
}

// run runs call, following its blocks as they run. With home, the card
// the run started from, the view comes back to that card when the run
// is over, where it was, and the card is selected: following the run
// may have scrolled away, up to the blocks it depends on. Scrolling by
// hand during the run leaves the view where it is. home finds the card
// anew, since the run draws the document again.
function shellLost(text: string) {
  lostNote = text;
  status(text);
}

async function run(call: () => Promise<void>, home?: () => HTMLElement | null) {
  if (running) return;
  if (noRun) return status(noRun);
  const back = home ? homeOffset(home()) : null;
  scrolledByHand = false;
  // The History pane may have set the terminal's size to its own.
  sizeTerminal();
  busy(true);
  ranHere = true;
  lostNote = "";
  status("running");
  try {
    await call();
    // A shell the run ended, or new shells, are worth saying after it.
    status(lostNote);
  } catch (err) {
    fail(err);
  }
  ranHere = false;
  busy(false);
  closeAsks();
  await refresh();
  const card = home?.();
  const ended = card?.classList.contains("block") ? card.getAttribute("aria-label") ?? "" : "";
  announce(ended ? "Run finished: " + ended : "Run finished");
  if (card) {
    following = null;
    if (!scrolledByHand) goHome(card, back);
    const c = cellOf.get(card);
    if (c && renumbered(c)) {
      const i = blocks().indexOf(c);
      if (i >= 0) select(i, false);
    } else if (c) selectSection(c);
  }
  // The output is drawn in full now, and may be longer than it was.
  if (following) showEnd(following, false);
  following = null;
  backgroundStatus();
}

// homeOffset is how far card's top is below the rendered view's top,
// or null if the card isn't in sight.
function homeOffset(card: HTMLElement | null): number | null {
  const pane = $("doc-scroll");
  if (!card || pane.hidden) return null;
  const p = pane.getBoundingClientRect();
  const offset = card.getBoundingClientRect().top - p.top;
  return offset >= 0 && offset < p.height - followMargin ? offset : null;
}

// goHome scrolls the rendered view so card's top is offset below the
// view's top, or near the top without offset, and then down, as far as
// its top, until its end is in sight.
function goHome(card: HTMLElement, offset: number | null) {
  const pane = $("doc-scroll");
  if (pane.hidden) return;
  const p = pane.getBoundingClientRect();
  const r = card.getBoundingClientRect();
  const top = pane.scrollTop + r.top - p.top;
  const bottom = top + r.height;
  const at = top - (offset ?? followMargin);
  const to = Math.max(at, Math.min(bottom + followMargin - p.height, top - followMargin));
  if (Math.abs(to - pane.scrollTop) > 1) jump(to - pane.scrollTop, scrollBehavior(), (behavior) => pane.scrollTo({ top: to, behavior }));
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
  // A Neovim that is still starting can't take the jump yet: wait for it,
  // or the cursor stays on line 1. If it fails, the pane falls back to
  // CodeMirror, which then needs the text.
  if (nvimStarting) {
    await nvimStarting;
    if (!nvimDoc && source) await loadSource();
  }
  if (nvimDoc && nvim) {
    const pane = $("source-editor").getBoundingClientRect();
    const rows = view.rendered ? Math.round((card.getBoundingClientRect().top - pane.top) / nvim.rowHeight) : 2;
    holdScroll();
    await nvimCall("jump", line, rows, open, 0).catch(fail);
    if (open) nvim.focus();
    return;
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

// goToLine puts the source pane's cursor on line, opening the pane if
// need be, with the rendered view brought to the cell holding it.
async function goToLine(line: number) {
  if (!doc.file) return;
  const c = cellAt(line);
  const card = c && view.rendered ? cardFor(c) : null;
  card?.scrollIntoView({ block: "nearest" });
  await showInSource(card ?? $("doc"), line, true);
}

// editBlock puts the cursor on the first line of block c's code.
async function editBlock(card: HTMLElement, c: Cell) {
  await showInSource(card, c.line, true);
  if (nvimDoc) return void (await nvimCall("jump", c.line + 1, c.line + 1 - Math.max(1, c.line - 2), true, -1).catch(fail));
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

// typedAt is when the source pane was last typed in, and behind is set
// while the rendered view waits for the typing to settle before drawing
// an edit that changes more than manyCards cards. settle is how long, in
// milliseconds, the typing must stop for.
let typedAt = 0;
let behind = false;

// typingPause is how long typing must stop for before what was typed
// is drawn, in milliseconds. A document whose edits draw in a few
// milliseconds is drawn soon after each key, so it follows the typing;
// one that takes longer waits, up to half a second, so drawing it
// doesn't hold up the keys. drawTook is how long the last edit took to
// draw.
let drawTook = 0;
const typingPause = () => Math.round(Math.min(500, Math.max(40, 6 * drawTook)));
const manyCards = 30;
const settle = 1500;
let sourceTurn: Promise<unknown> = Promise.resolve();

// inTurn runs f once the source pane's reads and writes before it are
// done.
function inTurn<T>(f: () => Promise<T>): Promise<T> {
  const p = sourceTurn.then(f, f);
  sourceTurn = p.catch(() => {});
  return p;
}

function openSource() {
  if (useNvim()) return void startNvim();
  if (source) return;
  source = markdownEditor($("source-editor"), {
    text: "",
    vim: settings.vim,
    spellCheck: !settings.noSpellCheck,
    attrs,
    names: () => known,
    blocks: sourceBlocks,
    lineNumbers: true,
    // The last lines can scroll up from the window's bottom edge, and
    // the cursor keeps clear of it.
    extensions: [runGutter((line) => void runLine(line)), secretMask(), sourceFolding(), scrollPastEnd(), EditorView.scrollMargins.of(() => ({ bottom: 48 }))],
    onChange() {
      typedAt = Date.now();
      clearTimeout(sourceTimer);
      sourceTimer = window.setTimeout(() => void applySource(true), typingPause());
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
  for (const type of ["wheel", "touchstart", "pointerdown", "keydown"]) source.view.dom.addEventListener(type, leadFrom("source"), { capture: true, passive: true });
  void loadSource();
}

function closeSource() {
  stopNvim();
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
// is over, and draws the document again. While typing, which is when
// typing calls it, an edit that changes many cards waits to be drawn
// until the typing settles: opening a fence turns everything below it
// into code until the fence is closed, and drawing every card below at
// each pause, and then again once it closed, made typing choppy.
function applySource(typing = false): Promise<void> {
  clearTimeout(sourceTimer);
  return inTurn(async () => {
    if (nvimDoc) return applyNvim(nvimDoc);
    const ed = source;
    if (!ed || (!ed.pending && !behind)) return;
    if (running) {
      sourceTimer = window.setTimeout(() => void applySource(), 1000);
      return;
    }
    if (ed.pending) {
      try {
        await sendSource(ed);
        status("");
      } catch (err) {
        fail(err);
        return;
      }
      $("dirty").hidden = false;
      if (lintAgain && source === ed) {
        lintAgain = false;
        forceLinting(ed.view);
      }
    }
    let r: Relayout;
    try {
      r = await relayout();
    } catch (err) {
      fail(err);
      return;
    }
    const settles = typedAt + settle - Date.now();
    if (typing && settles > 0) {
      if (changedCells(r) > manyCards) {
        behind = true;
        sourceTimer = window.setTimeout(() => void applySource(true), settles);
        return;
      }
    }
    r = await drawnBodies(r);
    const t = performance.now();
    updateDoc(r);
    drawTook = performance.now() - t;
    if (source !== ed) return;
    markRunLines();
    markCurrent(ed.view.state.doc.lineAt(ed.view.state.selection.main.head).number);
  });
}

// sendSource hands Go what was typed in ed as line edits, folding Go's
// changes into it first whenever Go has some the pane hasn't seen.
async function sendSource(ed: MarkdownEditor) {
  await sendSync(ed.sync, (changes) => ed.remote(changes));
}

// LinesSince is how Go's text changed since a version, as Go's Edit and
// LinesSince say.
type LinesSince = { version: Version; events: LineEdit[] | null; reset: boolean };

// sendSync hands Go what was typed, as sync has it, as line edits made
// against the version Go last had. When Go has moved on since, sync takes
// in Go's changes, apply makes them in the pane, and it tries again.
async function sendSync(sync: Synced, apply: (changes: ChangeSet) => void | Promise<void>) {
  for (let tries = 0; sync.pending; tries++) {
    const edits = sync.take();
    let r: LinesSince & { applied: boolean };
    try {
      r = await App.Edit(sync.version, edits);
    } catch (err) {
      sync.refused();
      throw err;
    }
    if (r.applied) return sync.sent(r.version);
    sync.refused();
    if (tries === 5) throw new Error("the file keeps changing; try again");
    await apply(await syncFrom(sync, r));
  }
}

// syncFrom takes Go's changes, as l says them, into sync, and returns
// the changes that bring the pane's text up to Go's around what was
// typed. Only when Go no longer has the changes since sync's version, or
// the file is another, is the whole text read.
async function syncFrom(sync: Synced, l: LinesSince): Promise<ChangeSet> {
  if (!l.reset) return sync.incoming(l.events ?? [], l.version);
  const t = await App.TextAt();
  return sync.reset(t.text, t.version);
}

// sourceBlocks asks Go about the blocks in the source pane's text, for
// depends= (GoBlocks in editor.ts): about Go's text, with the lines moved
// past what was typed since, so the pane's text needn't cross the
// bridge. Checking asks only for what is wrong.
const sourceBlocks: GoBlocks = {
  blocks: async (state) => {
    const r = await App.SourceBlocksAt();
    return fromGo(state, r.version, r.blocks ?? []);
  },
  depends: async (state) => {
    const r = await App.DependsAt();
    return fromGo(state, r.version, r.problems ?? []);
  },
};

// fromGo moves things on lines of Go's text at version to the lines of
// the source pane's text in state, or is null if the pane's idea of
// Go's text isn't version. Typing Go hasn't had may make or unmake
// blocks, so what is checked against it is checked again once Go has it.
function fromGo<T extends { line: number }>(state: EditorState, version: Version, got: T[]): T[] | null {
  const ed = source;
  if (!ed) return null;
  const s = ed.sync;
  const typed = s.sending ? s.sending.compose(s.local) : s.local;
  if (!typed.empty) lintAgain = true;
  if (version.doc !== s.version.doc || version.tick !== s.version.tick || typed.newLength !== state.doc.length) {
    lintAgain = true;
    return null;
  }
  const out: T[] = [];
  for (const x of got) {
    if (x.line > s.base.lines) continue;
    out.push({ ...x, line: state.doc.lineAt(typed.mapPos(s.base.line(x.line).from)).number });
  }
  return out;
}

// lintAgain is set when the source pane's depends= were checked against
// blocks Go didn't have all of, so they are checked again once it does.
let lintAgain = false;

// sourceFoldsFor is the file whose folds the source pane opened with.
let sourceFoldsFor = "";

// loadSource reads Go's text into the source pane, keeping what was
// typed and not yet sent.
function loadSource(): Promise<void> {
  return inTurn(loadSourceNow);
}

async function loadSourceNow() {
  if (useNvim() && view.split) {
    if (!nvimDoc) {
      if (!nvim) void startNvim();
      return;
    }
    try {
      const d = nvimDoc;
      await mergeNvim(d, await App.LinesSince(d.sync.version));
    } catch (err) {
      fail(err);
    }
    markRunLines();
    return;
  }
  if (!source || doc.file === "") return;
  const ed = source;
  const marks = foldMarks(ed.view);
  try {
    const l = await App.LinesSince(ed.sync.version);
    if (source !== ed) return;
    const changes = await syncFrom(ed.sync, l);
    if (source !== ed) return;
    ed.remote(changes);
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

// With Vim on and the Neovim setting, the source pane is an embedded
// Neovim instead (nvim.ts). If it can't start, the pane falls back to
// CodeMirror until the settings change.

let nvim: NvimPane | null = null;
let nvimFailed = false;

// NvimDoc is the text a Neovim edits. sync keeps it in step with Go's,
// as the CodeMirror pane's does. text is Neovim's text as the page has
// heard it, at seq, Neovim's count of the lines it sent and the edits it
// took; owed is Go's changes, on text, that Neovim hasn't taken yet, so
// that sync's text is owed made on text. queue holds lines Neovim sent
// while an edit was going to it, and waiters wait for seq to move.
type NvimDoc = { sync: Synced; text: Text; seq: number; owed: ChangeSet | null; applying: boolean; queue: NvimLines[]; waiters: (() => void)[] };
let nvimDoc: NvimDoc | null = null;

// NvimLines is lines typed in Neovim: lines [first, last) of its text
// became text, Neovim's seq-th change.
type NvimLines = LineEdit & { seq: number };

function useNvim() {
  return settings.vim && settings.neovim && !nvimFailed;
}

// nvimStarting is startNvim's promise while a Neovim starts.
let nvimStarting: Promise<void> | null = null;

function startNvim(): Promise<void> {
  if (nvimStarting) return nvimStarting;
  if (nvim || doc.file === "") return Promise.resolve();
  const p = startNvimNow().finally(() => {
    if (nvimStarting === p) nvimStarting = null;
  });
  nvimStarting = p;
  return p;
}

async function startNvimNow() {
  const pane = new NvimPane($("source-editor"), nvimKey);
  nvim = pane;
  $("source-editor").classList.add("nvim");
  try {
    const { text, version, buffer, seq } = await pane.start();
    if (nvim !== pane) return;
    const sync = new Synced(text, version);
    // A Neovim already running may have typing the file doesn't.
    if (buffer !== text) sync.typed(diff(text, buffer));
    nvimDoc = { sync, text: textOf(buffer), seq, owed: null, applying: false, queue: [], waiters: [] };
    await nvimReady();
  } catch (err) {
    stopNvim();
    nvimFailed = true;
    status("Neovim: " + message(err) + "; using the built-in Vim");
    if (view.split) openSource();
  }
}

function stopNvim() {
  if (!nvim) return;
  dropNvim();
  void App.NvimStop();
}

// dropNvim takes the pane down. What was typed still goes to Go.
function dropNvim() {
  nvim?.destroy();
  nvim = null;
  sourceFoldsFor = "";
  $("source-editor").classList.remove("nvim");
  const d = nvimDoc;
  nvimDoc = null;
  if (!d) return;
  const finish = (): Promise<void> =>
    inTurn(async () => {
      if (running && d.sync.pending) return void window.setTimeout(() => void finish(), 1000);
      await sendNvim(d).catch(fail);
    });
  void finish();
}

// applyNvim is applySource for Neovim.
async function applyNvim(d: NvimDoc) {
  if (!d.sync.pending) return;
  if (running) {
    sourceTimer = window.setTimeout(() => void applySource(), 1000);
    return;
  }
  try {
    await sendNvim(d);
    status("");
  } catch (err) {
    fail(err);
    return;
  }
  $("dirty").hidden = false;
  try {
    updateDoc(await drawnBodies(await relayout()));
  } catch (err) {
    fail(err);
    return;
  }
  markRunLines();
}

// sendNvim hands Go what was typed in Neovim, merging Go's changes into
// it first whenever Go has some Neovim hasn't seen.
async function sendNvim(d: NvimDoc) {
  await sendSync(d.sync, (changes) => oweNvim(d, changes));
}

// mergeNvim brings d up to Go's text, as l says it changed, keeping what
// was typed, and gives Neovim Go's changes as edits to the lines they
// change, so keys typed elsewhere meanwhile stay.
async function mergeNvim(d: NvimDoc, l: LinesSince) {
  await oweNvim(d, await syncFrom(d.sync, l));
}

// oweNvim notes changes, on sync's text, that Neovim is to make, and
// gives them to it.
async function oweNvim(d: NvimDoc, changes: ChangeSet) {
  if (!changes.empty) d.owed = d.owed ? d.owed.compose(changes) : changes;
  if (nvimDoc === d) await giveNvim(d);
}

// giveNvim gives Neovim what it owes, as line edits. When Neovim has
// typing the page hasn't heard yet, it sends that instead of taking the
// edits, and once the page has it, giveNvim tries again.
async function giveNvim(d: NvimDoc) {
  for (let tries = 0; d.owed && !d.owed.empty; tries++) {
    if (tries === 5) throw new Error("Neovim keeps changing; try again");
    const owed = d.owed;
    const ok = await toNvim(d, owed);
    if (ok) d.owed = null;
    else await waitNvim(d, d.seq + 1);
  }
}

// toNvim makes changes, on d.text, in Neovim, and reports whether it
// took them.
async function toNvim(d: NvimDoc, changes: ChangeSet): Promise<boolean> {
  const seq = d.seq;
  d.applying = true;
  let ok = false;
  try {
    ok = await App.NvimApply(seq, lineEditsOf(d.text, changes));
  } finally {
    d.applying = false;
  }
  if (ok) {
    d.text = changes.apply(d.text);
    d.seq = seq + 1;
  }
  drainNvim(d);
  return ok;
}

// typedNvim notes lines typed in Neovim. Lines that come while an edit
// is going to Neovim, or ahead of their turn, wait.
function typedNvim(l: NvimLines) {
  const d = nvimDoc;
  if (!d) return;
  d.queue.push(l);
  if (!d.applying) drainNvim(d);
}

// drainNvim takes in the lines Neovim sent, in turn: they go to sync as
// typed, past what Neovim owes, and what it owes moves past them.
function drainNvim(d: NvimDoc) {
  d.queue.sort((a, b) => a.seq - b.seq);
  let typed = false;
  while (d.queue.length && d.queue[0].seq <= d.seq + 1) {
    const l = d.queue.shift()!;
    if (l.seq <= d.seq) continue;
    const t = eventChanges(d.text, l);
    d.sync.typed(d.owed ? t.map(d.owed, true) : t);
    if (d.owed) d.owed = d.owed.map(t);
    d.text = t.apply(d.text);
    d.seq = l.seq;
    typed = true;
  }
  if (!typed) return;
  for (const w of d.waiters.splice(0)) w();
  void applySource();
}

// waitNvim waits until the page has heard Neovim's seq-th change, or
// gives up after a moment.
function waitNvim(d: NvimDoc, seq: number): Promise<void> {
  if (d.seq >= seq) return Promise.resolve();
  return new Promise((resolve) => {
    const timer = window.setTimeout(resolve, 2000);
    const check = () => {
      if (d.seq >= seq || nvimDoc !== d) {
        clearTimeout(timer);
        resolve();
      } else d.waiters.push(check);
    };
    d.waiters.push(check);
  });
}

// nvimCall calls fn in Neovim's mdsh.lua.
async function nvimCall<T = unknown>(fn: string, ...args: unknown[]): Promise<T | null> {
  if (!nvimDoc) return null;
  return JSON.parse(await App.NvimCall(fn, JSON.stringify(args))) as T;
}

// NvimState is Neovim's mode and where the cursor is: the line from 1
// and the column in bytes, and in visual mode the selection's other end.
// seq is Neovim's count of its changes once what was typed is sent.
type NvimState = { seq: number; mode: string; line?: number; col?: number; vline?: number; vcol?: number };

// nvimState is Neovim's state, once the page has what was typed.
async function nvimState(): Promise<NvimState | null> {
  try {
    const st = await nvimCall<NvimState>("state");
    if (st && nvimDoc) await waitNvim(nvimDoc, st.seq);
    return st;
  } catch (err) {
    fail(err);
    return null;
  }
}

// nvimLine is the line Neovim's cursor is on, once what was typed is
// applied, or 0.
async function nvimLine(): Promise<number> {
  const st = await nvimState();
  await applySource();
  return st?.line ?? 0;
}

// nvimReady gives a Neovim that just started the settings and the
// file's folds and run buttons. Its language server does the rest.
async function nvimReady() {
  await nvimSettings();
  markRunLines();
  if (sourceFoldsFor !== doc.file) {
    sourceFoldsFor = doc.file;
    await nvimCall("set_folds", renderedFoldLines(true)).catch(fail);
  }
}

// nvimSettings gives Neovim the spell check setting and the theme's
// colors, which it uses unless the user keeps their own.
async function nvimSettings() {
  await nvimCall("set_spell", !settings.noSpellCheck).catch(() => {});
  await nvimCall("set_colors", nvimColors(), !settings.neovimOwnColors).catch(() => {});
}
// The page's colors change with the Theme and Look settings and, when
// the Theme follows it, the system's theme: applyTheme says so.
window.addEventListener("mdsh-look", () => void nvimSettings());

// insertNvim runs a markdown command, such as the Insert menu's, on
// Neovim's text at its cursor or selection, and gives Neovim the result
// and the selection the command made.
async function insertNvim(cmd: StateCommand) {
  const st = await nvimState();
  const d = nvimDoc;
  if (!st || !d || st.line === undefined) return;
  const line = st.line;
  await inTurn(async () => {
    try {
      // What Go changed goes first, so the command acts on what Neovim
      // shows.
      await giveNvim(d);
      if (nvimDoc !== d) return;
      const doc0 = d.text;
      const head = offsetOf(doc0, line, st.col ?? 0);
      let anchor = st.vline !== undefined ? offsetOf(doc0, st.vline, st.vcol ?? 0) : head;
      let to = head;
      // Visual mode takes in the character under the cursor.
      if (anchor !== head) {
        const [lo, hi] = anchor < head ? [anchor, head] : [head, anchor];
        anchor = lo;
        to = Math.min(hi + 1, doc0.length);
      }
      let state = stateOf(doc0.toString(), anchor, to);
      let changes = ChangeSet.empty(doc0.length);
      cmd({
        state,
        dispatch: (tr) => {
          changes = changes.compose(tr.changes);
          state = tr.state;
        },
      });
      if (changes.empty) return;
      if (!(await toNvim(d, changes))) return status("Neovim changed meanwhile; try again");
      // To Go, the command is typing.
      d.sync.typed(d.owed ? changes.map(d.owed, true) : changes);
      if (d.owed) d.owed = d.owed.map(changes);
      const sel = state.selection.main;
      const a = posOf(state.doc, sel.from);
      const b = posOf(state.doc, sel.to);
      await nvimCall("select", a.line, a.col, b.line, b.col);
    } catch (err) {
      fail(err);
    }
  });
  nvim?.focus();
  void applySource();
}

// nvimKey takes the app's own keys from Neovim: cmd/ctrl+enter runs the
// block the cursor is in, with alt in a side shell, and with shift its
// section, as in the built-in editor. The menu then doesn't run it a
// second time.
function nvimKey(e: KeyboardEvent): boolean {
  if (e.key !== "Enter" || !(e.metaKey || e.ctrlKey) || (e.altKey && e.shiftKey)) return false;
  void (e.shiftKey ? runCurrentSection() : runCurrent(e.altKey));
  return true;
}

// nvimEvent is something Neovim's buffer reports.
async function nvimEvent(name: string, data: any) {
  switch (name) {
    case "cursor":
      return markCurrent(data.line);
    case "scroll":
      return syncScroll("source", data.line);
    case "run":
      if (data.what === "block") return runCurrent();
      if (data.what === "section") return runCurrentSection();
      await nvimState();
      return runLine(data.line);
  }
}

Events.On("nvim-redraw", mine<unknown[][]>((ev) => nvim?.redraw(ev)));
Events.On("nvim-lines", mine<NvimLines>(typedNvim));
Events.On(
  "nvim-event",
  mine<{ name: string; data: string }>((ev) => {
    if (nvimDoc) void nvimEvent(ev.name, JSON.parse(ev.data)).catch(fail);
  }),
);
// :w in Neovim.
Events.On(
  "nvim-write",
  mine<number>(async (seq) => {
    const d = nvimDoc;
    if (!d) return;
    await waitNvim(d, seq);
    await actions.save();
  }),
);
Events.On(
  "nvim-exit",
  mine<string>((err) => {
    if (!nvim) return;
    dropNvim();
    status(err ? "Neovim quit: " + err : "Neovim quit");
    if (view.split) toggle("split", false);
  }),
);

// markRunLines puts a run button on each runnable block's first line,
// and on each heading whose section has one.
function markRunLines() {
  const lines = cells.filter((c) => c.block >= 0).map((c) => c.line);
  const sections = cells.filter((c) => c.heading && c.sectionTo).map((c) => c.line);
  source?.view.dispatch({ effects: setRunLines.of({ blocks: lines, sections }) });
  if (nvimDoc) void nvimCall("set_runs", lines, sections).catch(() => {});
  maskSource();
}

// maskSource masks in the source pane what the rendered view masks: a
// secret block's code, and the output of a block whose output may hold
// a secret, unless its eye button shows it.
function maskSource() {
  const hidden = cells.filter((c) => c.block >= 0 && c.secret && !revealed.has(c.block));
  const code = hidden.filter((c) => c.secretCode).map((c) => c.line);
  const output = hidden.filter((c) => c.outputLine).map((c) => c.outputLine!);
  source?.view.dispatch({ effects: setSecretLines.of({ code, output }) });
  if (nvimDoc) void nvimCall("set_secrets", code, output).catch(() => {});
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
  if (nvimDoc) {
    const line = await nvimLine();
    return runSection(line ? sectionCell(line) : sectionCell());
  }
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
async function runCurrent(side = false) {
  if (running || !doc.file) return;
  const method = side ? "RunBlockSide" : "RunBlock";
  if (nvimDoc) {
    const line = await nvimLine();
    const c = nvimDoc && line ? blockAtLine(nvimDoc.sync.base.toString(), line) : undefined;
    if (c) await start(method, c.block);
    else status("the cursor isn't in a block that runs");
    return;
  }
  if (!source) {
    const all = blocks();
    if (selected >= 0 && selected < all.length) await start(method, all[selected].block);
    else status("no block selected");
    return;
  }
  await applySource();
  const cm = source.view.state;
  const c = blockAtLine(cm.doc.toString(), cm.doc.lineAt(cm.selection.main.head).number);
  if (c) await start(method, c.block);
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

// cellAt is the cell holding line, or the last one before it. The
// cells are in the order of their lines, so the search halves them each
// step: going through every one, as each scroll of the split view does,
// is too slow on a long document.
function cellAt(line: number): Cell | null {
  let lo = 0;
  let hi = cells.length;
  while (lo < hi) {
    const mid = (lo + hi) >> 1;
    if (cells[mid].line > line) hi = mid;
    else lo = mid + 1;
  }
  return lo > 0 ? cells[lo - 1] : null;
}

// currentCell is the cell the source pane's cursor is in.
let currentCell: Cell | null = null;

// markCurrent marks the cell the source pane's cursor is in. A block
// the cursor is in becomes the selected one, so the rendered view, the
// outline and enter all go by the same block.
function markCurrent(line: number) {
  // The cursor moves with every key typed, mostly on the same line.
  const mark = [line, drawn, selected, selectedSection].join(" ");
  if (mark === marked) return;
  for (const e of document.querySelectorAll("#doc > .current")) e.classList.remove("current");
  const c = cellAt(line);
  currentCell = c;
  if (c) cardOf.get(c)?.classList.add("current");
  if (c && renumbered(c)) select(blocks().indexOf(c), false);
  else if (c?.heading && c.line === line) selectSection(c);
  marked = [line, drawn, selected, selectedSection].join(" ");
}

// marked is what markCurrent last marked: the line, which drawing of
// the cards, and the selection it left.
let marked = "";

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

// lead is the pane the user last scrolled, clicked or typed in. Only it
// leads: the other pane also scrolls when the app draws it again, as
// after typing, or when a selected card changes, and WebKit on macOS
// tells of that, which must not move the pane the user is in.
let lead: "source" | "doc" = "doc";
function leadFrom(pane: "source" | "doc") {
  return () => void (lead = pane);
}

// For Neovim, nvimTop is the source pane's top line, when it scrolled.
function syncScroll(from: "source" | "doc", nvimTop?: number) {
  if (from === "doc" && nudging) return;
  if (!(source || nvimDoc) || !view.split || (scrolling && scrolling !== from)) return;
  if (source && from !== lead) return;
  scrolling = from;
  clearTimeout(scrollingTimer);
  scrollingTimer = window.setTimeout(() => (scrolling = null), 150);
  const pane = $("doc-scroll");
  // The pane is positioned, for the dependency lines, so a cell's
  // offsetTop is from the pane's top.
  const toCell = (line: number) => {
    const c = cellAt(line);
    const i = c ? cellIndex.get(c) : undefined;
    if (i !== undefined) scrollToCell(i);
  };
  if (!source) {
    if (from === "source") {
      if (nvimTop) toCell(nvimTop);
      return;
    }
    const i = indexAt(pane.scrollTop);
    if (i >= 0) void nvimCall("jump", cells[i].line, 0, false, 0).catch(() => {});
    return;
  }
  const cm = source.view;
  if (from === "source") {
    toCell(cm.state.doc.lineAt(cm.lineBlockAtHeight(cm.scrollDOM.scrollTop).from).number);
    return;
  }
  const i = indexAt(pane.scrollTop);
  if (i < 0) return;
  const line = Math.min(cells[i].line, cm.state.doc.lines);
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
  cells.forEach((c, i) => {
    if (hidden[i]) return;
    const box = c.textbox ?? c.choice;
    if (box) {
      // An input whose var= binds nothing is marked !.
      const taken = !!box.problem;
      const item = outlineItem(box.id, level + 1, c, "step input" + (taken ? " bad" : ""), taken ? "!" : "", c.choice ? "choice" : "textbox");
      items.push(outlineRow(item, c, 0));
      return;
    }
    if (renumbered(c)) {
      const card = cardOf.get(c);
      const mark = card ? blockMark(card) : cellMark(c);
      const name = cellName(c);
      const item = outlineItem(name, level + 1, c, "step" + markClass(mark), mark, "mdsh", blockTitle(name));
      if (mark === "▶") item.title = name + " (running now)";
      else if (mark === "◐") item.title = name + " (running in the background)";
      const block = c.block;
      const play = iconButton("play", hinted(playHint, "Enter"), (e) => start(e.altKey ? "RunBlockSide" : "RunBlock", block));
      play.classList.add("run");
      play.disabled = running || !!noRun;
      const row = outlineRow(item, c, 0);
      // The block waiting for an answer is ringed, to be found at a glance.
      row.classList.toggle("asking", mark === "?");
      row.append(play);
      row.dataset.block = String(block);
      items.push(row);
      return;
    }
    if (!c.heading) return;
    level = c.heading;
    const item = outlineItem(c.title ?? "", level, c, "heading", "", "heading");
    const key = foldKeys.get(c) ?? "";
    const folded = shownFolded(c);
    // The mark folds the section, in the outline and the view.
    const mark = item.querySelector<HTMLElement>(".mark")!;
    mark.replaceChildren(svg(icons.chevron));
    mark.classList.add("fold");
    mark.classList.toggle("closed", folded);
    mark.title = "Fold or unfold this section";
    // The mark is a mouse target; the heading's own fold button and the
    // Folding menu do the same from the keyboard.
    item.setAttribute("aria-label", item.getAttribute("aria-label") + (folded ? ", folded" : ""));
    mark.onclick = (ev) => {
      ev.stopPropagation();
      foldSection(c);
    };
    // From the keyboard, left folds the section and right unfolds it.
    item.onkeydown = (ev) => {
      if (ev.key !== "ArrowLeft" && ev.key !== "ArrowRight") return;
      ev.preventDefault();
      foldSection(c, ev.key === "ArrowLeft");
    };
    const row = outlineRow(item, c, level);
    row.dataset.section = "";
    if (c.sectionTo) {
      const play = iconButton("play", "Run this section (s)", () => void runSection(c));
      play.classList.add("run");
      play.disabled = running || !!noRun;
      row.append(play);
      row.dataset.section = key;
    }
    items.push(row);
  });
  // Drawing the outline anew would lose the focus; it comes back to the
  // same entry.
  const focused = nav.contains(document.activeElement) ? (document.activeElement as HTMLElement).closest<HTMLElement>(".row")?.dataset.line : undefined;
  nav.replaceChildren(...(items.length ? items : [el("p", { className: "muted" }, "No headings or blocks")]));
  outlineRows = new Map();
  activeRows = [];
  for (const row of items) {
    const key = row.dataset.block !== undefined ? "b" + row.dataset.block : row.dataset.section ? "s" + row.dataset.section : "";
    if (!key) continue;
    const list = outlineRows.get(key) ?? [];
    outlineRows.set(key, list);
    list.push(row);
  }
  if (focused !== undefined) nav.querySelector<HTMLElement>(`.row[data-line="${focused}"] .item`)?.focus();
  markOutline();
}

// Up and down move between the outline's entries.
$("outline").addEventListener("keydown", (e) => {
  if (e.key !== "ArrowUp" && e.key !== "ArrowDown") return;
  const items = [...$("outline").querySelectorAll<HTMLElement>(".item")];
  const i = items.indexOf(document.activeElement as HTMLElement);
  if (i < 0) return;
  e.preventDefault();
  items[Math.max(0, Math.min(items.length - 1, i + (e.key === "ArrowDown" ? 1 : -1)))]?.focus();
});

// markOutline shows the play button on the selected block's or
// heading's entry. Only the entries marked before and the ones to mark
// now are touched, found in outlineRows: going through every entry, on
// each move of the selection, was slow on a long document.
//
// A newly selected entry is scrolled into the outline's view. The
// outline doesn't follow the rendered view's scrolling, and once the
// outline is scrolled by hand it stays there until something else is
// selected.
function markOutline() {
  const b = blocks()[selected]?.block;
  const key = selectedSection ? "s" + selectedSection : b !== undefined ? "b" + b : "";
  const rows = (key && outlineRows.get(key)) || [];
  for (const row of activeRows) {
    if (rows.includes(row)) continue;
    row.classList.remove("active");
    row.querySelector(".item")?.removeAttribute("aria-current");
  }
  for (const row of rows) {
    row.classList.add("active");
    row.querySelector(".item")?.setAttribute("aria-current", "true");
  }
  activeRows = rows;
  if (key !== outlineShown) revealRow(key, rows[0]);
  markDeps();
}

// outlineRows holds the outline's entries by what selects them: "b"
// and a block's number, or "s" and a heading's section.
let outlineRows = new Map<string, HTMLElement[]>();
let activeRows: HTMLElement[] = [];

// outlineShown is what was last scrolled into the outline's view, so that
// drawing the outline again, or selecting the same block again as the
// view redraws, leaves the outline where the user scrolled it.
let outlineShown = "";

// revealRow scrolls the outline just enough to show row, the entry for
// key, once the browser is ready to paint, so the selection doesn't
// wait for the page to be laid out. With the outline hidden, or the
// entry not drawn yet, it waits for the next time the outline is marked.
function revealRow(key: string, row: HTMLElement | undefined) {
  if (!key) {
    outlineShown = "";
    return;
  }
  const nav = $("outline");
  if (!row || nav.hidden || !nav.contains(row)) return;
  outlineShown = key;
  cancelAnimationFrame(revealFrame);
  revealFrame = requestAnimationFrame(() => {
    if (!nav.contains(row)) return;
    const box = nav.getBoundingClientRect();
    const r = row.getBoundingClientRect();
    if (r.top >= box.top && r.bottom <= box.bottom) return;
    const by = r.top < box.top ? r.top - box.top - 8 : r.bottom - box.bottom + 8;
    // A long way goes at once, as the rendered view's jumps do.
    nav.scrollTo({ top: nav.scrollTop + by, behavior: Math.abs(by) > nav.clientHeight ? "auto" : scrollBehavior() });
  });
}

let revealFrame = 0;

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

// cellMark is the mark for block c's state, as its cell has it, for a
// block whose card isn't drawn.
function cellMark(c: Cell): string {
  return c.running ? "◐" : c.failed ? "✗" : c.ran ? "✓" : "";
}

function markClass(mark: string): string {
  return { "✗": " bad", "✓": " ok", "▶": " now", "◐": " bg", "?": " now" }[mark] ?? "";
}

// blockTitle is how the outline shows a block's name: with spaces for
// its underscores, so make_db reads as make db. Its tooltip, and
// everywhere else, keeps the name as written.
function blockTitle(name: string): string {
  return name.replaceAll("_", " ");
}

// outlineRow holds an outline entry for c, with a grip to drag the
// block, or with level the heading's whole section, when it can move.
function outlineRow(item: HTMLElement, c: Cell, level: number): HTMLElement {
  const row = el("div", { className: "row" }, item);
  row.dataset.from = String(c.from);
  // The cell's line, so the dependency rings find its row.
  row.dataset.line = String(c.line);
  if (c.drop) row.dataset.drop = "1";
  if (c.dropAfter >= 0) row.dataset.dropAfter = String(c.dropAfter);
  if (c.dropBefore >= 0) row.dataset.dropBefore = String(c.dropBefore);
  if (level) row.dataset.level = String(level);
  const movable = level ? !!c.drop : !!c.movable;
  if (movable) {
    const title = level ? "Drag to move this section, with all that is in it" : "Drag to move this block";
    row.prepend(grip(title, (ev, handle) => startDrag(ev, handle, row, Number(row.dataset.from), level, "outline")));
  }
  return row;
}

// outlineItem is an outline entry for target, named text, or shown as
// label. Its mark is its state, or a heading's fold, and icon says what
// it is: a block, a heading, a textbox or a choice.
function outlineItem(text: string, level: number, target: Cell, kind: string, mark: string, icon: keyof typeof icons, label = text): HTMLElement {
  const b = el("button", { className: "item " + kind, title: text }, el("span", { className: "mark", ariaHidden: "true" }, mark), el("span", { className: "kind" }, svg(icons[icon])), el("span", { className: "text" }, label));
  b.style.paddingLeft = 8 + (level - 1) * 12 + "px";
  // The mark is said in words, and what the entry is, after its name.
  const what = ({ heading: "heading, level " + level, mdsh: "block", textbox: "textbox", choice: "choice" } as Record<string, string>)[icon] ?? "";
  const states: Record<string, string> = { "?": "waiting for an answer", "▶": "running now", "◐": "running in the background", "✗": "failed", "✓": "ran", "!": "its variable is taken" };
  b.setAttribute("aria-label", [text, what, states[mark] ?? ""].filter(Boolean).join(", "));
  b.onclick = () => jumpTo(target);
  return b;
}

// jumpTo brings target, a cell or a card or what is in one, to the top
// of the rendered view and its markdown to the top of the source pane,
// and selects it if it is a block.
function jumpTo(target: HTMLElement | Cell) {
  const c = target instanceof HTMLElement ? cellOf.get(target.closest<HTMLElement>("#doc > *") ?? target) : target;
  const i = c ? cellIndex.get(c) : undefined;
  if (!c || i === undefined) return;
  reveal(c);
  // Both panes go to the entry, so neither follows the other meanwhile.
  holdScroll();
  const pane = $("doc-scroll");
  const near = cardOf.get(c);
  // A block waiting for an answer shows its question, in the middle.
  const ask = near?.querySelector<HTMLElement>(":scope > .ask");
  if (ask && !ask.hidden) {
    handScrollAt = 0;
    centerAsk(ask, scrollBehavior());
  } else if (near && Math.abs(near.getBoundingClientRect().top - pane.getBoundingClientRect().top) <= pane.clientHeight) {
    near.scrollIntoView({ block: "start", behavior: scrollBehavior() });
  } else {
    // Far, it goes at once, drawing the cards around it.
    scrollToCell(i, 0);
    repaint(pane);
  }
  const k = blocks().indexOf(c);
  if (k >= 0) select(k, false);
  if (c.heading) selectSection(c);
  if (nvimDoc) void nvimCall("jump", c.line, 0, true, 0).catch(fail);
  if (source) {
    const cm = source.view;
    const line = cm.state.doc.line(Math.min(c.line, cm.state.doc.lines));
    cm.dispatch({ selection: { anchor: line.from }, effects: EditorView.scrollIntoView(line.from, { y: "start" }) });
  }
}

// drawRunning lists the blocks running now in the toolbar: the run's
// current step first (or the block waiting for an answer), then the
// ones in the background. A click on one jumps to it.
function drawRunning() {
  // Blocks in folded sections run too. Only the blocks that could be
  // running are looked at, not every one in a long document.
  const asking = [...document.querySelectorAll<HTMLElement>("#doc .block .ask:not([hidden])")].map((a) => a.closest<HTMLElement>(".block")!);
  const some = [...new Set([...document.querySelectorAll<HTMLElement>("#doc .block:is(.now, .background)"), ...asking])];
  some.sort((a, b) => (a.compareDocumentPosition(b) & Node.DOCUMENT_POSITION_FOLLOWING ? -1 : 1));
  const now = some.filter((e) => "?▶".includes(blockMark(e) || "x"));
  const bg = some.filter((e) => blockMark(e) === "◐");
  const chip = (e: HTMLElement) => {
    const mark = blockMark(e);
    const name = cardName(e) || "block " + (Number(e.dataset.block) + 1);
    const why = mark === "?" ? "waiting for an answer" : mark === "▶" ? "running now" : "running in the background";
    const b = el("button", { className: "chip" + markClass(mark), title: name + ": " + why + ". Click to go to it." }, el("span", { className: "mark", ariaHidden: "true" }, mark), name);
    b.setAttribute("aria-label", name + ", " + why + ". Go to it");
    b.onclick = () => jumpTo(e);
    return b;
  };
  const list = $("running");
  list.replaceChildren(...now.map(chip), ...bg.map(chip));
  list.hidden = list.childElementCount === 0;
}

// applyView shows what view says, and keeps it for the next window and,
// for the file's own part, the next time the file opens.
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
  $("outline-toggle").setAttribute("aria-expanded", String(view.outline));
  $("view-split").classList.toggle("on", view.split);
  $("view-split").setAttribute("aria-pressed", String(view.split));
  $("view-rendered").classList.toggle("on", view.rendered);
  $("view-rendered").setAttribute("aria-pressed", String(view.rendered));
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
  rememberView();
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
  // Turning History on starts the shells again and then asks for the
  // prompt, so the bar doesn't ask the shells that are going away.
  if ($("history").hidden) showBar(true, !view.history || doc.history);
  if (view.history === doc.history && $("history").classList.contains("closed") === !view.history) return;
  const had = doc.history;
  doc.history = view.history;
  void showHistory(view.history, had);
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
  // The History transcript's right-click menu.
  historycopy: () => copyHistory(),
  historyselectall: () => selectAllHistory(),
  async save() {
    // Go takes no edits, and saves nothing, while a block runs: what was
    // typed waits for the run to end, and so does the save.
    if (running) {
      saveAfterRun = true;
      status("not saved yet: a block is running; it saves when the block ends");
      return;
    }
    saveAfterRun = false;
    try {
      await nvimState();
      await applySource();
      // Saving what Go has without what it refused would write the file
      // as it was before the typing, and say it saved.
      if (source?.pending || nvimDoc?.sync.pending) throw new Error("not saved: the edits didn't reach the file; save again");
      await App.Save();
      if (nvimDoc) await App.NvimSaved();
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
  // clearoutputs takes all output out of the document, once asked. In
  // the source pane it is one edit, so undo brings the output back.
  async clearoutputs() {
    if (!doc.file || running || $<HTMLDialogElement>("clear-dialog").open) return;
    // What was typed may have taken output out, or put some in.
    await applySource();
    if (!(await App.HasOutput())) return status("no output to clear");
    if (!(await confirmClear())) return;
    try {
      await App.ClearOutputs();
      status("outputs cleared");
    } catch (err) {
      fail(err);
    }
    await refresh();
  },
  runcurrent: () => runCurrent(),
  runside: () => runCurrent(true),
  runsection: () => runCurrentSection(),
  report: () => (doc.file && !running ? openReport() : undefined),
  stop: () => void stopRun(),
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
  depends: () => openDepends(),
  // Edit › Front Matter opens the sheet, in the rendered view.
  frontmatter() {
    if (!doc.file) return;
    if (!view.rendered) toggle("rendered", true);
    return showFrontMatter(!frontMatterOpen(), true);
  },
  history: () => toggle("history"),
  // A session already open in this tab: show History and type there.
  "history-open"() {
    toggle("history", true);
    focusHistory();
  },
  console: () => toggle("console"),
  bugreport: () => void copyBugReport(status),
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
  if (at?.closest(".nvim-input")) return false; // Neovim has its own keys
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
  const i = input != null ? Number(input) : selected >= 0 ? (blocks()[selected]?.block ?? -1) : -1;
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
    const k = blocks().findIndex((b) => b.block === m.block);
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
  if (view.state.doc.toString() !== m.base) await loadSource();
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
// which clears just the output. A block the History pane added is
// scratch work: its trash buttons always show, and it deletes without
// asking, since the bar's Undo is there.

function trashButton(c: Cell): HTMLButtonElement {
  const b = iconButton("trash", "Delete this block (delete)", () => void deleteBlock(c.block));
  b.classList.add("trash", "delete-block");
  markTrash(b, c);
  return b;
}

// markTrash disables the delete button on card (or the button itself)
// while something else, such as another block's depends=, uses the
// block, and says so in its tooltip. It stays clickable, as a disabled
// button might not show its tooltip, and a click says the same in the
// status line.
function markTrash(card: HTMLElement, c: Cell) {
  const b = card.matches("button") ? (card as HTMLButtonElement) : card.querySelector<HTMLButtonElement>(".head button.delete-block");
  if (!b) return;
  b.classList.toggle("off", !!c.undeletable);
  b.setAttribute("aria-disabled", c.undeletable ? "true" : "false");
  // Either key deletes; the hint names the one the keyboard calls delete.
  b.title = c.undeletable || hinted("Delete this block", isMac ? "Backspace" : "Delete");
  b.setAttribute("aria-label", b.title);
}

// clearOutputButton clears the output on the selected card, without
// asking, since the bar's Undo puts it back.
function clearOutputButton(c: Cell): HTMLButtonElement {
  const b = iconButton("trash", "Clear this block's output", () => void clearOutput(c.block));
  b.classList.add("trash", "clear-output");
  return b;
}

// clearEarlierButton clears the earlier runs on the selected card, and
// keeps the current output. The bar's Undo puts them back.
function clearEarlierButton(c: Cell): HTMLButtonElement {
  const b = iconButton("clearEarlier", "Clear earlier runs, keeping this output", () => void clearEarlier(c.block));
  b.classList.add("trash", "clear-earlier");
  return b;
}

async function clearEarlier(i: number) {
  await dropEarlier(() => App.ClearEarlierRuns(i), "Cleared the earlier runs.", "cleared earlier runs");
}

async function deleteEarlier(i: number, k: number) {
  await dropEarlier(() => App.DeleteEarlierRun(i, k), "Deleted an earlier run.", "deleted earlier run");
}

async function dropEarlier(call: () => Promise<void>, said: string, short: string) {
  if (running) return status("can't change the file while a block runs");
  await applySource();
  try {
    await call();
  } catch (err) {
    return fail(err);
  }
  await refresh();
  $("moved-text").textContent = said;
  $("moved-undo").onclick = () => {
    $("moved").hidden = true;
    void undoEdit(false);
  };
  $("moved").hidden = false;
  status(short);
}

// copyEarlier copies earlier run k of block c, as copyOutput copies the
// current output.
async function copyEarlier(c: Cell, k: number, out: HTMLElement, b: HTMLElement) {
  const text = outputText(out).replace(/\n$/, "");
  if (!text) return status("no text output to copy");
  try {
    await setClipboard(text, !!c.secret);
  } catch (err) {
    return status("couldn't copy: " + message(err));
  }
  const n = text.split("\n").length;
  const name = c.name ?? c.id ?? "";
  status(`Copied run ${k} back of ` + (name ? name + ", " : "") + n + (n === 1 ? " line" : " lines") + (c.secret ? secretNote() : ""));
  b.classList.add("copied");
  clearTimeout(copiedTimers.get(b));
  copiedTimers.set(b, setTimeout(() => b.classList.remove("copied"), 1500));
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

// deleteBlock asks whether to delete block i, unless the History pane
// added it, and does.
async function deleteBlock(i: number) {
  if (running) return status("can't change the file while a block runs");
  const c = cells.find((c) => c.block === i);
  if (!c?.movable) return status("only top-level blocks can be deleted");
  if (c.undeletable) return status(c.undeletable);
  const id = c.name ?? c.id ?? "";
  if (!c.history && !(await confirmDelete(id ? "#" + id : `the block at line ${c.line}`))) return;
  await applySource();
  const k = blocks().findIndex((b) => b.block === i);
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

// fixWarning asks whether to make the fix the console offers for
// warning, saying what it does, and makes it, saying done, if given.
async function fixWarning(warning: string, done?: string) {
  if (running) return status("can't change the file while a block runs");
  await applySource();
  let f;
  try {
    f = await App.DescribeFix(warning);
  } catch (err) {
    return fail(err);
  }
  const dialog = $<HTMLDialogElement>("fix-dialog");
  $("fix-title").textContent = f.label + "?";
  $("fix-detail").textContent = f.detail;
  $("fix-ok").textContent = f.label;
  dialog.returnValue = "";
  dialog.showModal();
  $("fix-ok").focus();
  const ok = await new Promise<boolean>((resolve) => (dialog.onclose = () => resolve(dialog.returnValue === "fix")));
  if (!ok) return;
  try {
    await App.FixWarning(warning);
  } catch (err) {
    return fail(err);
  }
  await refresh();
  $("moved-text").textContent = done ?? `Fixed: ${warning}.`;
  $("moved-undo").onclick = () => {
    $("moved").hidden = true;
    void undoEdit(false);
  };
  $("moved").hidden = false;
  status(done ?? "fixed: " + warning);
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

// confirmClear asks whether to clear all outputs.
function confirmClear(): Promise<boolean> {
  const dialog = $<HTMLDialogElement>("clear-dialog");
  dialog.returnValue = "";
  dialog.showModal();
  $("clear-ok").focus();
  return new Promise((resolve) => (dialog.onclose = () => resolve(dialog.returnValue === "clear")));
}

// Dragging
//
// A card's grip drags its block, with its output, to a spot between
// top-level cells; the outline's grips drag a block, or a heading's whole
// section, between its entries. A section goes only before a heading of
// its level or higher, or to the end. Neither goes where a block would
// land above one its depends= names: the line stops at the last spot
// that keeps them in order, and the status line says why. A line shows
// the spot; near the top or bottom edge the pane scrolls; Esc cancels.

function grip(title: string, start: (ev: PointerEvent, handle: HTMLElement) => void): HTMLElement {
  const g = el("button", { className: "grip", title, tabIndex: -1 }, "⋮⋮");
  g.setAttribute("aria-label", title);
  g.onpointerdown = (ev) => {
    if (ev.button === 0) start(ev, g);
  };
  g.onclick = (ev) => ev.stopPropagation();
  return g;
}

interface Spot {
  at: number; // a cell's From, or -1 for the end
  y: () => number; // where the line goes, in the viewport
  blocked?: "above" | "below"; // a depends= keeps the move from here
}

// spots are where a drag in pane can drop: before each row or card
// marked drop, and at the end. For a section (level > 0), only before
// headings of its level or higher. Each says whether a block would land
// above one it depends on there, against item's drop limits (see
// Cell.dropAfter): "above" if the moved blocks would go above one they
// depend on, "below" if below one that depends on them.
function spots(pane: "doc" | "outline", level: number, item: HTMLElement): Spot[] {
  const after = Number(item.dataset.dropAfter ?? -1);
  const before = Number(item.dataset.dropBefore ?? -1);
  const blocked = (at: number): Spot["blocked"] => (at >= 0 && at <= after ? "above" : before >= 0 && (at < 0 || at > before) ? "below" : undefined);
  const rows = pane === "doc" ? [...$("doc").children] : [...$("outline").querySelectorAll<HTMLElement>(":scope > .row")];
  const shown = (rows as HTMLElement[]).filter((e) => e.offsetParent !== null);
  const out: Spot[] = [];
  for (const e of shown) {
    if (e.dataset.drop !== "1") continue;
    if (level && !(Number(e.dataset.level ?? 0) > 0 && Number(e.dataset.level) <= level)) continue;
    const at = Number(e.dataset.from);
    out.push({ at, y: () => e.getBoundingClientRect().top, blocked: blocked(at) });
  }
  const last = shown[shown.length - 1];
  if (last) out.push({ at: -1, y: () => last.getBoundingClientRect().bottom, blocked: blocked(-1) });
  return out;
}

function startDrag(ev: PointerEvent, handle: HTMLElement, item: HTMLElement, from: number, level: number, pane: "doc" | "outline") {
  if (running) return void status("can't change the file while a block runs");
  ev.preventDefault();
  const scroller = pane === "doc" ? $("doc-scroll") : $("outline");
  // The rendered view draws other cards as it scrolls, so the spots are
  // found again each time the line moves.
  let all = spots(pane, level, item);
  let open = all.filter((s) => !s.blocked);
  if (!open.length) return;
  handle.setPointerCapture(ev.pointerId);
  const line = el("div", { className: "drop-line", hidden: true });
  document.body.append(line);
  const startY = ev.clientY;
  let y = ev.clientY;
  let dragging = false;
  let spot: Spot | null = null;
  let warned: Spot["blocked"]; // the limit the status line says, if any
  let frame = 0;
  const place = () => {
    if (pane === "doc") {
      all = spots(pane, level, item);
      open = all.filter((s) => !s.blocked);
    }
    const box = scroller.getBoundingClientRect();
    const nearest = (list: Spot[]) => {
      let near: Spot | null = null;
      let best = Infinity;
      for (const s of list) {
        const d = Math.abs(s.y() - y);
        if (d < best) [best, near] = [d, s];
      }
      return near as Spot | null;
    };
    // Past a depends= limit the line stays at the last spot short of
    // it, and the status line says why.
    const want = nearest(all);
    spot = nearest(open);
    const why = want?.blocked;
    if (why !== warned) {
      status(why === "below" ? "can't go below a block that depends on it" : why === "above" ? "can't go above a block it depends on" : "");
      warned = why;
    }
    line.classList.toggle("held", !!why);
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
    if (warned) status("");
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
    const k = blocks().findIndex((b) => b.block === m.block);
    if (k >= 0) select(k);
  }
  const moved = cells.find((c) => c.line === m.line);
  if (moved) cardFor(moved)?.scrollIntoView({ block: "nearest" });
  showMoved(m, () => undoEdit(false));
}

// undoEdit is Edit › Undo, or Redo. Where the focus is decides what it
// takes back: the source view keeps its own history (Neovim's too), a
// text field its own, and everything else is the rendered view's, whose edits (tasks,
// textboxes and choices) Go keeps.
async function undoEdit(again: boolean) {
  const at = document.activeElement as HTMLElement | null;
  if (nvim && at?.closest(".nvim-input")) {
    await App.NvimInput(again ? "<Cmd>redo<CR>" : "<Cmd>undo<CR>").catch(fail);
    return;
  }
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

// Depends on…
//
// Edit › Depends On… and d open a dialog for the selected block (or in
// the source view the block the cursor is in) listing the blocks above
// it that can be depended on, with its depends= checked, in order.
// Checking adds to the end; Apply writes depends= as dragging a
// dependency line does, so Undo takes it back. It is the keyboard's
// way to the lines.

async function openDepends() {
  if (!doc.file || running) return;
  let i = -1;
  const at = document.activeElement as HTMLElement | null;
  if (nvimDoc && at?.closest(".nvim-input")) {
    const line = await nvimLine();
    i = line ? (blockAtLine(nvimDoc.sync.base.toString(), line)?.block ?? -1) : -1;
  } else if (source && at?.closest(".cm-editor")) {
    await applySource();
    if (!source) return;
    const cm = source.view.state;
    i = blockAtLine(cm.doc.toString(), cm.doc.lineAt(cm.selection.main.head).number)?.block ?? -1;
  } else if (selected >= 0) i = blocks()[selected]?.block ?? -1;
  const c = cells.find((c) => c.block === i);
  if (!c || c.textbox || c.choice) return status("select a block that runs first");
  const canRun = (x: Cell) => x.block >= 0 && !!x.id && !x.textbox && !x.choice;
  const above = cells.filter((x) => canRun(x) && x.block < i);
  const byID = new Map(above.map((x) => [x.id!, x.block]));
  const chosen = (c.depends ?? []).map((id) => byID.get(id)).filter((b): b is number => b !== undefined);
  $("depends-name").textContent = c.name ?? c.id ?? "This block";
  const list = $("depends-list");
  list.replaceChildren(
    ...above.map((x) => {
      const box = el("input", { type: "checkbox", checked: chosen.includes(x.block) });
      box.dataset.block = String(x.block);
      box.onchange = () => {
        const k = chosen.indexOf(x.block);
        if (box.checked && k < 0) chosen.push(x.block);
        else if (!box.checked && k >= 0) chosen.splice(k, 1);
        numberDepends(chosen);
      };
      return el("label", { className: "check" }, box, el("span", { className: "order", ariaHidden: "true" }), el("code", {}, x.name ?? x.id ?? ""));
    }),
  );
  numberDepends(chosen);
  $("depends-empty").hidden = above.length > 0;
  $<HTMLButtonElement>("depends-ok").disabled = above.length === 0;
  const dialog = $<HTMLDialogElement>("depends-dialog");
  dialog.returnValue = "";
  dialog.showModal();
  (list.querySelector<HTMLElement>("input") ?? $("depends-ok")).focus();
  dialog.onclose = () => {
    if (dialog.returnValue !== "ok") return;
    const before = (c.depends ?? []).join(",");
    if (chosen.map((b) => cells.find((x) => x.block === b)?.id ?? "").join(",") === before) return;
    void setDepends(i, chosen, chosen.length ? "now depends on " + chosen.map((b) => cells.find((x) => x.block === b)?.name ?? "").join(", ") : "depends on nothing now");
  };
}

// numberDepends shows each checked block's place in depends=, and says
// it in the checkbox's name.
function numberDepends(chosen: number[]) {
  for (const label of $("depends-list").querySelectorAll<HTMLElement>("label")) {
    const box = label.querySelector<HTMLInputElement>("input")!;
    const k = chosen.indexOf(Number(box.dataset.block));
    label.querySelector(".order")!.textContent = k >= 0 ? String(k + 1) + "." : "";
    box.setAttribute("aria-label", (label.querySelector("code")?.textContent ?? "") + (k >= 0 ? `, ${k + 1} of ${chosen.length}` : ""));
  }
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

let startScreen: Start = { dir: "", home: "", files: [], collections: [], full: false, recent: [], pinned: [], lastSession: "", writable: [], outside: [] };

async function drawStart() {
  try {
    startScreen = await App.StartScreen();
    $("start-dir").textContent = tilde(startScreen.dir) + (startScreen.full ? " (showing the first " + (startScreen.files ?? []).length + " files)" : "");
  } catch (err) {
    startScreen = { dir: "", home: "", files: [], collections: [], full: false, recent: [], pinned: [], lastSession: "", writable: [], outside: [] };
    $("start-dir").textContent = message(err);
  }
  const last = $<HTMLButtonElement>("last-session");
  last.disabled = !startScreen.lastSession;
  last.title = startScreen.lastSession ? "Open " + tilde(startScreen.lastSession) : "No sessions yet: New Session makes one";
  filterStart();
  $("filter").focus();
}

type StartFile = NonNullable<Start["files"]>[number];
type Collection = NonNullable<Start["collections"]>[number];

function fileRow(path: string, label: string, detail: string, desc = "", tags: string[] = []): HTMLElement {
  const name = el("span", { className: "fname" }, label);
  if (tags.length) name.append(el("span", { className: "ftag" }, tags.map((t) => "#" + t).join(" ")));
  if ((startScreen.outside ?? []).includes(path)) name.append(el("span", { className: "fbadge", title: "Allowed to reach files outside its folder" }, "outside"));
  const left = desc ? el("span", { className: "fleft" }, name, el("span", { className: "fdesc muted", title: desc }, desc)) : name;
  const b = el("button", { className: "file", title: path }, left, el("span", { className: "muted" }, detail));
  b.onclick = () => openPath(path);
  return el("li", {}, b, pinButton(path, label));
}

const isPinned = (path: string) => (startScreen.pinned ?? []).includes(path);

// pinButton pins path to the top of the start screen, or unpins it.
function pinButton(path: string, label: string): HTMLButtonElement {
  const on = isPinned(path);
  const b = iconButton("pin", (on ? "Unpin " : "Pin ") + label, () => void setPinned(path, !on));
  b.classList.add("pin");
  b.classList.toggle("pinned", on);
  b.title = on ? "Unpin" : "Pin to the top";
  b.setAttribute("aria-pressed", String(on));
  b.dataset.path = path;
  return b;
}

async function setPinned(path: string, pin: boolean) {
  // Keep the keyboard on the button, which is drawn again where it was clicked.
  const section = document.activeElement?.closest("#pinned-list, #recent-list, #file-list")?.id;
  try {
    await App.Pin(path, pin);
  } catch (err) {
    $("start-dir").textContent = message(err);
    return;
  }
  const pinned = (startScreen.pinned ?? []).filter((p) => p !== path);
  startScreen.pinned = pin ? [...pinned, path] : pinned;
  filterStart();
  announce(pin ? "Pinned" : "Unpinned");
  const again = [...document.querySelectorAll<HTMLButtonElement>(`#${section ?? "file-list"} button.pin`)].find((b) => b.dataset.path === path);
  (again ?? $("filter")).focus();
}

// startFilter splits the filter box into #tags, which a file must all
// have, and the rest, which must appear in its name, description, tags
// or its collection's title or description.
function startFilter(): { tags: string[]; text: string } {
  const words = $<HTMLInputElement>("filter").value.trim().toLowerCase().split(/\s+/).filter((w) => w !== "");
  const isTag = (w: string) => w.length > 1 && w.startsWith("#");
  return { tags: words.filter(isTag).map((w) => w.slice(1)), text: words.filter((w) => !isTag(w)).join(" ") };
}

// toggleTag adds #tag to the filter box, or takes it out.
function toggleTag(tag: string) {
  const box = $<HTMLInputElement>("filter");
  const words = box.value.trim().split(/\s+/).filter((w) => w !== "");
  const i = words.findIndex((w) => w.toLowerCase() === "#" + tag);
  if (i < 0) words.push("#" + tag);
  else words.splice(i, 1);
  box.value = words.join(" ");
  filterStart();
}

function tagButton(tag: string, on: boolean, count?: number): HTMLElement {
  const b = el("button", { className: "tag" + (on ? " on" : ""), title: (on ? "Stop filtering on #" : "Show files tagged #") + tag }, "#" + tag);
  b.setAttribute("aria-pressed", String(on));
  if (count !== undefined) {
    b.append(el("span", { className: "n", ariaHidden: "true" }, String(count)));
    b.setAttribute("aria-label", `${tag}, ${count} ${count === 1 ? "file" : "files"}`);
  }
  b.onclick = () => toggleTag(tag);
  return b;
}

// writable is true for a listed folder whose .mdsh.yaml can be saved.
const writable = (dir: string) => (startScreen.writable ?? []).includes(dir);

// folderName is the last part of a listed folder's path.
function folderName(dir: string): string {
  return dir === "" ? startScreen.dir.split(/[\\/]/).pop() || startScreen.dir : dir.slice(dir.lastIndexOf("/") + 1);
}

function dirOf(rel: string): string {
  const i = rel.lastIndexOf("/");
  return i < 0 ? "" : rel.slice(0, i);
}

// collectionHeader shows a collection's icon, title, description and
// tags, in its color and over its background image.
function collectionHeader(c: Collection, dir: string, active: string[]): HTMLElement {
  const title = c.title || folderName(dir);
  const icon = el("div", { className: "cicon" });
  if (c.iconURL) icon.append(el("img", { src: c.iconURL, alt: "" }));
  else icon.textContent = c.icon || [...title][0]?.toUpperCase() || "";
  const text = el("div", { className: "ctext" }, el("div", { className: "ctitle" }, el("span", {}, title), el("span", { className: "muted" }, dir === "" ? "this folder" : dir + "/")));
  if (c.description) text.append(el("div", { className: "cdesc muted" }, c.description));
  if (c.tags?.length) text.append(el("div", { className: "ctags" }, ...c.tags.map((t) => tagButton(t, active.includes(t)))));
  const head = el("div", { className: "chead" }, icon, text);
  if (writable(dir)) head.append(folderGear(dir, title, drawStart));
  if (c.backgroundURL) {
    head.classList.add("bg");
    head.style.setProperty("--img", `url("${c.backgroundURL}")`);
  }
  if (c.problems?.length) head.title = ".mdsh.yaml: " + c.problems.join("\n");
  return head;
}

function filterStart() {
  const { tags, text } = startFilter();
  const has = (s: string | undefined) => text === "" || (s ?? "").toLowerCase().includes(text);
  const collections = new Map<string, Collection>();
  const broken = new Map<string, string>();
  for (const c of startScreen.collections ?? []) {
    if (c.error) broken.set(c.rel, c.error);
    else collections.set(c.rel, c);
  }
  const files = startScreen.files ?? [];
  const byPath = new Map(files.map((f) => [f.path, f]));
  const matches = (f: StartFile) => {
    const ft = f.tags ?? [];
    if (!tags.every((t) => ft.includes(t))) return false;
    const c = collections.get(dirOf(f.rel));
    return has(f.rel) || has(f.description) || ft.some(has) || has(c?.title) || has(c?.description);
  };

  // Pinned and recent files match on their path; with #tags, only listed files that have them.
  const shown = (p: string) => {
    if (tags.length === 0) return has(p);
    const f = byPath.get(p);
    return f !== undefined && matches(f);
  };
  const pathRow = (p: string) => fileRow(p, p.split(/[\\/]/).pop() ?? p, tilde(p.replace(/[\\/][^\\/]*$/, "")));
  const pinned = (startScreen.pinned ?? []).filter(shown);
  $("pinned-list").hidden = pinned.length === 0;
  $("pinned-list").querySelector("ul")!.replaceChildren(...pinned.map(pathRow));
  // A pinned file is listed once, under Pinned.
  const recent = (startScreen.recent ?? []).filter((p) => !isPinned(p) && shown(p));
  $("recent-list").hidden = recent.length === 0;
  $("recent-list").querySelector("ul")!.replaceChildren(...recent.map(pathRow));

  // Every tag in the listing, with how many files have it.
  const counts = new Map<string, number>();
  for (const f of files) for (const t of f.tags ?? []) counts.set(t, (counts.get(t) ?? 0) + 1);
  const bar = $("tag-bar");
  bar.hidden = counts.size === 0;
  bar.replaceChildren(el("span", { className: "muted" }, "Tags"), ...[...counts.keys()].sort().map((t) => tagButton(t, tags.includes(t), counts.get(t))));

  // Group the matching files by folder. A collection whose files are
  // all in subfolders still gets its header.
  const groups = new Map<string, StartFile[]>();
  for (const f of files) {
    if (!matches(f)) continue;
    const dir = dirOf(f.rel);
    if (!groups.has(dir)) groups.set(dir, []);
    groups.get(dir)!.push(f);
    for (let up = dir; up !== ""; ) {
      up = dirOf(up);
      if (collections.has(up) && !groups.has(up)) groups.set(up, []);
    }
  }
  // Folders in path order, each just before its subfolders.
  const key = (dir: string) => dir.replaceAll("/", "\0");
  const dirs = [...groups.keys()].sort((a, b) => (key(a) < key(b) ? -1 : key(a) > key(b) ? 1 : 0));
  const list = $("file-list");
  list.replaceChildren();
  for (const dir of dirs) {
    const c = collections.get(dir);
    const rows = groups.get(dir)!.map((f) => {
      const own = (f.tags ?? []).filter((t) => !c?.tags?.includes(t));
      return fileRow(f.path, f.rel.slice(dir === "" ? 0 : dir.length + 1), blocksLabel(f.blocks) + " · " + ago(f.mod), f.description, own);
    });
    const ul = el("ul", { className: "files" }, ...rows);
    if (c) {
      const section = el("section", { className: "coll" }, collectionHeader(c, dir, tags));
      if (c.color) section.style.setProperty("--c", c.color);
      if (rows.length) section.append(ul);
      list.append(section);
      continue;
    }
    const h = el("h2", {}, dir === "" ? "This folder" : dir + "/");
    const err = broken.get(dir);
    if (err) {
      h.title = err;
      h.append(" ⚠");
    }
    // A folder whose .mdsh.yaml doesn't parse is fixed by hand.
    if (!err && writable(dir)) list.append(el("div", { className: "fhead" }, h, folderGear(dir, folderName(dir), drawStart)), ul);
    else list.append(h, ul);
  }
  $("start-empty").hidden = groups.size > 0 || recent.length > 0 || pinned.length > 0;
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
initFolders();

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
$("new-session").onclick = () => App.NewSession().catch(fail);
$("last-session").onclick = () => App.LastSession().catch(fail);

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

// Run timers. A running block's card shows how long it has run, ticking
// every tenth of a second, so a long block plainly hasn't hung. With
// timeout= it shows the time against the timeout, and a bar under the
// card's head fills toward it, turning to a warning near the end. When
// the block is done the timer stops at how long it took, which the card
// shows from then on, from the cell's ms. timerStarts holds when each
// running block started, by its index, so a card drawn again goes on.
const timerStarts = new Map<number, number>();
let ticker = 0;

function startTimer(block: HTMLElement) {
  timerStarts.set(Number(block.dataset.block), performance.now());
  block.classList.add("timing");
  tick();
  if (!ticker) ticker = window.setInterval(tick, 100);
}

function stopTimer(block: HTMLElement) {
  const i = Number(block.dataset.block);
  const t0 = timerStarts.get(i);
  if (t0 === undefined) return;
  timerStarts.delete(i);
  block.classList.remove("timing", "late", "later");
  restTimer(block.querySelector<HTMLElement>(".head .timer")!, performance.now() - t0, false, Number(block.dataset.timeout ?? 0));
  if (!timerStarts.size) stopTicker();
}

// Each output says when its run started, as "3 mins ago", on its
// title bar beside its tools, with the exact time in its tooltip. A
// ticker keeps them current; it reads each card's cell, which a run
// updates in place, so a card needn't be drawn again for its time.
let atTicker = 0;

function startAtTicker() {
  if (!atTicker) atTicker = window.setInterval(tickAts, 1000);
}

function tickAts() {
  const now = new Date();
  for (const span of document.querySelectorAll<HTMLElement>("#doc .output-at")) {
    const block = span.closest<HTMLElement>(".block");
    const c = block && cellOf.get(block);
    if (block && c) drawAt(block, c, now);
  }
}

// drawAt shows when block's output ran. A block that is running has
// no time to show: the output under way isn't the one the time was for.
function drawAt(block: HTMLElement, c: Cell, now: Date) {
  const span = block.querySelector<HTMLElement>(".output-at");
  if (!span) return;
  const at = c.running || timerStarts.has(c.block) ? "" : (c.at ?? "");
  const text = at ? timeAgo(at, now) : "";
  if (span.textContent !== text) span.textContent = text;
  const title = at ? "Ran " + fullTime(at).replace("\n", "\nat=") : "";
  if (span.title !== title) span.title = title;
  span.hidden = !text;
}

// stopTimers stops the timers of the blocks that aren't running in the
// background, when a run ends, in case a block's done never came.
function stopTimers() {
  for (const i of [...timerStarts.keys()]) {
    const block = document.getElementById("block-" + i);
    if (!block?.classList.contains("background")) {
      if (block) stopTimer(block);
      else timerStarts.delete(i);
    }
  }
  if (!timerStarts.size) stopTicker();
}

function stopTicker() {
  window.clearInterval(ticker);
  ticker = 0;
}

function tick() {
  const now = performance.now();
  for (const [i, t0] of timerStarts) {
    const block = document.getElementById("block-" + i);
    if (block) drawTimer(block, now - t0, true);
  }
}

// drawTimer shows a running block's time, ms, and against its timeout.
function drawTimer(block: HTMLElement, ms: number, live: boolean) {
  const span = block.querySelector<HTMLElement>(".head .timer");
  if (!span) return;
  const limit = Number(block.dataset.timeout ?? 0);
  span.textContent = limit ? runTime(ms) + " / " + limitTime(limit) : runTime(ms);
  span.title = "Running for " + runTime(ms) + (limit ? ", of a " + limitTime(limit) + " timeout" : "");
  span.classList.remove("bad");
  if (limit && live) {
    const f = Math.min(ms / limit, 1);
    block.style.setProperty("--timeout-done", String(f));
    block.classList.toggle("late", f >= 0.75 && f < 0.9);
    block.classList.toggle("later", f >= 0.9);
  }
}

// restTimer shows how long a block that is done took.
function restTimer(span: HTMLElement, ms: number, timedOut: boolean, limit: number) {
  const card = span.closest<HTMLElement>(".block");
  card?.style.removeProperty("--timeout-done");
  if (timedOut) {
    span.textContent = "timed out at " + limitTime(limit || ms);
    span.title = "Timed out after " + runTime(ms);
    span.classList.add("bad");
    return;
  }
  span.textContent = runTime(ms);
  span.title = "Took " + runTime(ms) + (limit ? ", of a " + limitTime(limit) + " timeout" : "");
  span.classList.remove("bad");
}

// runTime says ms to a tenth of a second under a minute, as 12.3s, and
// to the second past it, as 1m 05s or 1h 02m 05s.
function runTime(ms: number): string {
  if (ms < 60_000) return (Math.floor(ms / 100) / 10).toFixed(1) + "s";
  const s = Math.floor(ms / 1000);
  const two = (n: number) => String(n).padStart(2, "0");
  if (s < 3600) return Math.floor(s / 60) + "m " + two(s % 60) + "s";
  return Math.floor(s / 3600) + "h " + two(Math.floor(s / 60) % 60) + "m " + two(s % 60) + "s";
}

// limitTime says a timeout as written, more or less: 30s, 1.5s, 5m, 1h,
// or as runTime does when it doesn't come out even.
function limitTime(ms: number): string {
  const s = ms / 1000;
  if (s < 60) return String(Math.round(s * 10) / 10) + "s";
  if (s % 3600 === 0) return s / 3600 + "h";
  if (s % 60 === 0) return s / 60 + "m";
  return runTime(ms);
}

// liveColors is the ANSI state of each stream of a running block's
// output.
const liveColors = new WeakMap<HTMLElement, Record<string, Ansi>>();

function onBlock(e: BlockEvent) {
  // A background block marked tty has a terminal of its own in its
  // card, apart from the pane a tty block runs in.
  if (e.data === "background" && (e.kind === "terminal" || e.kind === "tty" || e.kind === "terminal-end")) return onBackgroundTerminal(e);
  // The live History pane's shell, and what was typed at its prompt.
  if (e.block === -1 && historyLive(e.kind, e.data, e.bytes)) return;
  if (e.kind === "changed") return changedSoon();
  if (e.kind === "error" && e.block === -1) return status(e.data ?? "");
  // A command typed in the History pane runs there, before its card is
  // drawn.
  if (e.kind === "terminal" && historyTerminal(e.block, e.data)) return;
  if (e.kind === "tty" && historyWrite(e.block, e.bytes)) return;
  if (e.kind === "terminal-end" && historyEnd(e.block)) return;
  const block = blockCard(e.block);
  if (!block) return;
  const out = block.querySelector<HTMLElement>(".output")!;
  switch (e.kind) {
    case "background":
      // The block runs on while others do; done comes when it ends.
      block.querySelector<HTMLElement>(".ask")!.hidden = true;
      block.classList.remove("ran", "failed");
      block.classList.add("background");
      block.querySelector(".exit")!.textContent = backgroundNote;
      setOutput(out);
      startTimer(block);
      break;
    case "started":
      for (const b of document.querySelectorAll("#doc .block.now")) b.classList.remove("now");
      block.classList.add("now");
      closeAsks(cellOf.get(block));
      block.querySelector<HTMLElement>(".ask")!.hidden = true;
      setOutput(out, el("pre", { className: "live" }));
      // The last run's exit would sit by the new run's timer.
      block.querySelector(".exit")!.textContent = "";
      startTimer(block);
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
      if (blockFolded(cellOf.get(block)!, "output")) drawPart(block, "output");
      if (block === following) followOutput();
      break;
    }
    case "done":
      $("dirty").hidden = false;
      block.classList.remove("now");
      stopTimer(block);
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
  if (e.kind !== "stdout" && e.kind !== "stderr") {
    labelBlock(block);
    drawOutline();
  }
}

// changedSoon draws the document again once what was typed at the live
// History pane's prompt has changed it, as a note or a block, unless a
// run of the page's goes, which draws it when it ends.
let changedTimer = 0;
function changedSoon() {
  clearTimeout(changedTimer);
  changedTimer = window.setTimeout(() => {
    if (!running) void refresh();
  }, 50);
}

// jobBadge marks a History block whose command is a job of the live
// shell: stopped, or running on in the background.
function jobBadge(block: number): HTMLElement | "" {
  const j = shellJobs().find((j) => j.block === block);
  if (!j) return "";
  const stopped = /^(stopped|suspended)/i.test(j.state);
  const what = stopped ? "stopped" : j.state.toLowerCase();
  return el("span", { className: "job-badge" + (stopped ? " stopped" : ""), title: `${jobLabel(j)} is ${what} in the History shell${stopped ? ": fg there brings it back" : ""}` }, what);
}

// drawJobBadges marks the drawn cards again, the live shell's jobs
// having changed.
function drawJobBadges() {
  for (const b of document.querySelectorAll("#doc .job-badge")) b.remove();
  for (const j of shellJobs()) {
    const exit = document.querySelector(`#block-${j.block} > .head > .exit`);
    const badge = jobBadge(j.block);
    if (exit && badge) exit.before(badge);
  }
}

// jobsWarning names the live shell's jobs, which a restart ends too.
function jobsWarning(): string {
  const jobs = shellJobs();
  if (!jobs.length) return "";
  return ` Its ${jobs.length === 1 ? "job" : "jobs"} end too: ${jobs.map((j) => `${jobLabel(j)} (${j.state.toLowerCase()})`).join(", ")}.`;
}

function onBackgroundTerminal(e: BlockEvent) {
  switch (e.kind) {
    case "terminal": {
      const block = blockCard(e.block);
      if (!block) return;
      if (!view.rendered) toggle("rendered", true);
      openBackground(e.block);
      follow(block);
      labelBlock(block);
      drawOutline();
      return;
    }
    case "tty":
      return writeBackground(e.block, e.bytes);
    case "terminal-end":
      return endBackground(e.block);
  }
}

function onConfirm({ block, question, of }: Question) {
  const card = blockCard(block);
  if (!card) return;
  // The question is asked in the block's card, so show it, opening the
  // sections it's folded in until the run moves past them.
  if (!view.rendered) toggle("rendered", true);
  const ask = card.querySelector<HTMLElement>(".ask")!;
  const why =
    of === undefined
      ? "This block is marked confirm, so it waits for you before it runs."
      : "A block you ran lists this one in depends=, and it hasn't run in this session.";
  const info = el("span", { className: "info", title: why, role: "img", tabIndex: 0 }, svg(icons.info));
  info.setAttribute("aria-label", why);
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
  ask.setAttribute("role", "group");
  ask.setAttribute("aria-label", question);
  ask.hidden = false;
  const cell = cellOf.get(card)!;
  closeAsks(cell);
  peekAsk(cell);
  // The run waits now, so the block it last followed is done; its
  // output mustn't scroll the view away from the question.
  following = null;
  cancelAnimationFrame(followFrame);
  followFrame = 0;
  centerAsk(ask);
  status("waiting for you");
  labelBlock(card);
  drawOutline();
  // The question goes to the keyboard and the screen reader: Run has
  // the focus, and the question is said with its reason.
  announce(question + " " + why + " Run, skip or stop?");
  run.focus({ preventScroll: true });
}

// SourcesQuestion asks, before a shell starts, whether to read in the
// files the front matter's env.source names.
interface SourcesQuestion {
  files: string[];
}

// onSources asks whether the shell may read in files, in a dialog, since
// the question isn't any one block's: a run or the History pane's prompt
// is waiting to start a shell. Cancel, the focus, and Esc stop it.
function onSources({ files }: SourcesQuestion) {
  const dialog = $<HTMLDialogElement>("sources-dialog");
  $("sources-list").replaceChildren(...files.map((f) => el("li", {}, el("code", {}, tilde(f)))));
  dialog.returnValue = "";
  dialog.onclose = () => {
    status("");
    void App.Answer(dialog.returnValue || "stop").catch(fail);
  };
  if (!dialog.open) dialog.showModal();
  $("sources-cancel").focus();
  status("waiting for you");
}

// checkOutside asks, once a file opens, whether it may reach the files
// its includes and out= name outside its folder, or hidden in it,
// as mdsh --allow-outside allows them on the command line. The answer is
// remembered for the file, until the list changes; Ask me later and Esc
// leave it unanswered. Once answered, a banner says which way, with a
// button to change it.
let outsideFile = "";

async function checkOutside() {
  let o: OutsideFiles;
  try {
    o = await App.Outside();
  } catch {
    $("outside").hidden = true;
    return;
  }
  const paths = o.paths ?? [];
  const shown = paths.map(tilde).join(", ");
  $("outside").hidden = paths.length === 0 || o.answer === "";
  $("outside-text").textContent = o.answer === "allow" ? "This document may reach outside its folder: " + shown : "Kept inside its folder, so these fail: " + shown;
  const change = $<HTMLButtonElement>("outside-change");
  change.textContent = o.answer === "allow" ? "Stop allowing" : "Allow…";
  change.onclick = () => (o.answer === "allow" ? void answerOutside("deny") : askOutside(o));
  if (paths.length > 0 && o.answer === "") askOutside(o);
}

function askOutside(o: OutsideFiles) {
  const dialog = $<HTMLDialogElement>("outside-dialog");
  $("outside-root").textContent = o.root;
  $("outside-list").replaceChildren(...(o.paths ?? []).map((p) => el("li", {}, el("code", {}, p))));
  $("outside-vcs").hidden = !(o.paths ?? []).some(isHidden);
  dialog.returnValue = "";
  dialog.onclose = () => {
    if (dialog.returnValue !== "") void answerOutside(dialog.returnValue);
  };
  if (!dialog.open) dialog.showModal();
  $("outside-later").focus();
}

// isHidden says whether a path, as the document writes it, names a
// hidden file or folder or something in one, as docroot.Hidden does.
function isHidden(path: string): boolean {
  return path.split(/[\\/]/).some((part) => part.startsWith(".") && part !== "." && part !== "..");
}

async function answerOutside(answer: string) {
  try {
    await App.AnswerOutside(answer);
    announce(answer === "allow" ? "Allowed outside its folder" : "Kept inside its folder");
  } catch (err) {
    status(message(err));
  }
}

Events.On("block", mine(onBlock));
Events.On("sources", mine(onSources));
Events.On("confirm", mine(onConfirm));
// A run this page didn't start is over.
Events.On(
  "ran",
  mine(() => {
    if (ranHere || !running) return;
    busy(false);
    closeAsks();
    status("");
    void refresh().then(() => (following = null));
  }),
);

Events.On("opened", mine(() => ((selected = -1), (outsideFile = ""), (saveAfterRun = false), void refresh())));
Events.On("audit", mine(addEvent));
Events.On("view-image", mine((src: string) => viewSrc(src, "")));
Events.On(
  "saved",
  mine<string>((file) => {
    // Save As saves to a new file, so name it. The new file keeps the
    // view the tab shows.
    const moved = doc.file !== file;
    doc = { ...doc, file, dirty: false, changed: false };
    if (moved && viewFile !== "") {
      viewFile = file;
      rememberView();
    }
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
    if (insert) {
      const cmd = nvimDoc ? insertCommand(insert[1]) : null;
      if (cmd) return void insertNvim(cmd);
      return void (source && insertAction(source.view, insert[1]));
    }
    void actions[action]?.();
  }),
);
// Settings come from the Settings tab, which may be in another window.
Events.On("settings", (ev: { data: Settings }) => {
  settings = ev.data;
  applyTheme(settings);
  source?.setVim(settings.vim);
  source?.setSpellCheck(!settings.noSpellCheck);
  void nvimSettings();
  nvimFailed = false;
});

$("run-all").onclick = () => void actions.runall();
$("clear-outputs").onclick = () => void actions.clearoutputs();
let resizeTimer = 0;
window.addEventListener("resize", () => {
  clearTimeout(resizeTimer);
  resizeTimer = window.setTimeout(() => {
    if (!doc.file) return;
    sizeTerminal();
    fitHistory();
  }, 200);
});
applyKeyHints();
initSplitGrip();
initDeps($("doc-scroll"), {
  jump: (block) => {
    const c = blockCells.get(block);
    if (c) jumpTo(c);
  },
  setDepends,
  peek,
  endPeek,
  shown: shownCard,
  places: placesNow,
  name: (block) => {
    const c = blockCells.get(block);
    return (c && cellName(c)) || "block " + (block + 1);
  },
});
initHistory({
  canRun: () => !running,
  async enter(call, added) {
    busy(true);
    ranHere = true;
    status("running");
    await call();
    status("");
    ranHere = false;
    busy(false);
    await refresh();
    // What was added is at the end. A command run while recording is
    // off added nothing, so the view stays where it was.
    if (!added) return;
    const scroll = $("doc-scroll");
    scroll.scrollTop = scroll.scrollHeight;
  },
  status: (i) => cells.find((c) => c.block === i)?.status,
  toggle: () => void actions.history(),
  stop: () => void actions.stop(),
  async restart() {
    if (!(await confirmShells("Restart the shells?", "This file's shells end, along with any background block, and new ones start with the next block. " + lostWarning + jobsWarning(), "Restart Shells"))) return false;
    try {
      await App.RestartShells();
    } catch (err) {
      fail(err);
      return false;
    }
    await refresh();
    return true;
  },
  say: status,
  jobs: drawJobBadges,
});
initFrontMatter({ edit });
bugReportPage(() => [
  `Run going (page): ${running}`,
  `Unsaved (page): ${doc.dirty}`,
  `Source editor: ${nvimDoc ? "Neovim" : "CodeMirror"}`,
]);
initConsole({ toggle: () => void actions.console(), jump: (line) => void goToLine(line), fix: (warning) => void fixWarning(warning), lost: shellLost });
initReport({
  dirty: () => !$("dirty").hidden,
  before: applySource,
  runAll: () => start("RunFrom", 0),
});
$("outline-toggle").onclick = () => toggle("outline");
$("view-split").onclick = () => toggle("split");
$("view-rendered").onclick = () => toggle("rendered");
$("doc-scroll").addEventListener("scroll", () => syncScroll("doc"));
$("doc-scroll").addEventListener("scroll", () => virtual && placeSoon(), { passive: true });
// Folding, hiding code or output, and resizing the window bring other
// cards into sight.
const sized = new ResizeObserver(() => virtual && placeSoon());
sized.observe($("doc"));
sized.observe($("doc-scroll"));
for (const type of ["wheel", "touchstart", "pointerdown", "keydown"]) $("doc-scroll").addEventListener(type, leadFrom("doc"), { capture: true, passive: true });
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
// between blocks, enter runs the selected one, a runs from it, e edits
// it and c copies its code. s runs the selected heading's section, or the selected block's.
document.addEventListener("keydown", (e) => {
  if (!doc.file || e.metaKey || e.ctrlKey || e.altKey) return;
  const target = e.target as HTMLElement | null;
  if (target?.closest?.("input, textarea, select, dialog, [contenteditable], .cm-editor, .terminal, .cast-player")) return;
  // These keys move around the rendered view, so it leads the scrolling.
  lead = "doc";
  const all = blocks();
  const current = () => all[selected].block;
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
        const c = all[selected];
        const card = c && cardFor(c);
        if (card) void editBlock(card, c);
      }
      break;
    case "c":
      if (selected >= 0) {
        const c = cells.find((c) => c.block === current());
        if (c) void copyCode(c.body ?? "", c.name ?? c.id ?? "", !!c.secretCode);
      }
      break;
    case "d":
      void openDepends();
      break;
    default:
      return;
  }
  e.preventDefault();
});

startupMark("set up");
// These go together, as each waits on Go.
[me, settings, attrs] = await Promise.all([Window.Name(), App.Settings(), App.BlockAttrs().then((a) => a ?? [])]);
applyTheme(settings);
applyView();
startupMark("settings");
await refresh();
startupDrawn("tab " + tabID);
