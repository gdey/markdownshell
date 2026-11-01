// Small helpers for building the page.

export const $ = <T extends HTMLElement = HTMLElement>(id: string): T => {
  const e = document.getElementById(id);
  if (!e) throw new Error("no #" + id);
  return e as T;
};

type Props<K extends keyof HTMLElementTagNameMap> = Partial<Omit<HTMLElementTagNameMap[K], "style">>;

export function el<K extends keyof HTMLElementTagNameMap>(tag: K, props: Props<K> = {}, ...children: (Node | string)[]): HTMLElementTagNameMap[K] {
  const e = Object.assign(document.createElement(tag), props);
  e.append(...children);
  return e;
}

export const icons = {
  play: '<path d="M4 2.5v11l9-5.5z"/>',
  runFrom: '<path d="M2.5 2.5v11l8-5.5z"/><rect x="11.5" y="2.5" width="2" height="11"/>',
  stop: '<rect x="3.5" y="3.5" width="9" height="9" rx="1"/>',
  pencil: '<path d="M11.1 1.9a1.5 1.5 0 0 1 2.1 0l.9.9a1.5 1.5 0 0 1 0 2.1L5.6 13.4 2 14l.6-3.6z"/>',
  sliders: '<rect x="2" y="3.3" width="12" height="1.4"/><rect x="2" y="7.3" width="12" height="1.4"/><rect x="2" y="11.3" width="12" height="1.4"/><circle cx="5" cy="4" r="1.8"/><circle cx="11" cy="8" r="1.8"/><circle cx="7" cy="12" r="1.8"/>',
  trash: '<path d="M6 1.5h4l.5 1H14v1.4H2V2.5h3.5zM3.2 5h9.6l-.7 9.5H3.9zm2.3 1.6.3 6.4h1.2l-.2-6.4zm3.8 0-.2 6.4h1.2l.3-6.4z"/>',
  info: '<circle cx="8" cy="8" r="6.5" fill="none" stroke="currentColor" stroke-width="1.3"/><rect x="7.3" y="7" width="1.4" height="4.5"/><rect x="7.3" y="4.4" width="1.4" height="1.4"/>',
  // The markdown bar, the outline drawer and the view switch.
  heading: '<path d="M3 2.5h1.6v4.7h6.8V2.5H13v11h-1.6V8.7H4.6v4.8H3z"/>',
  bold: '<path d="M4 2.5h4.6c2 0 3.2 1 3.2 2.7 0 1-.6 1.9-1.5 2.2 1.2.3 2 1.3 2 2.6 0 2-1.4 3.5-3.6 3.5H4zm1.8 1.5v3h2.6c1 0 1.6-.6 1.6-1.5S9.4 4 8.4 4zm0 4.5V12h2.9c1.1 0 1.8-.7 1.8-1.8s-.7-1.7-1.8-1.7z"/>',
  italic: '<path d="M6.5 2.5h6v1.5h-2.3l-2.4 8h2.2v1.5h-6V12h2.3l2.4-8H6.5z"/>',
  code: '<path d="M5.4 4.2 6.5 5.3 3.8 8l2.7 2.7-1.1 1.1L1.6 8zm5.2 0L14.4 8l-3.8 3.8-1.1-1.1L12.2 8 9.5 5.3z"/>',
  link: '<path d="M7.2 9.9 6.1 11a2.3 2.3 0 0 1-3.2-3.2l2-2a2.3 2.3 0 0 1 3.2 0l.5.5 1-1-.5-.5a3.7 3.7 0 0 0-5.2 0l-2 2A3.7 3.7 0 0 0 7.1 12l1.1-1.1zm1.6-3.8L9.9 5a2.3 2.3 0 0 1 3.2 3.2l-2 2a2.3 2.3 0 0 1-3.2 0l-.5-.5-1 1 .5.5a3.7 3.7 0 0 0 5.2 0l2-2A3.7 3.7 0 0 0 8.9 4l-1.1 1.1z"/>',
  image: '<path d="M2 3h12v10H2zm1.4 1.4v6.2l2.8-2.9 2.2 2.2 1.5-1.4 2.7 2.6V4.4zm7.1 1.1a1.2 1.2 0 1 1 0 2.4 1.2 1.2 0 0 1 0-2.4z"/>',
  ul: '<circle cx="2.8" cy="4" r="1.2"/><circle cx="2.8" cy="8" r="1.2"/><circle cx="2.8" cy="12" r="1.2"/><rect x="5.5" y="3.3" width="9" height="1.4"/><rect x="5.5" y="7.3" width="9" height="1.4"/><rect x="5.5" y="11.3" width="9" height="1.4"/>',
  ol: '<path d="M2.2 2.5h1.2v3.3h-1V3.6h-.6zm-.4 6.1c0-.8.6-1.3 1.3-1.3s1.3.5 1.3 1.2c0 .5-.3.8-.7 1.1l-.6.5h1.3v.9H1.8v-.8l1.3-1.1c.2-.2.3-.3.3-.5s-.1-.3-.3-.3-.3.1-.3.4z"/><rect x="5.5" y="3.3" width="9" height="1.4"/><rect x="5.5" y="7.3" width="9" height="1.4"/><rect x="5.5" y="11.3" width="9" height="1.4"/>',
  task: '<path d="M1.5 2.5h4v4h-4zm1.2 1.2v1.6h1.6V3.7zM1.5 9.5h4v4h-4zm.6 1.4.8-.8.7.7 1.4-1.4.8.8-2.2 2.2z"/><rect x="7" y="3.8" width="7.5" height="1.4"/><rect x="7" y="10.8" width="7.5" height="1.4"/>',
  quote: '<path d="M2 3h1.5v10H2zm3.5 1h8.5v1.4H5.5zm0 3.3h8.5v1.4H5.5zm0 3.3h6v1.4h-6z"/>',
  table: '<path d="M1.5 2.5h13v11h-13zm1.4 1.4v2.2h4.4V3.9zm5.8 0v2.2h4.4V3.9zM2.9 7.5v4.6h4.4V7.5zm5.8 0v4.6h4.4V7.5z"/>',
  fence: '<path d="M2 2.5h12v11H2zm1.4 1.4v8.2h9.2V3.9zm1.4 1.5 2.3 2.3-2.3 2.3-.9-.9 1.4-1.4-1.4-1.4zm3 3.7h3.4v1.2H7.8z"/>',
  mdsh: '<path d="M2 2.5h12v11H2zm1.4 1.4v8.2h9.2V3.9zM6 5.5l4.5 2.5L6 10.5z"/>',
  sidebar: '<path d="M1.5 2.5h13v11h-13zm1.4 1.4v8.2h3.2V3.9zm4.6 0v8.2h5.6V3.9z"/>',
  // A fold: open points down, and CSS turns it right when closed.
  chevron: '<path d="M3.7 5.6 8 9.9l4.3-4.3 1 1L8 11.9 2.7 6.6z"/>',
  rendered: '<path d="M3.5 1.5h6l3.5 3.5v9.5h-9.5zm1.4 1.4v10.2h6.7V6H8.5V2.9z"/>',
};

export function svg(path: string): SVGSVGElement {
  const s = document.createElementNS("http://www.w3.org/2000/svg", "svg");
  s.setAttribute("viewBox", "0 0 16 16");
  s.setAttribute("aria-hidden", "true");
  s.innerHTML = path;
  return s;
}

export function iconButton(icon: keyof typeof icons, title: string, onclick: () => void): HTMLButtonElement {
  const b = el("button", { className: "icon", title }, svg(icons[icon]));
  b.onclick = (e) => {
    e.stopPropagation();
    onclick();
  };
  b.setAttribute("aria-label", title);
  return b;
}
