// Heights must add up as a plain loop over the cells does, through
// changes. internal/build's tests bundle and run it with node.

import { Heights } from "../src/heights.js";

const fail = (msg: string) => {
  throw new Error(msg);
};

let seed = 7;
const rand = (n: number) => {
  seed = (seed * 1103515245 + 12345) % 2147483648;
  return seed % n;
};

for (const n of [0, 1, 2, 3, 7, 64, 1000]) {
  const plain = Array.from({ length: n }, () => (rand(4) ? rand(300) : 0));
  const h = new Heights(plain);
  for (let round = 0; round < 200; round++) {
    if (n && round % 2) {
      const i = rand(n);
      plain[i] = rand(3) ? rand(300) : 0;
      h.set(i, plain[i]);
    }
    const from = rand(n + 1);
    const to = from + rand(n + 1 - from);
    const want = plain.slice(from, to).reduce((a, b) => a + b, 0);
    if (h.sum(from, to) !== want) fail(`n=${n}: sum(${from}, ${to}) = ${h.sum(from, to)}, want ${want}`);
    const total = plain.reduce((a, b) => a + b, 0);
    if (h.total() !== total) fail(`n=${n}: total ${h.total()}, want ${total}`);
    const y = rand(total + 50);
    let at = -1;
    let acc = 0;
    for (let i = 0; i < n; i++) {
      acc += plain[i];
      if (acc > y) {
        at = i;
        break;
      }
    }
    if (n && at < 0) at = n - 1;
    if (h.find(y) !== at) fail(`n=${n}: find(${y}) = ${h.find(y)}, want ${at}`);
  }
}
console.log("heights ok");
