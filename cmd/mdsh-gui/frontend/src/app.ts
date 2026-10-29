import { EditorView } from "@codemirror/view";
import { Browser, Events, Window } from "/wails/runtime.js";

import { App, type AttrSpec, type Cell, type Doc, type Names, type Settings, type Start, message } from "./api.js";
import { $, el, iconButton, icons, svg } from "./dom.js";
import { type MarkdownEditor, insertAction, markdownBar, markdownEditor } from "./markdown.js";
import { initReport, openReport } from "./report.js";
import { runGutter, setRunLines } from "./rungutter.js";
import { closeTerminal, endTerminal, openTerminal, sizeTerminal, writeTerminal } from "./terminal.js";
import { applyTheme } from "./theme.js";

let me = ""; // this window's name; events from other windows are ignored
let doc: Doc = { file: "", dirty: false, changed: false, run: { args: [], only: "", step: false, stopOnError: false, shell: "" }, args: "" };
let cells: Cell[] = [];
let running = false;
let selected = -1; // index into the block cards, for j/k
let settings: Settings;
let attrs: AttrSpec[] = [];
let known: Names = { ids: [], envs: [] }; // for completion in the source pane

// The view: what each window shows. It starts as the last window left it.
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
  const name = doc.file.split(/[\\/]/).pop() ?? "";
  $("file").textContent = name;
  $("file").title = doc.file;
  $("dirty").hidden = !doc.dirty;
  $("changed").hidden = !doc.changed || $("changed").dataset.kept === doc.file;
  $("changed-text").textContent = name + " changed on disk.";
  drawOptions();
  cells = (await App.Cells()) ?? [];
  drawDoc();
  known = await names();
  await loadSource();
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
  closeTerminal();
  main.replaceChildren();
  for (const c of cells) {
    const e = c.block < 0 ? drawProse(c) : drawBlock(c);
    e.dataset.line = String(c.line);
    main.append(e);
  }
  runAllButton();
  select(Math.min(selected, blocks().length - 1), false);
  drawOutline();
  sizeTerminal();
}

// drawProse draws one top-level block of prose. Clicking it shows its
// markdown in the source pane; cmd/ctrl+click on a link opens the link
// instead.
function drawProse(c: Cell): HTMLElement {
  // goldmark's HTML, with raw HTML in the markdown left out.
  const s = el("section", { className: "prose", innerHTML: c.html ?? "", title: "Click to show it in the source" });
  s.onclick = (e) => {
    const a = (e.target as HTMLElement).closest("a");
    if (a) {
      e.preventDefault();
      if ((e.metaKey || e.ctrlKey) && /^https?:/.test(a.href)) return void Browser.OpenURL(a.href);
    }
    if (window.getSelection()?.toString()) return;
    void showInSource(s, c.line, true);
  };
  return s;
}

function drawBlock(c: Cell): HTMLElement {
  const name = el("span", { className: "name" }, c.id ?? "", el("span", { className: "muted" }, " · " + (c.lang ?? "") + (c.attrs ? " · " + c.attrs : "")));
  const exit = el("span", { className: "exit" + (c.failed ? " bad" : "") }, c.ran ? "exit " + c.status : "");
  const block = el(
    "section",
    { className: "block" + (c.failed ? " failed" : c.ran ? " ran" : ""), id: "block-" + c.block },
    el(
      "div",
      { className: "head" },
      name,
      exit,
      iconButton("pencil", "Edit this block in the source (e)", () => void editBlock(block, c)),
      iconButton("play", "Run this block (enter)", () => start("RunBlock", c.block)),
      iconButton("runFrom", "Run from here (a)", () => start("RunFrom", c.block)),
    ),
    el("pre", { className: "code", textContent: c.body ?? "" }),
    el("div", { className: "ask", hidden: true }),
    el("div", { className: "output", innerHTML: c.output ?? "" }),
  );
  block.querySelector<HTMLElement>(".output")!.onclick = (e) => {
    const img = (e.target as HTMLElement).closest("img");
    if (img) viewImage(img);
  };
  block.dataset.block = String(c.block);
  block.onclick = (e) => {
    select(blocks().indexOf(block), false);
    if (!(e.target as HTMLElement).closest("button")) void showInSource(block, c.line, false);
  };
  return block;
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

const blocks = () => [...document.querySelectorAll<HTMLElement>("#doc .block")];

function select(i: number, scroll = true) {
  const all = blocks();
  for (const b of all) b.classList.remove("selected");
  selected = i;
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
  runAllButton();
  for (const x of document.querySelectorAll<HTMLButtonElement>(".block .head button")) x.disabled = on;
}

async function start(method: "RunBlock" | "RunFrom", block: number) {
  if (running) return;
  busy(true);
  status("running");
  try {
    await App[method](block);
    status("");
  } catch (err) {
    fail(err);
  }
  busy(false);
  await refresh();
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
    return { ids: [], envs: [] };
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
    extensions: [runGutter((line) => void runLine(line))],
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

async function loadSource() {
  if (!source || sourcePending) return;
  try {
    source.setText(await App.Text());
  } catch (err) {
    fail(err);
  }
  markRunLines();
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

// cellAt is the drawn cell holding line, or the last one before it.
function cellAt(line: number): HTMLElement | null {
  let found: HTMLElement | null = null;
  for (const e of document.querySelectorAll<HTMLElement>("#doc > [data-line]")) {
    if (Number(e.dataset.line) > line) break;
    found = e;
  }
  return found;
}

// markCurrent marks the cell the source pane's cursor is in.
function markCurrent(line: number) {
  for (const e of document.querySelectorAll("#doc > .current")) e.classList.remove("current");
  cellAt(line)?.classList.add("current");
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
// ✗ failed, … running.

function drawOutline() {
  const nav = $("outline");
  if (!view.outline) return;
  const items: HTMLElement[] = [];
  let level = 0;
  for (const e of document.querySelectorAll<HTMLElement>("#doc > section")) {
    if (e.classList.contains("block")) {
      const ask = e.querySelector<HTMLElement>(".ask");
      const mark = ask && !ask.hidden ? "?" : e.querySelector(".live") && running ? "…" : e.classList.contains("failed") ? "✗" : e.classList.contains("ran") ? "✓" : "";
      const name = e.querySelector(".name")?.firstChild?.textContent ?? "";
      items.push(outlineItem(name, level + 1, e, "step" + (mark === "✗" ? " bad" : mark === "✓" ? " ok" : ""), mark));
      continue;
    }
    for (const h of e.querySelectorAll<HTMLElement>("h1, h2, h3, h4, h5, h6")) {
      level = Number(h.tagName[1]);
      items.push(outlineItem(h.textContent ?? "", level, h, "heading", ""));
    }
  }
  nav.replaceChildren(...(items.length ? items : [el("p", { className: "muted" }, "No headings or blocks")]));
}

function outlineItem(text: string, level: number, target: HTMLElement, kind: string, mark: string): HTMLElement {
  const b = el("button", { className: "item " + kind, title: text }, el("span", { className: "mark" }, mark), el("span", { className: "text" }, text));
  b.style.paddingLeft = 8 + (level - 1) * 12 + "px";
  b.onclick = () => {
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
  };
  return b;
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
    // Only a convenience: the next window starts with the default view.
  }
  void App.SetView(view).catch(() => {});
}

function toggle(key: keyof View, on = !view[key]) {
  view = { ...view, [key]: on };
  // Turning off the only pane shown shows the other one.
  if (!view.split && !view.rendered) view = { ...view, [key === "split" ? "rendered" : "split"]: true };
  applyView();
}

// Actions run from menus, buttons and keys.

const actions: Record<string, () => Promise<void> | void> = {
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
  report: () => (doc.file && !running ? openReport() : undefined),
  stop: () => App.Stop(),
  runoptions: () => openRunOptions(),
  split: () => toggle("split"),
  rendered: () => toggle("rendered"),
  outline: () => toggle("outline"),
  code: () => toggle("code"),
  output: () => toggle("output"),
};

// Run options

function openRunOptions() {
  if (!doc.file || running) return;
  $<HTMLInputElement>("opt-args").value = doc.args;
  const only = $<HTMLSelectElement>("opt-only");
  only.replaceChildren(el("option", { value: "" }, "All blocks"));
  for (const c of cells) if (c.block >= 0 && c.id) only.append(el("option", { value: c.id }, c.id));
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
$("open-other").onclick = () => App.Open().catch(fail);

// Events from Go

interface BlockEvent {
  kind: string;
  block: number;
  data?: string;
  bytes?: string; // base64
}

Events.On(
  "block",
  mine<BlockEvent>((e) => {
    const block = document.getElementById("block-" + e.block);
    if (!block) return;
    const out = block.querySelector(".output")!;
    switch (e.kind) {
      case "started":
        block.querySelector<HTMLElement>(".ask")!.hidden = true;
        out.replaceChildren(el("pre", { className: "live" }));
        block.scrollIntoView({ block: "nearest" });
        break;
      case "stdout":
      case "stderr":
        out.querySelector(".live")?.append(el("span", { className: e.kind, textContent: e.data ?? "" }));
        break;
      case "done":
        $("dirty").hidden = false;
        break;
      case "terminal":
        // The block runs in its card, so show it, as a confirm does.
        if (!view.rendered) toggle("rendered", true);
        openTerminal(e.block);
        break;
      case "tty":
        return writeTerminal(e.block, e.bytes);
      case "terminal-end":
        endTerminal(e.block);
        break;
    }
    if (e.kind !== "stdout" && e.kind !== "stderr") drawOutline();
  }),
);

Events.On(
  "confirm",
  mine<{ block: number; question: string }>(({ block, question }) => {
    const card = document.getElementById("block-" + block);
    if (!card) return;
    // The question is asked in the block's card, so show it.
    if (!view.rendered) toggle("rendered", true);
    const ask = card.querySelector<HTMLElement>(".ask")!;
    const info = el("span", { className: "info", title: "This block is marked confirm, so it waits for you before it runs." }, svg(icons.info));
    const answer = (reply: string) => async () => {
      ask.hidden = true;
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
    card.scrollIntoView({ block: "center" });
    status("waiting for you");
    drawOutline();
  }),
);

Events.On("opened", mine(() => ((selected = -1), void refresh())));
Events.On(
  "saved",
  mine(() => {
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
