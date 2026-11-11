// parseCast reads an asciicast v2 recording for the cast player. It
// takes nothing from the page, so the frontend's tests can run it under
// node.

export type Cast = { cols: number; rows: number; events: [number, string][] };

// Limits on the casts the player reads, as termrec.ReadCast's, so a file
// made to be huge, as one beside a document can be, can't make xterm.js
// allocate without bound and take the window down. A cast past them
// isn't played; the card says why.
export const maxCols = 1000;
export const maxRows = 500;
export const maxEvents = 100_000;
export const maxOutput = 16 << 20;

// utf8Length is how many bytes s is in UTF-8, as Go counts output.
function utf8Length(s: string): number {
  let n = s.length;
  for (let i = 0; i < s.length; i++) {
    const c = s.charCodeAt(i);
    if (c >= 0xd800 && c < 0xdc00) {
      // A surrogate pair, two units, is four bytes.
      n += 2;
      i++;
    } else if (c >= 0x800) n += 2;
    else if (c >= 0x80) n += 1;
  }
  return n;
}

// size is a header's width or height: def when it's missing or 0, and
// an error when it isn't a whole number from 0 to max.
function size(v: unknown, name: string, max: number, def: number): number {
  if (v === undefined || v === null || v === 0) return def;
  if (typeof v !== "number" || !Number.isInteger(v) || v < 0) throw new Error(`cast ${name} ${JSON.stringify(v)} isn't a size`);
  if (v > max) throw new Error(`recording too large: ${name} ${v}, at most ${max}`);
  return v;
}

// parseCast reads asciicast v2: a JSON header line, then one
// [seconds, type, text] line per event. Only output events are kept.
export function parseCast(text: string): Cast {
  const lines = text.split("\n").filter((l) => l.trim());
  const h = JSON.parse(lines[0] ?? "");
  if (h?.version !== 2) throw new Error("not an asciicast v2 file");
  const cols = size(h.width, "width", maxCols, 80);
  const rows = size(h.height, "height", maxRows, 24);
  const events: [number, string][] = [];
  let bytes = 0;
  for (const l of lines.slice(1)) {
    const e = JSON.parse(l);
    if (!(Array.isArray(e) && e[1] === "o" && typeof e[0] === "number" && typeof e[2] === "string")) continue;
    if (events.length >= maxEvents) throw new Error(`recording too large: more than ${maxEvents} events`);
    if ((bytes += utf8Length(e[2])) > maxOutput) throw new Error(`recording too large: more than ${maxOutput >> 20} MB of output`);
    events.push([e[0], e[2]]);
  }
  return { cols, rows, events };
}
