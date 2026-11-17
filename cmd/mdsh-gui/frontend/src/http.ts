// The response view on an http block's card: the status, the body and
// the headers as two tabs, body first, and the checks with a tick or a
// cross. The output under it still holds the response as the file saves
// it, for copying.

import { type HTTPCell } from "./api.js";
import { el, icons, keyActivates, svg } from "./dom.js";
import { codeLines } from "./highlight.js";

// statusClass is the class for a status code's badge: ok for 2xx,
// redirect for 3xx, bad for 4xx and 5xx.
export function statusClass(code: number): string {
  if (code >= 200 && code < 300) return "ok";
  if (code >= 300 && code < 400) return "redirect";
  return "bad";
}

// statusBadge is the response's status for the card's head.
export function statusBadge(h: HTTPCell): HTMLElement {
  return el("span", { className: "http-status " + statusClass(h.code ?? 0), title: "The response's status" }, h.status ?? "");
}

let views = 0;

// bodyFolds are the response bodies folded by hand, by the key drawHTTP
// is given, so a fold outlives the card being drawn again. They last
// while the app is open; the file keeps only the whole output's fold.
const bodyFolds = new Map<string, boolean>();

// drawHTTP draws the response view. A tab is picked with a click or,
// once focused, the left and right arrows. The body folds on its own to
// its first line and how many more there are, leaving the status in the
// card's head and the checks under it in view; key names the block.
export function drawHTTP(h: HTTPCell, key = ""): HTMLElement {
  const id = "http-" + ++views;
  const body = h.body ?? "";
  const json = /^\s*[[{]/.test(body);
  const full = el("div", { className: "http-body" }, ...(body ? codeLines(body, json ? "json" : "") : [el("span", { className: "http-empty" }, "No body")]));
  const lines = body.split("\n").filter((l) => l.trim());
  const summary = el(
    "div",
    { className: "http-body-summary", title: "Unfold the body", role: "button", tabIndex: 0, ariaLabel: "Unfold the body" },
    el("span", { className: "first" }, lines[0] ?? ""),
    el("span", { className: "muted" }, lines.length > 1 ? ` … ${lines.length - 1} more ${lines.length === 2 ? "line" : "lines"}` : ""),
  );
  const bodyPane = el("pre", { className: "http-pane code" }, full, summary);
  const fold = el("button", { className: "fold http-fold", title: "Fold or unfold the body" }, svg(icons.chevron));
  const setFold = (on: boolean) => {
    bodyPane.classList.toggle("folded", on);
    fold.classList.toggle("folded", on);
    fold.setAttribute("aria-expanded", String(!on));
    if (key) bodyFolds.set(key, on);
  };
  // The fold sits beside the Body tab, and from the Headers tab it also
  // shows the body.
  fold.onclick = (e) => {
    e.stopPropagation();
    pick(0, false);
    setFold(!bodyPane.classList.contains("folded"));
  };
  summary.onclick = (e) => {
    e.stopPropagation();
    setFold(false);
  };
  summary.onkeydown = (e) => {
    if (keyActivates(e)) setFold(false);
  };
  // A body of one line or none has nothing to fold.
  fold.hidden = lines.length < 2;
  setFold(!fold.hidden && (bodyFolds.get(key) ?? false));
  const panes = [
    { name: "Body", el: bodyPane },
    { name: "Headers", el: el("pre", { className: "http-pane code" }, ...(h.headers ? codeLines(h.headers, "http-headers") : [el("span", { className: "http-empty" }, "No headers")])) },
  ];
  const tabs = panes.map((p, i) =>
    el("button", { className: "http-tab", role: "tab", id: `${id}-tab-${i}`, tabIndex: i === 0 ? 0 : -1 }, p.name + (i === 1 && h.headers ? ` (${h.headers.trimEnd().split("\n").length})` : "")),
  );
  const pick = (i: number, focus: boolean) => {
    tabs.forEach((t, j) => {
      t.setAttribute("aria-selected", String(i === j));
      t.tabIndex = i === j ? 0 : -1;
      panes[j].el.hidden = i !== j;
    });
    if (focus) tabs[i].focus();
  };
  tabs.forEach((t, i) => {
    panes[i].el.setAttribute("role", "tabpanel");
    panes[i].el.setAttribute("aria-labelledby", t.id);
    t.setAttribute("aria-controls", (panes[i].el.id = `${id}-pane-${i}`));
    t.onclick = (e) => {
      e.stopPropagation();
      pick(i, false);
    };
    t.onkeydown = (e) => {
      if (e.key === "ArrowRight" || e.key === "ArrowLeft") {
        e.preventDefault();
        e.stopPropagation();
        pick((i + (e.key === "ArrowRight" ? 1 : panes.length - 1)) % panes.length, true);
      }
    };
  });
  pick(0, false);
  const bar = el("div", { className: "http-tabs", role: "tablist", ariaLabel: "Response" }, tabs[0], fold, ...tabs.slice(1));
  const view = el("div", { className: "http-view" }, bar, ...panes.map((p) => p.el));
  if (h.checks?.length) {
    const failed = h.checks.filter((c) => !c.ok).length;
    view.append(
      el(
        "ul",
        { className: "http-checks", ariaLabel: failed ? `Checks: ${failed} of ${h.checks.length} failed` : `Checks: all ${h.checks.length} passed` },
        ...h.checks.map((c) =>
          el(
            "li",
            { className: c.ok ? "pass" : "fail" },
            el("span", { className: "mark", ariaLabel: c.ok ? "passed" : "failed" }, c.ok ? "✓" : "✗"),
            el("code", {}, c.text),
            c.ok ? "" : el("span", { className: "got" }, "got " + (c.got ?? "")),
          ),
        ),
      ),
    );
  }
  return view;
}
