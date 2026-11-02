// The start screen's folder settings: a gear on each folder that can
// take a .mdsh.yaml, and the dialog that writes it.

import { App, message } from "./api.js";
import { $, el, iconButton } from "./dom.js";

let editing = "";
let saved: () => void = () => {};

// folderGear is the button that opens the settings of rel, the folder
// named name on the start screen. after runs once they are saved.
export function folderGear(rel: string, name: string, after: () => void): HTMLButtonElement {
  const b = iconButton("gear", `Folder settings for ${name}`, () => void openFolder(rel, name, after));
  b.classList.add("gear");
  b.title = "Folder settings";
  return b;
}

async function openFolder(rel: string, name: string, after: () => void) {
  let form;
  try {
    form = await App.FolderSettings(rel);
  } catch (err) {
    $("start-dir").textContent = message(err);
    return;
  }
  editing = rel;
  saved = after;
  const s = form.settings;
  $("folder-title").textContent = `Settings for ${name}`;
  $("folder-where").textContent = `Saved in ${form.path}/.mdsh.yaml. Every field is optional; other keys in the file are kept.`;
  const input = (id: string) => $<HTMLInputElement>(id);
  input("fs-title").value = s.title;
  input("fs-title").placeholder = name;
  input("fs-desc").value = s.description;
  input("fs-icon").value = s.icon;
  input("fs-color").value = s.color;
  input("fs-bg").value = s.background;
  input("fs-tags").value = (s.tags ?? []).join(", ");
  syncPicker();
  $("fs-images").replaceChildren(...(form.images ?? []).map((p) => el("option", { value: p })));
  $("fs-error").hidden = true;
  $<HTMLDialogElement>("folder-dialog").showModal();
  input("fs-title").focus();
}

// syncPicker shows the color box's color in the picker, when it is one
// the picker can show.
function syncPicker() {
  const v = $<HTMLInputElement>("fs-color").value.trim();
  const hex = /^#([0-9a-f]{3}|[0-9a-f]{6})$/i.exec(v);
  if (!hex) return;
  const h = hex[1].length === 3 ? [...hex[1]].map((c) => c + c).join("") : hex[1];
  $<HTMLInputElement>("fs-color-pick").value = "#" + h.toLowerCase();
}

export function initFolders() {
  $("fs-color").oninput = syncPicker;
  $("fs-color-pick").oninput = () => ($<HTMLInputElement>("fs-color").value = $<HTMLInputElement>("fs-color-pick").value);
  $("fs-ok").onclick = async (e) => {
    e.preventDefault();
    const v = (id: string) => $<HTMLInputElement>(id).value.trim();
    try {
      await App.SetFolderSettings(editing, {
        title: v("fs-title"),
        description: v("fs-desc"),
        icon: v("fs-icon"),
        color: v("fs-color"),
        background: v("fs-bg"),
        tags: v("fs-tags").split(/[\s,]+/).filter((t) => t !== ""),
      });
      $<HTMLDialogElement>("folder-dialog").close();
      saved();
    } catch (err) {
      $("fs-error").textContent = message(err);
      $("fs-error").hidden = false;
    }
  };
}
