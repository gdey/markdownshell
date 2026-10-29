// The attribute form: the block's language and attributes as fields.
// Each change edits the opening fence in the editor, so the form and
// the text never disagree, and attributes the form doesn't show are
// kept as written.

import { App, AttrKind, type AttrSpec, type Names } from "./api.js";
import type { BlockEditor } from "./editor.js";
import { el } from "./dom.js";

export async function attrForm(editor: BlockEditor, attrs: AttrSpec[], names: Names): Promise<HTMLElement> {
  const form = el("div", { className: "attr-form" });
  const info = await App.ReadInfo(editor.info());
  const values = info.attrs ?? {};
  const include = "<include" in values;

  const set = async (key: string, value: string | null, bare = false) => {
    const next = await App.EditInfo(editor.info(), key, value ?? "", bare, value === null);
    editor.setInfo(next);
  };

  const lang = el("input", { type: "text", value: info.lang, spellcheck: false });
  lang.onchange = async () => editor.setInfo(await App.SetLang(editor.info(), lang.value.trim()));
  form.append(row("Language", "The fence's language; mdsh runs sh and zsh", lang));

  for (const a of attrs) {
    if (a.name === "<include" || !!a.include !== include) continue;
    if (include && a.name === "#") continue;
    form.append(field(a, values, names, set));
  }
  return form;
}

function row(label: string, help: string, ...input: HTMLElement[]): HTMLElement {
  const l = el("label", { title: help }, el("span", {}, label), ...input);
  return l;
}

function field(
  a: AttrSpec,
  values: Record<string, string | undefined>,
  names: Names,
  set: (key: string, value: string | null, bare?: boolean) => Promise<void>,
): HTMLElement {
  const has = a.name in values;
  const value = values[a.name] ?? "";
  const label = a.name === "#" ? "id" : a.name;
  const help = a.help + (a.default ? ` (default: ${a.default})` : "");

  if (a.kind === AttrKind.AttrFlag) {
    const box = el("input", { type: "checkbox", checked: has });
    box.onchange = () => set(a.name, box.checked ? "" : null, true);
    return el("label", { className: "check", title: help }, box, el("span", {}, label));
  }

  const input = el("input", { type: "text", value, spellcheck: false, placeholder: a.default ?? "" });
  if (a.kind === AttrKind.AttrChoice || a.name === "env") {
    const list = el("datalist", { id: "attr-" + a.name });
    for (const v of a.name === "env" ? (names.envs ?? []) : (a.values ?? [])) list.append(el("option", { value: v }));
    input.setAttribute("list", list.id);
    input.after(list);
    document.body.append(list);
  }

  if (a.bare) {
    // confirm and show-stderr: on or off, with an optional value.
    const box = el("input", { type: "checkbox", checked: has });
    input.disabled = !has;
    input.placeholder = a.name === "confirm" ? "Run this block?" : "always";
    const apply = () => {
      input.disabled = !box.checked;
      const v = input.value.trim();
      return set(a.name, box.checked ? v : null, box.checked && v === "");
    };
    box.onchange = apply;
    input.onchange = apply;
    return el("label", { className: "bare", title: help }, box, el("span", {}, label), input);
  }

  input.onchange = () => {
    const v = input.value.trim();
    return set(a.name, v === "" ? null : v);
  };
  return row(label, help, input);
}
