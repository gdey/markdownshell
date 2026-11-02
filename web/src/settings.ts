import { App, type Settings, message } from "./api.js";
import { $, el } from "./dom.js";
import { applyTheme } from "./theme.js";
import { Events } from "/wails/runtime.js";
import type { About, CLI, Tool } from "./bindings/github.com/gdey/markdownshell/cmd/mdsh-gui/models.js";

let s: Settings = await App.Settings();
let zsh = await App.ZshWrapperState().catch(() => null);
let cli: CLI | null = await App.CLIState().catch(() => null);

const input = (id: string) => $<HTMLInputElement>(id);
const select = (id: string) => $<HTMLSelectElement>(id);

// The tabs across the top: one section shows at a time.
const tabs = Array.from(document.querySelectorAll<HTMLButtonElement>('[role="tab"]'));
function showTab(name: string, focus = false) {
  for (const t of tabs) {
    const on = t.dataset.tab === name;
    t.setAttribute("aria-selected", String(on));
    t.tabIndex = on ? 0 : -1;
    $(t.dataset.tab!).hidden = !on;
    if (on && focus) t.focus();
  }
  if (name === "about") void drawAbout();
}
tabs.forEach((t, i) => {
  t.id = "tab-" + t.dataset.tab;
  t.setAttribute("aria-controls", t.dataset.tab!);
  $(t.dataset.tab!).setAttribute("aria-labelledby", t.id);
  t.onclick = () => showTab(t.dataset.tab!);
  // The arrow keys move between the tabs, as in a native tab bar.
  t.onkeydown = (e) => {
    const n = tabs.length;
    const to = e.key === "ArrowLeft" ? tabs[(i + n - 1) % n] : e.key === "ArrowRight" ? tabs[(i + 1) % n] : e.key === "Home" ? tabs[0] : e.key === "End" ? tabs[n - 1] : null;
    if (!to) return;
    e.preventDefault();
    showTab(to.dataset.tab!, true);
  };
});
// The window opens at the tab its address names, such as #about from
// the app menu's About item, and switches when asked while open.
const named = (name: string) => tabs.some((t) => t.dataset.tab === name);
const first = location.hash.slice(1);
showTab(named(first) ? first : "general");
Events.On("settings-tab", (ev: { data: string }) => {
  if (named(ev.data)) showTab(ev.data);
});

function draw() {
  input("autosave").checked = s.autosave;
  // A time set by hand in the file that the menu doesn't list shows as
  // the default.
  select("secret-show").value = String(s.secretShowSeconds || 0);
  if (select("secret-show").selectedIndex < 0) select("secret-show").value = "0";
  input("vim").checked = s.vim;
  input("spellcheck").checked = !s.noSpellCheck;
  input("neovim").checked = s.neovim;
  input("neovim").disabled = !s.vim;
  input("neovim-clean").checked = s.neovimClean;
  input("neovim-colors").checked = !s.neovimOwnColors;
  input("neovim-path").value = s.neovimPath;
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
  input("session-folder").value = s.sessionFolder;
  $("reset-sessions").hidden = !s.sessionFolder;
  input("session-template").value = s.sessionTemplate;
  $("clear-session-template").hidden = !s.sessionTemplate;
  $("fixed").hidden = !s.startFolder && select("start-mode").value !== "fixed";
  input("zsh-wrapper").checked = !!zsh?.on;
  input("zsh-wrapper").disabled = !zsh;
  if (zsh) $("zshrc").textContent = tilde(zsh.path, zsh.home);
  $("bashrc-note").hidden = !zsh?.bashPath;
  if (zsh?.bashPath) $("bashrc").textContent = tilde(zsh.bashPath, zsh.home);
  drawCLI();
}

// drawCLI shows where the mdsh this app carries stands: on PATH, not,
// or behind another mdsh. Builds without one show no row.
function drawCLI() {
  const c = cli;
  $("cli-row").hidden = !c?.bundled;
  if (!c?.bundled) return;
  const link = tilde(c.link, c.home);
  const status = $("cli-status");
  const code = (t: string) => el("code", { textContent: t });
  let install = true;
  let remove = false;
  if (c.linked) {
    status.replaceChildren(code(tilde(c.linked, c.home)), " runs this app's mdsh.");
    install = false;
  } else if (c.other) {
    status.replaceChildren("Another mdsh is on PATH at ", code(tilde(c.other, c.home)), ". Remove it to use this app's.");
    install = false;
  } else if (c.broken) {
    status.replaceChildren(code(link), " is broken: it points at ", code(c.broken), ". Install points it here.");
  } else if (c.installed && !c.inPath) {
    status.replaceChildren("Installed at ", code(link), ", but that folder isn't on PATH. Add ", code(`export PATH="${tilde(c.link, c.home).replace(/\/mdsh$/, "").replace(/^~/, "$HOME")}:$PATH"`), " to your shell's startup file.");
    install = false;
    remove = true;
  } else if (c.installed) {
    status.replaceChildren("Installed at ", code(link), c.copy ? ", a copy from this AppImage, kept at the app's version." : ", a link to this app, so it updates with the app.");
    install = false;
    remove = true;
  } else {
    status.replaceChildren("Not on PATH. Install ", c.copy ? "copies mdsh to " : "links ", code(link), c.copy ? "." : " to the one inside this app.");
  }
  $("cli-install").hidden = !install;
  $("cli-remove").hidden = !remove;
}

// cliOnPath is whether a terminal would find an mdsh, this app's or
// another, or there's no bundled one to offer.
const cliOnPath = () => !cli?.bundled || cli.installed || !!cli.linked || !!cli.other;

async function setCLI(install: boolean): Promise<boolean> {
  try {
    cli = await (install ? App.InstallCLI() : App.UninstallCLI());
    $("error").hidden = true;
    return true;
  } catch (err) {
    if (message(err) !== "canceled") showError(err);
    cli = await App.CLIState().catch(() => cli);
    return false;
  } finally {
    draw();
  }
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
select("secret-show").onchange = () => ((s.secretShowSeconds = Number(select("secret-show").value)), save());
input("vim").onchange = () => ((s.vim = input("vim").checked), save().then(draw));
input("spellcheck").onchange = () => ((s.noSpellCheck = !input("spellcheck").checked), save());
input("neovim").onchange = () => ((s.neovim = input("neovim").checked), save());
input("neovim-clean").onchange = () => ((s.neovimClean = input("neovim-clean").checked), save());
input("neovim-colors").onchange = () => ((s.neovimOwnColors = !input("neovim-colors").checked), save());
input("neovim-path").onchange = async () => {
  s.neovimPath = input("neovim-path").value.trim();
  await save();
  await drawNeovim();
};

// drawNeovim says which Neovim the source pane would run, or why none.
async function drawNeovim() {
  const n = await App.Neovim();
  $("neovim-info").textContent = n.error ? n.error : `Found Neovim ${n.version} at ${n.path}.`;
}

select("theme").onchange = () => lookChange(() => (s.theme = select("theme").value));
select("look").onchange = () => lookChange(() => (s.look = select("look").value));

// The Look tab's options. Each change shows at once in every window.
// The font menus list the families installed on this computer, the
// code font's those of one width. Where the system can't say, they list
// those of these common faces that are installed.
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
  const names = [...families];
  if (chosen && !names.includes(chosen)) names.push(chosen);
  sel.replaceChildren(el("option", { value: "" }, "The look's own"), ...names.map((n) => el("option", { value: n }, n)));
  sel.value = chosen;
}
const system = await App.Fonts().catch(() => null);
fillFonts("ui-font", system?.all?.length ? system.all : fonts.ui.filter(installed), s.uiFont);
fillFonts("code-font", system?.mono?.length ? system.mono : fonts.code.filter(installed), s.codeFont);

const presets = ["", "#3fb950", "#d4a017", "#2f81f7", "#d1495b", "#8250df"];

function drawLook() {
  const own = s.look === "tty" ? 115 : 100;
  input("text-size").value = String(s.textSize || own);
  $("text-size-value").textContent = `${s.textSize || own}%${s.textSize ? "" : ", the look's own"}.`;
  input("text-size").setAttribute("aria-valuetext", `${s.textSize || own} percent${s.textSize ? "" : ", the look's own"}`);
  $("text-size-reset").hidden = !s.textSize;
  select("ui-font").value = s.uiFont;
  select("code-font").value = s.codeFont;
  select("line-spacing").value = s.lineSpacing;
  const custom = !presets.includes(s.accent);
  select("accent").value = custom ? "custom" : s.accent;
  input("accent-custom").hidden = !custom;
  if (custom) input("accent-custom").value = s.accent;
  select("history-pane").value = s.historyPane;
  select("history-focus").value = s.historyFocus;
  // Teletype's paper has sprockets, rules and shadows; Blueprint's
  // sheet only its grid; Bubble Gum's wrapper has bubbles and shadows;
  // Witchcraft's grimoire has its night sky and the glow around cards.
  const tty = s.look === "tty";
  const gum = s.look === "gum";
  const witch = s.look === "witch";
  $("paper").hidden = !tty && !gum && !witch && s.look !== "blueprint";
  $("paper-label").textContent = tty ? "Teletype paper" : gum ? "Bubble gum wrapper" : witch ? "Grimoire" : "Blueprint sheet";
  $("sprockets-check").hidden = !tty;
  $("shadows-check").hidden = !tty && !gum && !witch;
  $("shadows-label").textContent = witch ? "Glow around cards" : "Shadows under cards";
  $("rules-label").textContent = tty ? "Ruled lines" : gum ? "Bubbles" : witch ? "Stars and moons" : "Grid";
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
select("history-focus").onchange = () => lookChange(() => (s.historyFocus = select("history-focus").value));
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
$("choose-sessions").onclick = async () => {
  const dir = await App.ChooseFolder("Choose the sessions folder", s.sessionFolder);
  if (dir) {
    s.sessionFolder = dir;
    await save();
  }
  draw();
};
$("reset-sessions").onclick = async () => {
  s.sessionFolder = "";
  await save();
  draw();
};
$("choose-session-template").onclick = async () => {
  const file = await App.ChooseFile("Choose the session template", s.sessionTemplate.replace(/[\\/][^\\/]*$/, ""));
  if (file) {
    s.sessionTemplate = file;
    await save();
  }
  draw();
};
$("clear-session-template").onclick = async () => {
  s.sessionTemplate = "";
  await save();
  draw();
};
$("cli-install").onclick = () => setCLI(true);
$("cli-remove").onclick = () => setCLI(false);
// The app menu's item changes it too.
Events.On("cli", (ev: { data: CLI }) => {
  cli = ev.data;
  draw();
});

input("zsh-wrapper").onchange = async () => {
  // A function that calls an mdsh that isn't there would only fail:
  // offer the one in the app first.
  if (input("zsh-wrapper").checked && !cliOnPath()) {
    input("zsh-wrapper").checked = false;
    $("wrapper-needs-cli").hidden = false;
    $("wrapper-install").focus();
    return;
  }
  try {
    zsh = await App.SetZshWrapper(input("zsh-wrapper").checked);
    $("error").hidden = true;
  } catch (err) {
    showError(err);
  }
  draw();
};

$("wrapper-cancel").onclick = () => {
  $("wrapper-needs-cli").hidden = true;
  input("zsh-wrapper").focus();
};
$("wrapper-install").onclick = async () => {
  $("wrapper-needs-cli").hidden = true;
  if (!(await setCLI(true))) return;
  input("zsh-wrapper").checked = true;
  input("zsh-wrapper").dispatchEvent(new Event("change"));
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
  row(paths, "Sessions", s.sessionFolder ? tilde(s.sessionFolder, a.home) : "~/.mdsh/sessions");

  const tools = $("tools");
  for (const t of a.tools ?? []) {
    const dd = el("dd", {});
    if (t.path) {
      dd.append(el("span", { className: "value", textContent: t.path }));
      if (t.bundled) dd.append(el("span", { className: "missing", textContent: "bundled, not on PATH" }));
      if (t.version) dd.append(el("span", { className: "muted", textContent: t.version }));
    } else {
      dd.append(el("span", { className: "missing", textContent: "Not found" }));
    }
    dd.append(el("span", { className: "muted note", textContent: t.note }));
    tools.append(el("dt", {}, t.name), dd);
  }

  // The changelog is built in: it opens in a read-only tab of the
  // document window, not on the website.
  const links = $("links");
  const changes = el("a", { href: "#", textContent: "Changelog" });
  changes.onclick = (e) => {
    e.preventDefault();
    App.ShowChangelog().catch(showError);
  };
  links.append(changes);
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
void drawNeovim();
