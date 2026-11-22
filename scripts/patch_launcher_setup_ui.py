"""The page's half of the first-run setup (launcher/ui/app.js), and the
release's defaults: the bundled runtime, no paths of the author's PC."""
import sys

p = sys.argv[1]
s = open(p, encoding='utf-8').read()


def rep(old, new):
    global s
    assert s.count(old) == 1, (old[:70], s.count(old))
    s = s.replace(old, new)


rep('''  paths: {
    runtime: "D:\\\\tools\\\\winapk\\\\dev\\\\bin\\\\winapk-runtime.exe",
    runtimeData: "D:\\\\tools\\\\winapk\\\\data-nier",
  },''', '''  // Empty: the bundled runtime (runtime\\\\bin) and its data (runtime-data).
  paths: { runtime: "", runtimeData: "" },''')

rep('''const SCENES = { field: "img/field.jpg", battle: "img/battle.jpg", dialog: "img/dialog.jpg" };''',
    '''// Screenshots to place points on, if the player saves some there (none ship).
const SCENES = { field: "game/field.jpg", battle: "game/battle.jpg", dialog: "game/dialog.jpg" };''')

rep('''    NIER_RUNTIME: settings.paths.runtime,
    NIER_RUNTIME_DATA: settings.paths.runtimeData,
  };''', '''  };
  if (settings.paths.runtime) env.NIER_RUNTIME = settings.paths.runtime;
  if (settings.paths.runtimeData) env.NIER_RUNTIME_DATA = settings.paths.runtimeData;''')

rep('''  } else if (kind === "info") {
    const info = JSON.parse(text || "{}");''', '''  } else if (kind === "info") {
    const info = JSON.parse(text || "{}");
    document.body.classList.toggle("setup", !info.ready);
    if (!info.ready) return;''')

rep('''  } else if (kind === "status") {''', '''  } else if (kind === "picked") {
    const [what, ...where] = text.split(":");
    picked(what, where.join(":"));
  } else if (kind === "setup-line") {
    setupLine(text);
  } else if (kind === "setup-exit") {
    setupRunning = false;
    if (text === "0") {
      setupBar.style.width = "100%";
      send("info");
    } else {
      setupGo.disabled = !(choice.apk && choice.assets && choice.master);
    }''')

rep('''// ---- pages -------------------------------------------------------------------------''', '''// ---- first-run setup -----------------------------------------------------------------
// The port ships without the game: NieR.exe runs tools/setup_game.py on the
// player's three files and passes its lines on ("step:i/n:what", "progress:n",
// "note:...", "error:...", "done").
const choice = { apk: "", assets: "", master: "" };
const setupGo = document.getElementById("setup-go");
const setupBar = document.getElementById("setup-bar");
let setupRunning = false;
let setupStep = [0, 1];
document.querySelectorAll("[data-pick]").forEach((b) => b.addEventListener("click", () => send("pick\\n" + b.dataset.pick)));
function picked(what, where) {
  const kind = what === "assets-folder" ? "assets" : what;
  choice[kind] = where;
  const label = document.getElementById("pick-" + kind);
  label.textContent = where;
  label.classList.add("ok");
  setupGo.disabled = setupRunning || !(choice.apk && choice.assets && choice.master);
}
function setupLine(line) {
  const [kind, ...rest] = line.split(":");
  const text = rest.join(":");
  if (kind === "step") {
    const [i, n] = text.split(":")[0].split("/").map(Number);
    setupStep = [i - 1, n];
    document.getElementById("setup-step").textContent = text.split(":").slice(1).join(":");
    setupBar.style.width = ((i - 1) / n) * 100 + "%";
  } else if (kind === "progress") {
    const [i, n] = setupStep;
    setupBar.style.width = ((i + Number(text) / 100) / n) * 100 + "%";
  } else if (kind === "note") {
    document.getElementById("setup-step").textContent += " · " + text;
  } else if (kind === "error") {
    document.getElementById("setup-error").textContent = text;
  }
}
setupGo.addEventListener("click", () => {
  setupRunning = true;
  setupGo.disabled = true;
  document.getElementById("setup-error").textContent = "";
  send(`setup\\n${choice.apk}\\n${choice.assets}\\n${choice.master}`);
});

// The title art is the player's (made at setup); a plain title without it.
{
  const art = new Image();
  art.onerror = () => document.body.classList.add("no-art");
  art.src = "game/title.jpg";
}

// ---- pages -------------------------------------------------------------------------''')
open(p, 'w', encoding='utf-8').write(s)
print('ok')
