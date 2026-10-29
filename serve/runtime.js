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
.mdsh-dialog { background: Canvas; color: CanvasText; border-radius: 8px; padding: 18px 20px; min-width: 320px; max-width: min(560px, 90vw); box-shadow: 0 8px 32px rgb(0 0 0 / 30%); font: 14px system-ui, sans-serif; }
.mdsh-dialog h2 { font-size: 15px; margin: 0 0 8px; }
.mdsh-dialog p { margin: 0 0 14px; white-space: pre-wrap; }
.mdsh-dialog input { width: 100%; box-sizing: border-box; font: 13px ui-monospace, monospace; padding: 6px; margin: 0 0 14px; }
.mdsh-dialog .buttons { display: flex; gap: 8px; justify-content: flex-end; }
.mdsh-dialog button { font: inherit; padding: 4px 14px; }
.mdsh-banner { position: fixed; left: 50%; top: 8px; transform: translateX(-50%); background: Canvas; color: CanvasText; border: 1px solid GrayText; border-radius: 6px; padding: 6px 12px; z-index: 1001; font: 13px system-ui, sans-serif; }
.mdsh-closed { display: grid; place-items: center; height: 100vh; font: 15px system-ui, sans-serif; }
`,
  );
  document.head.append(style);
}

// dialog shows a dialog with body and buttons, and resolves to what
// done is called with.
function dialog(title, body, buttons, def, cancel, value) {
  addStyle();
  return new Promise((resolve) => {
    const back = h("div", { className: "mdsh-backdrop" });
    const done = (i) => {
      back.remove();
      resolve(value(i));
    };
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

// promptPath asks for a path and resolves to it, or "" if cancelled.
function promptPath(p) {
  const input = h("input", { type: "text", value: p.value, spellcheck: false, ariaLabel: "Path" });
  const res = dialog(p.title, [h("p", {}, "Path on the machine mdsh runs on:"), input], ["Cancel", "OK"], 1, 0, (i) => (i === 1 ? input.value.trim() : ""));
  input.setSelectionRange(input.value.length, input.value.length);
  return res;
}

let bannerEl;
function banner(text, href, link) {
  addStyle();
  bannerEl?.remove();
  bannerEl = h("div", { className: "mdsh-banner", role: "status" }, text);
  if (href) bannerEl.append(h("a", { href, target: "_blank" }, link));
  document.body.append(bannerEl);
}
