// The History pane, at the bottom of the window, works like a paper
// tape: a command typed there runs in the document's shell and is added
// to the end of the document with its output, and a line starting with
// # is added as prose. The pane's terminal keeps a transcript, and is
// where a command runs, so what you type while it runs goes to it.

import { Terminal } from "@xterm/xterm";
import { App, message } from "./api.js";
import { $ } from "./dom.js";

const rows = 12;
const fontSize = 13;
const fontFamily = 'ui-monospace, SFMono-Regular, Menlo, Consolas, monospace';

let term: Terminal | null = null;
let block = -1; // the command running in the pane, or -1
let last = -1; // the last command that ran in the pane
let typed: string[] = []; // what was entered, for up and down
let at = 0; // where up and down are in typed

export interface HistoryHooks {
  // canRun says whether a run may start, which it can't while one goes.
  canRun: () => boolean;
  // enter runs the call that takes what was typed, as a run of the page.
  enter: (call: () => Promise<void>) => Promise<void>;
  // status is the exit status of block i, once the page has drawn it
  // again after the run.
  status: (i: number) => number | undefined;
}

let hooks: HistoryHooks;

const input = () => $<HTMLTextAreaElement>("history-input");

function colors(): Record<string, string> {
  const css = getComputedStyle(document.documentElement);
  const v = (name: string, or: string) => css.getPropertyValue(name).trim() || or;
  return { background: v("--term-bg", "#1e1e1e"), foreground: v("--term-fg", "#d4d4d4"), cursor: v("--term-fg", "#d4d4d4") };
}

// cols is how many cells fit across the pane.
function cols(): number {
  const host = $("history-term");
  const ctx = document.createElement("canvas").getContext("2d");
  if (!ctx || !host.clientWidth) return 80;
  ctx.font = `${fontSize}px ${fontFamily}`;
  const cell = ctx.measureText("W").width || 8;
  return Math.max(20, Math.floor((host.clientWidth - 2 * 8 - cell) / cell));
}

function ensureTerm(): Terminal {
  if (term) return term;
  term = new Terminal({ cols: cols(), rows, fontSize, fontFamily, theme: colors(), scrollback: 5000, disableStdin: true, convertEol: true, cursorInactiveStyle: "none" });
  term.open($("history-term"));
  term.onData((keys) => {
    if (block >= 0) App.Type(keys).catch(() => {});
  });
  return term;
}

// fit sizes the pane's terminal to its width, and tells Go, so a command
// runs at the size it is shown at.
export function fitHistory() {
  if (!term || $("history").hidden) return;
  const c = cols();
  if (c !== term.cols) term.resize(c, rows);
  App.SetTerminalSize(c, rows).catch(() => {});
}

// showHistory shows or hides the pane. Turning it on may start the
// shells again; the transcript says so.
export async function showHistory(on: boolean, docHas: boolean) {
  $("history").hidden = !on;
  if (!on) {
    if (docHas) await App.SetHistory(false).catch(() => {});
    return;
  }
  const t = ensureTerm();
  t.options.theme = colors();
  fitHistory();
  if (!docHas) {
    try {
      if (await App.SetHistory(true)) say("\x1b[2m(The shells started again, on a terminal, so variables that blocks set are gone.)\x1b[0m");
    } catch (err) {
      say(`\x1b[31m${message(err)}\x1b[0m`);
    }
  }
  input().focus();
}

function say(text: string) {
  ensureTerm().write(text + "\n");
}

// echo writes what was entered to the transcript: a command after a
// prompt, a note dimmed.
function echo(text: string) {
  const lines = text.split("\n");
  if (lines.every((l) => l.trimStart().startsWith("#"))) {
    say(lines.map((l) => `\x1b[2;3m${l}\x1b[0m`).join("\n"));
    return;
  }
  say(lines.map((l, i) => `\x1b[1;32m${i ? ">" : "$"}\x1b[0m ${l}`).join("\n"));
}

async function submit() {
  const text = input().value.replace(/\n+$/, "");
  if (!text.trim() || !hooks.canRun()) return;
  if (typed[typed.length - 1] !== text) typed.push(text);
  at = typed.length;
  input().value = "";
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
  const status = last >= 0 ? hooks.status(last) : undefined;
  if (status) say(`\x1b[31m[exit ${status}]\x1b[0m`);
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

async function onKey(e: KeyboardEvent) {
  const box = input();
  if (e.key === "Enter" && !e.shiftKey && !e.isComposing) {
    e.preventDefault();
    // An unfinished command, such as one ending in \ or inside quotes,
    // carries on to the next line.
    if (await App.HistoryComplete(box.value).catch(() => true)) return submit();
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

// historyBusy disables the input while a run goes that the pane didn't
// start.
export function historyBusy(on: boolean) {
  input().disabled = on && block < 0;
}

export function initHistory(h: HistoryHooks) {
  hooks = h;
  const box = input();
  box.onkeydown = (e) => void onKey(e);
  box.oninput = grow;
  $("history-term").onclick = () => {
    if (block < 0) input().focus();
  };
}
