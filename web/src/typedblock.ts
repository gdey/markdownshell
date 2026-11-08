// A block typed in the History pane starts with a line such as
// "# ```bash {#setup}": a # and a fence with its info string. The lines
// after it are kept, not run, until a line of just a fence as long as the
// opening one, or longer, ends it; then it is added to the end of the
// file. These mirror package history's OpenBlock, InBlock and
// BlockPrompt, so the prompt can change as the block is typed; Go
// decides what enter does.

const openFence = /^[ \t]*#[ \t]*(`{3,}|~{3,})[ \t]*(.*?)[ \t]*$/;

// openBlock returns the fence and info string of line if it opens a
// block.
export function openBlock(line: string): { fence: string; info: string } | null {
  const m = openFence.exec(line);
  if (!m || (m[1][0] === "`" && m[2].includes("`"))) return null;
  return { fence: m[1], info: m[2] };
}

// closes reports whether line closes a block opened with fence.
function closes(line: string, fence: string): boolean {
  const l = line.replace(/[ \t]+$/, "");
  const lead = l.length - l.replace(/^ +/, "").length;
  if (lead > 3) return false;
  const rest = l.slice(lead);
  return rest.length >= fence.length && [...rest].every((c) => c === fence[0]);
}

// inBlock returns the info string of the block text opens and doesn't
// close yet, once enter has started its second line, or null.
export function inBlock(text: string): string | null {
  const lines = text.split("\n");
  const open = openBlock(lines[0]);
  if (!open || lines.length < 2) return null;
  return closes(lines[lines.length - 1], open.fence) ? null : open.info;
}

// isBlock reports whether text is a whole block, opened and closed.
export function isBlock(text: string): boolean {
  const lines = text.replace(/\n+$/, "").split("\n");
  const open = openBlock(lines[0]);
  return !!open && lines.length > 1 && closes(lines[lines.length - 1], open.fence);
}

// blockPrompt is the prompt on the lines of a block: the fence and the
// block's language, as "```bash> ".
export function blockPrompt(info: string): string {
  let lang = info.replace(/^\{+/, "").split(" ")[0].split("}")[0];
  if (lang.startsWith("#") || lang.startsWith(".") || lang.includes("=")) lang = "";
  return "```" + lang + "> ";
}
