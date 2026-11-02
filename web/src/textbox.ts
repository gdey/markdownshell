// How tall a textbox is in the rendered view. A box is as tall as its
// lines= says, or, with lines=fit, as its text, growing and shrinking as
// it is typed in. The grip under it makes it taller: the height dragged
// to is kept until the window closes, by file and textbox id, so it
// survives the box being drawn again, but it is never written to the
// file, since lines= there is a limit on the text, shared by every copy
// of the textbox and everyone who opens the file.

import { el } from "./dom.js";

// tall is the height a grip was dragged to, in lines, by file and id.
const tall = new Map<string, number>();

// lastWidth is the width a fit box was last sized at, so that a change
// in width, which wraps its lines differently, sizes it again.
const lastWidth = new WeakMap<HTMLTextAreaElement, number>();

// A box is sized once it is on the page, when the observer first sees
// it, and again whenever its width changes.
const watch = new ResizeObserver((entries) => {
  for (const e of entries) {
    const box = e.target as HTMLTextAreaElement;
    const w = box.clientWidth;
    if (w === 0 || w === lastWidth.get(box)) continue;
    lastWidth.set(box, w);
    sizeTextbox(box);
  }
});

// metrics are the height of one line of box's text, and of its
// padding and border, in pixels.
function metrics(box: HTMLTextAreaElement): { line: number; chrome: number } {
  const s = getComputedStyle(box);
  const px = (v: string) => parseFloat(v) || 0;
  const line = px(s.lineHeight) || px(s.fontSize) * 1.5;
  return { line, chrome: px(s.paddingTop) + px(s.paddingBottom) + px(s.borderTopWidth) + px(s.borderBottomWidth) };
}

// natural is box's height without a grip's: its rows, or for a fit box
// its text, in pixels.
function natural(box: HTMLTextAreaElement): number {
  box.style.height = "";
  if (box.dataset.fit !== "1") return box.offsetHeight;
  box.style.overflowY = "hidden";
  box.style.height = "0";
  // scrollHeight is the text and padding; the border is what is left.
  const h = box.scrollHeight + box.offsetHeight - box.clientHeight;
  box.style.height = "";
  return h;
}

// sizeTextbox sets box's height: its natural height, or the grip's if
// that is taller.
export function sizeTextbox(box: HTMLTextAreaElement) {
  if (!box.isConnected) return;
  const scroll = box.closest<HTMLElement>("#doc-scroll");
  const top = scroll?.scrollTop;
  const n = natural(box);
  const { line, chrome } = metrics(box);
  const rows = tall.get(box.dataset.tallKey ?? "") ?? 0;
  const h = Math.max(n, Math.round(rows * line + chrome));
  box.style.height = h + "px";
  // A fit box's own text is never scrolled out of sight.
  if (box.dataset.fit === "1") box.style.overflowY = h > n ? "auto" : "hidden";
  // Shrinking to measure can scroll the page; put it back.
  if (scroll && top != null) scroll.scrollTop = top;
  const grip = box.nextElementSibling;
  if (grip?.classList.contains("textbox-grip")) grip.setAttribute("aria-valuenow", String(Math.round((h - chrome) / line)));
}

// textboxGrip makes box, for textbox id in file, sized as above, and
// returns the grip that goes under it. fit is whether it has lines=fit.
export function textboxGrip(box: HTMLTextAreaElement, file: string, id: string, fit: boolean): HTMLElement {
  const key = file + "\u0000" + id;
  box.dataset.tallKey = key;
  if (fit) {
    box.dataset.fit = "1";
    box.rows = 1;
    box.addEventListener("input", () => sizeTextbox(box));
  }
  watch.observe(box);
  const grip = el("div", { className: "textbox-grip", title: "Drag to make it taller; double-click for its own height", tabIndex: 0 });
  grip.setAttribute("role", "separator");
  grip.setAttribute("aria-orientation", "horizontal");
  grip.setAttribute("aria-label", `Height of ${id}, in lines: drag, or press up and down`);
  grip.setAttribute("aria-valuemin", "1");
  // set keeps a height of rows lines, or none when that is no taller
  // than the box would be anyway.
  const set = (rows: number) => {
    const { line, chrome } = metrics(box);
    const least = (natural(box) - chrome) / line;
    if (rows <= least + 0.25) tall.delete(key);
    else tall.set(key, rows);
    sizeTextbox(box);
  };
  grip.onpointerdown = (e) => {
    if (e.button !== 0) return;
    e.preventDefault();
    grip.setPointerCapture(e.pointerId);
    const y0 = e.clientY;
    const h0 = box.offsetHeight;
    document.body.classList.add("resizing-rows");
    grip.onpointermove = (m) => {
      const { line, chrome } = metrics(box);
      set((h0 + m.clientY - y0 - chrome) / line);
    };
    grip.onpointerup = grip.onpointercancel = () => {
      grip.onpointermove = grip.onpointerup = grip.onpointercancel = null;
      document.body.classList.remove("resizing-rows");
    };
  };
  // Down makes it a line taller and up a line shorter, as dragging the
  // grip down and up does.
  grip.onkeydown = (e) => {
    const dir = e.key === "ArrowDown" ? 1 : e.key === "ArrowUp" ? -1 : 0;
    if (!dir) return;
    e.preventDefault();
    const { line, chrome } = metrics(box);
    set(Math.round((box.offsetHeight - chrome) / line) + dir);
  };
  grip.ondblclick = () => {
    tall.delete(key);
    sizeTextbox(box);
  };
  return grip;
}
