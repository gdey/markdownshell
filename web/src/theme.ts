// A page's theme and look, from Settings. The editors' own look is in
// editorlook.ts, so the pages without an editor don't load CodeMirror.

import type { Settings } from "./api.js";
import { lookFor } from "./looks.js";

// The settings a page's look comes from: the theme, the look, and the
// look's options from Settings › Look.
export type LookSettings = Pick<
  Settings,
  "theme" | "look" | "textSize" | "uiFont" | "codeFont" | "lineSpacing" | "noSprockets" | "noRules" | "noShadows" | "accent" | "historyPane" | "historyFocus"
>;

let current: LookSettings | null = null;
const systemDark = window.matchMedia("(prefers-color-scheme: dark)");
systemDark.addEventListener("change", () => current && applyTheme(current));

// applyTheme sets the page's theme and look from s, on the root element:
// data-theme ("light", "dark" or none to follow the system), data-scheme
// (the light or dark the page shows), data-look (the look's id, from
// web/looks; none for Plain), data-history-scheme for the History pane,
// data-history-focus ("cursor" or none) for how its prompt shows focus, the texture
// switches, and the options as variables. Then it fires "mdsh-look" on
// window, for what draws on a canvas, such as the terminals.
export function applyTheme(s: LookSettings) {
  current = s;
  const root = document.documentElement;
  const set = (name: string, on: boolean, value = "") => (on ? (root.dataset[name] = value) : delete root.dataset[name]);
  const theme = s.theme === "light" || s.theme === "dark" ? s.theme : "";
  set("theme", !!theme, theme);
  const scheme = theme || (systemDark.matches ? "dark" : "light");
  root.dataset.scheme = scheme;
  const look = lookFor(s.look).id;
  set("look", !!look, look);
  const pane = s.historyPane || (look ? "match" : "dark");
  root.dataset.historyScheme = pane === "match" ? scheme : pane === "invert" ? (scheme === "dark" ? "light" : "dark") : "dark";
  set("historyFocus", s.historyFocus === "cursor", "cursor");
  set("noSprockets", s.noSprockets);
  set("noRules", s.noRules);
  set("noShadows", s.noShadows);

  const prop = (name: string, value: string) => (value ? root.style.setProperty(name, value) : root.style.removeProperty(name));
  prop("--text-scale", s.textSize ? String(s.textSize / 100) : "");
  prop("--font-ui", s.uiFont ? `"${s.uiFont}", ${fallbackUI}` : "");
  prop("--font-code", s.codeFont ? `"${s.codeFont}", ${fallbackCode}` : "");
  prop("--line-scale", s.lineSpacing === "compact" ? "0.87" : s.lineSpacing === "relaxed" ? "1.2" : "");
  const accent = /^#[0-9a-f]{6}$/i.test(s.accent) ? s.accent : "";
  prop("--accent", accent);
  prop("--link", accent);
  // Text on the accent is black or white, whichever reads better: at
  // a luminance of 0.179 both are 4.5:1, the WCAG AA ratio, so above
  // it black does and below it white does.
  prop("--accent-fg", accent ? (luminance(accent) > 0.179 ? "#000000" : "#ffffff") : "");
  window.dispatchEvent(new Event("mdsh-look"));
}

const fallbackUI = '-apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif';
const fallbackCode = "ui-monospace, SFMono-Regular, Menlo, Consolas, monospace";

// nerdSymbols are the Nerd Fonts of icons alone. A shell prompt drawn
// with a Nerd Font's icons shows them in the terminals, whatever the
// code font, where one of these is installed.
export const nerdSymbols = '"Symbols Nerd Font Mono", "Symbols Nerd Font"';

// luminance is a #rrggbb color's relative luminance, 0 to 1.
function luminance(hex: string): number {
  const c = [1, 3, 5].map((i) => {
    const v = parseInt(hex.slice(i, i + 2), 16) / 255;
    return v <= 0.03928 ? v / 12.92 : ((v + 0.055) / 1.055) ** 2.4;
  });
  return 0.2126 * c[0] + 0.7152 * c[1] + 0.0722 * c[2];
}

// codeFont is the page's code font, from --font-code, for the terminals,
// which draw on a canvas and so can't use the variable themselves; then
// the Nerd Font icons, for a prompt that uses them.
export function codeFont(el: Element = document.documentElement): string {
  return `${getComputedStyle(el).getPropertyValue("--font-code").trim() || fallbackCode}, ${nerdSymbols}`;
}

// textSize is a font size in px scaled by the look's --text-scale, for
// the terminals, which take a number rather than CSS.
export function textSize(px: number, el: Element = document.documentElement): number {
  const scale = parseFloat(getComputedStyle(el).getPropertyValue("--text-scale"));
  return Math.round(px * (scale > 0 ? scale : 1));
}
