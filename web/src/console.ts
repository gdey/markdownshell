// The Console pane, above the History bar, shows what the app logs:
// errors from calls to Go, such as a file that doesn't parse, and notes
// at info and above. The log is the app's, so every tab shows the same
// one; new lines come as "console" events.
//
// A line about a place in a file shows the file's name and the line.
// When that file is the one this tab shows, a click on the line takes
// the source pane (or Neovim, when it is the editor) to it.
//
// A warning an edit fixed stays, struck through. One with a fix, about
// this tab's file, has a Fix button, which asks first.
//
// Its Events tab shows this tab's file's event log, in mdsh --audit's
// events: its shells starting and ending, and why, blocks running and
// skipped, the file changing on disk and mdsh ctl calls. An event that
// lost what blocks set in a shell, its variables, functions and
// aliases, stands out, counts toward the toolbar's badge and shows in
// the status line. Copy there copies the events as JSON lines.

import { Clipboard, Events } from "/wails/runtime.js";
import { App, type AuditEvent, type ConsoleEntry } from "./api.js";
import { $, el } from "./dom.js";
import { hinted } from "./keys.js";

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
  // fix offers to make the fix for warning, a line's message, in the
  // file this tab shows.
  fix: (warning: string) => void;
  // lost tells of an event that lost what blocks set in a shell.
  lost: (text: string) => void;
}

let hooks: ConsoleHooks;
// tab is which of the pane's tabs shows.
let tab: "log" | "events" = "log";
const tabKey = "mdsh.consoleTab";
let events: AuditEvent[] = [];
let lastEvent = 0;
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
  `${time(e.time)} ${short[e.level] ?? e.level.toUpperCase()} ${where(e) ? where(e) + " " : ""}${e.message}${e.stale ? " (fixed)" : ""}`;

// rows holds the entry each drawn row is for.
const rows = new WeakMap<HTMLElement, ConsoleEntry>();

function row(e: ConsoleEntry): HTMLElement {
  const r = el(
    "div",
    { className: "entry " + e.level + (e.stale ? " stale" : "") },
    el("time", {}, time(e.time)),
    el("span", { className: "level" }, short[e.level] ?? e.level),
    el(
      "span",
      { className: "message" },
      ...(e.file ? [el("span", { className: "where" }, where(e)), " "] : []),
      e.message,
      // Said, not only drawn, for a screen reader.
      ...(e.stale ? [el("span", { className: "gone" }, " (fixed)")] : []),
    ),
  );
  if (e.fix && !e.stale) {
    const b = el("button", { className: "fix", type: "button" }, "Fix");
    b.title = e.fix;
    b.setAttribute("aria-label", `Fix: ${e.fix}`);
    b.onclick = (ev) => {
      ev.stopPropagation();
      hooks.fix(e.message);
    };
    r.append(b);
  }
  rows.set(r, e);
  mark(r, e);
  return r;
}

// canJump reports whether e is about a line of the file this tab shows.
// A fixed warning's line may since have moved, so it is no link.
const canJump = (e: ConsoleEntry) => !e.stale && !!e.file && !!e.line && e.file === here;

// mark styles r as a link when it is about this tab's file, and shows
// its Fix button only then.
function mark(r: HTMLElement, e: ConsoleEntry) {
  const on = canJump(e);
  const fix = r.querySelector<HTMLElement>(".fix");
  if (fix) fix.hidden = !e.file || e.file !== here;
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

// stale strikes out the lines with the given seqs: what they warned of
// is fixed.
function stale(seqs: number[]) {
  const set = new Set(seqs);
  for (const r of [...$("console-log").children] as HTMLElement[]) {
    const e = rows.get(r);
    if (!e || !set.has(e.seq) || e.stale) continue;
    e.stale = true;
    r.replaceWith(row(e));
  }
}

function drawAll() {
  const log = $("console-log");
  log.replaceChildren(...entries.map(row));
  $("console-empty").hidden = tab !== "log" || entries.length > 0;
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
  $("console-toggle").title = hinted("Console", "Mod-Shift-Y") + (unseen ? ` · ${unseen} new` : "");
  $("console-toggle").setAttribute("aria-label", "Console" + (unseen ? `, ${unseen} new ${unseen === 1 ? "warning or error" : "warnings and errors"}` : ""));
}

async function load() {
  entries = (await App.Console().catch(() => null)) ?? [];
  lastSeq = entries.length ? entries[entries.length - 1].seq : lastSeq;
  drawAll();
}

function setHeight(px: number) {
  height = Math.max(48, Math.min(Math.round(px), window.innerHeight - 160));
  $("console").style.height = `${height}px`;
  $("console-grip").setAttribute("aria-valuenow", String(height));
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

// copy puts the selected lines on the clipboard, or every line: in the
// Events tab, every event as a line of JSON.
async function copy() {
  const sel = window.getSelection()?.toString() ?? "";
  const log = tab === "events" ? $("events-log") : $("console-log");
  const all = tab === "events" ? events.map((e) => e.json).join("\n") : entries.map(line).join("\n");
  const text = sel && log.contains(window.getSelection()!.anchorNode) ? sel : all;
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

// eventRow draws one event.
function eventRow(e: AuditEvent): HTMLElement {
  const r = el("div", { className: "entry" + (e.lost ? " lost" : "") }, el("time", {}, time(e.time)), el("span", { className: "event" }, e.event), el("span", { className: "message" }, e.text));
  r.title = e.json;
  return r;
}

function drawEvents() {
  const log = $("events-log");
  log.replaceChildren(...events.map(eventRow));
  $("events-empty").hidden = tab !== "events" || events.length > 0;
  log.scrollTop = log.scrollHeight;
}

// loadEvents reads this tab's file's event log again, as when another
// file opens in the tab.
export async function loadEvents() {
  events = (await App.Events().catch(() => null)) ?? [];
  lastEvent = events.length ? events[events.length - 1].seq : 0;
  drawEvents();
}

// addEvent shows a new event of this tab's file's log.
export function addEvent(e: AuditEvent) {
  if (e.seq <= lastEvent) return;
  // One missed on the way, as when many come at once: read them all.
  if (e.seq !== lastEvent + 1 && lastEvent !== 0) return void loadEvents();
  events.push(e);
  lastEvent = e.seq;
  const log = $("events-log");
  const atEnd = log.scrollHeight - log.scrollTop - log.clientHeight < 8;
  log.append(eventRow(e));
  if (events.length > 2000) {
    events.shift();
    log.firstElementChild?.remove();
  }
  $("events-empty").hidden = true;
  if (atEnd) log.scrollTop = log.scrollHeight;
  if (e.lost) {
    hooks.lost(e.text);
    if (!open || tab !== "events") {
      unseen++;
      badge();
    }
  }
}

// showTab shows the Console's log or its Events.
function showTab(which: "log" | "events", focus = false) {
  tab = which;
  const ev = which === "events";
  for (const [id, on] of [
    ["console-tab-log", !ev],
    ["console-tab-events", ev],
  ] as const) {
    $(id).setAttribute("aria-selected", String(on));
    $(id).tabIndex = on ? 0 : -1;
  }
  $("console-log").hidden = ev;
  $("events-log").hidden = !ev;
  $("console-empty").hidden = ev || entries.length > 0;
  $("events-empty").hidden = !ev || events.length > 0;
  $("console-clear").hidden = ev;
  $("console-copy").title = ev ? "Copy the selected lines, or every event as JSON lines" : "Copy the selected lines, or all of them";
  const log = ev ? $("events-log") : $("console-log");
  log.scrollTop = log.scrollHeight;
  if (focus) $(ev ? "console-tab-events" : "console-tab-log").focus();
  try {
    localStorage.setItem(tabKey, which);
  } catch {
    // Not remembered, then.
  }
}

// showEvents opens the pane on its Events tab.
export function showEvents() {
  showTab("events");
}

// showConsole opens or closes the pane.
export function showConsole(on: boolean) {
  open = on;
  $("console").hidden = !on;
  $("console-toggle").classList.toggle("on", on);
  $("console-toggle").setAttribute("aria-expanded", String(on));
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
  $("console-grip").onkeydown = (e) => {
    const dir = e.key === "ArrowUp" ? 1 : e.key === "ArrowDown" ? -1 : 0;
    if (!dir) return;
    e.preventDefault();
    setHeight(height + dir * 24);
    try {
      localStorage.setItem(heightKey, String(height));
    } catch {
      // Not remembered, then.
    }
  };
  $("console-log").onclick = click;
  $("console-tab-log").onclick = () => showTab("log");
  $("console-tab-events").onclick = () => showTab("events");
  for (const id of ["console-tab-log", "console-tab-events"]) {
    $(id).onkeydown = (e) => {
      if (e.key !== "ArrowLeft" && e.key !== "ArrowRight") return;
      e.preventDefault();
      showTab(tab === "log" ? "events" : "log", true);
    };
  }
  try {
    if (localStorage.getItem(tabKey) === "events") showTab("events");
  } catch {
    // The Console it is.
  }
  $("console-clear").onclick = () => void App.ClearConsole().catch(() => {});
  $("console-copy").onclick = () => void copy();
  $("console-close").onclick = () => hooks.toggle();
  $("console-toggle").onclick = () => hooks.toggle();
  Events.On("console", (ev: { data: ConsoleEntry }) => {
    // A line missed on the way, as when many come at once: read them all.
    if (ev.data.seq !== lastSeq + 1 && lastSeq !== 0) void load();
    else add(ev.data);
  });
  Events.On("console-stale", (ev: { data: number[] }) => stale(ev.data ?? []));
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
