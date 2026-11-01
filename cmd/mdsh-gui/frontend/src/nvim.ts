// The source pane as an embedded Neovim (neovim.go): a canvas drawing
// Neovim's grid from its ext_linegrid redraw events, and keys, mouse and
// paste going back. A hidden textarea has the focus, so plain typing and
// input methods arrive as text and everything else as keys.

import { App } from "./api.js";

interface Cell {
  text: string;
  hl: number;
}

interface Attr {
  foreground?: number;
  background?: number;
  special?: number;
  reverse?: boolean;
  bold?: boolean;
  italic?: boolean;
  underline?: boolean;
  undercurl?: boolean;
  strikethrough?: boolean;
}

interface Mode {
  name?: string;
  cursor_shape?: "block" | "horizontal" | "vertical";
  cell_percentage?: number;
}

const font = "13px ui-monospace, SFMono-Regular, Menlo, monospace";
const isMac = navigator.platform.startsWith("Mac");

export class NvimPane {
  private canvas: HTMLCanvasElement;
  private input: HTMLTextAreaElement;
  private ctx: CanvasRenderingContext2D;
  private cw = 8; // cell width and height in CSS pixels
  private ch = 17;
  private rows: Cell[][] = [];
  private dirty = new Set<number>();
  private attrs = new Map<number, Attr>();
  private fg = 0x000000;
  private bg = 0xffffff;
  private sp = 0xff0000;
  private cursor = { row: 0, col: 0 };
  private modes: Mode[] = [];
  private mode: Mode = {};
  private composing = false;
  private resizeTimer = 0;
  private observer: ResizeObserver;
  private wheel = 0;

  // onKey sees each key first; true takes it from Neovim, as for the
  // app's own keys like cmd/ctrl+enter.
  constructor(
    private parent: HTMLElement,
    private onKey: (e: KeyboardEvent) => boolean = () => false,
  ) {
    this.canvas = document.createElement("canvas");
    this.canvas.className = "nvim-grid";
    this.input = document.createElement("textarea");
    this.input.className = "nvim-input";
    this.input.setAttribute("autocapitalize", "off");
    this.input.spellcheck = false;
    parent.append(this.canvas, this.input);
    this.ctx = this.canvas.getContext("2d")!;
    this.measure();
    this.listen();
    this.observer = new ResizeObserver(() => {
      clearTimeout(this.resizeTimer);
      this.resizeTimer = window.setTimeout(() => {
        const { cols, rows } = this.size();
        void App.NvimResize(cols, rows).catch(() => {});
      }, 50);
    });
    this.observer.observe(parent);
  }

  // start starts the tab's Neovim at the pane's size and returns the
  // text it started with.
  async start(): Promise<string> {
    const { cols, rows } = this.size();
    const theme = document.documentElement.dataset.theme;
    const dark = theme ? theme === "dark" : matchMedia("(prefers-color-scheme: dark)").matches;
    const text = await App.NvimStart(cols, rows, dark ? "dark" : "light");
    this.focus();
    return text;
  }

  focus() {
    this.input.focus({ preventScroll: true });
  }

  // rowHeight is the height of a grid row, in CSS pixels.
  get rowHeight(): number {
    return this.ch;
  }

  destroy() {
    this.observer.disconnect();
    clearTimeout(this.resizeTimer);
    this.canvas.remove();
    this.input.remove();
  }

  private measure() {
    this.ctx.font = font;
    this.cw = this.ctx.measureText("M").width;
    this.ch = Math.ceil(13 * 1.35);
  }

  private size() {
    const r = this.parent.getBoundingClientRect();
    return { cols: Math.max(1, Math.floor(r.width / this.cw)), rows: Math.max(1, Math.floor(r.height / this.ch)) };
  }

  // redraw applies the events up to one flush and draws what changed.
  redraw(events: unknown[][]) {
    const oldCursor = this.cursor.row;
    for (const [name, ...calls] of events) {
      for (const args of calls as unknown[][]) this.event(name as string, args);
    }
    this.dirty.add(oldCursor);
    this.dirty.add(this.cursor.row);
    this.paint();
  }

  private event(name: string, a: unknown[]) {
    switch (name) {
      case "grid_resize":
        this.resize(a[1] as number, a[2] as number);
        break;
      case "default_colors_set":
        if ((a[0] as number) >= 0) this.fg = a[0] as number;
        if ((a[1] as number) >= 0) this.bg = a[1] as number;
        if ((a[2] as number) >= 0) this.sp = a[2] as number;
        this.parent.style.background = hex(this.bg);
        this.rows.forEach((_, i) => this.dirty.add(i));
        break;
      case "hl_attr_define":
        this.attrs.set(a[0] as number, a[1] as Attr);
        break;
      case "grid_clear":
        for (const row of this.rows) row.fill({ text: " ", hl: 0 });
        this.rows.forEach((_, i) => this.dirty.add(i));
        break;
      case "grid_cursor_goto":
        this.cursor = { row: a[1] as number, col: a[2] as number };
        break;
      case "grid_line":
        this.line(a[1] as number, a[2] as number, a[3] as unknown[][]);
        break;
      case "grid_scroll":
        this.scroll(a[1] as number, a[2] as number, a[3] as number, a[4] as number, a[5] as number);
        break;
      case "mode_info_set":
        this.modes = a[1] as Mode[];
        break;
      case "mode_change":
        this.mode = this.modes[a[1] as number] ?? {};
        break;
    }
  }

  private resize(cols: number, rows: number) {
    const blank = () => Array.from({ length: cols }, () => ({ text: " ", hl: 0 }));
    this.rows = Array.from({ length: rows }, (_, i) => {
      const old = this.rows[i] ?? [];
      return blank().map((c, j) => old[j] ?? c);
    });
    const dpr = window.devicePixelRatio || 1;
    this.canvas.style.width = cols * this.cw + "px";
    this.canvas.style.height = rows * this.ch + "px";
    this.canvas.width = Math.ceil(cols * this.cw * dpr);
    this.canvas.height = Math.ceil(rows * this.ch * dpr);
    this.ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
    this.rows.forEach((_, i) => this.dirty.add(i));
  }

  private line(row: number, col: number, cells: unknown[][]) {
    const r = this.rows[row];
    if (!r) return;
    let hl = 0;
    for (const c of cells) {
      const text = c[0] as string;
      if (c.length > 1) hl = c[1] as number;
      const repeat = c.length > 2 ? (c[2] as number) : 1;
      for (let i = 0; i < repeat && col < r.length; i++) r[col++] = { text, hl };
    }
    this.dirty.add(row);
  }

  // scroll moves the rows from top to bot, between columns left and
  // right, up by n (or down when n is negative).
  private scroll(top: number, bot: number, left: number, right: number, n: number) {
    const move = (to: number, from: number) => {
      for (let c = left; c < right; c++) this.rows[to][c] = this.rows[from][c];
      this.dirty.add(to);
    };
    if (n > 0) for (let r = top; r < bot - n; r++) move(r, r + n);
    else for (let r = bot - 1; r >= top - n; r--) move(r, r + n);
  }

  private colors(hl: number) {
    const a = this.attrs.get(hl) ?? {};
    let fg = a.foreground ?? this.fg;
    let bg = a.background ?? this.bg;
    if (a.reverse) [fg, bg] = [bg, fg];
    return { a, fg, bg };
  }

  private paint() {
    const ctx = this.ctx;
    ctx.textBaseline = "middle";
    for (const row of this.dirty) {
      const cells = this.rows[row];
      if (!cells) continue;
      const y = row * this.ch;
      for (let col = 0; col < cells.length; col++) {
        const { text, hl } = cells[col];
        const { a, fg, bg } = this.colors(hl);
        const wide = cells[col + 1]?.text === "";
        // Whole pixels, so neighbouring cells meet without seams.
        const x = this.x(col);
        const w = this.x(col + (wide ? 2 : 1)) - x;
        ctx.fillStyle = hex(bg);
        ctx.fillRect(x, y, w, this.ch);
        if (text && text !== " ") {
          ctx.font = (a.italic ? "italic " : "") + (a.bold ? "bold " : "") + font;
          ctx.fillStyle = hex(fg);
          ctx.fillText(text, x, y + this.ch / 2);
        }
        if (a.underline || a.undercurl || a.strikethrough) {
          ctx.fillStyle = hex(a.undercurl ? (a.special ?? this.sp) : fg);
          ctx.fillRect(x, a.strikethrough ? y + this.ch / 2 : y + this.ch - 2, w, 1);
        }
      }
    }
    this.dirty.clear();
    this.drawCursor();
  }

  // x is where column col starts, in whole pixels.
  private x(col: number) {
    return Math.round(col * this.cw);
  }

  private drawCursor() {
    const cell = this.rows[this.cursor.row]?.[this.cursor.col];
    if (!cell) return;
    const { fg } = this.colors(cell.hl);
    const x = this.x(this.cursor.col);
    const y = this.cursor.row * this.ch;
    const pct = (this.mode.cell_percentage || 100) / 100;
    this.ctx.fillStyle = hex(fg);
    switch (this.mode.cursor_shape) {
      case "vertical":
        this.ctx.fillRect(x, y, Math.max(1, this.cw * pct), this.ch);
        break;
      case "horizontal":
        this.ctx.fillRect(x, y + this.ch * (1 - pct), this.cw, this.ch * pct);
        break;
      default: {
        this.ctx.fillRect(x, y, this.cw, this.ch);
        const { bg } = this.colors(cell.hl);
        this.ctx.fillStyle = hex(bg);
        this.ctx.font = font;
        if (cell.text.trim()) this.ctx.fillText(cell.text, x, y + this.ch / 2);
      }
    }
    // The textarea follows the cursor, so input methods show there.
    this.input.style.left = x + "px";
    this.input.style.top = y + "px";
  }

  private listen() {
    const send = (keys: string) => void App.NvimInput(keys).catch(() => {});
    this.input.addEventListener("keydown", (e) => {
      if (e.isComposing || this.composing) return;
      if (this.onKey(e)) {
        e.preventDefault();
        return;
      }
      const keys = keyNotation(e);
      if (keys === null) return;
      e.preventDefault();
      send(keys);
    });
    this.input.addEventListener("compositionstart", () => (this.composing = true));
    this.input.addEventListener("compositionend", (e) => {
      this.composing = false;
      if (e.data) send(e.data.replaceAll("<", "<lt>"));
      this.input.value = "";
    });
    this.input.addEventListener("input", () => {
      if (this.composing) return;
      if (this.input.value) send(this.input.value.replaceAll("<", "<lt>"));
      this.input.value = "";
    });
    this.input.addEventListener("paste", (e) => {
      e.preventDefault();
      const text = e.clipboardData?.getData("text/plain");
      if (text) void App.NvimPaste(text).catch(() => {});
    });

    const cellAt = (e: MouseEvent) => {
      const r = this.canvas.getBoundingClientRect();
      return { row: Math.floor((e.clientY - r.top) / this.ch), col: Math.floor((e.clientX - r.left) / this.cw) };
    };
    const buttons = ["left", "middle", "right"];
    let down = "";
    this.canvas.addEventListener("mousedown", (e) => {
      e.preventDefault();
      this.focus();
      down = buttons[e.button] ?? "";
      if (!down) return;
      const { row, col } = cellAt(e);
      void App.NvimMouse(down, "press", mouseMods(e), row, col).catch(() => {});
    });
    window.addEventListener("mousemove", (e) => {
      if (!down) return;
      const { row, col } = cellAt(e);
      void App.NvimMouse(down, "drag", mouseMods(e), row, col).catch(() => {});
    });
    window.addEventListener("mouseup", (e) => {
      if (!down) return;
      const { row, col } = cellAt(e);
      void App.NvimMouse(down, "release", mouseMods(e), row, col).catch(() => {});
      down = "";
    });
    this.canvas.addEventListener(
      "wheel",
      (e) => {
        e.preventDefault();
        this.wheel += e.deltaY;
        const { row, col } = cellAt(e);
        while (Math.abs(this.wheel) >= this.ch) {
          const dir = this.wheel > 0 ? "down" : "up";
          this.wheel -= Math.sign(this.wheel) * this.ch;
          void App.NvimMouse("wheel", dir, mouseMods(e), row, col).catch(() => {});
        }
      },
      { passive: false },
    );
  }
}

// colorTokens are the page's color tokens (style.css) Neovim's colors
// are made of, by their names there.
const colorTokens = [
  "bg", "fg", "muted", "border", "code-bg", "hover", "accent", "accent-fg", "link", "ok", "bad", "control",
  "warn-bg", "warn-border", "selection", "active-line",
  "syn-keyword", "syn-string", "syn-number", "syn-comment", "syn-variable", "syn-function", "syn-type", "syn-punct", "syn-heading", "syn-link", "syn-meta",
];

// nvimColors are the theme's colors as Neovim takes them: #rrggbb, with
// translucent ones (the selection, the active line) laid over the code
// background, and dark if the theme is. Keys have _ for -.
export function nvimColors(): Record<string, string | boolean> {
  const style = getComputedStyle(document.documentElement);
  const under = rgba(style.getPropertyValue("--code-bg")) ?? [255, 255, 255, 1];
  const out: Record<string, string | boolean> = {};
  for (const name of colorTokens) {
    const c = rgba(style.getPropertyValue("--" + name));
    if (!c) continue;
    const [r, g, b, a] = c;
    const mix = (x: number, y: number) => Math.round(x * a + y * (1 - a));
    out[name.replaceAll("-", "_")] = hex((mix(r, under[0]) << 16) | (mix(g, under[1]) << 8) | mix(b, under[2]));
  }
  out.dark = style.colorScheme.includes("dark");
  return out;
}

// rgba reads a CSS color as red, green and blue from 0 to 255 and alpha
// from 0 to 1, or null.
function rgba(css: string): [number, number, number, number] | null {
  const probe = document.createElement("canvas").getContext("2d")!;
  probe.fillStyle = "#000001";
  probe.fillStyle = css.trim();
  const v = String(probe.fillStyle);
  if (v === "#000001" && css.trim() !== "#000001") return null;
  let m = /^#([0-9a-f]{2})([0-9a-f]{2})([0-9a-f]{2})$/i.exec(v);
  if (m) return [parseInt(m[1], 16), parseInt(m[2], 16), parseInt(m[3], 16), 1];
  m = /^rgba?\(\s*([\d.]+)[,\s]+([\d.]+)[,\s]+([\d.]+)(?:[,\s/]+([\d.]+))?\s*\)$/.exec(v);
  if (m) return [Number(m[1]), Number(m[2]), Number(m[3]), m[4] === undefined ? 1 : Number(m[4])];
  return null;
}

function hex(rgb: number): string {
  return "#" + rgb.toString(16).padStart(6, "0");
}

function mouseMods(e: MouseEvent): string {
  return [e.shiftKey && "S", e.ctrlKey && "C", e.altKey && "A"].filter(Boolean).join("-");
}

const special: Record<string, string> = {
  Enter: "CR",
  Escape: "Esc",
  Backspace: "BS",
  Tab: "Tab",
  Delete: "Del",
  Insert: "Insert",
  Home: "Home",
  End: "End",
  PageUp: "PageUp",
  PageDown: "PageDown",
  ArrowUp: "Up",
  ArrowDown: "Down",
  ArrowLeft: "Left",
  ArrowRight: "Right",
};

// keyNotation is the key in Neovim's notation, or null to let the
// textarea have it: plain typing comes through its input event, and
// Cmd keys are the app's menu shortcuts.
export function keyNotation(e: KeyboardEvent): string | null {
  if (e.metaKey) return null;
  // AltGr types characters; it isn't Ctrl and Alt.
  const altGr = e.getModifierState?.("AltGraph") ?? false;
  const ctrl = e.ctrlKey && !altGr;
  // On a Mac, Option types accented letters, so it is left to them.
  const alt = e.altKey && !altGr && !isMac;
  const name = special[e.key] ?? (/^F\d{1,2}$/.test(e.key) ? e.key : "");
  if (name) return wrap(name, ctrl, alt, e.shiftKey);
  if ([...e.key].length !== 1 || !(ctrl || alt)) return null;
  // The key is already shifted, as "A" or "!", so S- isn't needed.
  const key = e.key === " " ? "Space" : e.key === "<" ? "lt" : e.key === "\\" ? "Bslash" : e.key === "|" ? "Bar" : e.key;
  return wrap(key, ctrl, alt, false);
}

function wrap(key: string, ctrl: boolean, alt: boolean, shift: boolean): string {
  const mods = (shift ? "S-" : "") + (ctrl ? "C-" : "") + (alt ? "M-" : "");
  return "<" + mods + key + ">";
}
