// A program's OSC 52 copy is decoded to the text it copies, and reads,
// bad data and too much of it copy nothing.

import { maxCopy, osc52Text } from "../src/osc52.js";

function fail(msg: string): never {
  console.error(msg);
  (globalThis as unknown as { process: { exit(n: number): void } }).process.exit(1);
  throw new Error(msg);
}

const b64 = (s: string) => btoa(String.fromCharCode(...new TextEncoder().encode(s)));

const cases: [string, string | null][] = [
  ["c;" + b64("hi\n"), "hi\n"],
  // Any selection, or none, is the one clipboard.
  ["p;" + b64("x"), "x"],
  ["s0;" + b64("x"), "x"],
  [";" + b64("x"), "x"],
  ["c;" + b64("héllo ✓"), "héllo ✓"],
  // A read is never answered.
  ["c;?", null],
  // Clearing leaves the clipboard as it is.
  ["c;", null],
  ["no semicolon", null],
  ["z;" + b64("x"), null],
  ["c;not base64!", null],
  // Not UTF-8.
  ["c;" + btoa("\xff\xfe"), null],
  ["c;" + btoa("x".repeat(maxCopy + 3)), null],
];
for (const [data, want] of cases) {
  const got = osc52Text(data);
  if (got !== want) fail(`osc52Text(${JSON.stringify(data.slice(0, 40))}) = ${JSON.stringify(got)}, want ${JSON.stringify(want)}`);
}
if (osc52Text("c;" + btoa("x".repeat(maxCopy))) !== "x".repeat(maxCopy)) fail("maxCopy bytes should copy");
