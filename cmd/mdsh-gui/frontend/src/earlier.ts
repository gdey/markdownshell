// A block with history=N keeps its earlier runs' outputs under its
// current output. The card draws them under the output as a line, "3
// earlier runs", folded at first; opened, each run shows its output
// under a line with its time and exit status, a copy button and a
// trash. The fold lasts while the app is open and isn't saved to the
// file. The output's tools gain Clear earlier runs, beside its trash.

import type { EarlierRun } from "./api.js";
import { el, iconButton, keyActivates, svg, icons } from "./dom.js";
import { colorOutput } from "./highlight.js";

// earlierOpen are the cards whose earlier runs are open, by the card's
// part key, so they stay open as the card is drawn again.
const earlierOpen = new Set<string>();

// EarlierHooks are what the card's earlier runs ask of the page.
export interface EarlierHooks {
  // key names the card, as its folds do.
  key: string;
  // copy puts an earlier run's output, out, on the clipboard.
  copy: (run: EarlierRun, out: HTMLElement, button: HTMLElement) => void;
  // remove takes earlier run k out of the file.
  remove: (k: number) => void;
  // view is called for a click on an image in an earlier run.
  view: (img: HTMLImageElement) => void;
}

// earlierLabel is the fold line's text for n earlier runs.
export const earlierLabel = (n: number) => (n === 1 ? "1 earlier run" : n + " earlier runs");

// drawEarlier is the part of a card that holds its earlier runs, newest
// first.
export function drawEarlier(runs: EarlierRun[], hooks: EarlierHooks): HTMLElement {
  const list = el("div", { className: "earlier-runs" });
  const line = el("div", { className: "earlier-line", role: "button", tabIndex: 0 }, svg(icons.chevron), el("span", {}, earlierLabel(runs.length)));
  const part = el("div", { className: "part earlier-part" }, line, list);
  const set = (open: boolean) => {
    part.classList.toggle("open", open);
    line.setAttribute("aria-expanded", String(open));
    line.title = open ? "Hide the earlier runs" : "Show the earlier runs";
    if (open) earlierOpen.add(hooks.key);
    else earlierOpen.delete(hooks.key);
    // Drawn when first opened: a long history costs nothing folded.
    if (open && !list.childElementCount) list.append(...runs.map((r) => drawRun(r, hooks)));
  };
  const toggle = () => set(!part.classList.contains("open"));
  line.onclick = (e) => {
    e.stopPropagation();
    toggle();
  };
  line.onkeydown = (e) => {
    if (keyActivates(e)) toggle();
  };
  set(earlierOpen.has(hooks.key));
  return part;
}

function drawRun(r: EarlierRun, hooks: EarlierHooks): HTMLElement {
  const out = el("div", { className: "output", innerHTML: r.output });
  // An empty stdout beside its stderr takes no room. An earlier run
  // never changes, so this is done once, here, not by a CSS :has().
  for (const pre of out.querySelectorAll("pre")) if (!pre.textContent) pre.remove();
  colorOutput(out);
  out.onclick = (e) => {
    const img = (e.target as HTMLElement).closest("img");
    if (img) hooks.view(img);
  };
  const when = r.at ? runTime(r.at, new Date()) : "";
  const exit = r.region && !r.failed ? "" : "exit " + r.exit;
  const copy = iconButton("copy", "Copy this run's output", () => hooks.copy(r, out, copy));
  copy.classList.add("copy-output");
  const trash = iconButton("trash", "Delete this earlier run", () => hooks.remove(r.k));
  trash.classList.add("trash");
  const head = el(
    "div",
    { className: "earlier-head" },
    el("span", { className: "earlier-when", title: r.at ? fullTime(r.at) : "Saved without a time" }, when || "time not saved"),
    exit ? el("span", { className: "exit" + (r.failed ? " bad" : "") }, exit) : "",
    el("span", { className: "earlier-tools" }, copy, trash),
  );
  return el("div", { className: "earlier-run" + (r.failed ? " failed" : "") }, head, out);
}

const pad = (n: number) => String(n).padStart(2, "0");

// runTime says when a run at at (RFC 3339) was, in local time, the way
// a person would: "today 19:02", "yesterday 19:02", "Mon 19:02" within
// the week, then "Nov 3 19:02", with the year once it isn't now's.
export function runTime(at: string, now: Date): string {
  const t = new Date(at);
  if (isNaN(t.getTime())) return at;
  const clock = pad(t.getHours()) + ":" + pad(t.getMinutes());
  const day = (d: Date) => new Date(d.getFullYear(), d.getMonth(), d.getDate()).getTime();
  const days = Math.round((day(now) - day(t)) / 86400000);
  if (days === 0) return "today " + clock;
  if (days === 1) return "yesterday " + clock;
  if (days > 1 && days < 7) return t.toLocaleDateString("en-US", { weekday: "short" }) + " " + clock;
  const date = t.toLocaleDateString("en-US", { month: "short", day: "numeric", ...(t.getFullYear() !== now.getFullYear() ? { year: "numeric" } : {}) });
  return date + " " + clock;
}

// fullTime is a run's time for its tooltip: the local date and time to
// the second, and the at= the file holds.
function fullTime(at: string): string {
  const t = new Date(at);
  if (isNaN(t.getTime())) return at;
  return t.toLocaleString([], { dateStyle: "full", timeStyle: "medium" }) + "\n" + at;
}
