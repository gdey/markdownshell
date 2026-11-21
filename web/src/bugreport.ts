// Help › Copy Bug Report and the Console's Bug Report button copy a
// Markdown report of the app, this tab and what it is doing, to paste
// into an issue or a chat with Claude. Go writes most of it; the page
// adds what only it knows, such as whether the History pane is
// minimized.

import { Clipboard } from "/wails/runtime.js";
import { App } from "./api.js";

// extra is what app.ts says of the page, as "name: value" lines.
let extra: () => string[] = () => [];

// bugReportPage has fn say what the page is doing, for bug reports.
export function bugReportPage(fn: () => string[]) {
  extra = fn;
}

// history says how the History pane shows: off, minimized, open or
// filling the tab, and what its status line says.
function history(): string[] {
  const pane = document.getElementById("history");
  if (!pane || pane.hidden) return ["History pane: not shown"];
  const how = document.body.classList.contains("history-full") ? "filling the tab" : pane.classList.contains("closed") ? "minimized" : "open";
  const status = document.getElementById("history-status")?.textContent?.trim();
  return [`History pane: ${how}`, ...(status ? [`History status: ${status}`] : [])];
}

// pageLines is what the page says of itself.
function pageLines(): string[] {
  const tab = document.querySelector('#console [role="tab"][aria-selected="true"]')?.textContent?.trim();
  let mine: string[] = [];
  try {
    mine = extra();
  } catch {
    // Said without it.
  }
  return [...mine, ...history(), ...(tab ? [`Console tab: ${tab}`] : []), `Window: ${window.innerWidth}×${window.innerHeight} at ${window.devicePixelRatio}x`];
}

// copyBugReport copies a bug report for this tab, and says how it went
// with note.
export async function copyBugReport(note: (text: string) => void) {
  try {
    await Clipboard.SetText(await App.BugReport(pageLines()));
    note("Bug report copied: paste it into an issue or a chat");
  } catch {
    note("Couldn't copy the bug report");
  }
}
