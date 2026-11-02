// Web addresses in a block's output are made clickable when the block
// has the links attribute, as blocks typed in the History pane do. Only
// http:, https: and mailto: count, the same as weblink.Find in Go, so a
// click opens the browser or the mail app and never a local file. The
// links are <a href> in #doc, which app.ts's followLinks follows through
// App.OpenLink, as it does the document's own links.

import type { IDisposable, ILinkHandler, Terminal } from "@xterm/xterm";
import { App, message } from "./api.js";

const stop = `[^\\s\\x00-\\x1f\\x7f"'<>\\\\\`]`;
// address matches a web address, after what comes before it: not the
// middle of a word. It stops where weblink's does: at space, control
// characters, quotes, angle brackets and backslashes. (No lookbehind,
// which older macOS web views lack.)
const address = new RegExp(`(^|[^A-Za-z0-9_\\u0080-\\uffff])((?:https?://${stop}+|mailto:[^\\s\\x00-\\x1f\\x7f"'<>\\\\\`@]+@${stop}+))`, "gi");

// trim leaves off what ends a sentence after an address, and a closing
// bracket without its opening one, as in (see https://example.com).
function trim(u: string): string {
  const pairs: Record<string, string> = { ")": "(", "]": "[", "}": "{" };
  for (;;) {
    const last = u.slice(-1);
    if (".,;:!?".includes(last) && last) u = u.slice(0, -1);
    else if (pairs[last] && u.split(pairs[last]).length < u.split(last).length) u = u.slice(0, -1);
    else return u;
  }
}

// findLinks returns each web address in text, with where it starts.
export function findLinks(text: string): { at: number; url: string }[] {
  const found: { at: number; url: string }[] = [];
  for (const m of text.matchAll(address)) {
    const url = trim(m[2]);
    const rest = url.replace(/^[a-z]+:(\/\/)?/i, "");
    if (rest && !rest.startsWith("/")) found.push({ at: m.index! + m[1].length, url });
  }
  return found;
}

// isWebLink reports whether href is one findLinks could have found.
export function isWebLink(href: string): boolean {
  return /^(https?:\/\/|mailto:)/i.test(href);
}

// linkify turns the web addresses in root's text into links. Text
// already in a link is left alone.
export function linkify(root: HTMLElement) {
  const walk = document.createTreeWalker(root, NodeFilter.SHOW_TEXT);
  const texts: Text[] = [];
  for (let n = walk.nextNode(); n; n = walk.nextNode()) {
    if (!n.parentElement?.closest("a")) texts.push(n as Text);
  }
  for (const t of texts) {
    const text = t.data;
    const found = findLinks(text);
    if (!found.length) continue;
    const parts: Node[] = [];
    let at = 0;
    for (const f of found) {
      if (f.at > at) parts.push(document.createTextNode(text.slice(at, f.at)));
      const a = document.createElement("a");
      a.className = "out-link";
      a.setAttribute("href", f.url);
      a.textContent = f.url;
      parts.push(a);
      at = f.at + f.url.length;
    }
    if (at < text.length) parts.push(document.createTextNode(text.slice(at)));
    t.replaceWith(...parts);
  }
}

// openWeb opens a web address from a terminal, which only http:, https:
// and mailto: do.
function openWeb(url: string, fail: (msg: string) => void) {
  if (!isWebLink(url)) return fail(`mdsh only opens http:, https: and mailto: links from a terminal, not ${url}`);
  App.OpenLink(url).catch((err) => fail(message(err)));
}

// terminalLinks makes term follow the web addresses a click lands on:
// those in its text, underlined under the pointer, and the OSC 8 links
// programs write. Without it, xterm would open OSC 8 links
// with window.open, which would load them over the app's page.
export function terminalLinks(term: Terminal, fail: (msg: string) => void): IDisposable {
  const handler: ILinkHandler = {
    activate: (e, url) => {
      if (e.button === 0) openWeb(url, fail);
    },
    allowNonHttpProtocols: true,
  };
  term.options.linkHandler = handler;
  return term.registerLinkProvider({
    provideLinks(y, callback) {
      const line = term.buffer.active.getLine(y - 1);
      if (!line) return callback(undefined);
      // A wrapped line is read with the line it carries on from, so an
      // address split across them is one link; text maps to cells one
      // to one for the plain text addresses are made of.
      let first = y - 1;
      while (first > 0 && term.buffer.active.getLine(first)?.isWrapped) first--;
      let text = "";
      const starts: number[] = [];
      for (let row = first; ; row++) {
        const l = term.buffer.active.getLine(row);
        if (!l || (row > first && !l.isWrapped)) break;
        starts.push(text.length);
        text += l.translateToString(!term.buffer.active.getLine(row + 1)?.isWrapped);
      }
      const cols = term.cols;
      const at = (i: number) => {
        let row = starts.length - 1;
        while (row > 0 && starts[row] > i) row--;
        return { x: i - starts[row] + 1, y: first + row + 1 };
      };
      const links = findLinks(text)
        .map((f) => ({ f, start: at(f.at), end: at(f.at + f.url.length - 1) }))
        .filter(({ start, end }) => start.y <= y && end.y >= y && start.x <= cols)
        .map(({ f, start, end }) => ({
          range: { start, end },
          text: f.url,
          decorations: { underline: true, pointerCursor: true },
          activate: (e: MouseEvent) => {
            if (e.button === 0) openWeb(f.url, fail);
          },
        }));
      callback(links.length ? links : undefined);
    },
  });
}
