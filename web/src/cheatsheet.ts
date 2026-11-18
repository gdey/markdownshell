// A cheat sheet tab: docs/cheatsheet.md as built into the app, rendered
// by Go and shown read-only, as the changelog is. Its web links open in
// the browser, and links to its own headings scroll to them.

import "./tab.js";
import { Events } from "/wails/runtime.js";

import { App, type Settings, message } from "./api.js";
import { $ } from "./dom.js";
import { applyTheme } from "./theme.js";

const page = $("changelog");

Events.On("settings", (ev: { data: Settings }) => applyTheme(ev.data));
applyTheme(await App.Settings());

try {
  page.innerHTML = await App.Cheatsheet();
} catch (err) {
  page.textContent = "The cheat sheet couldn't be shown: " + message(err);
}
page.removeAttribute("aria-busy");

page.addEventListener("click", (e) => {
  const a = (e.target as Element | null)?.closest("a[href]");
  if (!a) return;
  e.preventDefault();
  const href = a.getAttribute("href")!;
  if (href.startsWith("#")) {
    document.getElementById(decodeURIComponent(href.slice(1)))?.scrollIntoView();
    return;
  }
  if (/^https?:\/\//i.test(href)) App.OpenURL(href).catch((err) => ($("status").textContent = message(err)));
});
