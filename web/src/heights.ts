// Heights holds how tall each cell's card is, or is taken to be, in the
// rendered view, and adds them up quickly: the view of a long document
// draws only the cards near what is in sight, between spacers as tall as
// the cards they stand for, and finding the cell at a scroll position,
// or the position of a cell, can't go through every cell each time.
// It is a Fenwick tree: each sum and each change takes time in the log
// of the number of cells.

export class Heights {
  // v holds each cell's height, t the tree's partial sums, from 1.
  private v: Float64Array;
  private t: Float64Array;

  constructor(heights: ArrayLike<number> = []) {
    const n = heights.length;
    this.v = Float64Array.from(heights);
    this.t = new Float64Array(n + 1);
    for (let i = 1; i <= n; i++) {
      this.t[i] += this.v[i - 1];
      const up = i + (i & -i);
      if (up <= n) this.t[up] += this.t[i];
    }
  }

  get length(): number {
    return this.v.length;
  }

  get(i: number): number {
    return this.v[i] ?? 0;
  }

  // set makes cell i h tall.
  set(i: number, h: number) {
    const d = h - this.v[i];
    if (!d || i < 0 || i >= this.v.length) return;
    this.v[i] = h;
    for (let k = i + 1; k < this.t.length; k += k & -k) this.t[k] += d;
  }

  // sum is the height of cells from up to to, not counting to; with one
  // argument, of the cells before it.
  sum(from: number, to?: number): number {
    if (to === undefined) [from, to] = [0, from];
    return this.before(to) - this.before(from);
  }

  private before(i: number): number {
    let s = 0;
    for (let k = Math.min(i, this.v.length); k > 0; k -= k & -k) s += this.t[k];
    return s;
  }

  total(): number {
    return this.before(this.v.length);
  }

  // find is the cell that y, down from the top of the first, falls in:
  // the first whose bottom is below y. Past the end it is the last
  // cell, and with no cells -1. A cell 0 tall is never it, but where
  // every cell is, the last.
  find(y: number): number {
    const n = this.v.length;
    if (!n) return -1;
    let i = 0;
    let rest = y;
    let step = 1;
    while (step * 2 <= n) step *= 2;
    for (; step; step >>= 1) {
      const k = i + step;
      if (k <= n && this.t[k] <= rest) {
        i = k;
        rest -= this.t[k];
      }
    }
    return Math.min(i, n - 1);
  }
}
