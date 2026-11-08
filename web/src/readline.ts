// readline gives a textarea the emacs keys of a shell's prompt, as bash's
// readline and zsh's line editor have them: ctrl+a and ctrl+e go to the
// start and end of the line, ctrl+w, ctrl+u, ctrl+k and alt+d kill text,
// ctrl+y yanks it back, and so on. Edits go through the textarea's own
// undo, so cmd+z takes them back.

export interface LineHooks {
  // prev and next are ctrl+p and ctrl+n on the input's first or last
  // line: they recall an earlier or later command, and report whether
  // they did.
  prev: () => boolean;
  next: () => boolean;
  // clear is ctrl+l: the transcript is cleared.
  clear: () => void;
  // eof is ctrl+d with nothing typed.
  eof: () => void;
}

// killed is what the last kill took, for ctrl+y; kills one after another
// add to it, as readline's do.
let killed = "";
let killing = false; // the last key killed
let appending = false; // so this key's kill adds to it

const word = /[\p{L}\p{N}_]/u;
const space = /\s/;

// lineStart and lineEnd are where the line holding p starts and ends.
const lineStart = (v: string, p: number) => v.lastIndexOf("\n", p - 1) + 1;
function lineEnd(v: string, p: number): number {
  const at = v.indexOf("\n", p);
  return at < 0 ? v.length : at;
}

// back and forth step over one character, a surrogate pair as one.
function back(v: string, p: number): number {
  if (p <= 0) return 0;
  const c = v.charCodeAt(p - 1);
  return c >= 0xdc00 && c <= 0xdfff && p >= 2 ? p - 2 : p - 1;
}
function forth(v: string, p: number): number {
  if (p >= v.length) return v.length;
  const c = v.charCodeAt(p);
  return c >= 0xd800 && c <= 0xdbff && p + 1 < v.length ? p + 2 : p + 1;
}

// wordBack is where alt+b goes: back over what isn't a word, then over
// the word. wordForth is alt+f: on over what isn't a word, then to the
// word's end.
export function wordBack(v: string, p: number): number {
  while (p > 0 && !word.test(v[p - 1])) p--;
  while (p > 0 && word.test(v[p - 1])) p--;
  return p;
}
export function wordForth(v: string, p: number): number {
  while (p < v.length && !word.test(v[p])) p++;
  while (p < v.length && word.test(v[p])) p++;
  return p;
}

// spaceBack is where ctrl+w kills back to: over spaces, then to the
// space before the word, so a path or flag goes whole.
export function spaceBack(v: string, p: number): number {
  while (p > 0 && space.test(v[p - 1])) p--;
  while (p > 0 && !space.test(v[p - 1])) p--;
  return p;
}

// replace puts text in place of a to b and leaves the cursor after it,
// through the textarea's undo where the browser has it.
function replace(box: HTMLTextAreaElement, a: number, b: number, text: string) {
  if (a === b && !text) return;
  box.setSelectionRange(a, b);
  if (document.execCommand(text ? "insertText" : "delete", false, text)) return;
  box.setRangeText(text, a, b, "end");
  box.dispatchEvent(new Event("input"));
}

// kill takes a to b out, to the kill text: after it for a kill forward,
// before it for one back, when the last key killed too.
function kill(box: HTMLTextAreaElement, a: number, b: number, backward: boolean) {
  if (a >= b) return;
  const text = box.value.slice(a, b);
  if (!appending) killed = text;
  else killed = backward ? text + killed : killed + text;
  killing = true;
  replace(box, a, b, "");
}

function moveTo(box: HTMLTextAreaElement, p: number) {
  box.setSelectionRange(p, p);
}

// lineMove moves the cursor to the line above or below, keeping its
// column where the line is long enough.
function lineMove(box: HTMLTextAreaElement, dir: number): boolean {
  const v = box.value;
  const p = box.selectionStart;
  const start = lineStart(v, p);
  const col = p - start;
  if (dir < 0) {
    if (start === 0) return false;
    const above = lineStart(v, start - 1);
    moveTo(box, Math.min(above + col, start - 1));
  } else {
    const end = lineEnd(v, p);
    if (end === v.length) return false;
    moveTo(box, Math.min(end + 1 + col, lineEnd(v, end + 1)));
  }
  return true;
}

// readlineKey does what key e does at a shell's prompt, and reports
// whether it took it. Keys with cmd, and the rest, are left alone.
export function readlineKey(box: HTMLTextAreaElement, e: KeyboardEvent, h: LineHooks): boolean {
  // A modifier pressed on its own, as on the way to alt+d, isn't a key.
  if (["Alt", "Control", "Meta", "Shift"].includes(e.key)) return false;
  appending = killing;
  killing = false;
  if (e.isComposing || e.metaKey || e.shiftKey) return false;
  const ctrl = e.ctrlKey && !e.altKey;
  const alt = e.altKey && !e.ctrlKey;
  if (!ctrl && !alt) return false;
  // The key's place, not what it types: on a Mac, option+b types ∫.
  const k = e.code.startsWith("Key") ? e.code.slice(3).toLowerCase() : e.code;
  const v = box.value;
  const from = box.selectionStart;
  const to = box.selectionEnd;
  let took = true;
  if (ctrl) {
    switch (k) {
      case "a":
        moveTo(box, lineStart(v, from));
        break;
      case "e":
        moveTo(box, lineEnd(v, to));
        break;
      case "b":
        moveTo(box, from !== to ? from : back(v, from));
        break;
      case "f":
        moveTo(box, from !== to ? to : forth(v, to));
        break;
      case "w":
        if (from !== to) kill(box, from, to, true);
        else kill(box, spaceBack(v, from), from, true);
        break;
      case "u":
        kill(box, lineStart(v, from), from, true);
        break;
      case "k": {
        // At the end of a line, ctrl+k joins the next line to it.
        const end = lineEnd(v, from);
        kill(box, from, end === from ? Math.min(end + 1, v.length) : end, false);
        break;
      }
      case "y":
        if (killed) replace(box, from, to, killed);
        break;
      case "d":
        if (!v) h.eof();
        else if (from !== to) replace(box, from, to, "");
        else if (from < v.length) replace(box, from, forth(v, from), "");
        break;
      case "h":
        if (from !== to) replace(box, from, to, "");
        else if (from > 0) replace(box, back(v, from), from, "");
        break;
      case "l":
        h.clear();
        break;
      case "p":
        if (!lineMove(box, -1)) h.prev();
        break;
      case "n":
        if (!lineMove(box, 1)) h.next();
        break;
      default:
        took = false;
    }
  } else {
    switch (k) {
      case "b":
        moveTo(box, wordBack(v, from));
        break;
      case "f":
        moveTo(box, wordForth(v, to));
        break;
      case "d":
        kill(box, from, wordForth(v, to), false);
        break;
      case "Backspace":
        kill(box, wordBack(v, from), from, true);
        break;
      default:
        took = false;
    }
  }
  return took;
}
