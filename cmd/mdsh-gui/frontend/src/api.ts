// The Go calls and types the frontend uses, from the generated bindings.
export * as App from "./bindings/github.com/gdey/markdownshell/cmd/mdsh-gui/app.js";
export type { Doc, ReportStart, Start, Tab, Tabs } from "./bindings/github.com/gdey/markdownshell/cmd/mdsh-gui/models.js";
export type { Cell, Fold, Move, Names, Problem, Report, RunOptions } from "./bindings/github.com/gdey/markdownshell/cmd/mdsh-gui/notebook/models.js";
export type { Settings } from "./bindings/github.com/gdey/markdownshell/cmd/mdsh-gui/settings/models.js";
export type { File } from "./bindings/github.com/gdey/markdownshell/cmd/mdsh-gui/library/models.js";
export { Format } from "./bindings/github.com/gdey/markdownshell/report/models.js";
export { AttrKind } from "./bindings/github.com/gdey/markdownshell/doc/models.js";
export type { AttrSpec } from "./bindings/github.com/gdey/markdownshell/doc/models.js";

// message turns a rejected call into text to show.
export function message(err: unknown): string {
  if (err && typeof err === "object" && "message" in err) return String((err as { message: unknown }).message);
  return String(err);
}
