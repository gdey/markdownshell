// The Console pane, above the History bar, shows what the app logs:
// errors from calls to Go, such as a file that doesn't parse, and notes
// at info and above. The log is the app's, so every tab shows the same
// one; new lines come as "console" events.
//
// A line about a place in a file shows the file's name and the line.
// When that file is the one this tab shows, a click on the line takes
// the source pane (or Neovim, when it is the editor) to it.

import { Clipboard, Events } from "/wails/runtime.js";
import { App, type ConsoleEntry } from "./api.js";
import { $, el } from "./dom.js";

let entries: ConsoleEntry[] = [];
let lastSeq = 0;
// unseen counts the warnings and errors that came while the pane was
// closed, for the toolbar's badge.
let unseen = 0;
let open = false;

let height = 160;
const heightKey = "mdsh.consoleHeight";
const short: Record<string, string> = { debug: "DBG", info: "INF", warn: "WRN", error: "ERR" };

export interface ConsoleHooks {
  // toggle opens or closes the pane, as View › Console does.
  toggle: () => void;
  // jump puts the source pane's cursor on line of the file this tab shows.
  jump: (line: number) => void;
}

let hooks: ConsoleHooks;
// here is the file this tab shows, for telling which lines are about it.
let here = "";

function time(t: unknown): string {
  const d = new Date(String(t));
  return isNaN(d.getTime()) ? "" : d.toLocaleTimeString([], { hour: "numeric", minute: "2-digit", second: "2-digit" });
}

// base is the last element of a path.
const base = (file: string) => file.slice(Math.max(file.lastIndexOf("/"), file.lastIndexOf("\\")) + 1);

// where is the place an entry is about, as name:line, or "".
const where = (e: ConsoleEntry) => (e.file ? base(e.file) + (e.line ? `:${e.line}` : "") : "");

// line is an entry as text, for copying.
const line = (e: ConsoleEntry) =>
  `${time(e.time)} ${short[e.level] ?? e.level.toUpperCase()} ${where(e) ? where(e) + " " : ""}${e.message}`;

// rows holds the entry each drawn row is for.
const rows = new WeakMap<HTMLElement, ConsoleEntry>();

function row(e: ConsoleEntry): HTMLElement {
  const r = el(
    "div",
    { className: "entry " + e.level },
    el("time", {}, time(e.time)),
    el("span", { className: "level" }, short[e.level] ?? e.level),
    el("span", { className: "message" }, ...(e.file ? [el("span", { className: "where" }, where(e)), " "] : []), e.message),
  );
  rows.set(r, e);
  mark(r, e);
  return r;
}

// canJump reports whether e is about a line of the file this tab shows.
const canJump = (e: ConsoleEntry) => !!e.file && !!e.line && e.file === here;

// mark styles r as a link when it is about this tab's file.
function mark(r: HTMLElement, e: ConsoleEntry) {
  const on = canJump(e);
  r.classList.toggle("jump", on);
  if (on) r.title = `Show line ${e.line} in the source`;
  else if (e.file) r.title = e.file;
  else r.removeAttribute("title");
}

// consoleFile tells the pane which file this tab shows, "" for none.
export function consoleFile(file: string) {
  if (file === here) return;
  here = file;
  for (const r of $("console-log").children) {
    const e = rows.get(r as HTMLElement);
    if (e) mark(r as HTMLElement, e);
  }
}

// click takes the source pane to the line a clicked row is about,
// unless the click ended a drag that selected text to copy.
function click(ev: MouseEvent) {
  const r = (ev.target as HTMLElement).closest<HTMLElement>(".entry");
  const e = r && rows.get(r);
  if (!e || !canJump(e)) return;
  const sel = window.getSelection();
  if (sel && !sel.isCollapsed && $("console-log").contains(sel.anchorNode)) return;
  hooks.jump(e.line!);
}

function drawAll() {
  const log = $("console-log");
  log.replaceChildren(...entries.map(row));
  $("console-empty").hidden = entries.length > 0;
  log.scrollTop = log.scrollHeight;
}

function add(e: ConsoleEntry) {
  entries.push(e);
  lastSeq = e.seq;
  const log = $("console-log");
  // Follow the end, unless scrolled up to read.
  const atEnd = log.scrollHeight - log.scrollTop - log.clientHeight < 8;
  log.append(row(e));
  if (entries.length > 1000) {
    entries.shift();
    log.firstElementChild?.remove();
  }
  $("console-empty").hidden = true;
  if (atEnd) log.scrollTop = log.scrollHeight;
  if (!open && (e.level === "warn" || e.level === "error")) {
    unseen++;
    badge();
  }
}

// badge shows how many warnings and errors came since the pane was last
// open, on the toolbar's button.
function badge() {
  const b = $("console-badge");
  b.hidden = unseen === 0;
  b.textContent = unseen > 99 ? "99+" : String(unseen);
  $("console-toggle").title = "Console (cmd/ctrl+shift+Y)" + (unseen ? ` · ${unseen} new` : "");
}

async function load() {
  entries = (await App.Console().catch(() => null)) ?? [];
  lastSeq = entries.length ? entries[entries.length - 1].seq : lastSeq;
  drawAll();
}

function setHeight(px: number) {
  height = Math.max(48, Math.min(Math.round(px), window.innerHeight - 160));
  $("console").style.height = `${height}px`;
}

function dragGrip(e: PointerEvent) {
  if (e.button !== 0) return;
  e.preventDefault();
  const grip = e.currentTarget as HTMLElement;
  grip.setPointerCapture(e.pointerId);
  const y0 = e.clientY;
  const h0 = height;
  document.body.classList.add("resizing-rows");
  grip.onpointermove = (m) => setHeight(h0 + (y0 - m.clientY));
  grip.onpointerup = grip.onpointercancel = () => {
    grip.onpointermove = grip.onpointerup = grip.onpointercancel = null;
    document.body.classList.remove("resizing-rows");
    try {
      localStorage.setItem(heightKey, String(height));
    } catch {
      // Not remembered, then.
    }
  };
}

// copy puts the selected lines on the clipboard, or every line.
async function copy() {
  const sel = window.getSelection()?.toString() ?? "";
  const text = sel && $("console-log").contains(window.getSelection()!.anchorNode) ? sel : entries.map(line).join("\n");
  try {
    await Clipboard.SetText(text);
    flash("Copied");
  } catch {
    flash("Couldn't copy");
  }
}

let flashTimer = 0;
function flash(text: string) {
  const s = $("console-note");
  s.textContent = text;
  clearTimeout(flashTimer);
  flashTimer = window.setTimeout(() => (s.textContent = ""), 1500);
}

// showConsole opens or closes the pane.
export function showConsole(on: boolean) {
  open = on;
  $("console").hidden = !on;
  $("console-toggle").classList.toggle("on", on);
  if (on) {
    unseen = 0;
    badge();
    setHeight(height);
    const log = $("console-log");
    log.scrollTop = log.scrollHeight;
  }
}

export function initConsole(h: ConsoleHooks) {
  hooks = h;
  try {
    const saved = Number(localStorage.getItem(heightKey));
    if (saved > 0) height = saved;
  } catch {
    // No storage: the default it is.
  }
  $("console-grip").onpointerdown = dragGrip;
  $("console-log").onclick = click;
  $("console-clear").onclick = () => void App.ClearConsole().catch(() => {});
  $("console-copy").onclick = () => void copy();
  $("console-close").onclick = () => hooks.toggle();
  $("console-toggle").onclick = () => hooks.toggle();
  Events.On("console", (ev: { data: ConsoleEntry }) => {
    // A line missed on the way, as when many come at once: read them all.
    if (ev.data.seq !== lastSeq + 1 && lastSeq !== 0) void load();
    else add(ev.data);
  });
  Events.On("console-cleared", () => {
    entries = [];
    unseen = 0;
    badge();
    drawAll();
  });
  void load().then(() => {
    // What went wrong before this page opened counts as news too.
    unseen = entries.filter((e) => e.level === "warn" || e.level === "error").length;
    if (open) unseen = 0;
    badge();
  });
}
