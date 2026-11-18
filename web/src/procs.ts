// The Console's Processes tab: each of this tab's shells, with the
// processes under it as a tree (blocks running, background blocks, and
// whatever a block or the History pane left running, such as cmd &),
// how long each has run, and a Kill button on each process. It is read
// again every second while it shows.

import { App, type Proc, type ShellProcs } from "./api.js";
import { el } from "./dom.js";

let timer = 0;
let box: HTMLElement;
let empty: HTMLElement;
let note: (text: string) => void;

// running says how long a process has run: 42s, 3m 07s, 2h 05m, 4d 3h.
export function running(secs: number): string {
  const pad = (n: number) => String(n).padStart(2, "0");
  if (secs < 60) return secs + "s";
  if (secs < 3600) return Math.floor(secs / 60) + "m " + pad(secs % 60) + "s";
  if (secs < 86400) return Math.floor(secs / 3600) + "h " + pad(Math.floor(secs / 60) % 60) + "m";
  return Math.floor(secs / 86400) + "d " + (Math.floor(secs / 3600) % 24) + "h";
}

function row(depth: number, label: HTMLElement, secs: number, kill?: HTMLButtonElement): HTMLElement {
  const r = el("div", { className: "proc", role: "treeitem" }, el("span", { className: "proc-name" }, label), el("span", { className: "proc-time", title: "How long it has run" }, running(secs)), kill ?? el("span", { className: "proc-kill-gap" }));
  r.style.setProperty("--depth", String(depth));
  r.setAttribute("aria-level", String(depth + 1));
  return r;
}

function killButton(p: Proc): HTMLButtonElement {
  const b = el("button", { className: "proc-kill", type: "button", title: `Kill ${p.pid} and what it started` }, "Kill");
  b.setAttribute("aria-label", `Kill process ${p.pid}, ${p.command}`);
  b.onclick = async () => {
    b.disabled = true;
    const ok = await App.KillProcess(p.pid).catch(() => false);
    note(ok ? `Killed ${p.pid}` : `${p.pid} had already ended`);
    void refresh();
  };
  return b;
}

function procRows(ps: (Proc | null)[] | null | undefined, depth: number, out: HTMLElement[]) {
  for (const p of ps ?? []) {
    if (!p) continue;
    const label = el("span", {}, el("span", { className: "proc-pid" }, String(p.pid)), " ", p.block ? el("span", { className: "proc-block", title: "The background block " + p.block }, "◐ " + p.block) : "", p.block ? " " : "", el("span", { className: "proc-cmd", title: p.command }, p.command));
    out.push(row(depth, label, p.secs, killButton(p)));
    procRows(p.children, depth + 1, out);
  }
}

function draw(shells: ShellProcs[]) {
  const rows: HTMLElement[] = [];
  for (const s of shells) {
    const name = s.env ? `env ${s.env}` : "shell";
    const label = el("span", { className: "proc-shell" }, el("span", { className: "proc-pid" }, String(s.pid)), " ", name, (s.children?.length ?? 0) ? "" : el("span", { className: "muted" }, " · nothing running under it"));
    rows.push(row(0, label, s.secs));
    procRows(s.children, 1, rows);
  }
  // Keep a focused Kill button's place across redraws.
  const focused = document.activeElement?.closest(".proc")?.querySelector(".proc-pid")?.textContent;
  box.replaceChildren(...rows);
  empty.hidden = shells.length > 0 || box.hidden;
  if (focused) [...box.querySelectorAll<HTMLElement>(".proc")].find((r) => r.querySelector(".proc-pid")?.textContent === focused)?.querySelector<HTMLButtonElement>(".proc-kill")?.focus();
}

async function refresh() {
  draw((await App.Processes().catch(() => null)) ?? []);
}

// watchProcs reads the processes now and every second, or stops.
export function watchProcs(on: boolean) {
  clearInterval(timer);
  timer = 0;
  if (!on) return;
  void refresh();
  timer = window.setInterval(() => void refresh(), 1000);
}

export function initProcs(list: HTMLElement, none: HTMLElement, flash: (text: string) => void) {
  box = list;
  empty = none;
  note = flash;
}
