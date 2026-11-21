// The page's reading of the heads Go sends (notebook.HeadList) must give
// back what Go sent. internal/build's tests bundle and run it with node.

import { decodeHeads, partial, sameHead } from "../src/heads.js";

const fail = (msg: string) => {
  throw new Error(msg);
};

// Two heads, as notebook.HeadList writes them: a heading, then a block
// that moves, with depends.
const cells = decodeHeads([
  [1, 0, 35, "h1", -1, 0, 0, 1, 0, 0, 0, 3, "", "", "Top & tail"],
  [9, 239, 367, "b1", 0, 1, 1 | 1024, 0, 2, 9, 0, 0, "step1", "", "", ["step0"], 13],
  [2, 400, 10, "p1"],
]);
const [h, b, p] = cells;
if (h.heading !== 1 || h.title !== "Top & tail" || h.line !== 1 || h.from !== 0 || h.to !== 35 || h.sectionTo !== 3 || h.block !== -1) fail("heading: " + JSON.stringify(h));
if (b.block !== 0 || b.line !== 10 || b.from !== 239 || b.to !== 606 || !b.ran || !b.movable || b.lastLine !== 23 || b.name !== "step1" || b.depends?.[0] !== "step0" || b.codeLines !== 2 || b.outputLines !== 9) fail("block: " + JSON.stringify(b));
if (p.block !== -1 || p.line !== 12 || p.from !== 639 || p.to !== 649 || p.heading) fail("prose: " + JSON.stringify(p));
if (!cells.every((c) => partial.has(c))) fail("not partial");
if (!sameHead(b, { ...b }) || sameHead(b, { ...b, hash: "x" }) || sameHead(b, { ...b, ran: false })) fail("sameHead");
console.log("heads ok");
