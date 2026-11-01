import { App, type Settings, message } from "./api.js";
import { $, el } from "./dom.js";
import { applyTheme } from "./theme.js";
import type { About, Tool } from "./bindings/github.com/gdey/markdownshell/cmd/mdsh-gui/models.js";

let s: Settings = await App.Settings();
let zsh = await App.ZshWrapperState().catch(() => null);

const input = (id: string) => $<HTMLInputElement>(id);
const select = (id: string) => $<HTMLSelectElement>(id);

// The tabs across the top: one section shows at a time.
const tabs = Array.from(document.querySelectorAll<HTMLButtonElement>('[role="tab"]'));
function showTab(name: string) {
  for (const t of tabs) {
    const on = t.dataset.tab === name;
    t.setAttribute("aria-selected", String(on));
    $(t.dataset.tab!).hidden = !on;
  }
  if (name === "about") void drawAbout();
}
for (const t of tabs) t.onclick = () => showTab(t.dataset.tab!);

function draw() {
  input("autosave").checked = s.autosave;
  input("vim").checked = s.vim;
  input("spellcheck").checked = !s.noSpellCheck;
  select("theme").value = s.theme;
  select("look").value = s.look;
  drawLook();
  applyTheme(s);
  select("view").value = s.view;
  input("shell").value = s.shell;
  input("history-rc").value = s.historyRC;
  select("start-mode").value = s.startFolder ? "fixed" : "last";
  input("start-folder").value = s.startFolder;
  input("template-folder").value = s.templateFolder;
  $("clear-templates").hidden = !s.templateFolder;
  $("fixed").hidden = !s.startFolder && select("start-mode").value !== "fixed";
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
  const dir = await App.ChooseFolder("Choose the start folder", s.startFolder);
  if (dir) {
    s.startFolder = dir;
    await save();
  }
  draw();
}

input("autosave").onchange = () => ((s.autosave = input("autosave").checked), save());
input("vim").onchange = () => ((s.vim = input("vim").checked), save());
input("spellcheck").onchange = () => ((s.noSpellCheck = !input("spellcheck").checked), save());
select("theme").onchange = () => lookChange(() => (s.theme = select("theme").value));
select("look").onchange = () => lookChange(() => (s.look = select("look").value));

// The Look tab's options. Each change shows at once in every window.
// fonts are common faces, of which the menus list those installed.
const fonts = {
  ui: ["Inter", "SF Pro Text", "Helvetica Neue", "Helvetica", "Avenir Next", "Segoe UI", "Roboto", "Ubuntu", "Noto Sans", "DejaVu Sans", "Liberation Sans", "IBM Plex Sans", "Source Sans 3", "Georgia", "Charter", "Iowan Old Style"],
  code: ["SF Mono", "Menlo", "Monaco", "JetBrains Mono", "Fira Code", "Fira Mono", "Source Code Pro", "IBM Plex Mono", "Cascadia Code", "Consolas", "Hack", "Inconsolata", "Roboto Mono", "Ubuntu Mono", "DejaVu Sans Mono", "Liberation Mono", "Courier Prime", "Courier New"],
};

// installed says whether a font family is on this computer: text set
// in it measures differently from the same text in each fallback.
function installed(family: string): boolean {
  const ctx = document.createElement("canvas").getContext("2d");
  if (!ctx) return false;
  const text = "mmmmmmmmmmlli10WQ@";
  return ["monospace", "serif", "sans-serif"].some((base) => {
    ctx.font = `32px ${base}`;
    const without = ctx.measureText(text).width;
    ctx.font = `32px "${family}", ${base}`;
    return ctx.measureText(text).width !== without;
  });
}

function fillFonts(id: string, families: string[], chosen: string) {
  const sel = select(id);
  const names = families.filter(installed);
  if (chosen && !names.includes(chosen)) names.push(chosen);
  sel.replaceChildren(el("option", { value: "" }, "The look's own"), ...names.map((n) => el("option", { value: n }, n)));
  sel.value = chosen;
}
fillFonts("ui-font", fonts.ui, s.uiFont);
fillFonts("code-font", fonts.code, s.codeFont);

const presets = ["", "#3fb950", "#d4a017", "#2f81f7", "#d1495b", "#8250df"];

function drawLook() {
  const own = s.look === "tty" ? 115 : 100;
  input("text-size").value = String(s.textSize || own);
  $("text-size-value").textContent = `${s.textSize || own}%${s.textSize ? "" : ", the look's own"}.`;
  $("text-size-reset").hidden = !s.textSize;
  select("ui-font").value = s.uiFont;
  select("code-font").value = s.codeFont;
  select("line-spacing").value = s.lineSpacing;
  const custom = !presets.includes(s.accent);
  select("accent").value = custom ? "custom" : s.accent;
  input("accent-custom").hidden = !custom;
  if (custom) input("accent-custom").value = s.accent;
  select("history-pane").value = s.historyPane;
  $("paper").hidden = s.look !== "tty";
  input("sprockets").checked = !s.noSprockets;
  input("rules").checked = !s.noRules;
  input("shadows").checked = !s.noShadows;
}

// lookChange makes a change to the look, shows it here, and saves it.
async function lookChange(change: () => void) {
  change();
  drawLook();
  applyTheme(s);
  await save();
}

input("text-size").oninput = () => {
  s.textSize = Number(input("text-size").value);
  drawLook();
  applyTheme(s);
};
input("text-size").onchange = () => lookChange(() => (s.textSize = Number(input("text-size").value)));
$("text-size-reset").onclick = () => lookChange(() => (s.textSize = 0));
select("ui-font").onchange = () => lookChange(() => (s.uiFont = select("ui-font").value));
select("code-font").onchange = () => lookChange(() => (s.codeFont = select("code-font").value));
select("line-spacing").onchange = () => lookChange(() => (s.lineSpacing = select("line-spacing").value));
select("accent").onchange = () => {
  const v = select("accent").value;
  if (v !== "custom") return void lookChange(() => (s.accent = v));
  input("accent-custom").hidden = false;
  input("accent-custom").click();
};
input("accent-custom").onchange = () => lookChange(() => (s.accent = input("accent-custom").value));
select("history-pane").onchange = () => lookChange(() => (s.historyPane = select("history-pane").value));
input("sprockets").onchange = () => lookChange(() => (s.noSprockets = !input("sprockets").checked));
input("rules").onchange = () => lookChange(() => (s.noRules = !input("rules").checked));
input("shadows").onchange = () => lookChange(() => (s.noShadows = !input("shadows").checked));
select("view").onchange = () => ((s.view = select("view").value), save());
input("shell").onchange = () => ((s.shell = input("shell").value.trim()), save());
input("history-rc").onchange = () => ((s.historyRC = input("history-rc").value.trim()), save());
select("start-mode").onchange = async () => {
  if (select("start-mode").value === "last") {
    s.startFolder = "";
    await save();
    draw();
  } else {
    $("fixed").hidden = false;
    if (!s.startFolder) await choose();
  }
};
$("choose").onclick = choose;
$("choose-templates").onclick = async () => {
  const dir = await App.ChooseFolder("Choose the templates folder", s.templateFolder);
  if (dir) {
    s.templateFolder = dir;
    await save();
  }
  draw();
};
$("clear-templates").onclick = async () => {
  s.templateFolder = "";
  await save();
  draw();
};
input("zsh-wrapper").onchange = async () => {
  try {
    zsh = await App.SetZshWrapper(input("zsh-wrapper").checked);
    $("error").hidden = true;
  } catch (err) {
    showError(err);
  }
  draw();
};

// The About tab: the version and build, where things are, the programs
// mdsh-gui found, and where to read more. It's gathered when first
// shown, since finding the tools runs them.
let about: About | null = null;

async function drawAbout() {
  if (about) return;
  try {
    about = await App.About();
  } catch (err) {
    showError(err);
    return;
  }
  const a = about;
  $("version").textContent = a.version;
  $("build").textContent = buildLine(a);

  // A row of the info lists: a name, a value, and buttons after it.
  const row = (list: HTMLElement, name: string, value: string, ...extra: Node[]) => {
    list.append(el("dt", {}, name), el("dd", {}, el("span", { className: "value", textContent: value }), ...extra));
  };
  const show = (path: string) => {
    const b = el("button", { textContent: "Show" });
    b.onclick = () => App.Reveal(path).catch(showError);
    return b;
  };

  const paths = $("paths");
  row(paths, "Settings", tilde(a.settingsFile, a.home), show(a.settingsFile));
  if (a.zshrc) row(paths, "Shell startup", tilde(a.zshrc, a.home), show(a.zshrc));
  row(paths, "Start folder", s.startFolder ? tilde(s.startFolder, a.home) : "The last folder used", ...(s.startFolder ? [show(s.startFolder)] : []));
  row(paths, "Templates", s.templateFolder ? tilde(s.templateFolder, a.home) : "None", ...(s.templateFolder ? [show(s.templateFolder)] : []));

  const tools = $("tools");
  for (const t of a.tools ?? []) {
    const dd = el("dd", {});
    if (t.path) {
      dd.append(el("span", { className: "value", textContent: t.path }));
      if (t.version) dd.append(el("span", { className: "muted", textContent: t.version }));
    } else {
      dd.append(el("span", { className: "missing", textContent: "Not found" }));
    }
    dd.append(el("span", { className: "muted note", textContent: t.note }));
    tools.append(el("dt", {}, t.name), dd);
  }

  const links = $("links");
  for (const l of a.links ?? []) {
    const link = el("a", { href: l.url, textContent: l.name });
    link.onclick = (e) => {
      e.preventDefault();
      App.OpenURL(l.url).catch(showError);
    };
    links.append(link);
  }
}

// buildLine says what the build is, under the version.
function buildLine(a: About): string {
  const parts = [];
  if (a.commit) parts.push("commit " + a.commit + (a.modified ? " with uncommitted changes" : ""));
  if (a.buildTime) parts.push(a.buildTime.replace("T", " ").replace(/:\d\d(\.\d+)?Z$/, " UTC"));
  parts.push(`${a.goVersion} ${a.os}/${a.arch}`);
  return parts.join(" · ");
}

// aboutText is the About tab as text, to paste into a bug report.
function aboutText(a: About): string {
  const lines = [`mdsh-gui ${a.version}`];
  if (a.commit) lines.push(`commit: ${a.commit}${a.modified ? " (modified)" : ""}`);
  if (a.buildTime) lines.push(`built from: ${a.buildTime}`);
  lines.push(`go: ${a.goVersion} ${a.os}/${a.arch}`, `settings: ${a.settingsFile}`);
  if (s.shell) lines.push(`shell setting: ${s.shell}`);
  for (const t of a.tools ?? []) lines.push(toolLine(t));
  return lines.join("\n") + "\n";
}

function toolLine(t: Tool): string {
  if (!t.path) return `${t.name}: not found`;
  return `${t.name}: ${t.path}` + (t.version ? ` (${t.version})` : "");
}

$("copy").onclick = async () => {
  if (!about) return;
  try {
    await navigator.clipboard.writeText(aboutText(about));
    $("copy").textContent = "Copied";
    setTimeout(() => ($("copy").textContent = "Copy"), 1500);
  } catch (err) {
    showError(err);
  }
};

draw();
