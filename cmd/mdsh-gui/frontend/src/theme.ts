// The editors' look, from the page's color tokens in style.css, so the
// light and dark themes both read well and the Theme setting switches
// the editors with the rest of the window.

import { type Extension } from "@codemirror/state";
import { EditorView } from "@codemirror/view";
import { HighlightStyle, syntaxHighlighting } from "@codemirror/language";
import { tags as t } from "@lezer/highlight";
import type { Settings } from "./api.js";

const highlight = HighlightStyle.define([
  { tag: [t.keyword, t.operatorKeyword, t.modifier, t.controlKeyword], color: "var(--syn-keyword)" },
  { tag: [t.string, t.special(t.string), t.regexp], color: "var(--syn-string)" },
  { tag: [t.number, t.bool, t.atom, t.null], color: "var(--syn-number)" },
  { tag: [t.comment, t.lineComment, t.blockComment], color: "var(--syn-comment)", fontStyle: "italic" },
  { tag: [t.variableName, t.special(t.variableName)], color: "var(--syn-variable)" },
  { tag: [t.function(t.variableName), t.propertyName], color: "var(--syn-function)" },
  { tag: [t.typeName, t.className, t.tagName], color: "var(--syn-type)" },
  { tag: [t.operator, t.punctuation, t.bracket], color: "var(--syn-punct)" },
  { tag: t.heading, color: "var(--syn-heading)", fontWeight: "bold" },
  { tag: t.emphasis, fontStyle: "italic" },
  { tag: t.strong, fontWeight: "bold" },
  { tag: t.strikethrough, textDecoration: "line-through" },
  { tag: [t.link, t.url], color: "var(--syn-link)", textDecoration: "underline" },
  { tag: [t.monospace, t.processingInstruction, t.meta], color: "var(--syn-meta)" },
  { tag: t.quote, color: "var(--syn-comment)" },
  { tag: t.invalid, color: "var(--bad)" },
]);

const look = EditorView.theme({
  "&": { color: "var(--fg)", backgroundColor: "var(--code-bg)" },
  ".cm-content": { caretColor: "var(--fg)" },
  ".cm-cursor, .cm-dropCursor": { borderLeftColor: "var(--fg)" },
  "&.cm-focused > .cm-scroller > .cm-selectionLayer .cm-selectionBackground, .cm-selectionBackground, ::selection": { backgroundColor: "var(--selection)" },
  ".cm-activeLine": { backgroundColor: "var(--active-line)" },
  ".cm-activeLineGutter": { backgroundColor: "var(--active-line)" },
  ".cm-gutters": { backgroundColor: "var(--code-bg)", color: "var(--muted)", borderRightColor: "var(--border)" },
  ".cm-matchingBracket, &.cm-focused .cm-matchingBracket": { backgroundColor: "var(--selection)", outline: "none" },
  ".cm-searchMatch": { backgroundColor: "var(--warn-bg)", outline: "1px solid var(--warn-border)" },
  ".cm-tooltip": { color: "var(--fg)", backgroundColor: "var(--bg)", border: "1px solid var(--border)" },
  ".cm-tooltip-autocomplete > ul > li[aria-selected]": { color: "var(--accent-fg)", backgroundColor: "var(--accent)" },
  ".cm-completionInfo": { color: "var(--fg)", backgroundColor: "var(--bg)", border: "1px solid var(--border)" },
  ".cm-panels": { color: "var(--fg)", backgroundColor: "var(--bg)" },
  ".cm-panels input, .cm-panels button": { color: "var(--fg)" },
  // Vim's command line and fat cursor.
  ".cm-vim-panel input": { color: "var(--fg)" },
  "&:not(.cm-focused) .cm-fat-cursor": { outline: "1px solid var(--fg)" },
  ".cm-fat-cursor": { background: "var(--fg) !important", color: "var(--bg) !important" },
});

// editorLook is the theme every editor uses.
export const editorLook: Extension = [look, syntaxHighlighting(highlight)];

// The settings a page's look comes from: the theme, the look, and the
// look's options from Settings › Look.
export type LookSettings = Pick<
  Settings,
  "theme" | "look" | "textSize" | "uiFont" | "codeFont" | "lineSpacing" | "noSprockets" | "noRules" | "noShadows" | "accent" | "historyPane"
>;

let current: LookSettings | null = null;
const systemDark = window.matchMedia("(prefers-color-scheme: dark)");
systemDark.addEventListener("change", () => current && applyTheme(current));

// applyTheme sets the page's theme and look from s, on the root element:
// data-theme ("light", "dark" or none to follow the system), data-scheme
// (the light or dark the page shows), data-look ("tty" for Teletype,
// none for plain), data-history-scheme for the History pane, the texture
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
  const tty = s.look === "tty";
  set("look", tty, "tty");
  const pane = s.historyPane || (tty ? "match" : "dark");
  root.dataset.historyScheme = pane === "match" ? scheme : pane === "invert" ? (scheme === "dark" ? "light" : "dark") : "dark";
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
  prop("--accent-fg", accent ? (luminance(accent) > 0.45 ? "#000000" : "#ffffff") : "");
  window.dispatchEvent(new Event("mdsh-look"));
}

const fallbackUI = '-apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif';
const fallbackCode = "ui-monospace, SFMono-Regular, Menlo, Consolas, monospace";

// luminance is a #rrggbb color's relative luminance, 0 to 1.
function luminance(hex: string): number {
  const c = [1, 3, 5].map((i) => {
    const v = parseInt(hex.slice(i, i + 2), 16) / 255;
    return v <= 0.03928 ? v / 12.92 : ((v + 0.055) / 1.055) ** 2.4;
  });
  return 0.2126 * c[0] + 0.7152 * c[1] + 0.0722 * c[2];
}

// codeFont is the page's code font, from --font-code, for the terminals,
// which draw on a canvas and so can't use the variable themselves.
export function codeFont(el: Element = document.documentElement): string {
  return getComputedStyle(el).getPropertyValue("--font-code").trim() || "ui-monospace, SFMono-Regular, Menlo, Consolas, monospace";
}

// textSize is a font size in px scaled by the look's --text-scale, for
// the terminals, which take a number rather than CSS.
export function textSize(px: number, el: Element = document.documentElement): number {
  const scale = parseFloat(getComputedStyle(el).getPropertyValue("--text-scale"));
  return Math.round(px * (scale > 0 ? scale : 1));
}
