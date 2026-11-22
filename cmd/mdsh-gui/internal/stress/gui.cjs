// gui.cjs types into long documents in mdsh-gui's real frontend and Go
// side, in headless Chromium, and reports how the view kept up. With
// --check it fails when typing makes the rendered view redraw more than
// the cards whose markdown changed, or redraw them differently from
// drawing the whole document afresh. Those counts don't depend on how
// fast the machine is, so CI can run them; the times it prints do.
//
// It needs mdsh-gui built in server mode, which serves the window to a
// browser, and Playwright. On Linux, without GTK, build it with cgo off;
// on macOS it needs cgo, so leave CGO_ENABLED out:
//
//	CGO_ENABLED=0 go build -tags server -o /tmp/mdsh-gui-server .
//	node internal/stress/gui.cjs --server /tmp/mdsh-gui-server [--check] \
//	    [--shapes runbook,prose] [--lines 10000,100000] [--slow 4] [--lazy 100]
//
// --slow slows the browser's CPU that many times, closer to a laptop.
// --lazy is the most cells a document has with every card drawn; past
// it, only the cards near the view are (lazyCells in app.ts). --check
// sets it to 100, so the short documents it types into are drawn so too.
// --check also jumps around each document, drawn that way, and checks
// that the cards drawn after each jump are those drawing every card
// draws, and that there are fewer than 500 of them.
// Each document is written by stressdoc, in the root module's
// internal/stress, so go must be on the PATH.

const { chromium } = require("playwright");
const { execFileSync, spawn } = require("child_process");
const fs = require("fs");
const os = require("os");
const path = require("path");

function flag(name, def) {
  const i = process.argv.indexOf("--" + name);
  if (i < 0) return def;
  const v = process.argv[i + 1];
  return v === undefined || v.startsWith("--") ? true : v;
}

const server = flag("server", process.env.MDSH_GUI_SERVER);
const check = !!flag("check", false);
const shapes = String(flag("shapes", "runbook,prose,output,inputs,flat")).split(",");
const sizes = String(flag("lines", check ? "3000" : "10000")).split(",").map(Number);
const slow = Number(flag("slow", 1));
const port = Number(flag("port", 8765));
const lazy = flag("lazy", check ? "100" : "");
if (!server) {
  console.error("gui.cjs: --server, the path to mdsh-gui built with -tags server, is needed");
  process.exit(2);
}

const gui = path.resolve(__dirname, "../..");
const root = path.resolve(gui, "../..");
const tmp = fs.mkdtempSync(path.join(os.tmpdir(), "mdsh-stress-"));
let failed = false;

function fail(msg) {
  failed = true;
  console.log("FAIL " + msg);
}

// serve starts the server on file and waits until it answers.
async function serve(file) {
  try {
    await fetch(`http://localhost:${port}/`);
    throw new Error(`something already answers on port ${port}; pick another with --port`);
  } catch (e) {
    if (e.message.startsWith("something")) throw e;
  }
  const home = path.join(tmp, "home");
  fs.mkdirSync(home, { recursive: true });
  const p = spawn(server, [file], { env: { ...process.env, HOME: home, WAILS_SERVER_PORT: String(port) }, stdio: "ignore" });
  for (let i = 0; i < 100; i++) {
    try {
      const r = await fetch(`http://localhost:${port}/`);
      if (r.ok) return p;
    } catch {
      // Not up yet.
    }
    await new Promise((r) => setTimeout(r, 100));
  }
  p.kill();
  throw new Error("the server didn't start");
}

// cards is the rendered view's cards drawn, by the line each starts on,
// as HTML without the marks of the selection and the cursor, which
// follow the user rather than the file. lines are the lines every cell
// starts on, drawn or not, and loading says some are still coming, or
// some cards wait for their bodies.
const cards = (page) =>
  page.evaluate(() => {
    const plain = (e) => {
      const c = e.cloneNode(true);
      for (const x of [c, ...c.querySelectorAll("*")]) {
        x.classList.remove("selected", "current", "lit", "dep-target", "dep-of");
        x.removeAttribute("aria-current");
        x.style?.removeProperty("--dep-rings");
        if (x.getAttribute("style") === "") x.removeAttribute("style");
        if (x.getAttribute("class") === "") x.removeAttribute("class");
      }
      return c.outerHTML;
    };
    const drawn = {};
    for (const e of document.querySelectorAll("#doc > section")) drawn[e.dataset.line] = plain(e);
    return { drawn, lines: window.mdshLines(), loading: window.mdshLoading() };
  });

// differ is the first card a draws otherwise than b, which draws every
// card, or "" if none does.
const differ = (a, b) => {
  if (a.lines.join() !== b.lines.join()) return `the cells start on lines ${a.lines.slice(0, 20)}…, not ${b.lines.slice(0, 20)}…`;
  for (const [line, html] of Object.entries(a.drawn)) {
    const other = b.drawn[line];
    if (other === html) continue;
    // From a little before where they part.
    let at = 0;
    while (other && html[at] === other[at]) at++;
    const from = Math.max(0, at - 60);
    return `card on line ${line}, from character ${from}:\n  ${html.slice(from, from + 300)}\n  ${other?.slice(from, from + 300)}`;
  }
  return "";
};

// settled is the cards, as cards has them, once every cell is in, the
// cards drawn have their bodies, and what is sized once laid out, such
// as a textbox, is: once they stay as they are. It fails, saying what,
// if they don't in time.
async function settled(page, what, tries = 100) {
  let got;
  let was = "";
  for (let k = 0; k < tries; k++) {
    await page.waitForTimeout(100 + Math.min(k, 20) * 20);
    got = await cards(page);
    const now = JSON.stringify(got.drawn);
    if (!got.loading && now === was) return got;
    was = now;
  }
  fail(`${what}: the cards drawn never settled`);
  return got;
}

// load loads the page, with lazy, and waits for its cards to settle.
async function load(page, lazy, what) {
  await page.goto(`http://localhost:${port}/index.html?tab=1${lazy ? "&lazy=" + lazy : ""}`);
  await page.waitForSelector("#doc > section", { timeout: 600000 });
  return settled(page, what, 1000);
}

// jumps scrolls page's rendered view to places picked at random and
// fails where the cards drawn there differ from full, a draw of every
// card, or there are 500 or more.
async function jumps(page, full, what) {
  let seed = 1;
  const rand = () => (seed = (seed * 48271) % 2147483647) / 2147483647;
  for (let k = 0; k < 20; k++) {
    const at = rand();
    await page.evaluate((at) => {
      const p = document.getElementById("doc-scroll");
      p.scrollTop = at * (p.scrollHeight - p.clientHeight);
    }, at);
    const got = await settled(page, `${what}: jump ${k}`);
    const n = Object.keys(got.drawn).length;
    const inView = await page.evaluate(() => {
      const p = document.getElementById("doc-scroll").getBoundingClientRect();
      return [...document.querySelectorAll("#doc > section")].some((e) => {
        const r = e.getBoundingClientRect();
        return r.height && r.bottom > p.top && r.top < p.bottom;
      });
    });
    if (n >= 500) fail(`${what}: ${n} cards drawn after jump ${k}; want fewer than 500`);
    if (!inView) fail(`${what}: no card in sight after jump ${k}, to ${at.toFixed(3)} of the way down`);
    const d = differ(got, full);
    if (d) fail(`${what}: after jump ${k}, to ${at.toFixed(3)} of the way down, ${d}`);
  }
}

// typeAt puts the source pane's cursor at the end of the first line
// past the middle of the document that ends with text.
const typeAt = (page, text) =>
  page.evaluate((text) => {
    const v = document.querySelector(".cm-content").cmTile.view;
    const d = v.state.doc;
    for (let n = Math.floor(d.lines / 2); n <= d.lines; n++) {
      const l = d.line(n);
      if (l.text.endsWith(text)) {
        v.dispatch({ selection: { anchor: l.to }, scrollIntoView: true });
        v.focus();
        return n;
      }
    }
    throw new Error("no line ending " + text);
  }, text);

async function run(shape, lines) {
  const file = path.join(tmp, `${shape}-${lines}.md`);
  fs.writeFileSync(file, execFileSync("go", ["run", "./internal/stress/stressdoc", "-shape", shape, "-lines", String(lines)], { cwd: root, maxBuffer: 1 << 30 }));
  const proc = await serve(file);
  const browser = await chromium.launch();
  try {
    const page = await browser.newPage({ viewport: { width: 1400, height: 900 } });
    page.on("pageerror", (e) => fail(`${shape}/${lines}: page error: ${e.message}`));
    const cdp = await page.context().newCDPSession(page);
    if (slow > 1) await cdp.send("Emulation.setCPUThrottlingRate", { rate: slow });
    const t0 = Date.now();
    await page.goto(`http://localhost:${port}/index.html?tab=1${lazy ? "&lazy=" + lazy : ""}`);
    await page.waitForSelector("#doc > section", { timeout: 600000 });
    const open = Date.now() - t0;
    await page.click("#view-split", { timeout: 600000 });
    await page.waitForSelector(".cm-content", { timeout: 600000 });
    await page.waitForTimeout(1500);
    await page.evaluate(() => {
      window.stress = { long: [], added: 0, removed: 0, draws: 0, drawnAt: 0, most: 0 };
      new PerformanceObserver((l) => {
        for (const e of l.getEntries()) window.stress.long.push(e.duration);
      }).observe({ type: "longtask" });
      // Spacers, and cards drawn as the view comes to them, are not
      // counted: they are drawn for scrolling, not typing.
      const counted = (e) => e.tagName === "SECTION" && !window.mdshPlaced.has(e);
      new MutationObserver((ms) => {
        let added = 0;
        for (const m of ms) {
          added += [...m.addedNodes].filter(counted).length;
          window.stress.removed += [...m.removedNodes].filter(counted).length;
        }
        window.stress.added += added;
        window.stress.most = Math.max(window.stress.most, added);
        window.stress.draws++;
        window.stress.drawnAt = performance.now();
      }).observe(document.getElementById("doc"), { childList: true });
    });
    // Three bursts of typing at the end of a prose line, each followed
    // by a pause long enough for the view to draw the edit. preview is
    // how long the view took to show a burst's last key, from just
    // before it was typed.
    await typeAt(page, " in it.");
    // The rendered view follows the source pane to where the typing is,
    // as it does once the source pane leads, so the cards typing draws
    // again are in it.
    await page.keyboard.press("End");
    await page.evaluate(() => {
      const v = document.querySelector(".cm-content").cmTile.view;
      v.scrollDOM.scrollTop -= 1;
    });
    await page.waitForTimeout(500);
    const keys = [];
    const previews = [];
    for (let burst = 0; burst < 3; burst++) {
      let last = 0;
      for (let k = 0; k < 5; k++) {
        const t = Date.now();
        last = await page.evaluate(() => performance.now());
        await page.keyboard.type("x");
        await page.evaluate(() => new Promise((r) => requestAnimationFrame(() => setTimeout(r))));
        keys.push(Date.now() - t);
        await page.waitForTimeout(80);
      }
      await page.waitForTimeout(1500 * slow);
      previews.push(await page.evaluate((last) => Math.max(0, Math.round(window.stress.drawnAt - last)), last));
    }
    const s = await page.evaluate(() => window.stress);
    const total = Math.round(s.long.reduce((a, b) => a + b, 0));
    const worst = Math.round(Math.max(0, ...s.long));
    keys.sort((a, b) => a - b);
    const p50 = keys[Math.floor(keys.length / 2)];
    const nCards = await page.evaluate(() => document.querySelectorAll("#doc > section").length);
    console.log(
      [shape, lines, nCards, `${open}ms`, `${p50}ms`, `${keys[keys.length - 1]}ms`, `${Math.max(...previews)}ms`, s.long.length, `${total}ms`, `${worst}ms`, `${s.added}/${s.removed}`].join("\t"),
    );
    // Each burst changes one paragraph, so each time the view draws it
    // should draw that one card again, taking one out and putting one
    // in, whether it draws after each key, as on a short document, or
    // once a pause, as on a long one.
    if (check && s.most > 1) fail(`${shape}/${lines}: typing in one paragraph redrew ${s.most} cards at once; want 1`);
    if (check) {
      // A new block below the paragraph, typed fence first, numbers every
      // block after it anew and, until its fence is closed, turns them
      // into its code. The cards below are kept all the same: those that
      // change are the paragraph's, the new block's and the next one's,
      // and while the fence is open it may draw the few cards up to the
      // next fence as code, but it waits for the typing to settle
      // rather than draw more than 30 (manyCards in app.ts).
      await page.evaluate(() => Object.assign(window.stress, { added: 0, removed: 0, most: 0 }));
      for (const k of "\n\n```text\ncaptured output\n```") {
        await page.keyboard.type(k);
        await page.waitForTimeout(100);
      }
      await page.waitForTimeout(2500 * slow);
      const f = await page.evaluate(() => window.stress);
      if (f.most > 30) fail(`${shape}/${lines}: typing a new block redrew ${f.most} cards at once; want 30 at most`);
    }
    if (check) {
      // What typing drew must be what drawing it all afresh draws, every
      // card.
      const typed = await settled(page, `${shape}/${lines}: typing`);
      const all = await browser.newPage({ viewport: { width: 1400, height: 900 } });
      all.on("pageerror", (e) => fail(`${shape}/${lines}: page error drawing every card: ${e.message}`));
      const full = await load(all, "1000000", `${shape}/${lines}: every card`);
      const d = differ(typed, full);
      if (d) fail(`${shape}/${lines}: after typing, ${d}`);
      // And jumping around a fresh page draws them as that does.
      const fresh = await browser.newPage({ viewport: { width: 1400, height: 900 } });
      fresh.on("pageerror", (e) => fail(`${shape}/${lines}: page error jumping: ${e.message}`));
      await load(fresh, lazy, `${shape}/${lines}: jumping`);
      await jumps(fresh, full, `${shape}/${lines}`);
    }
  } finally {
    await browser.close();
    proc.kill();
    await new Promise((r) => proc.on("exit", r));
  }
}

(async () => {
  console.log(["shape", "lines", "cards", "open", "key p50", "key max", "preview", "long tasks", "total", "worst", "cards drawn/removed"].join("\t"));
  for (const lines of sizes) for (const shape of shapes) await run(shape, lines);
  fs.rmSync(tmp, { recursive: true, force: true });
  if (failed) process.exit(1);
})().catch((e) => {
  console.error(e);
  process.exit(1);
});
