// Ansi draws a hyperlink a program wrote (OSC 8) as its text with its
// target after it, so a link in output can't hide where it goes.
// internal/build's tests bundle and run it with node, which has no DOM,
// so a stand-in document makes nodes that are only their text.

import { Ansi } from "../src/highlight.js";

function fail(msg: string): never {
  console.error(msg);
  (globalThis as unknown as { process: { exit(n: number): void } }).process.exit(1);
  throw new Error(msg);
}

type Fake = { textContent: string; className?: string; style: Record<string, string>; append(...c: (Fake | string)[]): void };
const node = (text: string): Fake => ({
  textContent: text,
  style: {},
  append(...c) {
    this.textContent += c.map((n) => (typeof n === "string" ? n : n.textContent)).join("");
  },
});
(globalThis as unknown as { document: unknown }).document = {
  createTextNode: node,
  createElement: () => node(""),
};

const draw = (...pieces: string[]) => {
  const a = new Ansi();
  const out = pieces.flatMap((p) => a.write(p));
  return [...out, ...a.end()].map((n) => n.textContent).join("");
};

const cases: [string[], string][] = [
  [["\x1b]8;;https://evil\x1b\\ok\x1b]8;;\x1b\\"], "ok (https://evil)"],
  [["\x1b]8;;https://x.io\x07https://x.io\x1b]8;;\x07\n"], "https://x.io\n"],
  [["\x1b]8;id=1;file://h/tmp/a\x1b\\\x1b[34ma\x1b[0m\x1b]8;;\x1b\\  b"], "a (file://h/tmp/a)  b"],
  [["\x1b]8;;https://a\x07one\x1b]8;;https://b\x07two"], "one (https://a)two (https://b)"],
  [["\x1b]8;;https://a\x07\x1b]8;;\x07"], "https://a"],
  [["\x1b]8;;htt", "ps://a\x07o", "ne\x1b]8;", ";\x07!"], "one (https://a)!"],
  [["\x1b]8;;https://aé\x07x"], "x"],
  [["\x1b]52;c;ZXZpbAo=\x07hello"], "hello"],
];
for (const [pieces, want] of cases) {
  const got = draw(...pieces);
  if (got !== want) fail(`Ansi(${JSON.stringify(pieces)}) = ${JSON.stringify(got)}, want ${JSON.stringify(want)}`);
}
