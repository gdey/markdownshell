// The part of the Wails runtime mdsh's pages use, for mdsh --serve: calls
// to Go are POSTs to /wails/call, and events come over a WebSocket. A
// window's page holds the WebSocket and passes events on to its tabs'
// pages, as window.ts does with Wails's; a tab's page, in an iframe,
// takes its window from the page around it.
//
// The server also sends its own events, named mdsh:…, for what Wails
// would do natively: the window's title, dialogs, closing and opening
// windows. This file draws those.

const params = new URLSearchParams(location.search);
const parent = window.parent !== window ? window.parent : null;
const win = parent ? parent.__mdshWindow ?? "" : params.get("w") ?? "";
window.__mdshWindow = win;
const mac = /Mac|iPhone|iPad/.test(navigator.platform);

const listeners = new Map();
window._wails = {
  dispatchWailsEvent(e) {
    for (const cb of [...(listeners.get(e.name) ?? [])]) cb(e);
  },
};

export class CancellablePromise extends Promise {}

export const Call = {
  async ByName(name, ...args) {
    // window.fetch, looked up now: tab.ts adds the tab to it.
    const r = await window.fetch("/wails/call", {
      method: "POST",
      headers: { "content-type": "application/json", "x-mdsh-window": win },
      body: JSON.stringify({ name, args }),
    });
    const text = await r.text();
    if (!r.ok) throw new Error(text.trim() || r.statusText);
    return text ? JSON.parse(text) : undefined;
  },
};

export const Events = {
  On(name, cb) {
    if (!listeners.has(name)) listeners.set(name, new Set());
    listeners.get(name).add(cb);
    return () => listeners.get(name)?.delete(cb);
  },
  Off(name) {
    listeners.delete(name);
  },
};

export const Window = {
  async Name() {
    return "w" + win;
  },
};

export const Browser = {
  async OpenURL(url) {
    window.open(url, "_blank", "noopener");
  },
};

if (!parent && win) connect(false);
// A web link in a document opens in a new browser tab, as the click
// happens, so the browser lets it. app.ts sends the rest, such as links
// to other markdown files, to Go.
document.addEventListener(
  "click",
  (e) => {
    const a = e.target instanceof Element ? e.target.closest("#doc a[href]") : null;
    const href = a?.getAttribute("href") ?? "";
    if (!/^(https?:|mailto:|\/\/)/i.test(href)) return;
    e.preventDefault();
    e.stopImmediatePropagation();
    window.open(href, "_blank", "noopener");
  },
  true,
);
if (!parent && document.body.classList.contains("shell")) addSettingsButton();
document.addEventListener("keydown", onKey, true);
// A file dropped on the page would replace it; there is no path to open.
document.addEventListener("dragover", (e) => e.dataTransfer?.types.includes("Files") && e.preventDefault());
document.addEventListener("drop", (e) => e.dataTransfer?.types.includes("Files") && e.preventDefault());

// What the desktop app's menu shortcuts do, as the actions its "menu"
// events name. Browsers keep ctrl+T, ctrl+W and ctrl+N for themselves.
function keyAction(e) {
  if (!(mac ? e.metaKey : e.ctrlKey) || e.altKey) return "";
  const k = e.key.length === 1 ? e.key.toLowerCase() : e.key;
  const keys = e.shiftKey
    ? { s: "saveas", z: "redo", "|": "rendered", "\\": "rendered", o: "outline", "}": "next-tab", "]": "next-tab", "{": "prev-tab", "[": "prev-tab" }
    : { s: "save", z: "undo", r: "runall", ".": "stop", "\\": "split", o: "open", ",": "settings" };
  return keys[k] ?? "";
}

function onKey(e) {
  const action = keyAction(e);
  if (!action) return;
  e.preventDefault();
  e.stopPropagation();
  if (parent) return parent.__mdshMenu?.(action);
  window.__mdshMenu(action);
}

// __mdshMenu runs a shortcut's action, in the window's page.
window.__mdshMenu = (action) => {
  if (parent) return parent.__mdshMenu?.(action);
  const tabs = [...document.querySelectorAll("#tabs .tab")];
  const i = tabs.findIndex((t) => t.classList.contains("active"));
  switch (action) {
    case "settings":
      return void window.open("/settings", "_blank");
    case "open":
      return void Call.ByName("github.com/gdey/markdownshell/ui.App.Open").catch(showError);
    case "next-tab":
    case "prev-tab": {
      if (tabs.length < 2) return;
      const next = tabs[(i + (action === "next-tab" ? 1 : -1) + tabs.length) % tabs.length];
      return void Call.ByName("github.com/gdey/markdownshell/ui.App.SelectTab", Number(next.dataset.tab));
    }
  }
  // The rest are the tab's: send them to the tab in front as its menu.
  const id = Number(tabs[i]?.dataset.tab);
  // A page with no tabs, such as Settings, undoes as the browser does.
  if (!id && (action === "undo" || action === "redo")) return void document.execCommand(action);
  if (id) window._wails.dispatchWailsEvent({ name: "menu", data: { mdshTab: id, data: action }, sender: "w" + win });
};

function showError(err) {
  ask({ title: "mdsh", message: err.message ?? String(err), buttons: ["OK"], default: 0, cancel: 0 });
}

function addSettingsButton() {
  const b = h("button", { className: "icon", id: "mdsh-settings", title: `Settings (${mac ? "cmd" : "ctrl"}+,)`, ariaLabel: "Settings" });
  b.innerHTML =
    '<svg viewBox="0 0 16 16" width="16" height="16" aria-hidden="true" fill="currentColor"><path d="M8 5.5a2.5 2.5 0 1 0 0 5 2.5 2.5 0 0 0 0-5zm0 1.2a1.3 1.3 0 1 1 0 2.6 1.3 1.3 0 0 1 0-2.6z"/><path d="M6.9 1h2.2l.4 1.8 1 .5 1.6-1 1.6 1.6-1 1.6.5 1 1.8.4v2.2l-1.8.4-.5 1 1 1.6-1.6 1.6-1.6-1-1 .5-.4 1.8H6.9l-.4-1.8-1-.5-1.6 1-1.6-1.6 1-1.6-.5-1L1 9.1V6.9l1.8-.4.5-1-1-1.6 1.6-1.6 1.6 1 1-.5zm1 1.2-.3 1.5-1.9.8-1.3-.8-.4.4.8 1.3-.8 1.9-1.5.3v.6l1.5.3.8 1.9-.8 1.3.4.4 1.3-.8 1.9.8.3 1.5h.6l.3-1.5 1.9-.8 1.3.8.4-.4-.8-1.3.8-1.9 1.5-.3v-.6l-1.5-.3-.8-1.9.8-1.3-.4-.4-1.3.8-1.9-.8-.3-1.5z"/></svg>';
  b.style.cssText = "flex: none; align-self: center; margin-bottom: 4px;";
  b.onclick = () => window.__mdshMenu("settings");
  document.getElementById("tabbar")?.append(b);
}

function connect(again) {
  const proto = location.protocol === "https:" ? "wss:" : "ws:";
  const ws = new WebSocket(`${proto}//${location.host}/wails/events?w=${encodeURIComponent(win)}`);
  let opened = false;
  ws.onopen = () => {
    opened = true;
    // Events sent while this page was away are lost: start over.
    if (again) location.reload();
  };
  ws.onmessage = (m) => {
    const e = JSON.parse(m.data);
    if (e.name.startsWith("mdsh:")) return ours(e);
    window._wails.dispatchWailsEvent(e);
  };
  ws.onclose = () => {
    if (closed) return;
    banner(opened ? "Lost the connection to mdsh. Trying again…" : "Can't reach mdsh. Trying again…");
    setTimeout(() => connect(true), opened ? 500 : 2000);
  };
}

let closed = false;

function ours(e) {
  switch (e.name) {
    case "mdsh:title":
      document.title = e.data;
      break;
    case "mdsh:focus":
      window.focus();
      break;
    case "mdsh:error":
      ask({ title: "mdsh", message: e.data, buttons: ["OK"], default: 0, cancel: 0 });
      break;
    case "mdsh:ask":
      ask(e.data).then((i) => answer(e.data.id, String(i)));
      break;
    case "mdsh:prompt":
      promptPath(e.data).then((v) => answer(e.data.id, v));
      break;
    case "mdsh:open-url":
      if (!window.open(e.data, "_blank", "noopener")) banner("mdsh opened a link. ", e.data, "Show it");
      break;
    case "mdsh:open-window":
      if (!window.open(e.data, "_blank")) banner("mdsh opened a new window. ", e.data, "Show it");
      break;
    case "mdsh:closed":
      closed = true;
      document.body.replaceChildren(
        h("div", { className: "mdsh-closed" }, h("p", {}, "This window is closed."), h("p", {}, h("a", { href: "/" }, "Open a window"))),
      );
      document.title = "mdsh";
      break;
  }
}

function answer(id, value) {
  void fetch("/wails/answer", {
    method: "POST",
    headers: { "content-type": "application/json" },
    body: JSON.stringify({ id, value }),
  });
}

// h makes an element.
function h(tag, props, ...children) {
  const el = Object.assign(document.createElement(tag), props);
  el.append(...children);
  return el;
}

let style;
function addStyle() {
  if (style) return;
  style = h(
    "style",
    {},
    `
.mdsh-backdrop { position: fixed; inset: 0; background: rgb(0 0 0 / 30%); display: grid; place-items: center; z-index: 1000; }
.mdsh-dialog { background: Canvas; color: CanvasText; border-radius: 8px; padding: 18px 20px; min-width: min(480px, 90vw); max-width: min(640px, 90vw); box-shadow: 0 8px 32px rgb(0 0 0 / 30%); font: 14px system-ui, sans-serif; }
.mdsh-dialog h2 { font-size: 15px; margin: 0 0 8px; }
.mdsh-dialog p { margin: 0 0 14px; white-space: pre-wrap; }
.mdsh-dialog input { width: 100%; box-sizing: border-box; font: 13px ui-monospace, monospace; padding: 6px; margin: 0 0 6px; }
.mdsh-dialog .buttons { display: flex; gap: 8px; justify-content: flex-end; }
.mdsh-where { font: 12px ui-monospace, monospace; color: GrayText; margin: 0 0 4px; overflow-wrap: anywhere; }
.mdsh-files { list-style: none; margin: 0 0 14px; padding: 0; height: 260px; overflow: auto; border: 1px solid GrayText; border-radius: 4px; font: 13px ui-monospace, monospace; }
.mdsh-files li { padding: 2px 8px; cursor: default; white-space: nowrap; }
.mdsh-files li:hover { background: Highlight; color: HighlightText; }
.mdsh-files li.dir { font-weight: 600; }
.mdsh-dialog button { font: inherit; padding: 4px 14px; }
.mdsh-banner { position: fixed; left: 50%; top: 8px; transform: translateX(-50%); background: Canvas; color: CanvasText; border: 1px solid GrayText; border-radius: 6px; padding: 6px 12px; z-index: 1001; font: 13px system-ui, sans-serif; }
.mdsh-closed { display: grid; place-items: center; height: 100vh; font: 15px system-ui, sans-serif; }
`,
  );
  document.head.append(style);
}

// dialog shows a dialog with body and buttons, and resolves to what
// done is called with.
function dialog(title, body, buttons, def, cancel, value, hook) {
  addStyle();
  return new Promise((resolve) => {
    const back = h("div", { className: "mdsh-backdrop" });
    const done = (i) => {
      back.remove();
      resolve(value(i));
    };
    hook?.(done);
    const btns = buttons.map((label, i) => {
      const b = h("button", { type: "button" }, label);
      b.onclick = () => done(i);
      return b;
    });
    const box = h("div", { className: "mdsh-dialog", role: "dialog", ariaModal: "true" }, h("h2", {}, title), ...body, h("div", { className: "buttons" }, ...btns));
    back.append(box);
    back.onkeydown = (e) => {
      if (e.key === "Escape" && cancel >= 0) done(cancel);
      if (e.key === "Enter" && def >= 0 && !(e.target instanceof HTMLButtonElement)) done(def);
    };
    document.body.append(back);
    (body.find((el) => el instanceof HTMLInputElement) ?? btns[def >= 0 ? def : 0])?.focus();
  });
}

// ask asks a question and resolves to the button picked.
function ask(q) {
  return dialog(q.title, [h("p", {}, q.message)], q.buttons, q.default, q.cancel, (i) => i);
}

// promptPath asks for a path on the server's machine and resolves to
// it, or "" if cancelled. Under the path is a list of the folder it is
// in: a click on a folder goes into it, on a file picks it, and a double
// click picks it and says OK.
function promptPath(p) {
  const input = h("input", { type: "text", value: p.value, spellcheck: false, ariaLabel: "Path" });
  const list = h("ul", { className: "mdsh-files", role: "listbox", ariaLabel: "Files" });
  const where = h("div", { className: "mdsh-where" });
  let ok = () => {};
  const res = dialog(p.title, [input, where, list], ["Cancel", p.kind === "folder" ? "Choose" : p.kind === "save" ? "Save" : "Open"], 1, 0, (i) => (i === 1 ? input.value.trim() : ""), (done) => (ok = () => done(1)));
  let shown = "";
  const show = async (dir) => {
    shown = dir;
    try {
      const r = await fetch("/wails/files?" + new URLSearchParams({ dir, kind: p.kind, pattern: p.pattern ?? "" }));
      if (!r.ok) throw new Error((await r.text()).trim());
      const d = await r.json();
      if (shown !== dir) return;
      where.textContent = d.dir;
      const items = [];
      if (d.parent) items.push(item("..", true, d.parent));
      for (const e of d.entries ?? []) items.push(item(e.name, e.dir, d.dir.replace(/\/$/, "") + "/" + e.name));
      list.replaceChildren(...items);
    } catch (err) {
      where.textContent = err.message;
      list.replaceChildren();
    }
  };
  const item = (name, dir, path) => {
    const li = h("li", { role: "option", tabIndex: -1, className: dir ? "dir" : "file" }, dir ? name + "/" : name);
    li.onclick = () => {
      if (dir) {
        input.value = p.kind === "save" ? path + "/" + baseName(input.value) : path + "/";
        void show(path);
      } else input.value = path;
      input.focus();
    };
    li.ondblclick = () => {
      if (!dir) ok();
    };
    return li;
  };
  const dirOf = (v) => (v.endsWith("/") ? v : v.slice(0, v.lastIndexOf("/") + 1) || "/");
  input.addEventListener("input", () => {
    const dir = dirOf(input.value);
    if (dir !== shown) void show(dir);
  });
  input.setSelectionRange(input.value.length, input.value.length);
  void show(dirOf(p.value));
  return res;
}

function baseName(v) {
  return v.slice(v.lastIndexOf("/") + 1);
}

let bannerEl;
function banner(text, href, link) {
  addStyle();
  bannerEl?.remove();
  bannerEl = h("div", { className: "mdsh-banner", role: "status" }, text);
  if (href) bannerEl.append(h("a", { href, target: "_blank" }, link));
  document.body.append(bannerEl);
}
