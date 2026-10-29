import * as wails from "/wails/runtime.js";

const call = (method, ...args) => wails.Call.ByName("main.App." + method, ...args);
const $ = (id) => document.getElementById(id);

let s = await call("Settings");

function draw() {
  $("autosave").checked = s.autosave;
  $("shell").value = s.shell;
  $("start-mode").value = s.startFolder ? "fixed" : "last";
  $("start-folder").value = s.startFolder;
  $("fixed").hidden = !s.startFolder && $("start-mode").value !== "fixed";
}

// Changes save as they are made, like a settings window should.
async function save() {
  try {
    await call("SetSettings", s);
    $("error").hidden = true;
  } catch (err) {
    $("error").textContent = String(err?.message ?? err);
    $("error").hidden = false;
  }
}

async function choose() {
  const dir = await call("ChooseFolder");
  if (dir) {
    s.startFolder = dir;
    await save();
  }
  draw();
}

$("autosave").onchange = () => ((s.autosave = $("autosave").checked), save());
$("shell").onchange = () => ((s.shell = $("shell").value.trim()), save());
$("start-mode").onchange = async () => {
  if ($("start-mode").value === "last") {
    s.startFolder = "";
    await save();
    draw();
  } else {
    $("fixed").hidden = false;
    if (!s.startFolder) await choose();
  }
};
$("choose").onclick = choose;

draw();
