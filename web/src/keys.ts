// Shortcut hints, written as the platform writes them: modifier symbols
// on macOS (⌘⇧H) and words elsewhere (Ctrl+Shift+H). A shortcut is
// given as CodeMirror names its keys: "Mod-Shift-H", where Mod is cmd
// on macOS and ctrl elsewhere, and Ctrl is ctrl on both.

export const isMac = navigator.platform.startsWith("Mac");

// Apple's order for modifiers, and the order Windows and Linux spell
// them in.
const macOrder = ["Ctrl", "Alt", "Shift", "Cmd"];
const macMods: Record<string, string> = { Ctrl: "⌃", Alt: "⌥", Shift: "⇧", Cmd: "⌘" };
const macKeys: Record<string, string> = {
  Enter: "↩", Escape: "⎋", Backspace: "⌫", Delete: "⌦", Tab: "⇥",
  ArrowUp: "↑", ArrowDown: "↓", ArrowLeft: "←", ArrowRight: "→",
};
const otherOrder = ["Ctrl", "Alt", "Shift"];
const otherKeys: Record<string, string> = {
  Escape: "Esc",
  ArrowUp: "↑", ArrowDown: "↓", ArrowLeft: "←", ArrowRight: "→",
};

// keyHint renders one shortcut for the platform.
export function keyHint(spec: string, mac = isMac): string {
  // The key is what follows the last "-", which may itself be "-".
  const cut = spec.lastIndexOf("-", spec.length - 2);
  const key = spec.slice(cut + 1);
  const mods = new Set(cut < 0 ? [] : spec.slice(0, cut).split("-").map((m) => (m === "Mod" ? (mac ? "Cmd" : "Ctrl") : m)));
  const name = key.length === 1 ? key.toUpperCase() : key;
  if (mac) return macOrder.filter((m) => mods.has(m)).map((m) => macMods[m]).join("") + (macKeys[name] ?? name);
  return [...otherOrder.filter((m) => mods.has(m)), otherKeys[name] ?? name].join("+");
}

// hinted is a label with its shortcut after it: "History (⌘⇧H)".
export function hinted(label: string, spec: string, mac = isMac): string {
  return `${label} (${keyHint(spec, mac)})`;
}

// applyKeyHints adds the shortcut in each element's data-key to its
// title, at "{key}" if the title has one and after it in parentheses if
// not, so the HTML names a shortcut once for every platform.
export function applyKeyHints(root: ParentNode = document, mac = isMac) {
  for (const e of root.querySelectorAll<HTMLElement>("[data-key]")) {
    const hint = keyHint(e.dataset.key!, mac);
    e.title = e.title.includes("{key}") ? e.title.replace("{key}", hint) : `${e.title} (${hint})`;
    delete e.dataset.key;
  }
}
