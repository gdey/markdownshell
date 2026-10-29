import { Events, Window } from "/wails/runtime.js";

import { App, type AttrSpec, type Cell, type Doc, type Names, type Settings, type Start, message } from "./api.js";
import { attrForm } from "./attrform.js";
import { $, el, iconButton, icons, svg } from "./dom.js";
import { type BlockEditor, blockEditor } from "./editor.js";

let me = ""; // this window's name; events from other windows are ignored
let doc: Doc = { file: "", dirty: false, changed: false, run: { args: [], only: "", step: false, stopOnError: false, shell: "" }, args: "" };
let cells: Cell[] = [];
let running = false;
let selected = -1; // index into the block cards, for j/k
let settings: Settings;
let attrs: AttrSpec[] = [];
let editing: { block: number; editor: BlockEditor } | null = null;

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
  $("doc").hidden = !open;
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
  closeEditor();
  const main = $("doc");
  main.replaceChildren();
  for (const c of cells) {
    if (c.block < 0) {
      // goldmark's HTML, with raw HTML in the markdown left out.
      main.append(el("section", { className: "prose", innerHTML: c.html ?? "" }));
      continue;
    }
    main.append(drawBlock(c));
  }
  const add = el("button", { className: "add-block", title: "Add a block at the end" }, svg(icons.plus), el("span", {}, "Add block"));
  add.onclick = () => addBlock(add);
  main.append(add);
  runAllButton();
  select(Math.min(selected, blocks().length - 1), false);
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
      iconButton("pencil", "Edit this block (e)", () => editBlock(c.block)),
      iconButton("play", "Run this block (enter)", () => start("RunBlock", c.block)),
      iconButton("runFrom", "Run from here (a)", () => start("RunFrom", c.block)),
    ),
    el("pre", { className: "code", textContent: c.body ?? "" }),
    el("div", { className: "ask", hidden: true }),
    el("div", { className: "output", innerHTML: c.output ?? "" }),
  );
  block.dataset.block = String(c.block);
  block.onclick = () => select(blocks().indexOf(block), false);
  return block;
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
  for (const x of document.querySelectorAll<HTMLButtonElement>(".block .head button, .add-block")) x.disabled = on;
}

async function start(method: "RunBlock" | "RunFrom", block: number) {
  if (running || editing) return;
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

// Editing

function closeEditor() {
  if (!editing) return;
  editing.editor.destroy();
  editing = null;
}

async function names(): Promise<Names> {
  try {
    return await App.Names();
  } catch {
    return { ids: [], envs: [] };
  }
}

// openEditor swaps a card's code for the block editor.
async function openEditor(card: HTMLElement, block: number, text: string, save: (text: string) => Promise<void>) {
  if (running) return;
  closeEditor();
  const code = card.querySelector<HTMLElement>(".code");
  const host = el("div", { className: "editor" });
  const error = el("p", { className: "error", hidden: true });
  const formHost = el("div", { className: "form-host", hidden: true });
  const close = () => void refresh();
  const editor = blockEditor(host, {
    text,
    vim: settings.vim,
    attrs,
    names: await names(),
    async save(text) {
      try {
        await save(text);
        error.hidden = true;
      } catch (err) {
        error.textContent = message(err);
        error.hidden = false;
        throw err;
      }
    },
    saveFile: async () => void (await actions.save()),
    close,
  });
  editing = { block, editor };
  const formButton = iconButton("sliders", "Attributes", async () => {
    if (formHost.hidden) formHost.replaceChildren(await attrForm(editor, attrs, await names()));
    formHost.hidden = !formHost.hidden;
  });
  const done = el("button", { className: "primary", title: "cmd/ctrl+enter" }, "Done");
  done.onclick = async () => {
    try {
      await save(editor.view.state.doc.toString());
      close();
    } catch (err) {
      error.textContent = message(err);
      error.hidden = false;
    }
  };
  const cancel = el("button", { title: settings.vim ? ":q" : "esc" }, "Cancel");
  cancel.onclick = close;
  const bar = el("div", { className: "edit-bar" }, formButton, el("span", { className: "muted hint" }, settings.vim ? "Vim: :w saves the file, :q closes" : "cmd/ctrl+enter is Done, esc cancels"), cancel, done);
  const box = el("div", { className: "editing" }, formHost, host, error, bar);
  if (code) code.replaceWith(box);
  else card.append(box);
  editor.view.focus();
}

async function editBlock(block: number) {
  const card = $("block-" + block);
  try {
    const text = await App.BlockSource(block);
    await openEditor(card, block, text, async (t) => {
      await App.SetBlock(block, t);
      $("dirty").hidden = false;
    });
  } catch (err) {
    fail(err);
  }
}

async function addBlock(button: HTMLElement) {
  const card = el("section", { className: "block new" }, el("div", { className: "head" }, el("span", { className: "name" }, "New block")));
  button.replaceWith(card);
  let added = -1;
  await openEditor(card, -1, "```sh\n\n```\n", async (t) => {
    if (added < 0) added = await App.AddBlock(t);
    else await App.SetBlock(added, t);
    $("dirty").hidden = false;
  });
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
  stop: () => App.Stop(),
  runoptions: () => openRunOptions(),
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

function fileRow(path: string, label: string, detail: string): HTMLElement {
  const b = el("button", { className: "file", title: path }, el("span", { className: "fname" }, label), el("span", { className: "muted" }, detail));
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
    if (!match(f.rel)) continue;
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
      el("ul", { className: "files" }, ...files.map((f) => fileRow(f.path, f.rel.slice(dir === "" ? 0 : dir.length + 1), blocksLabel(f.blocks) + " · " + ago(f.mod)))),
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
    }
  }),
);

Events.On(
  "confirm",
  mine<{ block: number; question: string }>(({ block, question }) => {
    const card = document.getElementById("block-" + block);
    if (!card) return;
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
  }),
);

Events.On("opened", mine(() => ((selected = -1), void refresh())));
Events.On(
  "saved",
  mine(() => {
    $("changed").hidden = true;
    $("dirty").hidden = true;
  }),
);
Events.On(
  "file-changed",
  mine(() => {
    delete $("changed").dataset.kept;
    $("changed").hidden = false;
  }),
);
Events.On("menu", mine<string>((action) => void actions[action]?.()));
// Settings come from the Settings window, so they aren't this window's.
Events.On("settings", (ev: { data: Settings }) => {
  settings = ev.data;
  editing?.editor.setVim(settings.vim);
});

$("run-all").onclick = () => void actions.runall();
$("reload").onclick = () => void actions.reload();
$("keep").onclick = () => {
  $("changed").dataset.kept = doc.file;
  $("changed").hidden = true;
};

// Keys follow mdsh -i where they don't clash with the OS: j/k move
// between blocks, enter runs the selected one, a runs from it and e edits
// it.
document.addEventListener("keydown", (e) => {
  if (!doc.file || e.metaKey || e.ctrlKey || e.altKey || editing) return;
  const target = e.target as HTMLElement | null;
  if (target?.closest?.("input, textarea, select, dialog, [contenteditable], .cm-editor")) return;
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
      if (selected >= 0) void editBlock(current());
      break;
    default:
      return;
  }
  e.preventDefault();
});

me = await Window.Name();
settings = await App.Settings();
attrs = (await App.BlockAttrs()) ?? [];
await refresh();
