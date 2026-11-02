// The grip between the source view and the rendered view: dragging it
// shares the width between them, and the share is kept for next time.

import { $ } from "./dom.js";

const shareKey = "mdsh.sourceShare";
const least = 0.15; // neither view gets less of the width than this

// share sizes the source view to f of the width the two views have.
function share(f: number) {
  f = Math.min(1 - least, Math.max(least, f));
  $("source").style.flex = `${f} 1 0`;
  $("doc-scroll").style.flex = `${1 - f} 1 0`;
  $("split-grip").setAttribute("aria-valuenow", String(Math.round(f * 100)));
  return f;
}

function keep(f: number) {
  try {
    localStorage.setItem(shareKey, String(f));
  } catch {
    // Not kept, then.
  }
  window.dispatchEvent(new Event("resize"));
}

// showSplitGrip shows the grip while both views are.
export function showSplitGrip(on: boolean) {
  $("split-grip").hidden = !on;
  if (!on) {
    $("source").style.flex = "";
    $("doc-scroll").style.flex = "";
    return;
  }
  let f = 0.5;
  try {
    f = Number(localStorage.getItem(shareKey)) || 0.5;
  } catch {
    // No storage: half each.
  }
  share(f);
}

function drag(e: PointerEvent) {
  if (e.button !== 0) return;
  e.preventDefault();
  const grip = e.currentTarget as HTMLElement;
  grip.setPointerCapture(e.pointerId);
  const left = $("source").getBoundingClientRect().left;
  const width = $("source").offsetWidth + $("doc-scroll").offsetWidth;
  let f = 0.5;
  document.body.classList.add("resizing-cols");
  grip.onpointermove = (m) => {
    f = share((m.clientX - left) / width);
  };
  grip.onpointerup = grip.onpointercancel = () => {
    grip.onpointermove = grip.onpointerup = grip.onpointercancel = null;
    document.body.classList.remove("resizing-cols");
    keep(f);
  };
}

export function initSplitGrip() {
  const grip = $("split-grip");
  grip.onpointerdown = drag;
  // From the keyboard, left and right move the split by a twentieth.
  grip.onkeydown = (e) => {
    const dir = e.key === "ArrowLeft" ? -1 : e.key === "ArrowRight" ? 1 : 0;
    if (!dir) return;
    e.preventDefault();
    const width = $("source").offsetWidth + $("doc-scroll").offsetWidth;
    keep(share($("source").offsetWidth / width + dir * 0.05));
  };
  // A double click shares the width evenly again.
  grip.ondblclick = () => {
    share(0.5);
    try {
      localStorage.removeItem(shareKey);
    } catch {
      // Nothing kept.
    }
    window.dispatchEvent(new Event("resize"));
  };
}
