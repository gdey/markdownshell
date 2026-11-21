// Heads are what the page holds of every cell of the document: enough
// to place its card, guess its height, and list it in the outline and
// the dependency lines, without its prose, code or output, which come
// from Go only for the cards that are drawn. Go sends them in chunks as
// it lays the document out, top down, so a long file's first screen is
// drawn as soon as its cells exist (docs/design/ui-protocol.md, stage
// 2; notebook.Heads in Go).

import type { Cell } from "./api.js";

// A head as it comes from Go: an array, as notebook.HeadList says,
//
//	[line, from, to, hash, block, kind, flags, level, codeLines,
//	 outputLines, sectionFrom, sectionTo, id, name, title, depends,
//	 lastLine]
//
// with what is at its zero at the end left off, line and from counted
// on from the head before, to from from, and lastLine from line.
export type HeadRow = (number | string | string[])[];

// The kinds of cell, notebook.HeadProse and on.
export const kindProse = 0;
export const kindBlock = 1;
export const kindTextbox = 2;
export const kindChoice = 3;

// The flags of a head, notebook.HeadRan and on.
const flags = ["ran", "failed", "stale", "running", "hidden", "collapsed", "collapsedOutput", "secret", "terminal", "nosave", "movable"] as const;

// partial marks a cell made from its head, whose body Go hasn't sent.
export const partial = new WeakSet<Cell>();

// hiddenWhy is what a hidden cell made from its head says, until its
// body says why.
const hiddenWhy = "Left out when printed.";

// decodeHeads is the cells rows are the heads of, each without its body
// and marked partial.
export function decodeHeads(rows: HeadRow[] | null | undefined): Cell[] {
  const out: Cell[] = [];
  let line = 0;
  let from = 0;
  for (const r of rows ?? []) {
    const num = (k: number, def = 0) => (k < r.length ? (r[k] as number) : def);
    const str = (k: number) => (k < r.length ? (r[k] as string) : "");
    line += num(0);
    from += num(1);
    const kind = num(5);
    const block = num(4, -1);
    const f = num(6);
    const c: Cell = { block: kind === kindBlock ? block : -1, ran: false, status: 0, failed: false, running: false, from, to: from + num(2), line, dropAfter: -1, dropBefore: -1, hash: str(3) };
    for (let k = 0; k < flags.length; k++) {
      if (!(f & (1 << k))) continue;
      if (flags[k] === "hidden") c.hidden = hiddenWhy;
      else (c as unknown as Record<string, boolean>)[flags[k]] = true;
    }
    const level = num(7);
    if (level) {
      c.heading = level;
      c.title = str(14);
    }
    if (num(8)) c.codeLines = num(8);
    if (num(9)) c.outputLines = num(9);
    if (num(11)) {
      c.sectionFrom = num(10);
      c.sectionTo = num(11);
    }
    const id = str(12);
    if (kind === kindBlock) {
      c.id = id;
      c.name = str(13) || id;
      if (r[15]) c.depends = r[15] as string[];
    } else if (kind === kindTextbox) {
      c.textbox = { block, id } as Cell["textbox"];
    } else if (kind === kindChoice) {
      c.choice = { block, id } as Cell["choice"];
    }
    if (c.movable) c.lastLine = line + num(16);
    partial.add(c);
    out.push(c);
  }
  return out;
}

// sameHead says cells a and b, either of which may be partial, have the
// same head: the same body, and the same state. A block's index isn't
// compared, as one added above changes it without changing the card.
export function sameHead(a: Cell, b: Cell): boolean {
  return (
    !!a.hash &&
    a.hash === b.hash &&
    a.block >= 0 === b.block >= 0 &&
    (a.heading ?? 0) === (b.heading ?? 0) &&
    (a.title ?? "") === (b.title ?? "") &&
    !!a.textbox === !!b.textbox &&
    !!a.choice === !!b.choice &&
    flags.every((k) => !!a[k] === !!b[k])
  );
}
