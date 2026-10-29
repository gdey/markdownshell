import * as wails from "/wails/runtime.js";

const call = (method, ...args) => wails.Call.ByName("main.App." + method, ...args);
const $ = (id) => document.getElementById(id);
const el = (tag, props = {}, ...children) => {
  const e = Object.assign(document.createElement(tag), props);
  e.append(...children);
  return e;
};
const svg = (path) => {
  const s = document.createElementNS("http://www.w3.org/2000/svg", "svg");
  s.setAttribute("viewBox", "0 0 16 16");
  s.setAttribute("aria-hidden", "true");
  s.innerHTML = path;
  return s;
};
const icons = {
  play: '<path d="M4 2.5v11l9-5.5z"/>',
  runFrom: '<path d="M2.5 2.5v11l8-5.5z"/><rect x="11.5" y="2.5" width="2" height="11"/>',
  stop: '<rect x="3.5" y="3.5" width="9" height="9" rx="1"/>',
  info: '<circle cx="8" cy="8" r="6.5" fill="none" stroke="currentColor" stroke-width="1.3"/><rect x="7.3" y="7" width="1.4" height="4.5"/><rect x="7.3" y="4.4" width="1.4" height="1.4"/>',
};

let me = ""; // this window's name; events from other windows are ignored
let doc = { file: "" };
let cells = [];
let running = false;
let selected = -1; // index into the runnable cells, for j/k

function mine(handler) {
  return (ev) => {
    if (ev.sender === me) handler(ev.data);
  };
}

function status(text) {
  $("status").textContent = text;
}

function fail(err) {
  status(String(err?.message ?? err));
}

// refresh reads the document's state and draws whichever screen fits.
async function refresh() {
  doc = await call("Doc");
  const open = doc.file !== "";
  $("toolbar").hidden = !open;
  $("doc").hidden = !open;
  $("start").hidden = open;
  if (!open) {
    $("changed").hidden = true;
    return drawStart();
  }
  $("file").textContent = doc.file.split(/[\\/]/).pop();
  $("file").title = doc.file;
  $("dirty").hidden = !doc.dirty;
  $("changed").hidden = !doc.changed || $("changed").dataset.kept === doc.file;
  $("changed-text").textContent = $("file").textContent + " changed on disk.";
  drawOptions();
  cells = await call("Cells");
  drawDoc();
}

function drawOptions() {
  const r = doc.run;
  const parts = [];
  if (doc.args) parts.push(doc.args);
  if (r.only) parts.push("only " + r.only);
  if (r.step) parts.push("step");
  if (r.stopOnError) parts.push("stop on error");
  $("options").hidden = parts.length === 0;
  $("options").textContent = parts.join(" · ");
}

function drawDoc() {
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
  $("run-all").disabled = running || !cells.some((c) => c.block >= 0);
  select(Math.min(selected, blocks().length - 1), false);
}

function drawBlock(c) {
  const name = el("span", { className: "name" }, c.id, el("span", { className: "muted" }, " · " + c.lang));
  const exit = el("span", { className: "exit" + (c.failed ? " bad" : "") }, c.ran ? "exit " + c.status : "");
  const runOne = iconButton("play", "Run this block", () => start("RunBlock", c.block));
  const runOn = iconButton("runFrom", "Run from here", () => start("RunFrom", c.block));
  const block = el(
    "section",
    { className: "block" + (c.failed ? " failed" : c.ran ? " ran" : ""), id: "block-" + c.block },
    el("div", { className: "head" }, name, exit, runOne, runOn),
    el("pre", { className: "code", textContent: c.body }),
    el("div", { className: "ask", hidden: true }),
    el("div", { className: "output", innerHTML: c.output ?? "" }),
  );
  block.dataset.block = c.block;
  block.onclick = () => select(blocks().indexOf(block), false);
  return block;
}

function iconButton(icon, title, onclick) {
  const b = el("button", { className: "icon", title, onclick: (e) => (e.stopPropagation(), onclick()) }, svg(icons[icon]));
  b.setAttribute("aria-label", title);
  return b;
}

const blocks = () => [...document.querySelectorAll("#doc .block")];

function select(i, scroll = true) {
  const all = blocks();
  for (const b of all) b.classList.remove("selected");
  selected = i;
  if (i < 0 || i >= all.length) return;
  all[i].classList.add("selected");
  if (scroll) all[i].scrollIntoView({ block: "nearest" });
}

function busy(on) {
  running = on;
  const b = $("run-all");
  b.replaceChildren(svg(on ? icons.stop : icons.play), el("span", {}, on ? "Stop" : "Run all"));
  b.classList.toggle("stop", on);
  b.disabled = !on && !cells.some((c) => c.block >= 0);
  for (const x of document.querySelectorAll(".block .head button")) x.disabled = on;
}

async function start(method, block) {
  if (running) return;
  busy(true);
  status("running");
  try {
    await call(method, block);
    status("");
  } catch (err) {
    fail(err);
  }
  busy(false);
  await refresh();
}

const actions = {
  async save() {
    try {
      await call("Save");
      status("saved");
    } catch (err) {
      fail(err);
    }
  },
  async saveas() {
    try {
      if (await call("SaveAs")) status("saved");
    } catch (err) {
      fail(err);
    }
  },
  async reload() {
    try {
      await call("Reload");
    } catch (err) {
      fail(err);
    }
  },
  runall: () => (running ? actions.stop() : start("RunFrom", 0)),
  stop: () => call("Stop"),
  runoptions: () => openRunOptions(),
};

// Run options

function openRunOptions() {
  if (!doc.file || running) return;
  $("opt-args").value = doc.args;
  const only = $("opt-only");
  only.replaceChildren(el("option", { value: "" }, "All blocks"));
  for (const c of cells) if (c.block >= 0 && c.id) only.append(el("option", { value: c.id }, c.id));
  only.value = doc.run.only;
  $("opt-step").checked = doc.run.step;
  $("opt-stop").checked = doc.run.stopOnError;
  $("opt-error").hidden = true;
  $("run-options").showModal();
  $("opt-args").focus();
}

$("opt-ok").onclick = async (e) => {
  e.preventDefault();
  try {
    await call("SetRunOptions", $("opt-args").value, $("opt-only").value, $("opt-step").checked, $("opt-stop").checked);
    $("run-options").close();
    await refresh();
  } catch (err) {
    $("opt-error").textContent = String(err?.message ?? err);
    $("opt-error").hidden = false;
  }
};
$("options").onclick = openRunOptions;

// Start screen

let start_ = { files: [], recent: [] };

async function drawStart() {
  try {
    start_ = await call("StartScreen");
    start_.files ??= [];
    start_.recent ??= [];
  } catch (err) {
    start_ = { dir: "", files: [], recent: [] };
    $("start-dir").textContent = String(err?.message ?? err);
  }
  $("start-dir").textContent = tilde(start_.dir) + (start_.full ? " (showing the first " + start_.files.length + " files)" : "");
  filterStart();
  $("filter").focus();
}

function fileRow(path, label, detail) {
  const li = el(
    "li",
    {},
    el("button", { className: "file", title: path, onclick: () => openPath(path) }, el("span", { className: "fname" }, label), el("span", { className: "muted" }, detail)),
  );
  return li;
}

function filterStart() {
  const q = $("filter").value.trim().toLowerCase();
  const match = (s) => s.toLowerCase().includes(q);
  const recent = start_.recent.filter(match);
  $("recent-list").hidden = recent.length === 0;
  $("recent-list").querySelector("ul").replaceChildren(
    ...recent.map((p) => fileRow(p, p.split(/[\\/]/).pop(), tilde(p.replace(/[\\/][^\\/]*$/, "")))),
  );
  const groups = new Map();
  for (const f of start_.files) {
    if (!match(f.rel)) continue;
    const i = f.rel.lastIndexOf("/");
    const dir = i < 0 ? "" : f.rel.slice(0, i);
    if (!groups.has(dir)) groups.set(dir, []);
    groups.get(dir).push(f);
  }
  const list = $("file-list");
  list.replaceChildren();
  for (const [dir, files] of groups) {
    list.append(
      el("h2", {}, dir === "" ? "This folder" : dir + "/"),
      el(
        "ul",
        { className: "files" },
        ...files.map((f) => fileRow(f.path, f.rel.slice(dir === "" ? 0 : dir.length + 1), blocksLabel(f.blocks) + " · " + ago(f.mod))),
      ),
    );
  }
  $("start-empty").hidden = groups.size > 0 || recent.length > 0;
}

// tilde shows a path under home as ~/….
function tilde(path) {
  const home = start_.home;
  if (home && (path === home || path.startsWith(home + "/"))) return "~" + path.slice(home.length);
  return path;
}

function blocksLabel(n) {
  if (n < 0) return "can't read";
  return n === 1 ? "1 block" : n + " blocks";
}

function ago(when) {
  const s = (Date.now() - new Date(when).getTime()) / 1000;
  if (s < 90) return "just now";
  if (s < 3600) return Math.round(s / 60) + " min ago";
  if (s < 86400) return Math.round(s / 3600) + " h ago";
  if (s < 86400 * 30) return Math.round(s / 86400) === 1 ? "1 day ago" : Math.round(s / 86400) + " days ago";
  return new Date(when).toLocaleDateString();
}

async function openPath(path) {
  try {
    await call("OpenPath", path);
  } catch (err) {
    $("start-dir").textContent = String(err?.message ?? err);
  }
}

$("filter").oninput = filterStart;
$("filter").onkeydown = (e) => {
  if (e.key === "Enter") document.querySelector("#start .file")?.click();
};
$("open-other").onclick = () => call("Open").catch(fail);

// Events from Go

wails.Events.On(
  "block",
  mine((e) => {
    const block = $("block-" + e.block);
    if (!block) return;
    const out = block.querySelector(".output");
    switch (e.kind) {
      case "started":
        block.querySelector(".ask").hidden = true;
        out.replaceChildren(el("pre", { className: "live" }));
        block.scrollIntoView({ block: "nearest" });
        break;
      case "stdout":
      case "stderr":
        out.querySelector(".live")?.append(el("span", { className: e.kind, textContent: e.data }));
        break;
      case "done":
        $("dirty").hidden = false;
        break;
    }
  }),
);

wails.Events.On(
  "confirm",
  mine(({ block, question }) => {
    const card = $("block-" + block);
    if (!card) return;
    const ask = card.querySelector(".ask");
    const info = el("span", { className: "info", title: "This block is marked confirm, so it waits for you before it runs." }, svg(icons.info));
    const answer = (reply) => async () => {
      ask.hidden = true;
      await call("Answer", reply).catch(fail);
    };
    ask.replaceChildren(
      info,
      el("span", { className: "question" }, question),
      el("button", { className: "primary", onclick: answer("run") }, "Run"),
      el("button", { onclick: answer("skip") }, "Skip"),
      el("button", { onclick: answer("stop") }, "Stop"),
    );
    ask.hidden = false;
    card.scrollIntoView({ block: "center" });
    status("waiting for you");
  }),
);

wails.Events.On("opened", mine(() => ((selected = -1), refresh())));
wails.Events.On("saved", mine(() => ($("changed").hidden = true, $("dirty").hidden = true)));
wails.Events.On("file-changed", mine(() => (delete $("changed").dataset.kept, ($("changed").hidden = false))));
wails.Events.On("menu", mine((action) => actions[action]?.()));

$("run-all").onclick = () => actions.runall();
$("reload").onclick = () => actions.reload();
$("keep").onclick = () => {
  $("changed").dataset.kept = doc.file;
  $("changed").hidden = true;
};

// Keys follow mdsh -i where they don't clash with the OS: j/k move
// between blocks, enter runs the selected one, a runs from it.
document.addEventListener("keydown", (e) => {
  if (!doc.file || e.metaKey || e.ctrlKey || e.altKey) return;
  if (e.target.closest?.("input, textarea, select, dialog, [contenteditable]")) return;
  const all = blocks();
  switch (e.key) {
    case "j":
      if (all.length) select(Math.min(selected + 1, all.length - 1));
      break;
    case "k":
      if (all.length) select(Math.max(selected - 1, 0));
      break;
    case "Enter":
      if (selected >= 0) start("RunBlock", Number(all[selected].dataset.block));
      break;
    case "a":
      if (selected >= 0) start("RunFrom", Number(all[selected].dataset.block));
      break;
    default:
      return;
  }
  e.preventDefault();
});

me = await wails.Window.Name();
await refresh();
