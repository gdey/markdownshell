// Syntax highlighting outside the editors, for the rendered view's block
// cards and the code blocks in its prose, with the parsers the source
// pane uses, and ANSI colors in output.

import { LanguageDescription, LanguageSupport, StreamLanguage, type StreamParser } from "@codemirror/language";
import { classHighlighter, highlightCode } from "@lezer/highlight";
import { shell } from "@codemirror/legacy-modes/mode/shell";
import { go } from "@codemirror/legacy-modes/mode/go";
import { python } from "@codemirror/legacy-modes/mode/python";
import { javascript, json, typescript } from "@codemirror/legacy-modes/mode/javascript";
import { yaml } from "@codemirror/legacy-modes/mode/yaml";
import { toml } from "@codemirror/legacy-modes/mode/toml";
import { rust } from "@codemirror/legacy-modes/mode/rust";
import { ruby } from "@codemirror/legacy-modes/mode/ruby";
import { lua } from "@codemirror/legacy-modes/mode/lua";
import { perl } from "@codemirror/legacy-modes/mode/perl";
import { swift } from "@codemirror/legacy-modes/mode/swift";
import { standardSQL } from "@codemirror/legacy-modes/mode/sql";
import { diff } from "@codemirror/legacy-modes/mode/diff";
import { dockerFile } from "@codemirror/legacy-modes/mode/dockerfile";
import { properties } from "@codemirror/legacy-modes/mode/properties";
import { css } from "@codemirror/legacy-modes/mode/css";
import { c, cpp, java, kotlin } from "@codemirror/legacy-modes/mode/clike";

import { el } from "./dom.js";

// modes are the languages code is highlighted in: a parser, the fence's
// language and its other names.
const modes: [StreamParser<unknown>, string, string[]][] = [
  [shell as StreamParser<unknown>, "sh", ["zsh", "bash", "shell", "console"]],
  [go as StreamParser<unknown>, "go", ["golang"]],
  [python as StreamParser<unknown>, "python", ["py", "python3"]],
  [javascript as StreamParser<unknown>, "javascript", ["js", "jsx", "mjs", "cjs"]],
  [typescript as StreamParser<unknown>, "typescript", ["ts", "tsx"]],
  [json as StreamParser<unknown>, "json", ["jsonc", "json5"]],
  [yaml as StreamParser<unknown>, "yaml", ["yml"]],
  [toml as StreamParser<unknown>, "toml", []],
  [rust as StreamParser<unknown>, "rust", ["rs"]],
  [ruby as StreamParser<unknown>, "ruby", ["rb"]],
  [lua as StreamParser<unknown>, "lua", []],
  [perl as StreamParser<unknown>, "perl", ["pl"]],
  [swift as StreamParser<unknown>, "swift", []],
  [standardSQL as StreamParser<unknown>, "sql", []],
  [diff as StreamParser<unknown>, "diff", ["patch"]],
  [dockerFile as StreamParser<unknown>, "dockerfile", ["docker"]],
  [properties as StreamParser<unknown>, "ini", ["properties", "cfg", "conf"]],
  [css as StreamParser<unknown>, "css", []],
  [c as StreamParser<unknown>, "c", ["h"]],
  [cpp as StreamParser<unknown>, "cpp", ["c++", "cc", "cxx", "hpp"]],
  [java as StreamParser<unknown>, "java", []],
  [kotlin as StreamParser<unknown>, "kotlin", ["kt"]],
];

// codeLanguages are the languages of fenced code, for the markdown
// editors and the rendered view.
export const codeLanguages = modes.map(([parser, name, alias]) => LanguageDescription.of({ name, alias, support: new LanguageSupport(StreamLanguage.define(parser)) }));

function languageOf(lang: string) {
  return lang ? LanguageDescription.matchLanguageName(codeLanguages, lang, false)?.support?.language : undefined;
}

// codeLines draws code in lang as one .line element per line, colored
// with the tok-* classes style.css colors. Code in a language it doesn't
// know is drawn plain.
export function codeLines(code: string, lang: string): HTMLElement[] {
  const text = code.endsWith("\n") ? code.slice(0, -1) : code;
  const lines: HTMLElement[] = [];
  let line = el("span", { className: "line" });
  const put = (t: string, cls: string) => line.append(cls ? el("span", { className: cls }, t) : t);
  const br = () => {
    lines.push(line);
    line = el("span", { className: "line" });
  };
  const language = languageOf(lang);
  if (language) highlightCode(text, language.parser.parse(text), classHighlighter, put, br);
  else
    text.split("\n").forEach((l, i) => {
      if (i) br();
      put(l, "");
    });
  lines.push(line);
  return lines;
}

// highlightProse highlights the fenced code goldmark drew in root,
// which marks its language with a language-* class. Output is colored
// for ANSI escapes instead.
export function highlightProse(root: HTMLElement) {
  for (const code of root.querySelectorAll<HTMLElement>("pre > code")) {
    const lang = /(?:^|\s)language-(\S+)/.exec(code.className)?.[1] ?? "";
    if (lang === "output") {
      colorOutput(code);
      continue;
    }
    code.replaceChildren(...codeLines(code.textContent ?? "", lang));
    code.parentElement!.classList.add("numbered");
  }
}

// colorOutput draws the ANSI colors in the output in root.
export function colorOutput(root: HTMLElement) {
  const codes = root.matches("code") ? [root] : root.querySelectorAll<HTMLElement>("pre > code");
  for (const code of codes) {
    const text = code.textContent ?? "";
    if (text.includes("\x1b")) code.replaceChildren(...new Ansi().write(text));
  }
}

// escape matches an escape sequence: CSI, OSC, a character set choice
// or a two-character one.
const escape = /\x1b(?:\[([0-9;:?<=>]*)([ -/]*)([@-~])|\][^\x07\x1b]*(?:\x07|\x1b\\)|[()*+][0-9A-Za-z]|[@-Z\\-_])/g;
// unfinished matches an escape sequence cut off at the end of the text.
const unfinished = /\x1b(?:\[[0-9;:?<=>]*[ -/]*|\][^\x07\x1b]*\x1b?|[()*+])?$/;

// Ansi turns text with ANSI escapes into nodes, colored as SGR
// sequences say, leaving out the other sequences. It keeps its state
// between writes, so output can come in pieces.
export class Ansi {
  private fg = "";
  private bg = "";
  private bold = false;
  private dim = false;
  private italic = false;
  private underline = false;
  private inverse = false;
  private rest = "";

  write(text: string): Node[] {
    text = this.rest + text;
    this.rest = "";
    const cut = unfinished.exec(text);
    if (cut) {
      this.rest = cut[0];
      text = text.slice(0, cut.index);
    }
    const out: Node[] = [];
    let last = 0;
    for (const m of text.matchAll(escape)) {
      this.put(out, text.slice(last, m.index));
      if (m[3] === "m" && !m[2] && !/[?<=>]/.test(m[1] ?? "")) this.sgr(m[1] ?? "");
      last = m.index! + m[0].length;
    }
    this.put(out, text.slice(last));
    return out;
  }

  private put(out: Node[], text: string) {
    if (!text) return;
    let fg = this.fg;
    let bg = this.bg;
    if (this.inverse) [fg, bg] = [bg || "var(--code-bg)", fg || "var(--fg)"];
    const cls = [this.bold && "ansi-bold", this.dim && "ansi-dim", this.italic && "ansi-italic", this.underline && "ansi-underline"].filter(Boolean).join(" ");
    if (!fg && !bg && !cls) return void out.push(document.createTextNode(text));
    const span = el("span", { className: cls }, text);
    if (fg) span.style.color = fg;
    if (bg) span.style.backgroundColor = bg;
    out.push(span);
  }

  private sgr(params: string) {
    const p = params.split(/[;:]/).map((s) => (s === "" ? 0 : Number(s)));
    for (let i = 0; i < p.length; i++) {
      const n = p[i];
      if (n === 0) {
        this.fg = this.bg = "";
        this.bold = this.dim = this.italic = this.underline = this.inverse = false;
      } else if (n === 1) this.bold = true;
      else if (n === 2) this.dim = true;
      else if (n === 3) this.italic = true;
      else if (n === 4) this.underline = true;
      else if (n === 7) this.inverse = true;
      else if (n === 21 || n === 22) this.bold = this.dim = false;
      else if (n === 23) this.italic = false;
      else if (n === 24) this.underline = false;
      else if (n === 27) this.inverse = false;
      else if (n >= 30 && n <= 37) this.fg = color(n - 30);
      else if (n >= 90 && n <= 97) this.fg = color(n - 90 + 8);
      else if (n === 39) this.fg = "";
      else if (n >= 40 && n <= 47) this.bg = color(n - 40);
      else if (n >= 100 && n <= 107) this.bg = color(n - 100 + 8);
      else if (n === 49) this.bg = "";
      else if (n === 38 || n === 48) {
        let c = "";
        if (p[i + 1] === 5) {
          c = color(p[i + 2] ?? 0);
          i += 2;
        } else if (p[i + 1] === 2) {
          c = `rgb(${p[i + 2] ?? 0} ${p[i + 3] ?? 0} ${p[i + 4] ?? 0})`;
          i += 4;
        }
        if (n === 38) this.fg = c;
        else this.bg = c;
      }
    }
  }
}

// color is the CSS color of xterm's color n: the 16 named ones follow
// the theme, from style.css.
function color(n: number): string {
  if (n < 16) return `var(--ansi-${n})`;
  if (n < 232) {
    const level = (v: number) => (v ? 55 + v * 40 : 0);
    n -= 16;
    return `rgb(${level(Math.floor(n / 36))} ${level(Math.floor(n / 6) % 6)} ${level(n % 6)})`;
  }
  const g = 8 + (n - 232) * 10;
  return `rgb(${g} ${g} ${g})`;
}
