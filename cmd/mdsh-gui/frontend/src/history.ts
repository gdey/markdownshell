// The History pane, at the bottom of the window, works like a paper
// tape: a command typed there runs in the document's shell and is added
// to the end of the document with its output, and a line starting with
// # is added as prose. The pane's terminal keeps a transcript, and is
// where a command runs, so what you type while it runs goes to it.

import { Terminal } from "@xterm/xterm";
import { App, message } from "./api.js";
import { $ } from "./dom.js";
import { Ansi } from "./highlight.js";
import { typeKeys, wanted } from "./terminal.js";
import { codeFont, textSize } from "./theme.js";

// rows is how many lines the transcript shows, which dragging the pane's
// top edge changes, from one to most of the window. Commands run at this
// height. The front matter can set the width.
let rows = 12;
const rowsKey = "mdsh.historyRows";

let term: Terminal | null = null;
let block = -1; // the command running in the pane, or -1
let last = -1; // the last command that ran in the pane
let typed: string[] = []; // what was entered, for up and down
let ps1 = ""; // the shell's prompts, expanded, or "" for none
let ps2 = "";
let asking = false; // a prompt is being asked for
let at = 0; // where up and down are in typed

export interface HistoryHooks {
  // canRun says whether a run may start, which it can't while one goes.
  canRun: () => boolean;
  // enter runs the call that takes what was typed, as a run of the page.
  enter: (call: () => Promise<void>) => Promise<void>;
  // status is the exit status of block i, once the page has drawn it
  // again after the run.
  status: (i: number) => number | undefined;
  // toggle opens or closes the pane, as View › History does.
  toggle: () => void;
  // stop stops what runs, as Run › Stop does.
  stop: () => void;
  // restart starts the shells again.
  restart: () => Promise<void>;
}

let hooks: HistoryHooks;

const input = () => $<HTMLTextAreaElement>("history-input");

// colors are the pane's own, which can be the window's, the opposite
// or always dark (Settings › Look › History pane).
function colors(): Record<string, string> {
  const css = getComputedStyle($("history"));
  const v = (name: string, or: string) => css.getPropertyValue(name).trim() || or;
  return { background: v("--bg", "#0d1117"), foreground: v("--fg", "#e6edf3"), cursor: v("--fg", "#e6edf3") };
}

// The look changed: the transcript takes the pane's new colors and font.
window.addEventListener("mdsh-look", () => {
  if (!term) return;
  term.options.theme = colors();
  term.options.fontFamily = codeFont();
  term.options.fontSize = textSize(13);
});

// cols is the front matter's width, or how many cells fit across the
// pane.
function cols(): number {
  if (wanted.cols) return wanted.cols;
  const host = $("history-term");
  const ctx = document.createElement("canvas").getContext("2d");
  if (!ctx || !host.clientWidth) return 80;
  ctx.font = `${textSize(13)}px ${codeFont()}`;
  const cell = ctx.measureText("W").width || 8;
  return Math.max(20, Math.floor((host.clientWidth - 2 * 8 - cell) / cell));
}

function ensureTerm(): Terminal {
  if (term) return term;
  try {
    const saved = Number(localStorage.getItem(rowsKey));
    if (saved >= 1) rows = Math.min(saved, maxRows());
  } catch {
    // No storage: the default it is.
  }
  term = new Terminal({ cols: cols(), rows, fontSize: textSize(13), fontFamily: codeFont(), theme: colors(), scrollback: 5000, disableStdin: true, convertEol: true, cursorInactiveStyle: "none" });
  term.open($("history-term"));
  term.onData((keys) => {
    if (block >= 0) typeKeys(keys);
  });
  return term;
}

// isOpen reports whether the pane is open, rather than only its bar.
const isOpen = () => !$("history").hidden && !$("history").classList.contains("closed");

// fit sizes the pane's terminal to its width, and tells Go, so a command
// runs at the size it is shown at.
export function fitHistory() {
  if (!term || !isOpen()) return;
  const c = cols();
  if (rows > maxRows()) rows = maxRows();
  if (c !== term.cols || rows !== term.rows) term.resize(c, rows);
  App.SetTerminalSize(c, rows).catch(() => {});
}

// cellHeight is the height of a line of the transcript, in pixels.
function cellHeight(): number {
  const screen = term?.element?.querySelector<HTMLElement>(".xterm-screen");
  if (term && screen?.clientHeight) return screen.clientHeight / term.rows;
  return Math.ceil(textSize(13) * 1.2);
}

// maxRows is the most lines the transcript can have and leave some of
// the window above it.
function maxRows(): number {
  return Math.max(1, Math.floor((window.innerHeight - 160) / cellHeight()));
}

// setRows sizes the transcript to n lines.
function setRows(n: number) {
  n = Math.max(1, Math.min(Math.round(n), maxRows()));
  if (n === rows) return;
  rows = n;
  try {
    localStorage.setItem(rowsKey, String(n));
  } catch {
    // Not remembered, then.
  }
  fitHistory();
}

// dragGrip sizes the pane as its top edge is dragged.
function dragGrip(e: PointerEvent) {
  if (e.button !== 0) return;
  e.preventDefault();
  const grip = e.currentTarget as HTMLElement;
  grip.setPointerCapture(e.pointerId);
  const y0 = e.clientY;
  const rows0 = rows;
  const cell = cellHeight();
  document.body.classList.add("resizing-rows");
  grip.onpointermove = (m) => setRows(rows0 + (y0 - m.clientY) / cell);
  grip.onpointerup = grip.onpointercancel = () => {
    grip.onpointermove = grip.onpointerup = grip.onpointercancel = null;
    document.body.classList.remove("resizing-rows");
  };
}

// slide opens or closes the pane: the transcript slides up out of the
// bar, or back down into it.
function slide(open: boolean) {
  const pane = $("history");
  const body = $("history-body");
  if (open === !pane.classList.contains("closed")) return;
  $("history-toggle").setAttribute("aria-expanded", String(open));
  const from = open ? 0 : body.offsetHeight;
  pane.classList.toggle("closed", !open);
  if (open) fitHistory();
  body.style.height = "auto";
  const to = open ? body.offsetHeight : 0;
  body.style.height = `${from}px`;
  void body.offsetHeight; // start from there
  body.style.height = `${to}px`;
  body.ontransitionend = () => {
    body.ontransitionend = null;
    // Open, it follows its content again, as when dragged.
    if (open) body.style.height = "auto";
  };
}

// showBar shows the History bar along the bottom while a file is open.
export function showBar(on: boolean) {
  $("history").hidden = !on;
  if (on) void updatePrompt();
}

// showHistory opens or closes the pane. Opening may start the shells
// again; the transcript says so.
export async function showHistory(on: boolean, docHas: boolean) {
  if (!on) {
    slide(false);
    if (docHas) await App.SetHistory(false).catch(() => {});
    return;
  }
  const t = ensureTerm();
  t.options.theme = colors();
  slide(true);
  fitHistory();
  if (!docHas) {
    try {
      if (await App.SetHistory(true)) say("\x1b[2m(The shells started again, on a terminal, so variables that blocks set are gone.)\x1b[0m");
    } catch (err) {
      say(`\x1b[31m${message(err)}\x1b[0m`);
    }
  }
  await updatePrompt();
  input().focus();
}

// updatePrompt asks the shell for its prompts, PS1 and PS2, and shows
// PS1 before the input: all its lines but the last above it. A shell
// with none gets a $. While a block runs the prompt stays as it was.
export async function updatePrompt() {
  if ($("history").hidden || asking) return;
  asking = true;
  try {
    const p = await App.HistoryPrompt();
    ps1 = p.ps1;
    ps2 = p.ps2;
  } catch {
    return;
  } finally {
    asking = false;
  }
  const at = ps1.lastIndexOf("\n");
  const head = $("history-prompt-head");
  const prompt = $("history-prompt");
  // The head's colors carry on into the last line, as on a terminal.
  const ansi = new Ansi();
  head.replaceChildren(...(at >= 0 ? ansi.write(ps1.slice(0, at)) : []));
  head.hidden = at < 0;
  prompt.classList.toggle("custom", ps1 !== "");
  if (ps1) prompt.replaceChildren(...ansi.write(ps1.slice(at + 1)));
  else prompt.textContent = "$";
}

// say writes a line to the transcript, on a line of its own even after
// output that didn't end one, as ^C or printf leave.
function say(text: string) {
  const t = ensureTerm();
  t.write((t.buffer.active.cursorX > 0 ? "\n" : "") + text + "\n");
}

// echo writes what was entered to the transcript: a command after a
// prompt, a note dimmed.
function echo(text: string) {
  const lines = text.split("\n");
  if (lines.every((l) => l.trimStart().startsWith("#"))) {
    say(lines.map((l) => `\x1b[2;3m${l}\x1b[0m`).join("\n"));
    return;
  }
  const first = ps1 ? `${ps1}\x1b[0m` : "\x1b[1;32m$\x1b[0m ";
  const more = ps2 ? `${ps2}\x1b[0m` : "\x1b[1;32m>\x1b[0m ";
  say(lines.map((l, i) => `${i ? more : first}${l}`).join("\n"));
}

// submit runs entered, what the input held when enter was pressed. What
// was typed since stays in the input.
async function submit(entered: string) {
  const text = entered.replace(/\n+$/, "");
  if (!text.trim() || !hooks.canRun()) return;
  if (typed[typed.length - 1] !== text) typed.push(text);
  at = typed.length;
  const box = input();
  box.value = box.value.startsWith(entered) ? box.value.slice(entered.length) : "";
  grow();
  echo(text);
  fitHistory();
  last = -1;
  await hooks.enter(async () => {
    try {
      await App.HistoryEnter(text);
    } catch (err) {
      say(`\x1b[31m${message(err)}\x1b[0m`);
    }
  });
  // A lone & at the end runs it as a background block, as Go decides.
  if (last < 0 && /(^|[^&|<>\\])&\s*$/.test(text)) say("\x1b[2m(running in the background; its output goes to its block when it ends)\x1b[0m");
  const status = last >= 0 ? hooks.status(last) : undefined;
  // 128 + SIGTSTP, 20 on Linux and 18 on macOS, is the status of a
  // command ctrl+z stopped.
  if (status === 148 || status === 146) say("\x1b[2m(stopped: fg brings it back, bg runs it in the background, jobs lists them)\x1b[0m");
  else if (status) say(`\x1b[31m[exit ${status}]\x1b[0m`);
  await updatePrompt();
  input().focus();
}

// grow fits the input to its lines, up to a few.
function grow() {
  const box = input();
  box.rows = Math.min(6, Math.max(1, box.value.split("\n").length));
}

// recall puts entry at of typed in the input, or empties it past the end.
function recall(to: number) {
  at = Math.max(0, Math.min(typed.length, to));
  input().value = typed[at] ?? "";
  grow();
}

// commonPrefix is the longest start all of items share.
function commonPrefix(items: string[]): string {
  let p = items[0] ?? "";
  for (const it of items) {
    while (!it.startsWith(p)) p = p.slice(0, -1);
  }
  return p;
}

// list shows items in the transcript in columns, as a shell does when
// Tab can't pick one.
function list(items: string[]) {
  const most = 200;
  const shown = items.slice(0, most);
  const width = Math.max(...shown.map((s) => s.length)) + 2;
  const across = Math.max(1, Math.floor(ensureTerm().cols / width));
  const lines: string[] = [];
  for (let i = 0; i < shown.length; i += across) {
    lines.push(shown.slice(i, i + across).map((s) => s.padEnd(width)).join("").trimEnd());
  }
  if (items.length > most) lines.push(`\x1b[2m… and ${items.length - most} more\x1b[0m`);
  say(lines.join("\n"));
}

// complete completes the word before the cursor, as Tab does in a
// shell: it fills in as much as every choice shares, and lists the
// choices when that is nothing more.
async function complete() {
  const box = input();
  const before = box.value.slice(0, box.selectionStart);
  const line = before.slice(before.lastIndexOf("\n") + 1);
  if (line.trimStart().startsWith("#")) return;
  let c;
  try {
    c = await App.HistoryCompletions(before);
  } catch {
    return;
  }
  // Typing went on while the shell was asked.
  if (!box.value.startsWith(before) || box.selectionStart !== before.length) return;
  const items = c.items ?? [];
  if (!items.length) return;
  const start = before.length - c.word.length;
  if (items.length === 1) {
    const text = items[0].endsWith("/") ? items[0] : items[0] + " ";
    box.setRangeText(text, start, before.length, "end");
  } else {
    const common = commonPrefix(items);
    if (common.length > c.word.length) box.setRangeText(common, start, before.length, "end");
    else list(items);
  }
  grow();
}

async function onKey(e: KeyboardEvent) {
  const box = input();
  if (e.key === "Tab" && !e.shiftKey && !e.ctrlKey && !e.metaKey && !e.altKey) {
    // Tab completes, as in a shell, rather than leaving the input.
    e.preventDefault();
    return complete();
  }
  if (e.key === "Enter" && !e.shiftKey && !e.isComposing) {
    e.preventDefault();
    // An unfinished command, such as one ending in \ or inside quotes,
    // carries on to the next line.
    const entered = box.value;
    if (await App.HistoryComplete(entered).catch(() => true)) return submit(entered);
    box.setRangeText("\n", box.selectionStart, box.selectionEnd, "end");
    grow();
    return;
  }
  const first = !box.value.slice(0, box.selectionStart).includes("\n");
  const last = !box.value.slice(box.selectionEnd).includes("\n");
  if (e.key === "ArrowUp" && first && at > 0) {
    e.preventDefault();
    recall(at - 1);
  } else if (e.key === "ArrowDown" && last && at < typed.length) {
    e.preventDefault();
    recall(at + 1);
  }
}

// historyTerminal takes block i's terminal if it was typed here: what
// it shows goes to the transcript and what you type there goes to it.
// It returns false for a block that runs in its card.
export function historyTerminal(i: number, pane: string | undefined): boolean {
  if (pane !== "history") return false;
  const t = ensureTerm();
  block = last = i;
  t.options.disableStdin = false;
  t.options.cursorBlink = true;
  t.focus();
  return true;
}

// historyWrite shows what block i wrote, if it runs here.
export function historyWrite(i: number, bytes: string | undefined): boolean {
  if (!term || i !== block) return false;
  if (!bytes) return true;
  const raw = atob(bytes);
  const data = new Uint8Array(raw.length);
  for (let k = 0; k < raw.length; k++) data[k] = raw.charCodeAt(k);
  term.write(data);
  return true;
}

// historyEnd gives the input back once block i is done.
export function historyEnd(i: number): boolean {
  if (!term || i !== block) return false;
  block = -1;
  term.options.disableStdin = true;
  term.options.cursorBlink = false;
  term.blur();
  return true;
}

// historyStatus shows text in the bar while the pane is closed, as the
// toolbar does; with none, the bar says how to open the pane.
let said = "";
export function historyStatus(text: string) {
  said = text;
  const idle = "History: cmd/ctrl+shift+H, or the arrow on the right";
  $("history-status").textContent = text || ($<HTMLButtonElement>("history-stop").disabled ? idle : "running…");
}

// historyBusy disables the input while a run goes that the pane didn't
// start.
export function historyBusy(on: boolean) {
  input().disabled = on && block < 0;
  $<HTMLButtonElement>("history-stop").disabled = !on;
  $<HTMLButtonElement>("history-restart").disabled = on;
  historyStatus(said);
  // A block may have changed the directory, or the prompt.
  if (!on) void updatePrompt();
}

export function initHistory(h: HistoryHooks) {
  hooks = h;
  const box = input();
  box.onkeydown = (e) => void onKey(e);
  box.oninput = grow;
  $("history-grip").onpointerdown = dragGrip;
  $("history-toggle").onclick = () => hooks.toggle();
  // A command typed here gets ctrl+c, as in a terminal, so the shell
  // and what blocks set in it live on; anything else stops as Run ›
  // Stop does.
  $("history-stop").onclick = () => (block >= 0 ? typeKeys("\x03") : hooks.stop());
  $("history-restart").onclick = async () => {
    await hooks.restart();
    if (term) say("\x1b[2m(The shells started again, so variables that blocks set are gone.)\x1b[0m");
    await updatePrompt();
  };
  // Clicking the closed bar opens it.
  $("history-status").onclick = () => hooks.toggle();
  historyStatus("");
  $("history-term").onclick = () => {
    if (block < 0) input().focus();
  };
}
