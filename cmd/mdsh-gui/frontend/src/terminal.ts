// The terminal pane: a block marked tty runs in an xterm.js terminal in
// its own card, where what you type goes to the block. Go records the
// session, so once the run ends the card shows the recording instead.

import { Terminal } from "@xterm/xterm";
import { App } from "./api.js";

const rows = 24;
const fontSize = 13;
const fontFamily = 'ui-monospace, SFMono-Regular, Menlo, Consolas, monospace';

let term: Terminal | null = null;
let block = -1;

// cols is how many cells fit across a block's output, so the terminal
// fills the card.
function cols(): number {
  const card = document.querySelector<HTMLElement>(".block") ?? document.getElementById("doc");
  const ctx = document.createElement("canvas").getContext("2d");
  if (!card || !ctx) return 80;
  ctx.font = `${fontSize}px ${fontFamily}`;
  const cell = ctx.measureText("W").width || 8;
  // The card's border, the output's padding and the pane's, and a cell
  // to spare for rounding.
  const width = card.clientWidth - 2 * 12 - 2 * 6 - cell;
  return Math.max(20, Math.floor(width / cell));
}

// sizeTerminal tells Go the terminal's size, from the window's width, so
// the next tty block runs and records at the size it is shown at.
export function sizeTerminal() {
  App.SetTerminalSize(cols(), rows).catch(() => {});
}

function colors(): Record<string, string> {
  const css = getComputedStyle(document.documentElement);
  const v = (name: string, or: string) => css.getPropertyValue(name).trim() || or;
  return { background: v("--term-bg", "#1e1e1e"), foreground: v("--term-fg", "#d4d4d4"), cursor: v("--term-fg", "#d4d4d4") };
}

// openTerminal shows the pane in block i's card, and focuses it.
export function openTerminal(i: number) {
  closeTerminal();
  const card = document.getElementById("block-" + i);
  const out = card?.querySelector<HTMLElement>(".output");
  if (!card || !out) return;
  const host = document.createElement("div");
  host.className = "terminal";
  out.replaceChildren(host);
  block = i;
  term = new Terminal({ cols: cols(), rows, fontSize, fontFamily, cursorBlink: true, theme: colors(), scrollback: 1000 });
  term.open(host);
  term.onData((keys) => App.Type(keys).catch(() => {}));
  term.focus();
  card.scrollIntoView({ block: "nearest" });
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
