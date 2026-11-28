// copyMark reads an OSC sequence's body (between ESC ] and BEL or ST)
// for text a program offers to copy, as package copyable does in Go.
// iTerm2's 1337;CopyToClipboard and 1337;EndCopy bracket shown text;
// 52 carries the text, base64 encoded, and shows nothing. A terminal
// writes the clipboard on either; mdsh never does from output, which is
// saved in the file, so it shows a copy button instead. It takes
// nothing from the page, so the frontend's tests run it under node.

export type CopyMark = { kind: "open" } | { kind: "close" } | { kind: "text"; text: string };

// maxText caps the text one OSC 52 sequence can offer, as Go's
// copyable.MaxText does.
const maxText = 1 << 20;

export function copyMark(body: string): CopyMark | null {
  if (body.startsWith("1337;CopyToClipboard")) return { kind: "open" };
  if (body === "1337;EndCopy") return { kind: "close" };
  if (!body.startsWith("52;")) return null;
  const parts = body.split(";");
  if (parts.length !== 3) return null;
  const text = osc52Text(parts[2]);
  return text === null ? null : { kind: "text", text };
}

// osc52Text is the text an OSC 52 payload sets the clipboard to, or null
// for a query (?), an empty payload, which clears the clipboard, and one
// that isn't base64 of UTF-8.
function osc52Text(data: string): string | null {
  if (!data || data === "?" || data.length > ((maxText + 2) / 3) * 4 + 4) return null;
  if (!/^[A-Za-z0-9+/]*={0,2}$/.test(data)) return null;
  const pad = data.replace(/=+$/, "");
  let bin: string;
  try {
    bin = atob(pad + "=".repeat((4 - (pad.length % 4)) % 4));
  } catch {
    return null;
  }
  if (!bin) return null;
  const bytes = Uint8Array.from(bin, (c) => c.charCodeAt(0));
  try {
    return new TextDecoder("utf-8", { fatal: true }).decode(bytes);
  } catch {
    return null;
  }
}

// copyLabel is what a copy button says, for a screen reader and as its
// tooltip: the start of the text, on one line, quoted.
export function copyLabel(text: string, hidden: boolean): string {
  if (hidden) {
    const n = [...text].length;
    return `Copy the text the program offered, ${n} ${n === 1 ? "character" : "characters"}`;
  }
  const one = text.replace(/\s+/g, " ").trim();
  return "Copy “" + (one.length > 40 ? one.slice(0, 39) + "…" : one) + "”";
}
