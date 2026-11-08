// The cast player. A tty block's output is an asciinema cast (asciicast
// v2), which Go links as an image; enableCasts swaps each such image for
// a player that plays the session in an xterm.js terminal, with play,
// pause, a scrubber and frame by frame stepping. Text on the screen can
// be selected and copied, as in a terminal, and the copy button copies
// the recording as an animated GIF. Where the head is left is written to
// the file as frame=N, so it opens there again. On a tty block's output,
// two more buttons swap the recording in the file for the text of the
// frame shown, or for an animated GIF. The cast is read from
// the window's own asset server, or from its data: URI, never the
// network.
//
// Frames follow termrec's rule, so frame=N names the same screen here
// as in a PDF: writes less than 100ms apart share a frame, and a write
// that changes nothing on screen makes none.

import { Terminal } from "@xterm/xterm";
import { Clipboard, Events } from "/wails/runtime.js";
import { App } from "./api.js";
import { announce, el, iconButton, svg, icons } from "./dom.js";
import { codeFont, textSize } from "./theme.js";

// step is the shortest time between frames, and maxDelay caps how long
// playback waits on one, as the GIF does.
const step = 0.1;
const maxDelay = 2;

type Cast = { cols: number; rows: number; events: [number, string][] };
// A frame is the screen once events[0..upto) are written, at time at.
type Frame = { at: number; upto: number };
// used is how many rows, from the top, any frame shows something on.
type Loaded = { text: string; cast: Cast; frames: Frame[]; used: number };

const castMIME = "application/x-asciicast";

// isCast reports whether src, an image's src, is a cast.
export function isCast(src: string): boolean {
  if (src.startsWith("data:")) return src.startsWith("data:" + castMIME);
  try {
    return new URL(src, location.href).pathname.toLowerCase().endsWith(".cast");
  } catch {
    return false;
  }
}

// parseCast reads asciicast v2: a JSON header line, then one
// [seconds, type, text] line per event. Only output events are kept.
export function parseCast(text: string): Cast {
  const lines = text.split("\n").filter((l) => l.trim());
  const h = JSON.parse(lines[0] ?? "");
  if (h?.version !== 2) throw new Error("not an asciicast v2 file");
  const events: [number, string][] = [];
  for (const l of lines.slice(1)) {
    const e = JSON.parse(l);
    if (Array.isArray(e) && e[1] === "o" && typeof e[0] === "number" && typeof e[2] === "string") events.push([e[0], e[2]]);
  }
  return { cols: h.width > 0 ? h.width : 80, rows: h.height > 0 ? h.height : 24, events };
}

// mouseModes are the DEC private modes that turn on mouse reporting,
// and the encodings that go with them.
const mouseModes = new Set([9, 1000, 1001, 1002, 1003, 1005, 1006, 1015, 1016]);

// ignoreMouseModes keeps a recording that turned on mouse reporting, as
// a full-screen program does, from taking the player's mouse: a drag
// over the screen selects text, as in any other recording.
function ignoreMouseModes(t: Terminal) {
  t.parser.registerCsiHandler({ prefix: "?", final: "h" }, (params) => {
    const modes = params.map((p) => (Array.isArray(p) ? p[0] : p));
    if (!modes.some((m) => mouseModes.has(m))) return false;
    if (modes.every((m) => mouseModes.has(m))) return true;
    // Mixed with other modes: let them through, then turn reporting off.
    t.write("\x1b[?9l\x1b[?1000l\x1b[?1002l\x1b[?1003l");
    return false;
  });
}

const write = (t: Terminal, data: string) => new Promise<void>((done) => t.write(data, done));

// cursorShown follows DECTCEM through data, as vt10x does, since xterm
// doesn't say whether the cursor is hidden.
function cursorShown(data: string, shown: boolean): boolean {
  const on = data.lastIndexOf("\x1b[?25h");
  const off = data.lastIndexOf("\x1b[?25l");
  if (on < 0 && off < 0) return shown;
  return on > off;
}

// screen is what t shows, cell by cell, for telling whether a write
// changed it.
function screen(t: Terminal, cursor: boolean): string {
  const b = t.buffer.active;
  const parts: string[] = [];
  let cell = b.getNullCell();
  for (let y = 0; y < t.rows; y++) {
    const line = b.getLine(b.baseY + y);
    if (!line) continue;
    for (let x = 0; x < t.cols; x++) {
      cell = line.getCell(x, cell) ?? cell;
      parts.push(cell.getChars() || " ", String(cell.getFgColorMode()), String(cell.getFgColor()), String(cell.getBgColorMode()), String(cell.getBgColor()), String(cell.isBold() | (cell.isInverse() << 1)));
    }
  }
  if (cursor) parts.push("cursor", String(b.cursorX), String(b.cursorY));
  return parts.join("\u0000");
}

// usedRows is how many rows, from the top, t shows text or the cursor
// on.
function usedRows(t: Terminal, cursor: boolean): number {
  const b = t.buffer.active;
  let used = cursor ? b.cursorY + 1 : 0;
  for (let y = t.rows - 1; y >= used; y--) {
    if (b.getLine(b.baseY + y)?.translateToString(true).trim()) return y + 1;
  }
  return used;
}

// frames plays cast through a terminal that isn't shown, and returns its
// frames and how many rows they use.
async function frames(cast: Cast): Promise<{ frames: Frame[]; used: number }> {
  const t = new Terminal({ cols: cast.cols, rows: cast.rows, scrollback: 0, allowProposedApi: true });
  const out: Frame[] = [];
  const ev = cast.events;
  let last = "";
  let cursor = true;
  let used = 1;
  try {
    for (let i = 0; i < ev.length; i++) {
      await write(t, ev[i][1]);
      cursor = cursorShown(ev[i][1], cursor);
      if (i + 1 < ev.length && out.length && ev[i + 1][0] - out[out.length - 1].at < step) continue;
      const s = screen(t, cursor);
      if (out.length && s === last) continue;
      out.push({ at: ev[i][0], upto: i + 1 });
      used = Math.max(used, usedRows(t, cursor));
      last = s;
    }
  } finally {
    t.dispose();
  }
  return { frames: out, used };
}

// loaded keeps casts already read, by src, which carries the file's
// modification time, so drawing the cells again doesn't read them again.
const loaded = new Map<string, Promise<Loaded>>();

async function readCast(src: string): Promise<string> {
  if (!src.startsWith("data:")) {
    const r = await fetch(src);
    if (!r.ok) throw new Error(r.status + " " + r.statusText);
    return r.text();
  }
  const comma = src.indexOf(",");
  const meta = src.slice(5, comma < 0 ? src.length : comma);
  const payload = comma < 0 ? "" : src.slice(comma + 1);
  if (!meta.endsWith(";base64")) return decodeURIComponent(payload);
  const raw = atob(payload);
  const bytes = new Uint8Array(raw.length);
  for (let i = 0; i < raw.length; i++) bytes[i] = raw.charCodeAt(i);
  return new TextDecoder().decode(bytes);
}

function load(src: string): Promise<Loaded> {
  const key = src.split("#")[0];
  let p = loaded.get(key);
  if (!p) {
    p = readCast(key).then(async (text) => {
      const cast = parseCast(text);
      return { text, cast, ...(await frames(cast)) };
    });
    p.catch(() => loaded.delete(key));
    loaded.set(key, p);
  }
  return p;
}

// players are the players drawn, so those no longer in the page give
// their terminals back.
type Player = { host: HTMLElement; menu: string; dispose: () => void; look: () => void; selected: () => string; say: (what: string) => void; copy: () => void; copyGIF: () => void };
const players = new Set<Player>();

// A player's right-click menu (Copy, Copy as GIF) is the app's "cast"
// menu, which sends back the menu's data, the player's own name, and
// what was picked.
Events.On("cast-menu", (ev: { data: string }) => {
  const [what, menu] = ev.data.split(":");
  for (const p of players) {
    if (p.menu !== menu) continue;
    if (what === "gif") p.copyGIF();
    else p.copy();
  }
});
let menus = 0;

function sweep() {
  for (const p of players) {
    if (!p.host.isConnected) {
      p.dispose();
      players.delete(p);
    }
  }
}

// CastOptions says where e's players open and how they keep where they
// were left.
export type CastOptions = {
  // frame is the frame to open at, counting from 1, over the link's
  // #frame=N.
  frame?: number;
  // save writes where the player was left to the file: frame, counting
  // from 1, or 0 for the last. body is the recording, for a cast fence.
  save?: (frame: number, body: string) => void;
  // replace, for a tty block's output, swaps the recording in the file
  // for the text of frame, counting from 1, or for an animated GIF of it.
  // The player then offers both.
  replace?: (as: "text" | "gif", frame: number) => void;
};

// heads are where each recording's player was left, by its cast, so a
// player drawn again, as when the file changes, opens there and takes
// the focus back.
const heads = new Map<string, number>();
let focused: { key: string; label: string } | null = null;

// enableCasts swaps each cast image in e for a player.
export function enableCasts(e: HTMLElement, opts: CastOptions = {}) {
  sweep();
  for (const img of e.querySelectorAll<HTMLImageElement>("img")) {
    const src = img.getAttribute("src") ?? "";
    if (!isCast(src)) continue;
    // A cast fence's recording is its code.
    const body = img.closest('figure.diagram[data-lang="cast"]')?.querySelector("pre")?.textContent ?? "";
    img.replaceWith(player(src, img.alt, opts, body));
  }
}

function clock(s: number): string {
  const m = Math.floor(s / 60);
  return m + ":" + (s - 60 * m).toFixed(1).padStart(4, "0");
}

function colors(): Record<string, string> {
  const css = getComputedStyle(document.documentElement);
  const v = (name: string, or: string) => css.getPropertyValue(name).trim() || or;
  // Selected text takes the look's selection color, as it does elsewhere
  // in the window.
  const sel = cssColor(v("--selection", "rgb(56 139 253 / 0.4)"));
  return { background: v("--term-bg", "#1e1e1e"), foreground: v("--term-fg", "#d4d4d4"), cursor: v("--term-fg", "#d4d4d4"), selectionBackground: sel, selectionInactiveBackground: sel };
}

// cssColor is color as rgba(r, g, b, a) or #rrggbb, which xterm reads,
// rather than the space-separated rgb() the looks write.
function cssColor(color: string): string {
  const ctx = document.createElement("canvas").getContext("2d");
  if (!ctx) return color;
  ctx.fillStyle = color;
  return ctx.fillStyle;
}

// copyKey reports whether e is the copy key: cmd+c on macOS, ctrl+c or
// ctrl+shift+c elsewhere.
function copyKey(e: KeyboardEvent): boolean {
  const mac = navigator.platform.startsWith("Mac");
  return e.key.toLowerCase() === "c" && !e.altKey && (mac ? e.metaKey && !e.ctrlKey : e.ctrlKey && !e.metaKey);
}

// plain is terminal text as it goes on the clipboard: no trailing spaces
// on a line, and no blank lines at the end.
const plain = (text: string) => text.replace(/[ \t]+$/gm, "").replace(/\n+$/, "");

// screenText is what t shows, as plain text.
function screenText(t: Terminal): string {
  const b = t.buffer.active;
  const lines: string[] = [];
  for (let y = 0; y < t.rows; y++) lines.push(b.getLine(b.baseY + y)?.translateToString(true) ?? "");
  return plain(lines.join("\n"));
}

// Edit › Copy, or the copy key where the menu takes it first, copies the
// text selected in the player the keys are in.
document.addEventListener("copy", (e) => {
  const at = document.activeElement;
  for (const p of players) {
    if (!at || !p.host.contains(at)) continue;
    const text = p.selected();
    if (!text || !e.clipboardData) return;
    e.preventDefault();
    e.clipboardData.setData("text/plain", text);
    p.say(lines(text));
    return;
  }
});

// lines says how many lines text is.
function lines(text: string): string {
  const n = text.split("\n").length;
  return `${n} ${n === 1 ? "line" : "lines"}`;
}

// saveDelay is how long the head has to rest before where it was left
// is written to the file.
const saveDelay = 800;

// player is a player for the cast at src, showing the frame opts or its
// #frame=N names, or the last, as still views do.
function player(src: string, alt: string, opts: CastOptions, body: string): HTMLElement {
  const key = src.split("#")[0];
  const screenHost = el("div", { className: "cast-screen" });
  const label = alt && alt !== "recording" ? "Recording: " + alt : "Recording";
  const box = el("div", { className: "cast-player loading" }, screenHost);
  box.setAttribute("role", "group");
  box.setAttribute("aria-label", label);
  const menu = `p${++menus}x${Math.floor(Math.random() * 1e9)}`;
  box.style.setProperty("--custom-contextmenu", "cast");
  box.style.setProperty("--custom-contextmenu-data", `"${menu}"`);
  const status = el("span", { className: "cast-status muted" }, "loading…");
  // note says what was copied, for a moment.
  const note = el("span", { className: "cast-note", hidden: true });
  note.setAttribute("role", "status");
  let noteTimer = 0;
  let saveTimer = 0;
  let saved = -1; // the frame the file says, from 0, or -1 for the last
  const scrub = el("input", { type: "range", min: "1", max: "1", value: "1", className: "cast-scrub", disabled: true });
  scrub.setAttribute("aria-label", "Frame");
  let term: Terminal | null = null;
  let data: Loaded | null = null;
  let frame = 0; // the frame shown, from 0
  let written = 0; // how many events the terminal has had
  let timer = 0;
  let drawing = Promise.resolve();

  const playing = () => timer !== 0;
  const playBtn = iconButton("play", "Play", () => toggle());
  const back = iconButton("stepBack", "Previous frame", () => show(frame - 1));
  const fwd = iconButton("stepForward", "Next frame", () => show(frame + 1));
  const shotBtn = iconButton("camera", "Copy this frame as text", () => void copyFrame());
  const copyBtn = iconButton("copy", "Copy as an animated GIF", () => void copyGIF());
  const swaps: HTMLButtonElement[] = [];
  if (opts.replace) {
    const swap = opts.replace;
    swaps.push(
      iconButton("text", "Replace the recording with this frame's text", () => swap("text", frame + 1)),
      iconButton("image", "Replace the recording with an animated GIF", () => swap("gif", 0)),
    );
  }
  const buttons = [playBtn, back, fwd, shotBtn, copyBtn, ...swaps];
  for (const b of buttons) b.disabled = true;
  const controls = el("div", { className: "cast-controls" }, back, playBtn, fwd, scrub, note, status, shotBtn, copyBtn);
  if (swaps.length) controls.append(el("span", { className: "cast-sep" }), ...swaps);
  box.append(controls);

  const selected = () => (term?.hasSelection() ? plain(term.getSelection()) : "");

  function say(what: string) {
    note.textContent = "Copied " + what;
    note.hidden = false;
    announce(note.textContent);
    clearTimeout(noteTimer);
    noteTimer = window.setTimeout(() => (note.hidden = true), 2000);
  }

  function failed(err: unknown) {
    note.textContent = "couldn't copy: " + (err instanceof Error ? err.message : String(err));
    note.hidden = false;
  }

  // copy puts the selected text on the clipboard, or, with none, the
  // whole frame shown.
  async function copy(why = "") {
    if (!term) return;
    const text = selected() || screenText(term);
    if (!text) return;
    try {
      await Clipboard.SetText(text);
    } catch (err) {
      return failed(err);
    }
    say(lines(text) + why);
  }

  // copyFrame puts the frame shown on the clipboard as plain text: what
  // is on the screen, with no colors or cursor moves, whatever is
  // selected.
  async function copyFrame() {
    if (!term) return;
    const text = screenText(term);
    try {
      await Clipboard.SetText(text);
    } catch (err) {
      return failed(err);
    }
    say(`frame ${frame + 1} as text, ${lines(text)}`);
  }

  // copyGIF puts an animated GIF of the recording on the clipboard. Where
  // the clipboard only takes text, it copies the text, as copy does.
  async function copyGIF() {
    if (!data) return;
    copyBtn.disabled = true;
    try {
      await App.CopyCastGIF(data.text);
      say("a GIF of the recording");
    } catch (err) {
      const why = err instanceof Error ? err.message : String(err);
      if (/needs macOS/.test(why)) await copy(" of text: " + why);
      else failed(err);
    } finally {
      copyBtn.disabled = false;
    }
  }

  // keep writes where the head was left to the file, once it rests.
  function keep() {
    clearTimeout(saveTimer);
    if (!opts.save || !data) return;
    const last = data.frames.length - 1;
    saveTimer = window.setTimeout(() => {
      if (playing() || !data) return;
      const at = frame === last ? -1 : frame;
      if (at === saved) return;
      saved = at;
      opts.save?.(at + 1, body);
    }, saveDelay);
  }

  // A click or drag on the screen selects text in it, rather than going
  // to the block's place in the source as a click on the card does.
  screenHost.onclick = (e) => e.stopPropagation();
  // The screen has no scrollback, so the wheel scrolls the page: xterm
  // would take it, even to send arrow keys to a full-screen program.
  screenHost.addEventListener("wheel", (e) => e.stopPropagation(), { capture: true, passive: true });

  // show draws frame n: the events after those written, or, going back,
  // all of them again from a reset terminal.
  function show(n: number, fromPlay = false) {
    if (!data || !term) return;
    if (!fromPlay) pause();
    const t = term;
    const { frames: fs, cast } = data;
    frame = Math.max(0, Math.min(n, fs.length - 1));
    const upto = fs[frame].upto;
    drawing = drawing.then(() => {
      if (upto < written) {
        t.reset();
        written = 0;
      }
      const chunk = cast.events.slice(written, upto).map((e) => e[1]).join("");
      written = upto;
      return chunk ? write(t, chunk) : undefined;
    });
    const end = fs[fs.length - 1].at;
    status.textContent = `${frame + 1} / ${fs.length} · ${clock(fs[frame].at)} / ${clock(end)}`;
    scrub.value = String(frame + 1);
    scrub.setAttribute("aria-valuetext", `frame ${frame + 1} of ${fs.length}, ${clock(fs[frame].at)}`);
    back.disabled = frame === 0;
    fwd.disabled = frame === fs.length - 1;
    heads.set(key, frame);
    if (!fromPlay && !opening) keep();
  }

  function setPlayIcon(on: boolean) {
    const title = on ? "Pause" : "Play";
    playBtn.title = title;
    playBtn.setAttribute("aria-label", title);
    playBtn.replaceChildren(svg(on ? icons.pause : icons.play));
  }

  function pause() {
    if (!timer) return;
    clearTimeout(timer);
    timer = 0;
    setPlayIcon(false);
    keep();
  }

  function tick() {
    if (!data) return;
    const fs = data.frames;
    if (frame >= fs.length - 1) {
      pause();
      return;
    }
    const wait = Math.min(fs[frame + 1].at - fs[frame].at, maxDelay);
    timer = window.setTimeout(() => {
      show(frame + 1, true);
      tick();
    }, wait * 1000);
  }

  function toggle() {
    if (!data) return;
    if (playing()) return pause();
    // At the end, play starts again from the top.
    if (frame >= data.frames.length - 1) show(0);
    setPlayIcon(true);
    timer = -1;
    tick();
  }

  scrub.oninput = () => show(Number(scrub.value) - 1);
  box.onkeydown = (e) => {
    if (!data || e.metaKey || e.ctrlKey || e.altKey) return;
    const keys: Record<string, () => void> = {
      " ": toggle,
      k: toggle,
      ArrowLeft: () => show(frame - 1),
      ",": () => show(frame - 1),
      ArrowRight: () => show(frame + 1),
      ".": () => show(frame + 1),
      Home: () => show(0),
      End: () => show(data!.frames.length - 1),
    };
    const f = keys[e.key];
    // The scrubber moves itself with the arrows, Home and End.
    if (!f || (e.target === scrub && e.key !== " ")) return;
    e.preventDefault();
    e.stopPropagation();
    f();
  };

  // Where the focus is in the player, so a player drawn again for the
  // same recording takes it back.
  box.addEventListener("focusin", (e) => {
    const t = e.target as HTMLElement;
    focused = { key, label: t.getAttribute("aria-label") ?? (t === scrub ? "Frame" : "") };
  });
  // A player taken out of the page as it is drawn again loses the focus
  // too, so the focus only counts as gone while the player is still in
  // the page.
  box.addEventListener("focusout", () => {
    setTimeout(() => {
      if (box.isConnected && !box.contains(document.activeElement) && focused?.key === key) focused = null;
    });
  });

  let opening = true;
  const want = opts.frame ?? Number(/#frame=(\d+)/.exec(src)?.[1] ?? 0);
  load(src).then(
    (d) => {
      box.classList.remove("loading");
      if (!d.frames.length) {
        status.textContent = "nothing was shown";
        return;
      }
      data = d;
      term = new Terminal({ cols: d.cast.cols, rows: d.cast.rows, fontSize: textSize(13), fontFamily: codeFont(), theme: colors(), scrollback: 0, disableStdin: true, cursorBlink: false, screenReaderMode: true, rightClickSelectsWord: false });
      term.open(screenHost);
      ignoreMouseModes(term);
      // With text selected, the copy key copies it.
      term.attachCustomKeyEventHandler((e) => {
        if (e.type !== "keydown" || !copyKey(e) || !term?.hasSelection()) return true;
        e.preventDefault();
        void copy();
        return false;
      });
      // Rows no frame reaches are left off, as in the GIF.
      // The padding above the rows is the same below them.
      const row = screenHost.querySelector<HTMLElement>(".xterm-rows > div")?.offsetHeight;
      const rows = screenHost.querySelector(".xterm-screen");
      if (row && rows && d.used < d.cast.rows) {
        const pad = rows.getBoundingClientRect().top - screenHost.getBoundingClientRect().top;
        screenHost.style.height = `${2 * pad + d.used * row}px`;
      }
      const t = term;
      players.add({
        host: box,
        menu,
        copy: () => void copy(),
        copyGIF: () => void copyGIF(),
        dispose: () => (pause(), clearTimeout(noteTimer), clearTimeout(saveTimer), t.dispose()),
        selected,
        say,
        look: () => {
          t.options.theme = colors();
          t.options.fontFamily = codeFont();
          t.options.fontSize = textSize(13);
        },
      });
      scrub.max = String(d.frames.length);
      scrub.disabled = false;
      for (const b of buttons) b.disabled = false;
      saved = want > 0 && want < d.frames.length ? want - 1 : -1;
      const left = heads.get(key);
      show(left ?? (saved < 0 ? d.frames.length - 1 : saved));
      opening = false;
      if (focused?.key === key && (!document.activeElement || document.activeElement === document.body)) {
        const again = [...box.querySelectorAll<HTMLElement>("button, input")].find((e) => (e.getAttribute("aria-label") ?? "") === focused!.label);
        (again ?? playBtn).focus();
      }
    },
    (err) => {
      box.classList.remove("loading");
      box.classList.add("failed");
      status.textContent = "can't play the recording: " + (err instanceof Error ? err.message : String(err));
    },
  );
  return box;
}

// The look changed: players take its colors and font.
window.addEventListener("mdsh-look", () => {
  sweep();
  for (const p of players) p.look();
});
