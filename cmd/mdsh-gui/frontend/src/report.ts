// The Report dialog: a PDF, HTML or markdown copy of the run, as the
// matching mdsh command would make it.

import { App, Format, type Report, type ReportStart, message } from "./api.js";
import { $ } from "./dom.js";

// Hooks are what the dialog needs from the window.
export interface Hooks {
  // dirty says the window has output or edits not saved.
  dirty(): boolean;
  // before applies what was typed in the source pane.
  before(): Promise<void>;
  // runAll runs every block, asking as Run all does.
  runAll(): Promise<void>;
}

let hooks: Hooks;
let start: ReportStart = { ran: 0, total: 0, dest: "", home: "" };
// The choices last made, kept while the window is open.
let format: Format = Format.PDF;
let dest = "";

const dialog = () => $<HTMLDialogElement>("report");
const from = () => document.querySelector<HTMLInputElement>('input[name="rep-from"]:checked')!.value;

function tilde(path: string): string {
  const home = start.home;
  if (home && path.startsWith(home + "/")) return "~" + path.slice(home.length);
  return path;
}

function untilde(path: string): string {
  return start.home && path.startsWith("~/") ? start.home + path.slice(1) : path;
}

// withExt swaps a known report extension on path for format's.
function withExt(path: string, f: Format): string {
  const ext = { [Format.PDF]: ".pdf", [Format.HTML]: ".html", [Format.Markdown]: ".md", [Format.$zero]: "" }[f];
  const base = path.replace(/\.(pdf|html?|md|markdown)$/i, "");
  return base + ext;
}

function request(): Report {
  return {
    format,
    hideCode: !$<HTMLInputElement>("rep-code").checked,
    hideOutput: !$<HTMLInputElement>("rep-output").checked,
    saved: from() === "saved",
    ran: from() === "run",
    dest: untilde($<HTMLInputElement>("rep-dest").value.trim()),
  };
}

async function drawCommand() {
  for (const b of document.querySelectorAll<HTMLButtonElement>("#report [data-format]")) {
    b.setAttribute("aria-checked", String(b.dataset.format === format));
  }
  $("rep-same").textContent = from() === "window" && hooks.dirty() ? "Once saved, same as" : "Same as";
  try {
    $("rep-cmd").textContent = await App.ReportCommand(request());
  } catch {
    $("rep-cmd").textContent = "";
  }
}

function showError(err: unknown) {
  $("rep-error").textContent = message(err);
  $("rep-error").hidden = false;
}

function showForm(form: boolean) {
  $("report-form").hidden = !form;
  $("rep-buttons").hidden = !form;
  $("rep-after").hidden = form;
  $("rep-done").hidden = form;
}

export async function openReport() {
  await hooks.before();
  try {
    start = await App.ReportStart();
  } catch {
    return; // no document open
  }
  if (!dest || !dest.startsWith(start.dest.replace(/[^/]*$/, ""))) dest = withExt(start.dest, format);
  $<HTMLInputElement>("rep-dest").value = tilde(dest);
  $("rep-window").textContent = `This tab's run (${start.ran} of ${start.total} ${start.total === 1 ? "block" : "blocks"} ran)`;
  $("rep-error").hidden = true;
  showForm(true);
  void drawCommand();
  dialog().showModal();
}

async function create() {
  const r = request();
  if (!r.dest) return showError("Pick a file to save the report to.");
  dest = r.dest;
  if (r.ran) {
    // Run all asks its questions in the window, so the dialog steps
    // aside until it is done.
    dialog().close();
    await hooks.runAll();
  }
  try {
    await App.CreateReport(r);
  } catch (err) {
    if (!dialog().open) dialog().showModal();
    return showError(err);
  }
  $("rep-done").textContent = "Saved the report to " + tilde(r.dest) + ".";
  $("rep-error").hidden = true;
  showForm(false);
  if (!dialog().open) dialog().showModal();
}

export function initReport(h: Hooks) {
  hooks = h;
  for (const b of document.querySelectorAll<HTMLButtonElement>("#report [data-format]")) {
    b.onclick = () => {
      format = b.dataset.format as Format;
      const input = $<HTMLInputElement>("rep-dest");
      input.value = withExt(input.value, format);
      void drawCommand();
    };
  }
  for (const x of document.querySelectorAll<HTMLInputElement>("#report input")) x.addEventListener("input", () => void drawCommand());
  $("rep-choose").onclick = async () => {
    try {
      const file = await App.ChooseReport(format, untilde($<HTMLInputElement>("rep-dest").value.trim()) || start.dest);
      if (file) {
        $<HTMLInputElement>("rep-dest").value = tilde(file);
        void drawCommand();
      }
    } catch (err) {
      showError(err);
    }
  };
  $("rep-ok").onclick = (e) => {
    e.preventDefault();
    void create();
  };
  $("rep-reveal").onclick = () => {
    App.Reveal(dest).catch(showError);
  };
}
