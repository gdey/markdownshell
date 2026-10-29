import { App, type Settings, message } from "./api.js";
import { $ } from "./dom.js";
import { applyTheme } from "./theme.js";

let s: Settings = await App.Settings();

const input = (id: string) => $<HTMLInputElement>(id);

function draw() {
  input("autosave").checked = s.autosave;
  input("vim").checked = s.vim;
  $<HTMLSelectElement>("theme").value = s.theme;
  applyTheme(s.theme);
  input("shell").value = s.shell;
  $<HTMLSelectElement>("start-mode").value = s.startFolder ? "fixed" : "last";
  input("start-folder").value = s.startFolder;
  $("fixed").hidden = !s.startFolder && $<HTMLSelectElement>("start-mode").value !== "fixed";
}

// Changes save as they are made, like a settings window should.
async function save() {
  try {
    await App.SetSettings(s);
    $("error").hidden = true;
  } catch (err) {
    $("error").textContent = message(err);
    $("error").hidden = false;
  }
}

async function choose() {
  const dir = await App.ChooseFolder();
  if (dir) {
    s.startFolder = dir;
    await save();
  }
  draw();
}

input("autosave").onchange = () => ((s.autosave = input("autosave").checked), save());
input("vim").onchange = () => ((s.vim = input("vim").checked), save());
$<HTMLSelectElement>("theme").onchange = async () => {
  s.theme = $<HTMLSelectElement>("theme").value;
  applyTheme(s.theme);
  await save();
};
input("shell").onchange = () => ((s.shell = input("shell").value.trim()), save());
$<HTMLSelectElement>("start-mode").onchange = async () => {
  if ($<HTMLSelectElement>("start-mode").value === "last") {
    s.startFolder = "";
    await save();
    draw();
  } else {
    $("fixed").hidden = false;
    if (!s.startFolder) await choose();
  }
};
$("choose").onclick = choose;

draw();
