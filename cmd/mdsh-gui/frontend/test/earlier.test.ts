// runTime says when an earlier run was the way a person would, in local
// time, and timeAgo how long before now. internal/build's tests bundle and run it with node.

import { earlierLabel, runTime, timeAgo } from "../src/earlier.js";

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

// timeAgo counts back from now in the largest whole unit that fits.
const back = (secs: number) => new Date(now.getTime() - secs * 1000).toISOString();
const min = 60, hour = 3600, day = 86400;
const agoCases: [string, string][] = [
  [back(-5), "just now"],
  [back(0), "just now"],
  [back(1), "1 sec ago"],
  [back(42), "42 secs ago"],
  [back(59), "59 secs ago"],
  [back(min), "1 min ago"],
  [back(3 * min + 10), "3 mins ago"],
  [back(59 * min + 59), "59 mins ago"],
  [back(hour), "1 hour ago"],
  [back(23 * hour + 59 * min), "23 hours ago"],
  [back(day), "1 day ago"],
  [back(6 * day), "6 days ago"],
  [back(7 * day), "1 week ago"],
  [back(29 * day), "4 weeks ago"],
  [back(30 * day), "1 month ago"],
  [back(364 * day), "11 months ago"],
  [back(365 * day), "1 year ago"],
  [back(800 * day), "2 years ago"],
  ["not a time", ""],
];
for (const [at, want] of agoCases) {
  const got = timeAgo(at, now);
  if (got !== want) fail(`timeAgo(${at}) = ${JSON.stringify(got)}, want ${JSON.stringify(want)}`);
}
console.log("ok");
