import { App, type Settings, message } from "./api.js";
import { $ } from "./dom.js";
import { applyTheme } from "./theme.js";

let s: Settings = await App.Settings();
let zsh = await App.ZshWrapperState().catch(() => null);

const input = (id: string) => $<HTMLInputElement>(id);

function draw() {
  input("autosave").checked = s.autosave;
  input("vim").checked = s.vim;
  input("neovim").checked = s.neovim;
  input("neovim").disabled = !s.vim;
  input("neovim-clean").checked = s.neovimClean;
  input("neovim-path").value = s.neovimPath;
  $<HTMLSelectElement>("theme").value = s.theme;
  applyTheme(s.theme);
  $<HTMLSelectElement>("view").value = s.view;
  input("shell").value = s.shell;
  $<HTMLSelectElement>("start-mode").value = s.startFolder ? "fixed" : "last";
  input("start-folder").value = s.startFolder;
  $("fixed").hidden = !s.startFolder && $<HTMLSelectElement>("start-mode").value !== "fixed";
  input("zsh-wrapper").checked = !!zsh?.on;
  input("zsh-wrapper").disabled = !zsh;
  if (zsh) $("zshrc").textContent = tilde(zsh.path, zsh.home);
}

// tilde shows a path under home as ~/….
function tilde(path: string, home: string): string {
  return home && path.startsWith(home + "/") ? "~" + path.slice(home.length) : path;
}

function showError(err: unknown) {
  $("error").textContent = message(err);
  $("error").hidden = false;
}

// Changes save as they are made, like a settings window should.
async function save() {
  try {
    await App.SetSettings(s);
    $("error").hidden = true;
  } catch (err) {
    showError(err);
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
input("vim").onchange = () => ((s.vim = input("vim").checked), save().then(draw));
input("neovim").onchange = () => ((s.neovim = input("neovim").checked), save());
input("neovim-clean").onchange = () => ((s.neovimClean = input("neovim-clean").checked), save());
input("neovim-path").onchange = async () => {
  s.neovimPath = input("neovim-path").value.trim();
  await save();
  await drawNeovim();
};

// drawNeovim says which Neovim the source pane would run, or why none.
async function drawNeovim() {
  const n = await App.Neovim();
  $("neovim-info").textContent = n.error ? n.error : `Neovim ${n.version} at ${n.path}. Needs Vim key bindings on.`;
}

$<HTMLSelectElement>("theme").onchange = async () => {
  s.theme = $<HTMLSelectElement>("theme").value;
  applyTheme(s.theme);
  await save();
};
$<HTMLSelectElement>("view").onchange = () => ((s.view = $<HTMLSelectElement>("view").value), save());
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
input("zsh-wrapper").onchange = async () => {
  try {
    zsh = await App.SetZshWrapper(input("zsh-wrapper").checked);
    $("error").hidden = true;
  } catch (err) {
    showError(err);
  }
  draw();
};

draw();
void drawNeovim();
