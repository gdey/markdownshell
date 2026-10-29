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
  plus: '<rect x="7.3" y="2.5" width="1.4" height="11"/><rect x="2.5" y="7.3" width="11" height="1.4"/>',
  info: '<circle cx="8" cy="8" r="6.5" fill="none" stroke="currentColor" stroke-width="1.3"/><rect x="7.3" y="7" width="1.4" height="4.5"/><rect x="7.3" y="4.4" width="1.4" height="1.4"/>',
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
