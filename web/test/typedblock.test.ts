// A block typed in the History pane: the TypeScript that changes the
// prompt must agree with package history's Go. internal/build's tests
// bundle and run it with node.

import { blockPrompt, inBlock, isBlock, openBlock } from "../src/typedblock.js";

let failed = false;
const check = (what: string, got: unknown, want: unknown) => {
  if (JSON.stringify(got) !== JSON.stringify(want)) {
    console.error(`${what} = ${JSON.stringify(got)}, want ${JSON.stringify(want)}`);
    failed = true;
  }
};

check("openBlock(# ```bash)", openBlock("# ```bash"), { fence: "```", info: "bash" });
check("openBlock(#```sh {#a})", openBlock("  #```sh {#a}  "), { fence: "```", info: "sh {#a}" });
check("openBlock(# ~~~ py)", openBlock("# ~~~ py"), { fence: "~~~", info: "py" });
check("openBlock(# ```x```)", openBlock("# ```x```"), null);
check("openBlock(# a note)", openBlock("# a note"), null);

for (const [text, want] of [
  ["# ```sh", null], // not yet: enter hasn't started its body
  ["# ```sh\n", "sh"],
  ["# ```sh\necho hi", "sh"],
  ["# ```sh\necho hi\n```", null],
  ["# ```sh\n```", null],
  ["# ````md\n```", "md"],
  ["# ~~~sh\n```", "sh"],
  ["# ```sh\n    ```", "sh"],
  ["echo hi\n", null],
] as const) {
  check(`inBlock(${JSON.stringify(text)})`, inBlock(text), want);
}

check("isBlock(closed)", isBlock("# ```sh\nls\n```\n"), true);
check("isBlock(open)", isBlock("# ```sh\nls"), false);
check("isBlock(note)", isBlock("# a\n# b"), false);

for (const [info, want] of [
  ["bash", "```bash> "],
  ["sh {#setup}", "```sh> "],
  ["python id=x", "```python> "],
  ["{#only-an-id}", "```> "],
  ["", "```> "],
]) {
  check(`blockPrompt(${info})`, blockPrompt(info), want);
}

if (failed) (globalThis as unknown as { process: { exit(n: number): void } }).process.exit(1);
