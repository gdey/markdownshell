// The front matter sheet: a tab at the top of the rendered view pulls the
// page down to show a form for the file's front matter, the keys mdsh
// and the start screen read in fields of their own and the rest as YAML.
// A field is written to the file when it changes (on leaving a text
// field, at once for the others), as one edit Undo takes back. The tab
// again, Done or escape puts the page back.

import { App, type FrontMatter, message } from "./api.js";
import { drawDeps } from "./deps.js";
import { $ } from "./dom.js";

// Hooks are what the sheet needs from the window.
export interface Hooks {
  // edit makes a rendered-view edit, as checking a task does, and
  // reports whether it was made.
  edit(call: () => Promise<void>): Promise<boolean>;
}

let hooks: Hooks;
let shown: FrontMatter | null = null;
let open = false;

const input = (id: string) => $<HTMLInputElement>(id);
const area = (id: string) => $<HTMLTextAreaElement>(id);
const choice = (id: string) => $<HTMLSelectElement>(id);
const shareAll = () => document.querySelector<HTMLInputElement>('input[name="fm-share"][value="all"]')!;
const shareSome = () => document.querySelector<HTMLInputElement>('input[name="fm-share"][value="some"]')!;

// words splits a field at commas and spaces.
const words = (s: string) => s.split(/[\s,]+/).filter(Boolean);

export function initFrontMatter(h: Hooks) {
  hooks = h;
  $("fm-toggle").onclick = () => void showFrontMatter(!open);
  $("fm-done").onclick = () => void showFrontMatter(false, true);
  $("fm-sheet").addEventListener("keydown", (e) => {
    if (e.key !== "Escape") return;
    e.preventDefault();
    e.stopPropagation();
    void showFrontMatter(false, true);
  });
  // Text is written on leaving the field (change), the rest at once.
  $("fm-form").addEventListener("change", () => void apply());
  $("fm-form").addEventListener("submit", (e) => e.preventDefault());
  // Enter in a one-line field writes it, as leaving it would.
  $("fm-form").addEventListener("keydown", (e) => {
    const t = e.target as HTMLElement;
    if (e.key === "Enter" && t instanceof HTMLInputElement && t.type !== "checkbox" && t.type !== "radio") {
      e.preventDefault();
      void apply();
    }
  });
  input("fm-share-list").addEventListener("input", () => {
    if (!shareSome().checked) shareSome().checked = true;
  });
  // The page moves as the sheet opens and closes, so the dependency
  // lines are drawn again as it does.
  new ResizeObserver(() => drawDeps()).observe($("fm-sheet"));
}

// showFrontMatter opens or closes the sheet. Opening reads the front
// matter and puts the keyboard in its first field; closing with home
// puts it back on the tab.
export async function showFrontMatter(on: boolean, home = false) {
  if (on) await loadFrontMatter();
  open = on;
  const sheet = $("fm-sheet");
  sheet.classList.toggle("open", on);
  sheet.inert = !on;
  sheet.setAttribute("aria-hidden", String(!on));
  $("fm-toggle").setAttribute("aria-expanded", String(on));
  if (on) {
    $("doc-scroll").scrollTo({ top: 0 });
    input("fm-title-in").focus({ preventScroll: true });
  } else if (home || $("fm-sheet").contains(document.activeElement)) {
    $("fm-toggle").focus({ preventScroll: true });
  }
}

export const frontMatterOpen = () => open;

// loadFrontMatter reads the file's front matter and shows it, keeping
// what is being typed in the focused field.
export async function loadFrontMatter() {
  try {
    shown = await App.FrontMatter();
  } catch (err) {
    shown = null;
    setError(message(err));
    return;
  }
  setError("");
  const f = shown;
  $("fm-toggle-text").textContent = f.exists ? "Front matter" : "Add front matter";
  const at = document.activeElement;
  const fill = (e: HTMLInputElement | HTMLTextAreaElement | HTMLSelectElement, v: string) => {
    if (e !== at) e.value = v;
  };
  fill(input("fm-title-in"), f.title);
  fill(area("fm-desc"), f.description);
  $("fm-desc-label").textContent = f.descriptionKey === "description" ? "Description" : `Description (${f.descriptionKey})`;
  fill(input("fm-tags"), (f.tags ?? []).join(", "));
  fill(input("fm-shell"), f.shell);
  fill(choice("fm-onerror"), f.onError);
  input("fm-step").checked = f.step;
  fill(choice("fm-interp"), f.interpolate);
  fill(choice("fm-stderr"), f.showStderr);
  fill(input("fm-cols"), f.cols ? String(f.cols) : "");
  fill(input("fm-rows"), f.rows ? String(f.rows) : "");
  shareAll().checked = f.shareAll;
  shareSome().checked = !f.shareAll;
  fill(input("fm-share-list"), (f.share ?? []).join(" "));
  fill(area("fm-source"), (f.source ?? []).join("\n"));
  fill(area("fm-other"), f.other);
}

// busyFrontMatter turns the form off while blocks run, as the file
// can't change then.
export function busyFrontMatter(on: boolean) {
  $<HTMLFieldSetElement>("fm-fields").disabled = on;
}

function setError(text: string) {
  $("fm-error").textContent = text;
  $("fm-error").hidden = text === "";
}

// form is the front matter as the form has it.
function form(): FrontMatter {
  const num = (id: string) => Math.max(0, Math.trunc(Number(input(id).value) || 0));
  return {
    exists: shown?.exists ?? false,
    title: input("fm-title-in").value,
    description: area("fm-desc").value,
    descriptionKey: shown?.descriptionKey ?? "description",
    tags: words(input("fm-tags").value),
    shell: input("fm-shell").value.trim(),
    onError: choice("fm-onerror").value,
    step: input("fm-step").checked,
    interpolate: choice("fm-interp").value,
    showStderr: choice("fm-stderr").value,
    cols: num("fm-cols"),
    rows: num("fm-rows"),
    shareAll: shareAll().checked,
    share: words(input("fm-share-list").value),
    source: area("fm-source").value.split("\n").map((s) => s.trim()).filter(Boolean),
    other: area("fm-other").value,
  };
}

// apply writes the form to the file. What mdsh refuses is said under
// the form and changes nothing, so it can be put right.
async function apply() {
  if (!shown) return;
  let refused = "";
  const f = form();
  const ok = await hooks.edit(async () => {
    try {
      await App.SetFrontMatter(f);
    } catch (err) {
      refused = message(err);
      throw err;
    }
  });
  if (!ok) {
    setError(refused || "the front matter can't change now");
    return;
  }
  await loadFrontMatter();
}
