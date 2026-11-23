// Writing above a shell's prompt.
//
// The live History pane is a terminal whose screen is the shell's: its
// line editor draws the prompt and what is typed, and moves the cursor
// from where it left it. A line mdsh writes at the cursor leaves the
// shell's idea of the screen out of step with the screen, so the cursor
// lands in the wrong place, and the next redraw scribbles over what
// shows. Written above the prompt instead, with the prompt, what is
// typed after it and the cursor moved down together, as many rows as
// the message takes, the shell's idea holds: what it drew is where it
// drew it, relative to the cursor, as its redraws assume.

// Screen is where things are on a terminal's screen, in rows from its
// top and columns from its left, from 0.
export interface Screen {
  rows: number;
  cols: number;
  // x and y are the cursor's column and row.
  x: number;
  y: number;
  // top is the prompt's first row, and last the last row of what it
  // drew or was typed after it.
  top: number;
  last: number;
  // free is how many rows under last, at the foot of the screen, are
  // blank.
  free: number;
}

// width is how many columns text takes, without its escapes.
export const width = (text: string) => [...text.replace(/\x1b\[[0-9;:?]*[ -/]*[@-~]/g, "")].length;

// aboveSequence is what to write to a terminal so that lines show above
// the prompt, as above. The rows the message needs come from blank
// rows under the prompt first, and then from scrolling what is over it
// up, into the scrollback. It returns null when they can't be found
// without scrolling the prompt itself away, as when it fills the screen.
export function aboveSequence(lines: string[], s: Screen): string | null {
  const cols = Math.max(1, s.cols);
  const heights = lines.map((l) => Math.max(1, Math.ceil(width(l) / cols)));
  const k = heights.reduce((a, b) => a + b, 0);
  if (k === 0 || s.top < 0 || s.top > s.y || s.last < s.y || s.last >= s.rows) return null;
  // up is how far the screen scrolls; the prompt moves down k - up.
  const up = Math.max(0, k - s.free);
  if (up > s.top) return null;
  const csi = (n: number, f: string) => `\x1b[${n}${f}`;
  // ESC 7 keeps the cursor and the colors the shell set, which ESC 8
  // gives back.
  let out = "\x1b7";
  if (up > 0) out += `\x1b[${s.rows};1H` + "\n".repeat(up);
  let row = s.top - up;
  out += `\x1b[${row + 1};1H` + csi(k, "L");
  lines.forEach((l, i) => {
    out += `\x1b[${row + 1};1H\x1b[0m` + l + "\x1b[0m";
    row += heights[i];
  });
  // ESC 8 puts the cursor back on its old row, but the prompt moved
  // down k - up.
  out += "\x1b8";
  if (k - up > 0) out += csi(k - up, "B");
  return out;
}
