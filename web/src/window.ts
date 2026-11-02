// A window's page: the tab bar, and each tab's page in an iframe under
// it. Events from Go for one tab carry its id, and this page passes them
// on to that tab's page alone; other events go to every tab.

import { Events, Window } from "/wails/runtime.js";

import { App, type Settings, type Tab, type Tabs } from "./api.js";
import { $, el } from "./dom.js";
import { applyTheme } from "./theme.js";

const frames = new Map<number, HTMLIFrameElement>();
interface State {
  tabs: Tab[];
  active: number;
}
let state: State = { tabs: [], active: 0 };
let me = "";
let dragging = 0; // the tab this window is dragging, if any

interface WailsEvent {
  name: string;
  data: unknown;
  sender?: string;
}
interface Wails {
  dispatchWailsEvent(e: WailsEvent): void;
}
const wails = (w: Window) => (w as unknown as { _wails?: Wails })._wails;

// Go sends events to this page; pass them on.
const own = wails(window)!;
const dispatch = own.dispatchWailsEvent.bind(own);
own.dispatchWailsEvent = (e: WailsEvent) => {
  const d = e.data as { mdshTab?: number; data?: unknown } | null;
  if (d && typeof d === "object" && typeof d.mdshTab === "number") {
    const f = frames.get(d.mdshTab)?.contentWindow;
    if (f) wails(f)?.dispatchWailsEvent({ ...e, data: d.data });
    return;
  }
  dispatch(e);
  for (const f of frames.values()) if (f.contentWindow) wails(f.contentWindow)?.dispatchWailsEvent(e);
};

function show(s: Tabs) {
  state = { tabs: s.tabs ?? [], active: s.active };
  draw();
}

function draw() {
  const bar = $("tabs");
  bar.replaceChildren(...state.tabs.map((t) => tabButton(t)));
  const seen = new Set<number>();
  for (const t of state.tabs) {
    seen.add(t.id);
    let f = frames.get(t.id);
    if (!f) {
      f = el("iframe", { src: "/index.html?tab=" + t.id, title: t.title, id: "panel-" + t.id });
      frames.set(t.id, f);
      $("frames").append(f);
    }
    f.hidden = t.id !== state.active;
  }
  for (const [id, f] of frames) {
    if (!seen.has(id)) {
      f.remove();
      frames.delete(id);
    }
  }
  const active = frames.get(state.active);
  // A tab picked from the keyboard keeps the focus in the tab bar.
  const inBar = document.activeElement?.closest("#tabs") != null;
  if (inBar) document.getElementById("tab-" + state.active)?.focus();
  else if (active && document.hasFocus()) active.contentWindow?.focus();
  document.body.classList.toggle("one-tab", state.tabs.length < 2);
}

function tabButton(t: Tab): HTMLElement {
  // The close button is for the mouse: a tab's children are
  // presentational, and delete on the tab closes it from the keyboard.
  const close = el("span", { className: "close", title: "Close tab (cmd/ctrl+W)", ariaHidden: "true" }, "×");
  close.onclick = (e) => {
    e.stopPropagation();
    void App.CloseTab(t.id);
  };
  const active = t.id === state.active;
  // A tab is a tab stop only while it is in front: the arrow keys move
  // between the tabs, and enter or space shows one.
  const b = el(
    "div",
    { className: "tab" + (active ? " active" : ""), title: t.file || "Start screen", draggable: true, role: "tab", id: "tab-" + t.id, tabIndex: active ? 0 : -1 },
    el("span", { className: "dot", hidden: !t.dirty, title: "Output not saved", ariaHidden: "true" }, "•"),
    el("span", { className: "label" }, t.title),
    close,
  );
  b.setAttribute("aria-selected", String(active));
  b.setAttribute("aria-controls", "panel-" + t.id);
  b.setAttribute("aria-label", t.title + (t.dirty ? ", output not saved" : ""));
  b.dataset.tab = String(t.id);
  b.onkeydown = (e) => {
    const i = state.tabs.indexOf(t);
    let to: Tab | undefined;
    if (e.key === "ArrowLeft") to = state.tabs[(i + state.tabs.length - 1) % state.tabs.length];
    else if (e.key === "ArrowRight") to = state.tabs[(i + 1) % state.tabs.length];
    else if (e.key === "Home") to = state.tabs[0];
    else if (e.key === "End") to = state.tabs[state.tabs.length - 1];
    else if (e.key === "Enter" || e.key === " ") to = t;
    else if (e.key === "Delete" || e.key === "Backspace") return void App.CloseTab(t.id);
    else return;
    e.preventDefault();
    if (to) void App.SelectTab(to.id);
  };
  b.onmousedown = (e) => {
    if (e.button === 1) e.preventDefault();
  };
  b.onauxclick = (e) => {
    if (e.button === 1) void App.CloseTab(t.id);
  };
  b.onclick = () => void App.SelectTab(t.id);
  b.ondragstart = (e) => {

    dragging = t.id;
    e.dataTransfer?.setData("application/x-mdsh-tab", String(t.id));
    e.dataTransfer?.setData("text/plain", t.title);
    if (e.dataTransfer) e.dataTransfer.effectAllowed = "move";
    b.classList.add("dragging");
    void App.DragTab(t.id);
  };
  b.ondragend = (e) => {
    b.classList.remove("dragging");
    const id = dragging;
    dragging = 0;
    const outside = e.screenX < window.screenX || e.screenX > window.screenX + window.outerWidth || e.screenY < window.screenY || e.screenY > window.screenY + window.outerHeight;
    // A window it was dropped on may still be taking it.
    setTimeout(() => void App.EndDrag(id, outside, Math.round(e.screenX), Math.round(e.screenY)), 250);
  };
  return b;
}

// Dropping a tab on the tab bar puts it where the marker shows.
function dropIndex(x: number): number {
  const tabs = [...document.querySelectorAll<HTMLElement>("#tabs .tab")];
  const i = tabs.findIndex((t) => {
    const r = t.getBoundingClientRect();
    return x < r.left + r.width / 2;
  });
  return i < 0 ? tabs.length : i;
}

function marker(i: number) {
  const tabs = [...document.querySelectorAll<HTMLElement>("#tabs .tab")];
  tabs.forEach((t, j) => {
    t.classList.toggle("drop-before", j === i);
    t.classList.toggle("drop-after", i === tabs.length && j === tabs.length - 1);
  });
}

const isTab = (e: DragEvent) => !!e.dataTransfer && [...e.dataTransfer.types].some((t) => t === "application/x-mdsh-tab" || t === "text/plain");

const bar = $("tabbar");
bar.ondragover = (e) => {
  if (!isTab(e)) return;
  e.preventDefault();
  if (e.dataTransfer) e.dataTransfer.dropEffect = "move";
  marker(dropIndex(e.clientX));
};
bar.ondragleave = (e) => {
  if (!bar.contains(e.relatedTarget as Node | null)) marker(-1);
};
bar.ondrop = (e) => {
  e.preventDefault();
  marker(-1);
  void App.DropTab(dropIndex(e.clientX));
};

// While a tab is dragged, the page under the tab bar takes it too: in
// another window it moves there, at the end of the tab bar, and in its
// own window it gets a window of its own.
const here = $("drop-here");
here.ondragover = (e) => {
  if (!isTab(e)) return;
  e.preventDefault();
  if (e.dataTransfer) e.dataTransfer.dropEffect = "move";
  here.classList.add("over");
};
here.ondragleave = () => here.classList.remove("over");
// Mouse moves don't happen during a drag, so one means it ended without
// saying so here.
here.onmousemove = () => (here.hidden = true);
here.ondrop = (e) => {
  e.preventDefault();
  here.classList.remove("over");
  here.hidden = true;
  if (dragging) void App.DetachTab(Math.round(window.screenX + e.clientX), Math.round(window.screenY + e.clientY));
  else void App.DropTab(state.tabs.length);
};
Events.On("tab-drag", (ev: { data: boolean }) => {
  here.hidden = !ev.data || (dragging !== 0 && state.tabs.length < 2);
  here.textContent = dragging ? "Drop here for a new window" : "Drop here to move the tab to this window";
  here.classList.remove("over");
});

$("new-tab").onclick = () => void App.NewTab();
$("tabbar").ondblclick = (e) => {
  if (e.target === $("tabs") || e.target === $("tabbar")) void App.NewTab();
};

Events.On("tabs", (ev: { sender?: string; data: Tabs }) => {
  if (ev.sender !== me) return;
  show(ev.data);
});
Events.On("settings", (ev: { data: Settings }) => applyTheme(ev.data));

me = await Window.Name();
applyTheme(await App.Settings());
show(await App.Tabs());
