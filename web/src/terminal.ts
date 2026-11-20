// The terminal pane: a block marked tty runs in an xterm.js terminal in
// its own card, where what you type goes to the block. Go records the
// session, so once the run ends the card shows the recording instead.

import { Terminal } from "@xterm/xterm";
import { App } from "./api.js";
import { setOutput } from "./dom.js";
import { codeFont, textSize } from "./theme.js";

const defaultRows = 24;

let term: Terminal | null = null;
let block = -1;

// typing is the last key send. Keys go to Go one call at a time, in
// order, since Wails runs calls at once: a program's query, such as
// OSC 11 for the background colour, gets two replies, and if the cursor
// report got there first the colour would reach the program as typing.
let typing: Promise<unknown> = Promise.resolve();

// typeKeys sends keys to the block running on a terminal, after the
// keys sent before them.
export function typeKeys(keys: string) {
  typing = typing.then(() => App.Type(keys)).catch(() => {});
}

// wanted is the size the file's front matter asks for, under
// mdsh: terminal:, with 0 where it leaves it to the window.
export const wanted = { cols: 0, rows: 0 };

// wantTerminalSize sets wanted, from the open file.
export function wantTerminalSize(cols: number, rows: number) {
  wanted.cols = cols > 0 ? cols : 0;
  wanted.rows = rows > 0 ? rows : 0;
}

const rows = () => wanted.rows || defaultRows;

// cols is the front matter's width, or how many cells fit across a
// block's output, so the terminal fills the card.
function cols(): number {
  if (wanted.cols) return wanted.cols;
  const card = document.querySelector<HTMLElement>(".block") ?? document.getElementById("doc");
  const ctx = document.createElement("canvas").getContext("2d");
  if (!card || !ctx) return 80;
  ctx.font = `${textSize(13)}px ${codeFont()}`;
  const cell = ctx.measureText("W").width || 8;
  // The card's border, the output's padding and the pane's, and a cell
  // to spare for rounding.
  const width = card.clientWidth - 2 * 12 - 2 * 6 - cell;
  return Math.max(20, Math.floor(width / cell));
}

// sizeTerminal tells Go the terminal's size, from the window's width, so
// the next tty block runs and records at the size it is shown at.
export function sizeTerminal() {
  App.SetTerminalSize(cols(), rows()).catch(() => {});
  sizeBackground();
}

function colors(): Record<string, string> {
  const css = getComputedStyle(document.documentElement);
  const v = (name: string, or: string) => css.getPropertyValue(name).trim() || or;
  return { background: v("--term-bg", "#1e1e1e"), foreground: v("--term-fg", "#d4d4d4"), cursor: v("--term-fg", "#d4d4d4") };
}

// The look changed: a terminal that is open takes its colors and font.
window.addEventListener("mdsh-look", () => {
  for (const t of [term, ...[...backgrounds.values()].map((b) => b.term)]) {
    if (!t) continue;
    t.options.theme = colors();
    t.options.fontFamily = codeFont();
    t.options.fontSize = textSize(13);
  }
});

// openTerminal shows the pane in block i's card, and focuses it.
export function openTerminal(i: number) {
  closeTerminal();
  const card = document.getElementById("block-" + i);
  const out = card?.querySelector<HTMLElement>(".output");
  if (!card || !out) return;
  const host = document.createElement("div");
  host.className = "terminal";
  setOutput(out, host);
  block = i;
  // screenReaderMode has xterm read out what the block writes.
  term = new Terminal({ cols: cols(), rows: rows(), fontSize: textSize(13), fontFamily: codeFont(), cursorBlink: !matchMedia("(prefers-reduced-motion: reduce)").matches, theme: colors(), scrollback: 1000, screenReaderMode: true });
  term.open(host);
  term.onData(typeKeys);
  term.focus();
}

// writeTerminal shows what block i wrote to its terminal.
export function writeTerminal(i: number, bytes: string | undefined) {
  if (!term || i !== block || !bytes) return;
  // Go sends the bytes as base64, since they may split a character.
  const raw = atob(bytes);
  const data = new Uint8Array(raw.length);
  for (let k = 0; k < raw.length; k++) data[k] = raw.charCodeAt(k);
  term.write(data);
}

// endTerminal leaves the pane showing how the block ended, until the
// card is drawn again with its recording.
export function endTerminal(i: number) {
  if (!term || i !== block) return;
  term.options.disableStdin = true;
  term.options.cursorBlink = false;
  term.blur();
}

export function closeTerminal() {
  term?.dispose();
  term = null;
  block = -1;
}

// A background block marked tty runs on a terminal of its own, in its
// card, while other blocks run: an editor, say, open on a file another
// block runs. Each keeps its xterm across redraws of the page, which
// move it to the card drawn in its place.
interface Background {
  term: Terminal;
  host: HTMLElement;
  typing: Promise<unknown>;
  opened: boolean;
}

const backgrounds = new Map<number, Background>();

// sizeBackground tells Go the size of background blocks' terminals: as
// wide as a card, and as tall as the front matter asks.
export function sizeBackground() {
  App.SetBackgroundTerminalSize(cols(), rows()).catch(() => {});
  for (const b of backgrounds.values()) {
    if (b.term.cols !== cols() || b.term.rows !== rows()) b.term.resize(cols(), rows());
  }
}

function newBackground(i: number): Background {
  const host = document.createElement("div");
  host.className = "terminal background-terminal";
  host.setAttribute("aria-label", "The block's terminal: what you type goes to it while other blocks run");
  const term = new Terminal({ cols: cols(), rows: rows(), fontSize: textSize(13), fontFamily: codeFont(), cursorBlink: !matchMedia("(prefers-reduced-motion: reduce)").matches, theme: colors(), scrollback: 1000, screenReaderMode: true });
  const b: Background = { term, host, typing: Promise.resolve(), opened: false };
  // Keys go to Go one call at a time, in order, as typeKeys does.
  term.onData((keys) => {
    b.typing = b.typing.then(() => App.TypeBackground(i, keys)).catch(() => {});
  });
  backgrounds.set(i, b);
  return b;
}

// showBackground opens b's xterm in its host, once the host is on the
// page: xterm measures its cells as it opens, which it can't off the
// page. What was written before then shows as it opens.
function showBackground(b: Background) {
  if (b.opened || !b.host.isConnected) return;
  b.opened = true;
  b.term.open(b.host);
}

// openBackground shows a new terminal in background block i's card, and
// focuses it, as the block starts.
export function openBackground(i: number) {
  backgrounds.get(i)?.term.dispose();
  backgrounds.delete(i);
  const out = document.getElementById("block-" + i)?.querySelector<HTMLElement>(".output");
  const b = newBackground(i);
  if (out) setOutput(out, b.host);
  showBackground(b);
  b.term.focus();
}

// adoptBackground puts background block i's terminal in out, the output
// of the card just drawn for it. A page that has none, as one opened
// while the block runs, makes one and has the program draw its screen
// again, since what it showed before isn't kept.
export function adoptBackground(i: number, out: HTMLElement) {
  let b = backgrounds.get(i);
  if (!b) {
    b = newBackground(i);
    App.RedrawBackground(i).catch(() => {});
  }
  const focused = b.host.contains(document.activeElement);
  setOutput(out, b.host);
  const t = b.term;
  const shown = b;
  requestAnimationFrame(() => {
    showBackground(shown);
    t.refresh(0, t.rows - 1);
    if (focused) t.focus();
  });
}

// writeBackground shows what background block i wrote to its terminal.
export function writeBackground(i: number, bytes: string | undefined) {
  const b = backgrounds.get(i);
  if (!b || !bytes) return;
  const raw = atob(bytes);
  const data = new Uint8Array(raw.length);
  for (let k = 0; k < raw.length; k++) data[k] = raw.charCodeAt(k);
  b.term.write(data);
}

// endBackground leaves background block i's terminal showing how it
// ended, until its card is drawn again with its recording.
export function endBackground(i: number) {
  const b = backgrounds.get(i);
  if (!b) return;
  backgrounds.delete(i);
  b.term.options.disableStdin = true;
  b.term.options.cursorBlink = false;
  b.term.blur();
}

// closeBackgrounds drops every background terminal, as when the tab
// shows another file.
export function closeBackgrounds() {
  for (const b of backgrounds.values()) b.term.dispose();
  backgrounds.clear();
}
