// The packages bundled into codemirror.js. Each name the frontend
// imports from @codemirror/*, @replit/codemirror-vim or the legacy shell
// mode must be exported here; internal/vendorjs checks no two packages
// export the same name.
export * from "@codemirror/state";
export * from "@codemirror/view";
export * from "@codemirror/commands";
export * from "@codemirror/language";
export * from "@codemirror/autocomplete";
export * from "@codemirror/search";
export * from "@codemirror/lint";
export { shell } from "@codemirror/legacy-modes/mode/shell";
export { vim, Vim, getCM } from "@replit/codemirror-vim";
