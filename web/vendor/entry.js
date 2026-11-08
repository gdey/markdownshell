// The packages bundled into codemirror.js. Each name the frontend
// imports from @codemirror/*, @replit/codemirror-vim, @lezer/highlight, the
// legacy modes or lang-markdown must be exported here; internal/vendorjs
// checks no two packages export the same name.
export * from "@codemirror/state";
export * from "@codemirror/view";
export * from "@codemirror/commands";
export * from "@codemirror/language";
export * from "@codemirror/autocomplete";
export * from "@codemirror/search";
export * from "@codemirror/lint";
export { shell } from "@codemirror/legacy-modes/mode/shell";
export { go } from "@codemirror/legacy-modes/mode/go";
export { python } from "@codemirror/legacy-modes/mode/python";
export { javascript, typescript, json } from "@codemirror/legacy-modes/mode/javascript";
export { yaml } from "@codemirror/legacy-modes/mode/yaml";
export { toml } from "@codemirror/legacy-modes/mode/toml";
export { rust } from "@codemirror/legacy-modes/mode/rust";
export { ruby } from "@codemirror/legacy-modes/mode/ruby";
export { lua } from "@codemirror/legacy-modes/mode/lua";
export { perl } from "@codemirror/legacy-modes/mode/perl";
export { swift } from "@codemirror/legacy-modes/mode/swift";
export { standardSQL } from "@codemirror/legacy-modes/mode/sql";
export { diff } from "@codemirror/legacy-modes/mode/diff";
export { dockerFile } from "@codemirror/legacy-modes/mode/dockerfile";
export { properties } from "@codemirror/legacy-modes/mode/properties";
export { css } from "@codemirror/legacy-modes/mode/css";
export { c, cpp, java, kotlin } from "@codemirror/legacy-modes/mode/clike";
export { vim, Vim, getCM } from "@replit/codemirror-vim";
export { markdown, markdownLanguage, markdownKeymap, insertNewlineContinueMarkup, deleteMarkupBackward } from "@codemirror/lang-markdown";
export { tags, highlightCode, classHighlighter } from "@lezer/highlight";
