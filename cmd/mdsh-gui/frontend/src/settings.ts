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
  select("theme").value = s.theme;
  applyTheme(s.theme);
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
select("theme").onchange = async () => {
  s.theme = select("theme").value;
  applyTheme(s.theme);
  await save();
};
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
