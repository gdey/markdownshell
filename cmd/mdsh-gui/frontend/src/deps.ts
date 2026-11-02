// The lines between a block and the blocks its depends= names, drawn in
// the rendered view's margin as if painted on the page behind the
// cards. A line runs from the dependent block's top anchor, on its head
// row, up to the bottom anchor of the block it depends on, with a dot at
// the corner where it turns in, since every line into a card arrives at
// the same spot. A block's
// first dependency is drawn in the first of four colors, its second in
// the second, and so on; a fifth and later are gray. Each line takes a
// lane of its own, out from the cards, so lines over the same rows
// don't merge. Selecting a block lights its lines and outlines the
// blocks they lead to in the same colors, and a double-click on a line
// goes to its far end.
//
// The lines are edited where they are drawn. The top anchor is a ring:
// drag it onto a block above to depend on it, and the line attaches at
// that block's bottom anchor. Click a line's dot to take that dependency out, or drag the dot to
// another block above to depend on that one instead, or out past the
// lanes, away from the cards, to tear it off. While a line is dragged,
// holding it over a folded section's heading opens the section for the
// moment, so the blocks in it can be dropped on (Host.peek).
//
// A button at the top of the margin hides the lines and what edits them,
// and shows them again; the tab remembers which. Selecting a block still
// outlines what it depends on while they are hidden.
//
// There are two layers: the paint, behind the cards, and over
// everything a clear one holding what is clicked or dragged, since the
// paint can't be reached where the page is over it.

import { type Cell } from "./api.js";
import { el, icons, svg as icon } from "./dom.js";

// An Edge is one dependency: block from depends on block to, as the
// k-th (from 0) in its depends=.
interface Edge {
  from: number;
  to: number;
  k: number;
}

// Host is what the layer asks of the page: to go to a card, to write a
// block's depends= as the blocks it names, by index, with a word for
// the status line, and, while a line is dragged, to open folded
// sections for the moment: peek is told the top-level cell under the
// pointer, or null between cells, and endPeek the block the line was
// dropped on, or null.
export interface Host {
  jump(block: HTMLElement): void;
  setDepends(block: number, deps: number[], what: string): Promise<void>;
  peek(cell: HTMLElement | null): void;
  endPeek(dropped: HTMLElement | null): void;
}

// The number of colors; dependencies past them share the gray of
// dep-more.
const colors = 4;

// Lanes start laneGap out from the card's edge and are laneStep apart.
const laneGap = 14;
const laneStep = 10;
const corner = 6;

let edges: Edge[] = [];
let lit = -1; // the selected block, whose lines are lit
let svg: SVGSVGElement | null = null; // the paint
let ui: SVGSVGElement | null = null; // dots, rings and the drag
let scroll: HTMLElement;
let host: Host = { jump() {}, setDepends: async () => {}, peek() {}, endPeek() {} };
let pending = 0;
// The line being dragged, kept over a redraw, and how to draw it again
// when the page under it moves, as when a section opens.
let ghost: SVGPathElement | null = null;
let redrawGhost: (() => void) | null = null;
// Whether the lines are shown, and the button that says.
let showLines = loadShown();
let toggle: HTMLButtonElement | null = null;

function loadShown(): boolean {
  try {
    return localStorage.getItem("mdsh-dep-lines") !== "0";
  } catch {
    return true;
  }
}

// The layout of the last draw, for a drag: the content coordinates'
// origin, the cards' edge on the line side, which way the lines go
// from it, and how far out the lanes reach.
let geo = { edge: 0, dir: -1, reach: laneGap };

// initDeps makes the layer in the scrolling view, and redraws it when
// the document or the view changes size: a fold, output arriving, the
// outline drawer or the source pane opening.
export function initDeps(scrollView: HTMLElement, h: Host) {
  scroll = scrollView;
  host = h;
  svg = make("svg", { id: "deps", "aria-hidden": "true" });
  ui = make("svg", { id: "deps-ui", "aria-hidden": "true" });
  // The button stays at the top of the margin as the view scrolls.
  toggle = el("button", { id: "deps-toggle", className: "icon" }, icon(icons.lines));
  toggle.onclick = (e) => {
    e.stopPropagation();
    showLines = !showLines;
    try {
      localStorage.setItem("mdsh-dep-lines", showLines ? "1" : "0");
    } catch {
      // Only a convenience: the next tab shows them.
    }
    drawDeps();
  };
  const bar = el("div", { id: "deps-bar" }, toggle);
  scroll.prepend(bar, svg, ui);
  const ro = new ResizeObserver(() => drawDeps());
  ro.observe(scroll);
  const main = scroll.querySelector("main");
  if (main) ro.observe(main);
}

// setDeps takes the document's cells and works out the edges, by block
// index, then draws them.
export function setDeps(cells: Cell[]) {
  const index = new Map<string, number>();
  // A textbox or a choice never runs, so depends= naming one is ignored
  // and gets no line.
  for (const c of cells) if (c.block >= 0 && c.id && !c.textbox && !c.choice) index.set(c.id, c.block);
  edges = [];
  for (const c of cells) {
    if (c.block < 0 || !c.depends) continue;
    c.depends.forEach((id, k) => {
      const to = index.get(id);
      if (to !== undefined && to !== c.block) edges.push({ from: c.block, to, k });
    });
  }
  drawDeps();
}

// litDeps lights the lines of block, or none for -1, and outlines the
// blocks they lead to.
export function litDeps(block: number) {
  lit = block;
  drawDeps();
}

// dependencies are the blocks block depends on, in order, by index.
export function dependencies(block: number): number[] {
  return edges.filter((e) => e.from === block).map((e) => e.to);
}

// drawDeps draws the lines again, once the browser is ready to paint.
export function drawDeps() {
  if (pending || !svg || !ui) return;
  pending = requestAnimationFrame(() => {
    pending = 0;
    draw();
  });
}

// An Anchor is where a line attaches to a block: the card, or the
// heading of the folded section hiding it, and the y of its head row in
// the scrolling view's content coordinates.
interface Anchor {
  card: HTMLElement;
  y: number;
}

function anchorOf(block: number, top: number, end: "top" | "bottom"): Anchor | null {
  const card = shown(block);
  if (!card) return null;
  return { card, y: end === "top" ? headY(card, top) : footY(card, top) };
}

// shown is block's card, or the heading of the folded section hiding it.
function shown(block: number): HTMLElement | null {
  const card = document.getElementById("block-" + block);
  if (!card || !card.classList.contains("in-fold")) return card;
  // The nearest cell above that isn't hidden is the heading that folds
  // it away.
  let e: Element | null = card;
  while (e && e.classList.contains("in-fold")) e = e.previousElementSibling;
  return e instanceof HTMLElement ? e : null;
}

// markDeps outlines what the lit block depends on, in the colors of its
// lines: each block's card in the rendered view and its row in the
// outline, or, for a block in a folded section, the section's heading
// and its row. A card or row standing for more than one gets a ring for
// each, the first dependency's nearest it.
export function markDeps() {
  for (const e of document.querySelectorAll<HTMLElement>(".dep-of")) {
    e.classList.remove("dep-of");
    e.style.removeProperty("--dep-rings");
  }
  const by = new Map<HTMLElement, string[]>();
  for (const e of edges) {
    if (e.from !== lit) continue;
    const card = shown(e.to);
    if (!card) continue;
    const list = by.get(card) ?? [];
    by.set(card, list);
    list.push(`var(--dep-${e.k < colors ? e.k + 1 : "more"})`);
  }
  for (const [card, list] of by) {
    // Outward from the card, past a hairline of the page.
    card.classList.add("dep-of");
    card.style.setProperty("--dep-rings", ["0 0 0 1px var(--bg)", ...list.map((c, i) => `0 0 0 ${3 + 2 * i}px ${c}`)].join(", "));
    const row = document.querySelector<HTMLElement>(`#outline .row[data-line="${card.dataset.line}"]`);
    if (!row) continue;
    // Inward from the row's edge, which its neighbors touch: the last
    // at the edge, on top, and each before it 2px wider, under it.
    row.classList.add("dep-of");
    row.style.setProperty("--dep-rings", list.map((c, i) => `inset 0 0 0 ${2 * (list.length - i)}px ${c}`).reverse().join(", "));
  }
}

// A card has two anchors on its edge: the top one, on its head row,
// where its own lines leave and are dragged from, and the bottom one,
// where lines from the blocks that depend on it arrive.

// headY is the y of a card's top anchor.
function headY(card: HTMLElement, top: number): number {
  const head = card.querySelector(":scope > .head") ?? card;
  const r = head.getBoundingClientRect();
  return r.top + Math.min(r.height / 2, 14) - top;
}

// footY is the y of a card's bottom anchor.
function footY(card: HTMLElement, top: number): number {
  const r = card.getBoundingClientRect();
  return r.bottom - Math.min(r.height / 2, 12) - top;
}

// origin is where content coordinates start: the scrolling view's top
// left corner, scrolled back to the start.
function origin(): { top: number; left: number } {
  const box = scroll.getBoundingClientRect();
  return { top: box.top - scroll.scrollTop, left: box.left - scroll.scrollLeft };
}

const ns = "http://www.w3.org/2000/svg";

function make<K extends keyof SVGElementTagNameMap>(tag: K, attrs: Record<string, string | number>): SVGElementTagNameMap[K] {
  const e = document.createElementNS(ns, tag);
  for (const [k, v] of Object.entries(attrs)) e.setAttribute(k, String(v));
  return e;
}

function draw() {
  if (!svg || !ui) return;
  const main = scroll.querySelector<HTMLElement>("main");
  markDeps();
  const cards = [...scroll.querySelectorAll<HTMLElement>("#doc > .block:not(.in-fold)")];
  const title = showLines ? "Hide the dependency lines" : "Show the dependency lines";
  if (toggle) {
    toggle.title = title;
    toggle.setAttribute("aria-label", title);
    toggle.classList.toggle("on", showLines);
    toggle.hidden = !cards.length;
  }
  scroll.classList.toggle("deps-hidden", !showLines);
  if (!main || scroll.hidden || !cards.length) {
    svg.replaceChildren();
    ui.replaceChildren();
    svg.style.display = ui.style.display = "none";
    return;
  }
  svg.style.display = ui.style.display = "";
  const { top, left } = origin();
  // The cards' edges are inside main's padding.
  const style = getComputedStyle(main);
  const mr = main.getBoundingClientRect();
  const cardLeft = mr.left + parseFloat(style.paddingLeft) - left;
  const cardRight = mr.right - parseFloat(style.paddingRight) - left;
  // The layers are sized to the page, so they mustn't hold it open: a
  // window that got narrower would keep scrolling sideways to their old
  // width.
  for (const layer of [svg, ui]) layer.setAttribute("width", "0"), layer.setAttribute("height", "0");
  const width = scroll.scrollWidth;
  const height = scroll.scrollHeight;
  for (const layer of [svg, ui]) {
    layer.setAttribute("width", String(width));
    layer.setAttribute("height", String(height));
    layer.setAttribute("viewBox", `0 0 ${width} ${height}`);
  }

  type Line = Edge & { a: Anchor; b: Anchor; lane: number };
  const lines: Line[] = [];
  for (const e of edges) {
    const a = anchorOf(e.from, top, "top");
    const b = anchorOf(e.to, top, "bottom");
    if (a && b && a.card !== b.card) lines.push({ ...e, a, b, lane: 0 });
  }
  // Lanes: the shortest lines nearest the cards, and a line takes the
  // first lane where no line already there runs over the same rows.
  const lanes: Line[][] = [];
  const span = (l: Line) => Math.abs(l.a.y - l.b.y);
  for (const l of [...lines].sort((p, q) => span(p) - span(q))) {
    const lo = Math.min(l.a.y, l.b.y);
    const hi = Math.max(l.a.y, l.b.y);
    let i = 0;
    while (lanes[i]?.some((o) => Math.min(o.a.y, o.b.y) < hi && lo < Math.max(o.a.y, o.b.y))) i++;
    (lanes[i] ??= []).push(l);
    l.lane = i;
  }
  // The side: the left margin when the lanes fit there, else the right
  // when they fit there, else whichever has more room, with the lanes
  // that don't fit sharing the last one that does.
  const need = laneGap + laneStep * Math.max(lanes.length - 1, 0) + 4;
  const roomLeft = cardLeft;
  const roomRight = scroll.clientWidth - cardRight;
  const onLeft = roomLeft >= need || (roomRight < need && roomLeft >= roomRight);
  const room = onLeft ? roomLeft : roomRight;
  const fit = Math.max(0, Math.floor((room - laneGap - 4) / laneStep));
  const dir = onLeft ? -1 : 1;
  const edge = onLeft ? cardLeft : cardRight;
  geo = { edge, dir, reach: laneGap + laneStep * Math.min(Math.max(lanes.length - 1, 0), fit) };
  // The button sits over the lanes nearest the cards, 18px wide.
  if (toggle) toggle.style.left = edge + dir * laneGap - 9 + "px";

  const out: SVGGElement[] = [];
  const hits: SVGElement[] = [];
  const dots: SVGElement[] = []; // over the hit paths, which cross them
  for (const l of lines) {
    const x = edge + dir * (laneGap + laneStep * Math.min(l.lane, fit));
    const y1 = l.a.y;
    const y2 = l.b.y;
    const color = "dep-" + (l.k < colors ? l.k + 1 : "more");
    const g = make("g", { class: "dep " + color + (l.from === lit ? " lit" : "") });
    g.append(make("path", { d: elbow(x, y1, y2) }));
    out.push(g);
    // What is clicked: the line, unseen and wider, and the dot at its
    // corner on the dependency's row, in its own lane.
    const hit = make("path", { class: "hit", d: elbow(x, y1, y2) });
    const title = make("title", {});
    title.textContent = `${name(l.a.card)} depends on ${name(l.b.card)}`;
    hit.append(title);
    hit.onpointerenter = () => g.classList.add("hover");
    hit.onpointerleave = () => g.classList.remove("hover");
    hit.ondblclick = (ev) => {
      ev.preventDefault();
      // The far end from where the click was.
      const y = ev.clientY - origin().top;
      host.jump(Math.abs(y - y1) >= Math.abs(y - y2) ? l.a.card : l.b.card);
    };
    const dot = make("circle", { class: "dot " + color + (l.from === lit ? " lit" : ""), cx: x, cy: y2, r: 3.5 });
    const hint = make("title", {});
    hint.textContent = "Click to take this dependency out, or drag it to another block above";
    dot.append(hint);
    dot.onpointerdown = (ev) => startDrag(ev, l.from, l.k, l.a.card);
    hits.push(hit);
    dots.push(dot);
  }
  // Lit lines go last, over the rest.
  out.sort((p, q) => Number(p.classList.contains("lit")) - Number(q.classList.contains("lit")));
  // The anchors: a ring at each card's top, to drag a new line from
  // (the first card has nothing above it to depend on), and a port at
  // its bottom, where lines arrive (the last card has nothing below it
  // to depend on it).
  const rings: SVGElement[] = [];
  cards.forEach((card, i) => {
    const block = Number(card.dataset.block);
    if (i > 0) {
      const ring = make("circle", { class: "handle", cx: edge + dir * 6, cy: headY(card, top), r: 4.5 });
      const hint = make("title", {});
      hint.textContent = `Drag onto a block above to make ${name(card)} depend on it`;
      ring.append(hint);
      ring.onpointerdown = (ev) => startDrag(ev, block, dependencies(block).length, card);
      rings.push(ring);
    }
    if (i < cards.length - 1) {
      const port = make("circle", { class: "port", cx: edge + dir * 6, cy: footY(card, top), r: 3 });
      port.dataset.block = String(block);
      rings.push(port);
    }
  });
  svg.replaceChildren(...out);
  ui.replaceChildren(...hits, ...dots, ...rings, ...(ghost ? [ghost] : []));
  redrawGhost?.();
}

// elbow is a line's path: out from the cards' edge at y1 to lane x, along
// it to y2, and back in.
function elbow(x: number, y1: number, y2: number): string {
  const { edge, dir } = geo;
  const up = y2 < y1 ? -1 : 1;
  const r = Math.min(corner, Math.abs(y1 - y2) / 2);
  const x0 = edge + dir * 2;
  return `M ${x0} ${y1} H ${x - dir * r} Q ${x} ${y1} ${x} ${y1 + up * r} V ${y2 - up * r} Q ${x} ${y2} ${x - dir * r} ${y2} H ${x0}`;
}

function name(card: HTMLElement): string {
  return card.querySelector(".head .id")?.textContent || card.querySelector("h1, h2, h3, h4, h5, h6")?.textContent || "this block";
}

// A drag makes block from depend on the block it ends on, as its k-th
// dependency: a new one from a ring, or in place of the one whose dot
// was dragged. A dot let go where it was taken, or out past the lanes,
// takes that dependency out. The pointer is captured by the page's
// body, not the ring or dot, since a section opening under the drag
// draws the layer again.
function startDrag(ev: PointerEvent, from: number, k: number, card: HTMLElement) {
  if (ev.button !== 0 || !ui) return;
  ev.preventDefault();
  ev.stopPropagation();
  const deps = dependencies(from);
  const moving = k < deps.length; // a dot, else a ring
  const body = document.body;
  const startY = ev.clientY;
  const startX = ev.clientX;
  const { edge, dir, reach } = geo;
  const x = edge + dir * (reach + laneStep);
  // Where the line leaves from, which moves down when a section above
  // opens.
  const y1 = () => headY(card, origin().top);
  const line = make("path", { class: "dep drag dep-" + (k < colors ? k + 1 : "more"), d: "" });
  let target: HTMLElement | null = null;
  let moved = false;
  let torn = false; // a dot dragged out past the line's lane, to let go of
  let last: { clientX: number; clientY: number } | null = null;
  ghost = line;
  ui.append(line);
  body.setPointerCapture(ev.pointerId);
  body.classList.add("dragging-dep");
  const clear = () => {
    target?.classList.remove("dep-target");
    target = null;
  };
  const finish = (dropped: HTMLElement | null) => {
    line.remove();
    ghost = null;
    redrawGhost = null;
    clear();
    body.classList.remove("dragging-dep");
    body.onpointermove = body.onpointerup = body.onpointercancel = null;
    if (body.hasPointerCapture(ev.pointerId)) body.releasePointerCapture(ev.pointerId);
    document.removeEventListener("keydown", onKey, true);
    host.endPeek(dropped);
  };
  const onKey = (e: KeyboardEvent) => {
    if (e.key !== "Escape") return;
    e.stopPropagation();
    finish(null);
  };
  document.addEventListener("keydown", onKey, true);
  const follow = (e: { clientX: number; clientY: number }, peek: boolean) => {
    last = e;
    // The line only goes up: below its block's row it stays there, and
    // nothing there is a target.
    const { top, left } = origin();
    const from1 = y1();
    let y = Math.min(e.clientY - top, from1);
    // A dot pulled out past the lane, where the line could no longer
    // turn back to the cards, is being torn off: letting go there takes
    // the dependency out.
    torn = moving && dir * (e.clientX - left - x) > 0;
    line.classList.toggle("tear", torn);
    const below = e.clientY - top > from1;
    const cell = below || torn ? null : (document.elementsFromPoint(e.clientX, e.clientY).find((el) => el.closest("#doc > *"))?.closest<HTMLElement>("#doc > *") ?? null);
    // A folded section's heading opens when the line is held over it,
    // and closes again when the line leaves the section.
    if (peek && !below && !torn) host.peek(cell);
    // The card under the pointer, when it is a block above from and not
    // one from depends on already (other than the one being moved).
    const under = cell?.classList.contains("block") ? cell : null;
    const to = under ? Number(under.dataset.block) : -1;
    const ok = under && to < from && !deps.some((d, i) => d === to && i !== k);
    if (under !== target) {
      clear();
      if (ok) {
        target = under;
        under.classList.add("dep-target");
      }
    }
    // Over a target, the line arrives at its bottom anchor.
    if (target) y = footY(target, top);
    line.setAttribute("d", elbow(x, from1, y));
  };
  // Drawn again after the page moves, without opening or closing more.
  redrawGhost = () => {
    if (moved && last) follow(last, false);
  };
  body.onpointermove = (e) => {
    if (!moved && Math.hypot(e.clientX - startX, e.clientY - startY) < 4) return;
    moved = true;
    follow(e, true);
  };
  body.onpointercancel = () => finish(null);
  body.onpointerup = () => {
    const dropped = target;
    finish(dropped);
    if (dropped) {
      const to = Number(dropped.dataset.block);
      const next = [...deps];
      next[k] = to;
      void host.setDepends(from, next, (moving ? "moved the dependency to " : "now depends on ") + name(dropped));
    } else if (moving && (!moved || torn)) {
      void host.setDepends(
        from,
        deps.filter((_, i) => i !== k),
        "took out the dependency on " + name(document.getElementById("block-" + deps[k]) ?? card),
      );
    }
  };
}
