// OSC 52 is how a program in a terminal puts text on the clipboard:
// ESC ] 52 ; c ; base64 ST. vim, tmux, lazygit and others use it. The
// TUI gets it from the real terminal; the GUI's xterm.js terminals
// handle it here and hand the text to the app's clipboard.

import type { Terminal } from "@xterm/xterm";

// maxCopy caps what a program may copy at once, in bytes of text, so a
// runaway program can't hand the clipboard megabytes.
export const maxCopy = 1 << 20;

// osc52Text is the text an OSC 52 sequence's data, what follows "52;",
// asks to put on the clipboard. It is null for a read ("?"), which is
// never answered, for data that isn't base64 or UTF-8, and for more
// than maxCopy. An empty payload, which asks some terminals to clear
// the clipboard, is null too: the clipboard is left as it is.
export function osc52Text(data: string): string | null {
  const semi = data.indexOf(";");
  if (semi < 0) return null;
  // The selection: c, p, q, s and 0-7, or none, meaning s0. All of them
  // mean the one clipboard here.
  if (!/^[cpqs0-7]*$/.test(data.slice(0, semi))) return null;
  const b64 = data.slice(semi + 1);
  if (b64 === "" || b64 === "?") return null;
  if (b64.length > Math.ceil(maxCopy / 3) * 4) return null;
  let raw: string;
  try {
    raw = atob(b64);
  } catch {
    return null;
  }
  const bytes = Uint8Array.from(raw, (ch) => ch.charCodeAt(0));
  try {
    return new TextDecoder("utf-8", { fatal: true }).decode(bytes);
  } catch {
    return null;
  }
}

// A Copier puts what a program copied on the clipboard. block is the
// block the program runs in, so a secret block's copy goes on marked
// secret, or -1 for History's shell.
export type Copier = (text: string, block: number) => void;

let copier: Copier = () => {};

// onTerminalCopy sets what is done with a program's copy.
export function onTerminalCopy(c: Copier) {
  copier = c;
}

// copyOsc52 has t hand OSC 52 writes to the clipboard, with the block
// that block() says is running on it. Reads are swallowed unanswered, as
// most terminals do, so nothing a program prints can read the clipboard
// back.
export function copyOsc52(t: Terminal, block: () => number) {
  t.parser.registerOscHandler(52, (data) => {
    const text = osc52Text(data);
    if (text !== null) copier(text, block());
    return true;
  });
}
