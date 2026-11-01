// The lines between a block and the blocks its depends= names, drawn in
// the rendered view's margin as if painted on the page behind the
// cards. A line runs from the dependent block's head up to the head of
// the block it depends on, where a dot marks the attachment. A block's
// first dependency is drawn in the first of four colors, its second in
// the second, and so on; a fifth and later are gray. Each line takes a
// lane of its own, out from the cards, so lines over the same rows
// don't merge. Selecting a block lights its lines and outlines the
// blocks they lead to in the same colors, and a double-click on a line
// goes to its far end.

import { type Cell } from "./api.js";

// An Edge is one dependency: block from depends on block to, as the
// k-th (from 0) in its depends=.
interface Edge {
  from: number;
  to: number;
  k: number;
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
let svg: SVGSVGElement | null = null;
let scroll: HTMLElement;
let jump: (block: HTMLElement) => void = () => {};
let pending = 0;

// initDeps makes the layer in the scrolling view, and redraws it when
// the document or the view changes size: a fold, output arriving, the
// outline drawer or the source pane opening. onJump is what a
// double-click on a line does, with the card at its far end.
export function initDeps(scrollView: HTMLElement, onJump: (block: HTMLElement) => void) {
  scroll = scrollView;
  jump = onJump;
  svg = document.createElementNS("http://www.w3.org/2000/svg", "svg");
  svg.id = "deps";
  svg.setAttribute("aria-hidden", "true");
  scroll.prepend(svg);
  const ro = new ResizeObserver(() => drawDeps());
  ro.observe(scroll);
  const main = scroll.querySelector("main");
  if (main) ro.observe(main);
}

// setDeps takes the document's cells and works out the edges, by block
// index, then draws them.
export function setDeps(cells: Cell[]) {
  const index = new Map<string, number>();
  for (const c of cells) if (c.block >= 0 && c.id) index.set(c.id, c.block);
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
  if (pending || !svg) return;
  pending = requestAnimationFrame(() => {
    pending = 0;
    draw();
  });
}

// anchor is where a line attaches to a block: the card, or the heading
// of the folded section hiding it, and the y of its head row in the
// scrolling view's coordinates.
interface Anchor {
  card: HTMLElement;
  y: number;
}

function anchorOf(block: number, top: number): Anchor | null {
  let card = document.getElementById("block-" + block);
  if (!card) return null;
  if (card.classList.contains("in-fold")) {
    // The nearest cell above that isn't hidden is the heading, or a
    // cell in the section, that folds it away.
    let e: Element | null = card;
    while (e && e.classList.contains("in-fold")) e = e.previousElementSibling;
    if (!(e instanceof HTMLElement)) return null;
    card = e;
  }
  const head = card.querySelector(":scope > .head") ?? card;
  const r = head.getBoundingClientRect();
  if (!r.height) return null;
  return { card, y: r.top + Math.min(r.height / 2, 14) - top };
}

function draw() {
  if (!svg) return;
  const main = scroll.querySelector<HTMLElement>("main");
  for (const e of scroll.querySelectorAll<HTMLElement>(".block.dep-of")) {
    e.classList.remove("dep-of");
    e.style.removeProperty("--dep-of");
  }
  if (!main || !edges.length || scroll.hidden) {
    svg.replaceChildren();
    svg.style.display = "none";
    return;
  }
  svg.style.display = "";
  const box = scroll.getBoundingClientRect();
  const top = box.top - scroll.scrollTop;
  const left = box.left - scroll.scrollLeft;
  // The cards' edges are inside main's padding.
  const style = getComputedStyle(main);
  const mr = main.getBoundingClientRect();
  const cardLeft = mr.left + parseFloat(style.paddingLeft) - left;
  const cardRight = mr.right - parseFloat(style.paddingRight) - left;
  const width = scroll.scrollWidth;
  const height = scroll.scrollHeight;
  svg.setAttribute("width", String(width));
  svg.setAttribute("height", String(height));
  svg.setAttribute("viewBox", `0 0 ${width} ${height}`);

  type Line = Edge & { a: Anchor; b: Anchor; lane: number };
  const lines: Line[] = [];
  for (const e of edges) {
    const a = anchorOf(e.from, top);
    const b = anchorOf(e.to, top);
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

  const ns = svg.namespaceURI;
  const out: SVGGElement[] = [];
  for (const l of lines) {
    const x = edge + dir * (laneGap + laneStep * Math.min(l.lane, fit));
    const y1 = l.a.y;
    const y2 = l.b.y;
    const up = y2 < y1 ? -1 : 1; // which way the line runs from a to b
    const r = Math.min(corner, Math.abs(y1 - y2) / 2);
    const x0 = edge + dir * 2;
    const d = `M ${x0} ${y1} H ${x - dir * r} Q ${x} ${y1} ${x} ${y1 + up * r} V ${y2 - up * r} Q ${x} ${y2} ${x - dir * r} ${y2} H ${x0}`;
    const g = document.createElementNS(ns, "g") as SVGGElement;
    g.setAttribute("class", "dep dep-" + (l.k < colors ? l.k + 1 : "more") + (l.from === lit ? " lit" : ""));
    const path = document.createElementNS(ns, "path");
    path.setAttribute("d", d);
    const title = document.createElementNS(ns, "title");
    title.textContent = `${name(l.a.card)} depends on ${name(l.b.card)}`;
    path.append(title);
    const dot = document.createElementNS(ns, "circle");
    dot.setAttribute("cx", String(edge + dir * 5));
    dot.setAttribute("cy", String(y2));
    dot.setAttribute("r", "3.5");
    g.append(path, dot);
    g.ondblclick = (ev) => {
      ev.preventDefault();
      // The far end from where the click was.
      const y = ev.clientY - top;
      jump(Math.abs(y - y1) >= Math.abs(y - y2) ? l.a.card : l.b.card);
    };
    out.push(g);
    if (l.from === lit && l.b.card !== l.a.card) {
      l.b.card.classList.add("dep-of");
      l.b.card.style.setProperty("--dep-of", `var(--dep-${l.k < colors ? l.k + 1 : "more"})`);
    }
  }
  // Lit lines go last, over the rest.
  out.sort((p, q) => Number(p.classList.contains("lit")) - Number(q.classList.contains("lit")));
  svg.replaceChildren(...out);
}

function name(card: HTMLElement): string {
  return card.querySelector(".head .id")?.textContent || card.querySelector("h1, h2, h3, h4, h5, h6")?.textContent || "this block";
}
