// copyMark reads the OSC sequences that offer text to copy, as Go's
// copyable.Split does. internal/build's tests bundle and run it with node.

import { copyLabel, copyMark } from "../src/copyable.js";

const fail = (msg: string) => {
  throw new Error(msg);
};
const eq = (a: unknown, b: unknown) => JSON.stringify(a) === JSON.stringify(b);

const b64 = (s: string) => btoa(String.fromCharCode(...new TextEncoder().encode(s)));
const cases: [string, unknown][] = [
  ["1337;CopyToClipboard=", { kind: "open" }],
  ["1337;CopyToClipboard=general", { kind: "open" }],
  ["1337;EndCopy", { kind: "close" }],
  ["52;c;" + b64("héllo"), { kind: "text", text: "héllo" }],
  ["52;c;aGk", { kind: "text", text: "hi" }],
  ["52;;aGk=", { kind: "text", text: "hi" }],
  ["52;c;?", null],
  ["52;c;", null],
  ["52;c;!!", null],
  ["52;c;//4=", null], // 0xff 0xfe isn't UTF-8
  ["0;title", null],
  ["1337;File=inline=1:AAAA", null],
];
for (const [body, want] of cases) {
  const got = copyMark(body);
  if (!eq(got, want)) fail(`copyMark(${JSON.stringify(body)}) = ${JSON.stringify(got)}, want ${JSON.stringify(want)}`);
}

if (copyLabel("abc\n  def", false) !== "Copy “abc def”") fail("copyLabel shown: " + copyLabel("abc\n  def", false));
if (!copyLabel("x".repeat(80), false).endsWith("…”")) fail("copyLabel long");
if (copyLabel("secret", true) !== "Copy the text the program offered, 6 characters") fail("copyLabel hidden: " + copyLabel("secret", true));
