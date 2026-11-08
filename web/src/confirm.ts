// confirmShells asks before the app ends shells that blocks have set
// things up in: the variables, functions and aliases they set go with
// them, so it never happens without the user's say.

import { $ } from "./dom.js";

let pending: Promise<boolean> | null = null;

// confirmShells shows the question, title and text, with ok as the
// button that goes ahead, and resolves true if it was pressed. A
// question already showing is answered for both.
export function confirmShells(title: string, text: string, ok: string): Promise<boolean> {
  if (pending) return pending;
  const dialog = $<HTMLDialogElement>("shell-dialog");
  $("shell-title").textContent = title;
  $("shell-text").textContent = text;
  $("shell-ok").textContent = ok;
  dialog.returnValue = "";
  dialog.showModal();
  // Cancel has the focus, so enter doesn't end a shell by accident.
  dialog.querySelector<HTMLButtonElement>("button[value=cancel]")!.focus();
  pending = new Promise((resolve) => {
    dialog.onclose = () => {
      pending = null;
      resolve(dialog.returnValue === "ok");
    };
  });
  return pending;
}

// dismissShells closes the question, as when what it asked about is
// over: answered no.
export function dismissShells() {
  const dialog = $<HTMLDialogElement>("shell-dialog");
  if (dialog.open) dialog.close("cancel");
}

// lostWarning is what a restart costs, for the questions.
export const lostWarning = "The variables, functions and aliases that blocks set in them will be gone.";
