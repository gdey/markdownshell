// runTime says when an earlier run was the way a person would, in local
// time. internal/build's tests bundle and run it with node.

import { earlierLabel, runTime } from "../src/earlier.js";

const fail = (msg: string) => {
  throw new Error(msg);
};

// Times are built in local time, so the test reads the same anywhere.
const local = (y: number, mo: number, d: number, h: number, mi: number) => new Date(y, mo - 1, d, h, mi).toISOString();
const now = new Date(2026, 10, 17, 9, 30); // Tue Nov 17 2026, 09:30

const cases: [string, string][] = [
  [local(2026, 11, 17, 8, 5), "today 08:05"],
  [local(2026, 11, 16, 19, 2), "yesterday 19:02"],
  [local(2026, 11, 13, 23, 59), "Fri 23:59"],
  [local(2026, 11, 10, 7, 0), "Nov 10 07:00"],
  [local(2025, 12, 31, 12, 0), "Dec 31, 2025 12:00"],
  ["not a time", "not a time"],
];
for (const [at, want] of cases) {
  const got = runTime(at, now);
  if (got !== want) fail(`runTime(${at}) = ${JSON.stringify(got)}, want ${JSON.stringify(want)}`);
}
if (earlierLabel(1) !== "1 earlier run" || earlierLabel(3) !== "3 earlier runs") fail("earlierLabel");
console.log("ok");
