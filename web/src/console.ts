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
// the status line. Copy there copies the events as JSON lines, and in
// the Processes tab the process tree as text.
//
// A line that repeats the one before it isn't drawn again: the line
// before shows how many times it came, and when the last one did.
//
// Bug Report copies a Markdown report of all of this, and of the app
// and what this tab is doing, to paste into an issue or a chat.

import { Clipboard, Events } from "/wails/runtime.js";
import { App, type AuditEvent, type ConsoleEntry } from "./api.js";
import { $, el } from "./dom.js";
import { hinted } from "./keys.js";
import { initProcs, watchProcs } from "./procs.js";
import { copyBugReport } from "./bugreport.js";

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
type Tab = "log" | "events" | "procs";
let tab: Tab = "log";
const tabs: Tab[] = ["log", "events", "procs"];
const tabIDs: Record<Tab, string> = { log: "console-tab-log", events: "console-tab-events", procs: "console-tab-procs" };
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

// A row stands for a run of entries that say the same thing, back to
// back: e is the first, n how many, lastTime when the last came and seqs
// their Seqs.
interface Run {
  e: ConsoleEntry;
  n: number;
  lastTime: unknown;
  seqs: number[];
}

// rows holds the run each drawn row is for.
const rows = new WeakMap<HTMLElement, Run>();

// same reports whether b says what a does, so it shows as a repeat.
const same = (a: ConsoleEntry, b: ConsoleEntry) =>
  a.level === b.level && a.message === b.message && (a.file ?? "") === (b.file ?? "") && (a.line ?? 0) === (b.line ?? 0) && (a.fix ?? "") === (b.fix ?? "") && !!a.stale === !!b.stale;

// runs groups entries into runs of repeats.
function runs(list: ConsoleEntry[]): Run[] {
  const out: Run[] = [];
  for (const e of list) {
    const r = out[out.length - 1];
    if (r && same(r.e, e)) {
      r.n++;
      r.lastTime = e.time;
      r.seqs.push(e.seq);
    } else out.push({ e, n: 1, lastTime: e.time, seqs: [e.seq] });
  }
  return out;
}

// runLine is a run as text, for copying.
const runLine = (r: Run) => line(r.e) + (r.n > 1 ? ` (×${r.n}, last ${time(r.lastTime)})` : "");

// count shows how many times a row's line came.
function count(r: HTMLElement, run: Run) {
  let c = r.querySelector<HTMLElement>(".count");
  if (run.n < 2) return void c?.remove();
  if (!c) {
    c = el("span", { className: "count" });
    r.querySelector(".level")!.after(c);
  }
  c.textContent = `×${run.n}`;
  c.title = `${run.n} times, the last at ${time(run.lastTime)}`;
  c.setAttribute("aria-label", `repeated ${run.n} times, the last at ${time(run.lastTime)}`);
}

function row(run: Run): HTMLElement {
  const e = run.e;
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
  rows.set(r, run);
  count(r, run);
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
    const run = rows.get(r as HTMLElement);
    if (run) mark(r as HTMLElement, run.e);
  }
}

// click takes the source pane to the line a clicked row is about,
// unless the click ended a drag that selected text to copy.
function click(ev: MouseEvent) {
  const r = (ev.target as HTMLElement).closest<HTMLElement>(".entry");
  const e = r && rows.get(r)?.e;
  if (!e || !canJump(e)) return;
  const sel = window.getSelection();
  if (sel && !sel.isCollapsed && $("console-log").contains(sel.anchorNode)) return;
  hooks.jump(e.line!);
}

// stale strikes out the lines with the given seqs: what they warned of
// is fixed.
function stale(seqs: number[]) {
  const set = new Set(seqs);
  for (const e of entries) if (set.has(e.seq)) e.stale = true;
  for (const r of [...$("console-log").children] as HTMLElement[]) {
    const run = rows.get(r);
    if (!run || run.e.stale || !run.seqs.some((s) => set.has(s))) continue;
    run.e.stale = true;
    r.replaceWith(row(run));
  }
}

function drawAll() {
  const log = $("console-log");
  log.replaceChildren(...runs(entries).map(row));
  $("console-empty").hidden = tab !== "log" || entries.length > 0;
  log.scrollTop = log.scrollHeight;
}

function add(e: ConsoleEntry) {
  entries.push(e);
  lastSeq = e.seq;
  const log = $("console-log");
  // Follow the end, unless scrolled up to read.
  const atEnd = log.scrollHeight - log.scrollTop - log.clientHeight < 8;
  const end = log.lastElementChild as HTMLElement | null;
  const run = end && rows.get(end);
  if (run && same(run.e, e)) {
    // A repeat: count it on the line it repeats.
    run.n++;
    run.lastTime = e.time;
    run.seqs.push(e.seq);
    count(end!, run);
  } else log.append(row({ e, n: 1, lastTime: e.time, seqs: [e.seq] }));
  if (entries.length > 1000) {
    entries.shift();
    const first = log.firstElementChild as HTMLElement | null;
    const r = first && rows.get(first);
    if (r && r.n > 1) {
      r.n--;
      r.seqs.shift();
      count(first!, r);
    } else first?.remove();
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
// Events tab, every event as a line of JSON, and in the Processes tab
// the tree, indented, as text.
async function copy() {
  const sel = window.getSelection()?.toString() ?? "";
  const log = $(tab === "events" ? "events-log" : tab === "procs" ? "procs-list" : "console-log");
  const picked = sel && log.contains(window.getSelection()!.anchorNode);
  let text = sel;
  if (!picked) {
    if (tab === "events") text = events.map((e) => e.json).join("\n");
    else if (tab === "procs") text = (await App.ProcessesText().catch(() => "")) || "No shells running.";
    else text = runs(entries).map(runLine).join("\n");
  }
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

// showTab shows the Console's log, its Events or its Processes.
function showTab(which: Tab, focus = false) {
  tab = which;
  const ev = which === "events";
  const pr = which === "procs";
  for (const t of tabs) {
    $(tabIDs[t]).setAttribute("aria-selected", String(t === which));
    $(tabIDs[t]).tabIndex = t === which ? 0 : -1;
  }
  $("console-log").hidden = which !== "log";
  $("events-log").hidden = !ev;
  $("procs-list").hidden = !pr;
  $("console-empty").hidden = which !== "log" || entries.length > 0;
  $("events-empty").hidden = !ev || events.length > 0;
  if (!pr) $("procs-empty").hidden = true;
  $("console-clear").hidden = which !== "log";
  $("console-copy").title = ev
    ? "Copy the selected lines, or every event as JSON lines"
    : pr
      ? "Copy the selected lines, or every process as an indented tree"
      : "Copy the selected lines, or all of them";
  watchProcs(pr && open);
  const log = ev ? $("events-log") : $("console-log");
  if (!pr) log.scrollTop = log.scrollHeight;
  if (focus) $(tabIDs[which]).focus();
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
  watchProcs(on && tab === "procs");
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
  $("console-tab-procs").onclick = () => showTab("procs");
  initProcs($("procs-list"), $("procs-empty"), flash);
  for (const t of tabs) {
    $(tabIDs[t]).onkeydown = (e) => {
      if (e.key !== "ArrowLeft" && e.key !== "ArrowRight") return;
      e.preventDefault();
      const i = tabs.indexOf(tab) + (e.key === "ArrowRight" ? 1 : -1);
      showTab(tabs[(i + tabs.length) % tabs.length], true);
    };
  }
  try {
    const saved = localStorage.getItem(tabKey);
    if (saved === "events" || saved === "procs") showTab(saved);
  } catch {
    // The Console it is.
  }
  $("console-clear").onclick = () => void App.ClearConsole().catch(() => {});
  $("console-copy").onclick = () => void copy();
  $("console-bugreport").onclick = () => void copyBugReport(flash);
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
