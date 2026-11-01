// The Console pane, above the History bar, shows what the app logs:
// errors from calls to Go, such as a file that doesn't parse, and notes
// at info and above. The log is the app's, so every tab shows the same
// one; new lines come as "console" events.

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
}

let hooks: ConsoleHooks;

function time(t: unknown): string {
  const d = new Date(String(t));
  return isNaN(d.getTime()) ? "" : d.toLocaleTimeString([], { hour: "numeric", minute: "2-digit", second: "2-digit" });
}

// line is an entry as text, for copying.
const line = (e: ConsoleEntry) => `${time(e.time)} ${short[e.level] ?? e.level.toUpperCase()} ${e.message}`;

function row(e: ConsoleEntry): HTMLElement {
  return el(
    "div",
    { className: "entry " + e.level },
    el("time", {}, time(e.time)),
    el("span", { className: "level" }, short[e.level] ?? e.level),
    el("span", { className: "message" }, e.message),
  );
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
