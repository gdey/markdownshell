// The Go calls and types the frontend uses, from the generated bindings.
import * as Bound from "./bindings/github.com/gdey/markdownshell/ui/app.js";

// App is the Go calls, each of which, when it fails, also logs to the
// Console which call it was: Wails' own line says only "Bound method
// returned an error", so a bug report couldn't tell a failed Save from
// any other call. The caller still gets the failure as before.
export const App: typeof Bound = Object.fromEntries(
  Object.entries(Bound).map(([name, fn]) => {
    if (typeof fn !== "function" || name === "LogCallFailed") return [name, fn];
    return [
      name,
      (...args: unknown[]) => {
        const p = (fn as (...a: unknown[]) => Promise<unknown>)(...args);
        // Watch it without changing what the caller gets.
        p.then(undefined, (err: unknown) => {
          // A call cancelled on purpose isn't a failure.
          if (err && typeof err === "object" && "name" in err && (err as { name: unknown }).name === "CancelError") return;
          Bound.LogCallFailed(name, message(err)).catch(() => {});
        });
        return p;
      },
    ];
  }),
) as typeof Bound;
export type { ConsoleEntry, Doc, OutsideFiles, ReportStart, Start, Tab, Tabs } from "./bindings/github.com/gdey/markdownshell/ui/models.js";
export type { AuditEvent, Cell, Deleted, Heads, EarlierRun, Fold, FrontMatter, HTTPCell, Move, Names, Problem, Report, RunOptions, SourceBlock } from "./bindings/github.com/gdey/markdownshell/notebook/models.js";
export type { Settings } from "./bindings/github.com/gdey/markdownshell/settings/models.js";
export type { File } from "./bindings/github.com/gdey/markdownshell/library/models.js";
export type { Proc, ShellProcs } from "./bindings/github.com/gdey/markdownshell/env/models.js";
export { Format } from "./bindings/github.com/gdey/markdownshell/report/models.js";
export { AttrKind } from "./bindings/github.com/gdey/markdownshell/doc/models.js";
export type { AttrSpec } from "./bindings/github.com/gdey/markdownshell/doc/models.js";

// message turns a rejected call into text to show.
export function message(err: unknown): string {
  if (err && typeof err === "object" && "message" in err) return String((err as { message: unknown }).message);
  return String(err);
}
