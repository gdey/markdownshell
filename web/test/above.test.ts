// What aboveSequence writes must put a message above the shell's
// prompt and leave the prompt, what is typed and the cursor together,
// so what the shell writes next lands where it means it to. This plays
// the sequence on a small terminal, of the escapes it uses, and checks
// the screen. internal/build's tests bundle and run it with node.

import { aboveSequence, type Screen } from "../src/above.js";

const fail = (msg: string) => {
  throw new Error(msg);
};

// Term is a terminal of the few escapes aboveSequence writes, with
// autowrap, LF without CR (as the live pane's), and a scrollback.
class Term {
  lines: string[][];
  scrollback: string[] = [];
  x = 0;
  y = 0;
  wrap = false;
  saved = { x: 0, y: 0 };
  constructor(
    public rows: number,
    public cols: number,
  ) {
    this.lines = Array.from({ length: rows }, () => this.blank());
  }
  blank = () => Array<string>(this.cols).fill(" ");
  lf() {
    if (this.y === this.rows - 1) {
      this.scrollback.push(this.lines.shift()!.join("").trimEnd());
      this.lines.push(this.blank());
    } else this.y++;
  }
  write(s: string) {
    for (let i = 0; i < s.length; i++) {
      const c = s[i];
      if (c === "\x1b") {
        if (s[i + 1] === "7") {
          this.saved = { x: this.x, y: this.y };
          i++;
          continue;
        }
        if (s[i + 1] === "8") {
          ({ x: this.x, y: this.y } = this.saved);
          this.wrap = false;
          i++;
          continue;
        }
        const m = /^\x1b\[([0-9;]*)([A-Za-z])/.exec(s.slice(i));
        if (!m) fail(`escape not known: ${JSON.stringify(s.slice(i, i + 8))}`);
        i += m![0].length - 1;
        const args = m![1].split(";").map((a) => (a === "" ? 1 : Number(a)));
        this.wrap = false;
        switch (m![2]) {
          case "H":
            this.y = Math.min(this.rows, args[0] ?? 1) - 1;
            this.x = Math.min(this.cols, args[1] ?? 1) - 1;
            break;
          case "A":
            this.y = Math.max(0, this.y - args[0]);
            break;
          case "B":
            this.y = Math.min(this.rows - 1, this.y + args[0]);
            break;
          case "L":
            for (let n = 0; n < args[0]; n++) {
              this.lines.splice(this.y, 0, this.blank());
              this.lines.pop();
            }
            this.x = 0;
            break;
          case "m":
            break;
          default:
            fail(`escape not known: ${m![0]}`);
        }
        continue;
      }
      if (c === "\n") {
        this.lf();
        continue;
      }
      if (c === "\r") {
        this.x = 0;
        continue;
      }
      if (this.wrap) {
        this.x = 0;
        this.lf();
        this.wrap = false;
      }
      this.lines[this.y][this.x] = c;
      if (this.x === this.cols - 1) this.wrap = true;
      else this.x++;
    }
  }
  text = () => this.lines.map((l) => l.join("").trimEnd());
}

// shell draws a screen: some lines, then the prompt and what's typed,
// with the cursor at its end, and says where things are on it.
function shell(rows: number, cols: number, before: string[], prompt: string): { t: Term; s: Screen } {
  const t = new Term(rows, cols);
  t.write(before.map((l) => l + "\r\n").join(""));
  const top = t.y;
  t.write(prompt);
  let free = 0;
  while (rows - 1 - free > t.y && t.lines[rows - 1 - free].join("").trim() === "") free++;
  return { t, s: { rows, cols, x: t.x, y: t.y, top, last: t.y, free } };
}

const eq = (got: unknown, want: unknown, what: string) => {
  if (JSON.stringify(got) !== JSON.stringify(want)) fail(`${what}:\ngot  ${JSON.stringify(got)}\nwant ${JSON.stringify(want)}`);
};

// Room under the prompt: the prompt moves down, nothing scrolls.
{
  const { t, s } = shell(6, 20, ["out 1"], "$ echo hi");
  t.write(aboveSequence(["(note)"], s)!);
  t.write("!");
  eq(t.text(), ["out 1", "(note)", "$ echo hi!", "", "", ""], "room under the prompt");
  eq(t.scrollback, [], "room under the prompt, scrollback");
}

// A full screen: what's over the prompt scrolls up, the prompt stays.
{
  const { t, s } = shell(4, 20, ["out 1", "out 2", "out 3"], "$ ls");
  t.write(aboveSequence(["(one)", "(two)"], s)!);
  t.write("x");
  eq(t.text(), ["out 3", "(one)", "(two)", "$ lsx"], "a full screen");
  eq(t.scrollback, ["out 1", "out 2"], "a full screen, scrollback");
}

// Some room, not enough: both.
{
  const { t, s } = shell(5, 20, ["a", "b", "c"], "$ ");
  t.write(aboveSequence(["1", "2", "3"], s)!);
  t.write("y");
  eq(t.text(), ["c", "1", "2", "3", "$ y"], "some room");
  eq(t.scrollback, ["a", "b"], "some room, scrollback");
}

// A prompt of two lines, the cursor in the middle of what's typed, and
// a message that wraps.
{
  const { t, s } = shell(6, 10, ["o"], "~/src\r\n$ abcdef");
  const back = { ...s, x: s.x - 3 };
  t.x = back.x;
  t.write(aboveSequence(["0123456789AB"], back)!);
  t.write("Z");
  eq(t.text(), ["o", "0123456789", "AB", "~/src", "$ abcZef", ""], "two-line prompt");
}

// A prompt that wraps: the cursor on its second row.
{
  const { t, s } = shell(5, 6, ["o"], "$ abcdefgh");
  eq(s.y - s.top, 1, "the input wrapped");
  t.write(aboveSequence(["m"], s)!);
  t.write("i");
  eq(t.text(), ["o", "m", "$ abcd", "efghi", ""], "a wrapped prompt");
}

// No room without scrolling the prompt away.
{
  const { s } = shell(3, 4, [], "$ abcdefghij");
  eq(aboveSequence(["m"], s), null, "a prompt that fills the screen");
}
