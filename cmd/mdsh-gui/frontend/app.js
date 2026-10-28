import * as wails from "/wails/runtime.js";

const call = (method, ...args) => wails.Call.ByName("main.App." + method, ...args);
const $ = (id) => document.getElementById(id);

let running = false;
let dirty = false;

function status(text) {
  $("status").textContent = text;
}

function busy(on) {
  running = on;
  $("stop").disabled = !on;
  $("run-all").disabled = on;
  $("save").disabled = on || !dirty;
  for (const b of document.querySelectorAll(".block button")) b.disabled = on;
}

// draw lays the document out again from the cells Go sends.
async function draw() {
  const file = await call("File");
  $("file").textContent = file.split(/[\\/]/).pop();
  $("file").title = file;
  if (!file) return;
  const cells = await call("Cells");
  const main = $("doc");
  main.replaceChildren();
  for (const c of cells) {
    if (c.block < 0) {
      const prose = document.createElement("section");
      prose.className = "prose";
      prose.innerHTML = c.html ?? ""; // goldmark's HTML, with raw HTML left out
      main.append(prose);
      continue;
    }
    const block = document.createElement("section");
    block.className = "block" + (c.failed ? " failed" : c.ran ? " ran" : "");
    block.id = "block-" + c.block;

    const head = document.createElement("div");
    head.className = "head";
    const name = document.createElement("span");
    name.textContent = c.id + " · " + c.lang + (c.ran ? " · exit " + c.status : "");
    const runOne = document.createElement("button");
    runOne.textContent = "Run";
    runOne.onclick = () => start("RunBlock", c.block);
    const runOn = document.createElement("button");
    runOn.textContent = "Run from here";
    runOn.onclick = () => start("RunFrom", c.block);
    head.append(name, runOne, runOn);

    const code = document.createElement("pre");
    code.className = "code";
    code.textContent = c.body;

    const out = document.createElement("div");
    out.className = "output";
    out.innerHTML = c.output ?? "";

    block.append(head, code, out);
    main.append(block);
  }
  $("run-all").disabled = cells.every((c) => c.block < 0);
}

async function start(method, block) {
  if (running) return;
  busy(true);
  status("running");
  try {
    await call(method, block);
    status("");
  } catch (err) {
    status(String(err.message ?? err));
  }
  busy(false);
  $("confirm").hidden = true;
  await draw();
}

wails.Events.On("block", (ev) => {
  const e = ev.data;
  const block = $("block-" + e.block);
  if (!block) return;
  const out = block.querySelector(".output");
  switch (e.kind) {
    case "started":
      out.replaceChildren(Object.assign(document.createElement("pre"), { className: "live" }));
      block.scrollIntoView({ block: "nearest" });
      break;
    case "stdout":
    case "stderr": {
      const live = out.querySelector(".live");
      if (!live) break;
      const span = document.createElement("span");
      span.className = e.kind;
      span.textContent = e.data;
      live.append(span);
      break;
    }
    case "done":
      dirty = true;
      break;
  }
});

wails.Events.On("confirm", (ev) => {
  $("question").textContent = ev.data.question;
  $("confirm").hidden = false;
  $("block-" + ev.data.block)?.scrollIntoView({ block: "nearest" });
});

for (const b of document.querySelectorAll("#confirm button")) {
  b.onclick = async () => {
    $("confirm").hidden = true;
    await call("Answer", b.dataset.answer);
  };
}

$("open").onclick = async () => {
  try {
    if (await call("Open")) {
      dirty = false;
      await draw();
    }
  } catch (err) {
    status(String(err.message ?? err));
  }
};
$("run-all").onclick = () => start("RunFrom", 0);
$("stop").onclick = () => call("Stop");
$("save").onclick = async () => {
  try {
    await call("Save");
    dirty = false;
    $("save").disabled = true;
    status("saved");
  } catch (err) {
    status(String(err.message ?? err));
  }
};

draw();
