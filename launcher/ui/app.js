"use strict";

// ---- settings ------------------------------------------------------------------
// Positions are fractions of the game's screen (0..1 from the top left). The
// defaults are a starting point: place each marker on the game's own button.
const DEFAULT_BINDS = [
  // Exploring the Cage.
  { name: "Interact / next", scene: "field", vk: 69, key: "E", pad: "A", x: 0.775, y: 0.866 },
  { name: "Tap centre", scene: "field", vk: 32, key: "Space", pad: "", x: 0.5, y: 0.5 },
  { name: "Mama", scene: "field", vk: 70, key: "F", pad: "LS", x: 0.91, y: 0.84 },
  { name: "Auto walk", scene: "field", vk: 82, key: "R", pad: "", x: 0.941, y: 0.146 },
  // Battle: three characters, each with its two skills beside its portrait
  // (the portrait, charged, is the third). Keys 1-3, 4-6, 7-9. On the
  // controller the shoulders and triggers are characters 1 and 2, X and Y
  // their portraits; character 3 is the D-pad's up and down, and its portrait
  // is A, whose Interact point (field) lies on that portrait in battle.
  { name: "1st: skill I", scene: "battle", vk: 49, key: "1", pad: "LB", x: 0.274, y: 0.787 },
  { name: "1st: skill II", scene: "battle", vk: 50, key: "2", pad: "LT", x: 0.323, y: 0.87 },
  { name: "1st: charged", scene: "battle", vk: 51, key: "3", pad: "X", x: 0.166, y: 0.829 },
  { name: "2nd: skill I", scene: "battle", vk: 52, key: "4", pad: "RB", x: 0.58, y: 0.787 },
  { name: "2nd: skill II", scene: "battle", vk: 53, key: "5", pad: "RT", x: 0.628, y: 0.87 },
  { name: "2nd: charged", scene: "battle", vk: 54, key: "6", pad: "Y", x: 0.471, y: 0.829 },
  { name: "3rd: skill I", scene: "battle", vk: 55, key: "7", pad: "DPadUp", x: 0.885, y: 0.787 },
  { name: "3rd: skill II", scene: "battle", vk: 56, key: "8", pad: "DPadDown", x: 0.932, y: 0.87 },
  { name: "3rd: charged", scene: "battle", vk: 57, key: "9", pad: "", x: 0.776, y: 0.829 },
  // The top bar: battle speed and auto stay as set from one battle to the
  // next, so they are on keys only (C, Q); the menu is Start.
  { name: "Battle speed", scene: "battle", vk: 67, key: "C", pad: "", x: 0.804, y: 0.034 },
  { name: "Battle auto", scene: "battle", vk: 81, key: "Q", pad: "", x: 0.893, y: 0.034 },
  { name: "Battle menu", scene: "battle", vk: 9, key: "Tab", pad: "Start", x: 0.967, y: 0.034 },
  // Choices in the story.
  { name: "Yes", scene: "dialog", vk: 13, key: "Enter", pad: "DPadRight", x: 0.805, y: 0.77 },
  { name: "No", scene: "dialog", vk: 8, key: "Backspace", pad: "DPadLeft", x: 0.445, y: 0.77 },
  { name: "Skip story", scene: "dialog", vk: 75, key: "K", pad: "RS", x: 0.128, y: 0.768 },
];
// Screenshots to place points on, if the player saves some there (none ship).
const SCENES = { field: "game/field.jpg", battle: "game/battle.jpg", dialog: "game/dialog.jpg" };

const DEFAULTS = {
  version: 6,
  display: { mode: "fullscreen", size: "1600x900" },
  graphics: { backend: "d3d11", adapter: "nvidia", quality: "", aa: "fxaa", vkCompat: "auto", diagnostics: "off", perf: "off" },
  performance: { engine: "aot", fps: "60" },
  // Graphics (custom): "" is the game's own behaviour (no variable set).
  custom: { renderScale: "", upscale: "", aniso: "", vsync: "", idleFps: "",
            shadows: "", reflections: "", bloom: "", vignette: "", textures: "" },
  language: { text: "", voice: "" },
  // Bunker's mods (save\\mods.json, read by the server: internal/mods). Off is
  // the original game; unlimited shop stock is on so summons can be bought as
  // often as you like.
  mods: {
    shopUnlimited: true, staminaFree: false, freeSkips: false, unlimitedDailyPlays: false, ticketsToGems: false,
    lostItemGems: false, pityRewards: false, noDuplicateFourStar: false, rewardMultiplier: "1", expMultiplier: "1",
    // Not a server rule: tools\patch_content.py reads it from mods.json and sells the
    // limited-time offers as ordinary items, so the home screen stops announcing them.
    hideSaleNotices: true,
  },
  // Empty: the bundled runtime (runtime\\bin) and its data (runtime-data).
  paths: { runtime: "", runtimeData: "" },
  controls: {
    enabled: true,
    move: { x: 0.2, y: 0.7, radius: 0.15, keys: { up: 87, left: 65, down: 83, right: 68 } },
    binds: DEFAULT_BINDS,
    // The controller's mouse mode: this button switches between the touch
    // layout and a cursor (win.apk's input map, "pad"). It always has one.
    pad: { mouseToggle: "Back", mouseSpeed: 1 },
  },
};

// The game's own quality levels (Dark.Component.PerformanceSetting): its
// PlayerPrefs "PerformanceLevel", set before the game starts. The values are
// the game's form of each number: DES (key "dead301x", zero IV), base64, URL
// encoding. "" leaves the level the game itself picked or was set to.
const QUALITY = {
  "6": { name: "Max", pref: "bxSSDb6ZpCk%3D", note: "The screen's full size, every effect." },
  "1": { name: "Best", pref: "PcUuO%2BMKZ9w%3D", note: "1080p, shadows, reflections, bloom and vignette." },
  "2": { name: "High", pref: "0PXR2aAID%2Bw%3D", note: "720p with the effects." },
  "3": { name: "Medium", pref: "oYD12QpSFkQ%3D", note: "540p, no shadows, reflections, bloom or vignette." },
  "4": { name: "Low", pref: "EwkARg3Xkuw%3D", note: "540p, no effects: the lightest on the GPU." },
  "5": { name: "Power saving", pref: "s78uC%2FowoEE%3D", note: "540p, no effects, at most 30 fps, to save power." },
};
// The text and voice languages: PlayerPrefs "CurrentSystemLanguage" and
// "CurrentVoiceLanguage", Unity's SystemLanguage numbers in the same form.
// The worldwide game takes English (10) or Korean (23) text only: any other
// text language makes it set both back to the system's (Language.
// SetupLanguageForWorldWide), so Japanese (22) is a voice only.
const LANGUAGE = { "10": "cWI5k7i3OLE%3D", "22": "242ZQj%2FEP%2FA%3D", "23": "wGmNosNTgUI%3D" };
const PADS = ["", "A", "X", "Y", "LB", "RB", "LT", "RT", "Back", "Start", "LS", "RS", "DPadUp", "DPadDown", "DPadLeft", "DPadRight"];
// What the mouse-mode switch can be: not A (it clicks) nor B (Back).
const TOGGLES = ["Back", "Start", "LS", "RS", "LB", "RB", "LT", "RT", "X", "Y", "DPadUp", "DPadDown", "DPadLeft", "DPadRight"];
const CURSOR = "cursors\\nier.cur";  // the game window's cursor: the port's own design (cursors\nier.cur)

let settings = structuredClone(DEFAULTS);

function merge(base, over) {
  if (typeof base !== "object" || base === null || Array.isArray(base)) return over === undefined ? base : over;
  const out = { ...base };
  for (const k of Object.keys(over || {})) out[k] = merge(base[k], over[k]);
  return out;
}
const get = (path) => path.split(".").reduce((o, k) => (o == null ? o : o[k]), settings);
function set(path, value) {
  const keys = path.split(".");
  let o = settings;
  keys.slice(0, -1).forEach((k) => (o = o[k] ??= {}));
  o[keys.at(-1)] = value;
  save();
}

// The mouse-mode button is the controller's alone: a bind on it loses its pad button.
function freeToggle() {
  const t = settings.controls.pad.mouseToggle;
  if (!TOGGLES.includes(t)) settings.controls.pad.mouseToggle = DEFAULTS.controls.pad.mouseToggle;
  for (const b of settings.controls.binds) if (b.pad === settings.controls.pad.mouseToggle) b.pad = "";
}

// ---- the native side ---------------------------------------------------------------
const native = window.chrome && window.chrome.webview;
const send = (text) => native && native.postMessage(text);
let saveTimer = 0;
function save() {
  clearTimeout(saveTimer);
  saveTimer = setTimeout(() => send("save\n" + JSON.stringify(settings, null, 2)), 250);
  summary();
}

native && native.addEventListener("message", (e) => {
  const msg = String(e.data);
  const [kind, ...rest] = msg.split(":");
  const text = rest.join(":");
  if (kind === "settings") {
    try { settings = merge(DEFAULTS, text ? JSON.parse(text) : {}); } catch { settings = structuredClone(DEFAULTS); }
    if ((settings.version || 1) < 2) settings.controls.move.radius = DEFAULTS.controls.move.radius;  // v2: a longer run drag
    if (!["vulkan", "d3d11", "swiftshader"].includes(settings.graphics.backend)) {  // not a renderer of this version
      settings.graphics.backend = "d3d11";
      save();
    }
    if (!["", "10", "23"].includes(settings.language.text)) {  // not a text language of this game
      settings.language.text = "10";
      save();
    }
    if ((settings.version || 1) < 4) {  // v4: binds for the game's own screens (field, battle, choices)
      settings.version = 4;
      settings.controls.binds = structuredClone(DEFAULT_BINDS);
      freeToggle();
      save();
    }
    if (settings.version < 5) {  // v5: three characters in battle (their skills and portraits, keys 1-9)
      settings.version = 5;
      const battle = DEFAULT_BINDS.filter((b) => b.scene === "battle");
      const taken = new Set(battle.map((b) => b.pad).filter(Boolean));
      settings.controls.binds = settings.controls.binds.filter((b) => b.scene !== "battle")
        .map((b) => (taken.has(b.pad) ? { ...b, pad: "" } : b))
        .concat(structuredClone(battle));
      freeToggle();
      save();
    }
    if (settings.version < 6) {  // v6: Direct3D 11 is the default renderer (Vulkan stays a choice)
      settings.version = 6;
      if (settings.graphics.backend === "vulkan") settings.graphics.backend = "d3d11";
      save();
    }
    render();
  } else if (kind === "info") {
    const info = JSON.parse(text || "{}");
    document.body.classList.toggle("setup", !info.ready);
    if (!info.ready) return;
    const files = document.getElementById("files");
    files.textContent = info.files ? "Ready" : "Missing";
    files.className = info.files ? "ok" : "alert";
    document.getElementById("save").textContent = info.save ? `game.db · ${info.save}` : "New";
  } else if (kind === "picked") {
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
    }
    setStatus(text);
    serverState(true);
  } else if (kind === "error") {
    setStatus(text, true);
    playButton.disabled = false;
  } else if (kind === "exporting") {
    exportState("Exporting the logs...", false, true);
  } else if (kind === "exported") {
    exportState("Saved " + text.split("\\").pop() + ": attach it to your bug report.", false, false);
  } else if (kind === "export-error") {
    exportState(text, true, false);
  } else if (kind === "exited") {
    setStatus(text === "0" ? "READY" : "THE GAME CLOSED (CODE " + text + ")", text !== "0");
    playButton.disabled = false;
    serverState(false);
    send("info");
    gameRunning = false;
    savesCall({ op: "list" });
  } else if (kind === "saves") {
    savesReply(text);
  }
});

// ---- first-run setup -----------------------------------------------------------------
// The port ships without the game: NieR.exe runs tools/setup_game.py on the
// player's three files and passes its lines on ("step:i/n:what", "progress:n",
// "note:...", "error:...", "done").
const choice = { apk: "", assets: "", master: "" };
const setupGo = document.getElementById("setup-go");
const setupBar = document.getElementById("setup-bar");
let setupRunning = false;
let setupStep = [0, 1];
document.querySelectorAll("[data-pick]").forEach((b) => b.addEventListener("click", () => send("pick\n" + b.dataset.pick)));
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
  send(`setup\n${choice.apk}\n${choice.assets}\n${choice.master}`);
});

// ---- pages -------------------------------------------------------------------------
const NAMES = { settings: "Settings", events: "Events", controls: "Controls" };
function go(view) {
  document.body.classList.toggle("title", view === "home");
  document.querySelectorAll(".view").forEach((v) => v.classList.toggle("on", v.id === view));
  document.querySelectorAll(".tabs [data-view]").forEach((a) => a.classList.toggle("on", a.dataset.view === view));
  if (NAMES[view]) document.getElementById("page-name").textContent = NAMES[view];
  if (view === "controls") renderControls();
  if (view === "events" && !ct.loaded) ctLoad();
}
document.querySelectorAll("[data-go]").forEach((el) => el.addEventListener("click", () => go(el.dataset.go)));

// Settings sub-tabs: one group of dialogs at a time; the last one is remembered.
const SUBS = [...document.querySelectorAll("#subtabs [data-sub]")].map((b) => b.dataset.sub);
function sub(name) {
  if (!SUBS.includes(name)) name = SUBS[0];
  document.querySelectorAll("#subtabs [data-sub]").forEach((b) => b.classList.toggle("on", b.dataset.sub === name));
  document.querySelectorAll(".subpage").forEach((p) => p.classList.toggle("on", p.dataset.sub === name));
  document.querySelector("main").scrollTop = 0;
  try { localStorage.setItem("settings-sub", name); } catch (e) { /* not kept: fine */ }
}
document.querySelectorAll("#subtabs [data-sub]").forEach((b) => b.addEventListener("click", () => sub(b.dataset.sub)));
let lastSub = null;
try { lastSub = localStorage.getItem("settings-sub"); } catch (e) { /* none */ }
sub(lastSub);

// ---- title -------------------------------------------------------------------------
const playButton = document.getElementById("play");
const statusLine = document.getElementById("status");
function serverState(on) {
  const c = document.getElementById("server");
  c.textContent = on ? "Running" : "Standing by";
  c.className = on ? "ok" : "";
}
function setStatus(text, error = false) {
  statusLine.textContent = text.toUpperCase();
  statusLine.classList.toggle("error", error);
}
function summary() {
  const d = settings.display.mode === "fullscreen" ? "Full screen" : settings.display.size.replace("x", "×");
  const q = QUALITY[settings.graphics.quality];
  const fps = settings.performance.fps === "0" ? "Unlimited" : settings.performance.fps + " fps";
  document.getElementById("summary").textContent = `${d} · ${q ? q.name : "In-game quality"} · ${fps}`;
}

function launchEnv() {
  const env = {
    JAVA_TOOL_OPTIONS: "-Xss16m",
    WINAPK_DIRECT_PRESENT: "1",
    WINAPK_ANGLE_BACKEND: settings.graphics.backend,
    WINAPK_CURSOR: CURSOR,
  };
  if (settings.paths.runtime) env.NIER_RUNTIME = settings.paths.runtime;
  if (settings.paths.runtimeData) env.NIER_RUNTIME_DATA = settings.paths.runtimeData;
  if (settings.graphics.adapter) {
    env.WINAPK_ANGLE_ADAPTER = settings.graphics.adapter;  // Direct3D 11's adapter
    // ANGLE's Vulkan backend takes the first GPU Windows lists unless told:
    // on a PC with the processor's graphics too, that drew on the wrong one.
    env.ANGLE_PREFERRED_DEVICE = settings.graphics.adapter;
  }
  if (settings.display.mode === "fullscreen") env.WINAPK_FULLSCREEN = "1";
  else env.WINAPK_PHONE_SIZE = settings.display.size;
  if (settings.performance.engine === "jit") env.WINAPK_ARM64_ENGINE = "jit";
  if (settings.performance.fps !== "0") env.WINAPK_FPS_CAP = settings.performance.fps;
  // Power saving: the game's own lower rate drew a frame in 24 here (the
  // runtime now draws every frame for it), so its saving is a 30 fps cap.
  if (settings.graphics.quality === "5" && !(Number(env.WINAPK_FPS_CAP) > 0 && Number(env.WINAPK_FPS_CAP) <= 30)) env.WINAPK_FPS_CAP = "30";
  if (settings.graphics.aa === "fxaa") env.WINAPK_FXAA = "1";
  env.WINAPK_VK_COMPAT = settings.graphics.vkCompat || "auto";
  if (settings.graphics.perf === "log") env.WINAPK_PERF = "1";
  if (settings.graphics.perf === "overlay") env.WINAPK_PERF = "overlay";
  // Graphics (custom): each one only when chosen.
  const c = settings.custom || {};
  if (c.renderScale) env.WINAPK_RENDER_SCALE = c.renderScale;
  if (c.upscale) env.WINAPK_UPSCALE = c.upscale;
  if (c.aniso) env.WINAPK_ANISO = c.aniso;
  if (c.vsync) env.WINAPK_VSYNC = c.vsync;
  if (c.idleFps) env.WINAPK_IDLE_FPS = c.idleFps;
  const effectsOff = ["shadows", "reflections", "bloom", "vignette"].filter((k) => c[k] === "off");
  if (effectsOff.length) env.WINAPK_GAME_EFFECTS_OFF = effectsOff.join("+");
  // Unity's own boot argument, through the runtime's launch override (the game's activity, its "unity" extra).
  const skip = { half: 1, quarter: 2 }[c.textures];
  if (skip) {
    env.MACAPK_LAUNCH_ACTIVITY = "com.google.firebase.MessagingUnityPlayerActivity";
    env.MACAPK_LAUNCH_EXTRAS = "unity=-texture-mipmap-skip-level " + skip;
  }
  // Graphics diagnostics in the game's log: shaders the GPU's driver did not
  // build, and how the game uses the depth buffer (each setting once).
  if (settings.graphics.diagnostics === "on") {
    env.WINAPK_GL_SHADER_LOG = "1";
    env.WINAPK_GL_DEPTH_TRACE = "1";
  }
  return env;
}

playButton.addEventListener("click", () => {
  if (edPendingCount() && !confirm("The changes in Settings › Saves are not applied yet. Start the game without them?")) return;
  playButton.disabled = true;
  gameRunning = true;
  setStatus("STARTING");
  freeToggle();
  const lines = ["play"];
  if (settings.controls.enabled) lines.push("input:" + JSON.stringify(settings.controls));
  lines.push("mods:" + JSON.stringify(modsFile()));
  const q = QUALITY[settings.graphics.quality];
  if (q) lines.push("pref:PerformanceLevel=" + q.pref);
  if (["10", "23"].includes(settings.language.text)) lines.push("pref:CurrentSystemLanguage=" + LANGUAGE[settings.language.text]);
  if (LANGUAGE[settings.language.voice]) lines.push("pref:CurrentVoiceLanguage=" + LANGUAGE[settings.language.voice]);
  for (const [k, v] of Object.entries(launchEnv())) lines.push(`env:${k}=${v}`);
  send(lines.join("\n"));
});
document.getElementById("quit").addEventListener("click", () => send("quit"));

// save\mods.json as the server reads it: the switches as they are, the
// multipliers as numbers (1 is off).
function modsFile() {
  const m = settings.mods;
  const out = {};
  for (const [k, v] of Object.entries(m)) out[k] = k.endsWith("Multiplier") ? Number(v) || 1 : v === true;
  return out;
}
document.getElementById("logs").addEventListener("click", () => send("logs"));
const exportButton = document.getElementById("export-logs");
const exportNote = document.getElementById("export-note");
function exportState(text, error, busy) {
  exportNote.textContent = text;
  exportNote.classList.toggle("alert", error);
  exportButton.disabled = busy;
}
exportButton.addEventListener("click", () => send("export-logs"));

// ---- settings fields ----------------------------------------------------------------
function rangeFill(input) {
  input.style.setProperty("--fill", ((input.value - input.min) / (input.max - input.min)) * 100 + "%");
}

function render() {
  document.querySelectorAll(".choice[data-key]").forEach((c) => {
    const value = String(get(c.dataset.key));
    c.querySelectorAll("button").forEach((b) => b.classList.toggle("on", b.dataset.value === value));
  });
  document.querySelectorAll("input[data-key]").forEach((i) => (i.value = get(i.dataset.key) ?? ""));
  document.querySelectorAll("[data-show]").forEach((f) => {
    const [k, v] = f.dataset.show.split("=");
    f.hidden = String(get(k)) !== v;
  });
  const q = QUALITY[settings.graphics.quality];
  document.getElementById("quality-note").textContent = q
    ? q.note + " Applied when the game starts."
    : "The game's own choice (Menu › Settings › Game Settings in the game).";
  renderPad();
  summary();
}
document.querySelectorAll(".choice[data-key]").forEach((c) =>
  c.addEventListener("click", (e) => {
    const b = e.target.closest("button");
    if (!b) return;
    let v = b.dataset.value;
    if (v === "true" || v === "false") v = v === "true";
    set(c.dataset.key, v);
    render();
  }));
document.querySelectorAll("input[data-key]").forEach((i) => i.addEventListener("change", () => set(i.dataset.key, i.value.trim())));

// ---- the controller's mouse mode ----------------------------------------------------
const toggleBox = document.getElementById("mouse-toggle");
const speed = document.getElementById("mouse-speed");
function renderPad() {
  const pad = settings.controls.pad;
  toggleBox.innerHTML = TOGGLES.map((p) => `<button data-value="${p}" class="${p === pad.mouseToggle ? "on" : ""}">${p}</button>`).join("");
  speed.value = pad.mouseSpeed;
  document.getElementById("mouse-speed-v").textContent = "×" + Number(pad.mouseSpeed).toFixed(1);
  rangeFill(speed);
}
toggleBox.addEventListener("click", (e) => {
  const b = e.target.closest("button");
  if (!b) return;
  settings.controls.pad.mouseToggle = b.dataset.value;
  freeToggle();
  save();
  renderPad();
  renderControls();
});
speed.addEventListener("input", () => {
  settings.controls.pad.mouseSpeed = +speed.value;
  save();
  renderPad();
});

// ---- bindings ------------------------------------------------------------------------
let selected = "move";  // "move" or a bind index
let scene = "field";      // the screen the markers are shown on
document.querySelectorAll("#scenes button").forEach((b) => b.addEventListener("click", () => {
  scene = b.dataset.value;
  renderControls();
}));
let listening = null;
const reach = document.getElementById("reach");
reach.addEventListener("input", () => {
  settings.controls.move.radius = +reach.value;
  save();
  drawMarkers();
});

function keyName(e) {
  if (e.key === " ") return "Space";
  if (e.key.length === 1) return e.key.toUpperCase();
  return e.key;
}

function renderControls() {
  const tbody = document.getElementById("binds");
  const toggle = settings.controls.pad.mouseToggle;
  tbody.innerHTML = "";
  const moveRow = document.createElement("tr");
  moveRow.className = selected === "move" ? "sel" : "";
  moveRow.innerHTML = `<td>Move</td><td><span class="where">W A S D</span></td><td><span class="where">L stick</span></td>
    <td class="where">${pos(settings.controls.move)}</td>`;
  moveRow.addEventListener("click", () => { selected = "move"; scene = "field"; renderControls(); });
  tbody.appendChild(moveRow);

  settings.controls.binds.forEach((b, i) => {
    const tr = document.createElement("tr");
    tr.className = selected === i ? "sel" : "";
    // The mouse-mode button is not offered: it is the controller's switch.
    const options = PADS.filter((p) => p !== toggle);
    tr.innerHTML = `<td><input class="name" value="${escapeHtml(b.name)}"></td>
      <td><button class="key">${escapeHtml(b.key || "—")}</button></td>
      <td><select>${options.map((p) => `<option ${p === b.pad ? "selected" : ""} value="${p}">${p || "—"}</option>`).join("")}</select></td>
      <td class="where">${pos(b)} <span class="del" title="Remove">✕</span></td>`;
    tr.addEventListener("click", () => {
      if (selected !== i) {
        selected = i;
        scene = b.scene || "field";
        renderControls();
      }
    });
    tr.querySelector(".name").addEventListener("change", (e) => { b.name = e.target.value; save(); drawMarkers(); });
    tr.querySelector("select").addEventListener("change", (e) => { b.pad = e.target.value; save(); });
    const keyButton = tr.querySelector(".key");
    keyButton.addEventListener("click", (e) => {
      e.stopPropagation();
      listening = { bind: b, button: keyButton };
      keyButton.classList.add("listen");
      keyButton.textContent = "Press a key";
    });
    tr.querySelector(".del").addEventListener("click", (e) => {
      e.stopPropagation();
      settings.controls.binds.splice(i, 1);
      selected = "move";
      save();
      renderControls();
    });
    tbody.appendChild(tr);
  });
  drawMarkers();
}

function pos(p) { return `${Math.round(p.x * 100)}%, ${Math.round(p.y * 100)}%`; }
function escapeHtml(s) { return String(s).replace(/[&<>"]/g, (c) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;" }[c])); }

function drawMarkers() {
  reach.value = settings.controls.move.radius;
  document.getElementById("reach-v").textContent = Math.round(settings.controls.move.radius * 100) + "%";
  rangeFill(reach);
  const box = document.getElementById("markers");
  box.innerHTML = "";
  const add = (p, label, cls) => {
    const m = document.createElement("div");
    m.className = "marker " + cls;
    m.style.left = p.x * 100 + "%";
    m.style.top = p.y * 100 + "%";
    m.textContent = label;
    box.appendChild(m);
  };
  if (scene === "field") add(settings.controls.move, "", "move" + (selected === "move" ? " sel" : ""));
  settings.controls.binds.forEach((b, i) => {
    if ((b.scene || "field") === scene) add(b, b.key || b.name, selected === i ? "sel" : "");
  });
  document.querySelector(".screen-img").style.backgroundImage = `url("${SCENES[scene]}")`;
  document.querySelectorAll("#scenes button").forEach((b) => b.classList.toggle("on", b.dataset.value === scene));
}

document.getElementById("screen").addEventListener("click", (e) => {
  const r = e.currentTarget.getBoundingClientRect();
  const p = { x: +((e.clientX - r.left) / r.width).toFixed(3), y: +((e.clientY - r.top) / r.height).toFixed(3) };
  const target = selected === "move" ? settings.controls.move : settings.controls.binds[selected];
  if (!target) return;
  target.x = p.x;
  target.y = p.y;
  save();
  renderControls();
});

document.addEventListener("keydown", (e) => {
  if (!listening) return;
  e.preventDefault();
  const { bind, button } = listening;
  listening = null;
  button.classList.remove("listen");
  if (e.key === "Escape") { button.textContent = bind.key || "—"; return; }  // Esc stays Back
  bind.vk = e.keyCode;
  bind.key = keyName(e);
  save();
  renderControls();
});

document.getElementById("add-bind").addEventListener("click", () => {
  settings.controls.binds.push({ name: "New action", scene, vk: 0, key: "", pad: "", x: 0.5, y: 0.5 });
  selected = settings.controls.binds.length - 1;
  save();
  renderControls();
});
document.getElementById("reset-binds").addEventListener("click", () => {
  settings.controls.binds = structuredClone(DEFAULT_BINDS);
  settings.controls.move = structuredClone(DEFAULTS.controls.move);
  freeToggle();
  selected = "move";
  save();
  renderControls();
});

// ---- Settings › Saves ------------------------------------------------------------------
// Save files ("slots") and what a save owns. The server does the work
// (nier-server --admin, internal/pcsave): the page sends "saves\n<json>" and
// gets "saves:<json>" back, one request at a time. Nothing changes while the
// game runs, and a copy of a save is kept before each change.
let gameRunning = false;
let saves = { slots: [], active: "", running: false };
const savesQueue = [];
let savesBusy = false;
// The answer shows at the top of the page and under the editor's buttons.
function savesMsg(text, error = false) {
  for (const id of ["saves-msg", "ed-msg"]) {
    const line = document.getElementById(id);
    line.textContent = text || "";
    line.classList.toggle("alert", !!error);
  }
}
function savesCall(req) {
  return new Promise((resolve) => {
    savesQueue.push([req, resolve]);
    savesNext();
  });
}
function savesNext() {
  if (savesBusy || !savesQueue.length) return;
  savesBusy = true;
  document.body.classList.add("working");
  send("saves\n" + JSON.stringify(savesQueue[0][0]));
  if (!native) setTimeout(() => savesReply('{"ok":false,"error":"Open the launcher (NieR.exe) to change saves."}'), 0);
}
function savesReply(text) {
  let r;
  try { r = JSON.parse(text); } catch { r = { ok: false, error: "The save tool gave no answer." }; }
  const [, resolve] = savesQueue.shift() || [];
  savesBusy = false;
  document.body.classList.remove("working");
  if (r.slots) {
    saves = { slots: r.slots, active: r.active, running: !!r.running };
    renderSlots();
  }
  if (resolve) resolve(r);
  savesNext();
}
async function savesDo(req, done) {
  if (gameRunning || saves.running) {
    savesMsg("The game is running. Close it before changing saves.", true);
    return null;
  }
  savesMsg("Working...");
  const r = await savesCall(req);
  if (!r.ok) savesMsg(r.error || "That did not work.", true);
  else savesMsg(typeof done === "function" ? done(r) : done || "");
  return r.ok ? r : null;
}

// The question before a change: resolves to true (or the typed name), or null.
const modal = document.getElementById("modal");
const modalInput = document.getElementById("modal-input");
let modalDone = null;
function ask(title, text, { input = null, ok = "OK" } = {}) {
  document.getElementById("modal-title").textContent = title;
  document.getElementById("modal-text").textContent = text;
  document.querySelector("#modal-ok span").textContent = ok;
  modalInput.hidden = input === null;
  modalInput.value = input || "";
  modal.hidden = false;
  if (input !== null) setTimeout(() => { modalInput.focus(); modalInput.select(); }, 0);
  return new Promise((resolve) => (modalDone = resolve));
}
function closeModal(answer) {
  modal.hidden = true;
  const done = modalDone;
  modalDone = null;
  if (done) done(answer);
}
document.getElementById("modal-ok").addEventListener("click", () => {
  if (!modalInput.hidden) {
    const name = modalInput.value.trim();
    if (!name) return modalInput.focus();
    closeModal(name);
  } else closeModal(true);
});
document.getElementById("modal-cancel").addEventListener("click", () => closeModal(null));
modalInput.addEventListener("keydown", (e) => {
  if (e.key === "Enter") document.getElementById("modal-ok").click();
  if (e.key === "Escape") closeModal(null);
});

const slotName = (id) => (saves.slots.find((s) => s.id === id) || {}).name || "this save";
function when(ms) {
  if (!ms) return "";
  const d = new Date(ms);
  const two = (n) => String(n).padStart(2, "0");
  return `${d.getFullYear()}/${two(d.getMonth() + 1)}/${two(d.getDate())} ${two(d.getHours())}:${two(d.getMinutes())}`;
}
function renderSlots() {
  const box = document.getElementById("slots");
  if (saves.running) savesMsg("The game is running: saves can be changed once it is closed.", true);
  box.innerHTML = saves.slots.map((s) => {
    const meta = s.started
      ? `${s.player ? escapeHtml(s.player) + " · Lv " + s.level : "No player yet"} · last played ${when(s.modified)}`
      : "Not played yet: the game starts from the beginning with it";
    return `<div class="slot${s.active ? " on" : ""}${ed && ed.id === s.id ? " edit" : ""}" data-id="${escapeHtml(s.id)}">
      <div><span class="name">${escapeHtml(s.name)}</span>${s.active ? '<span class="tag">In use</span>' : ""}</div>
      <div class="meta">${meta}</div>
      <div class="acts">
        <button class="tool" data-act="use" ${s.active ? "disabled" : ""}>Use</button>
        <button class="tool" data-act="edit" ${s.started ? "" : "disabled"}>Edit</button>
        <button class="tool" data-act="rename">Rename</button>
        <button class="tool warn" data-act="delete" ${s.active ? "disabled title=\"Use another save first\"" : ""}>Delete</button>
      </div></div>`;
  }).join("") || '<p class="lead">No saves yet.</p>';
  const act = saves.slots.find((s) => s.active);
  if (act) document.getElementById("save").textContent = act.name + (act.started ? (act.player ? ` · ${act.player} Lv ${act.level}` : "") : " · New game");
}
document.getElementById("slots").addEventListener("click", async (e) => {
  const b = e.target.closest("button[data-act]");
  if (!b) return;
  const id = b.closest(".slot").dataset.id;
  const name = slotName(id);
  if (b.dataset.act === "use") {
    if (!(await keepPending())) return;
    if (!(await ask("Use this save", `The game will start with "${name}" next time. "${slotName(saves.active)}" is kept as it is and can be used again.`, { ok: "Use it" }))) return;
    if (await savesDo({ op: "switch", id }, `"${name}" is in use. Press START to play it.`)) closeEditor();
  } else if (b.dataset.act === "edit") {
    if (!(await keepPending())) return;
    loadEditor(id);
  } else if (b.dataset.act === "rename") {
    const to = await ask("Rename", "The save's new name:", { input: name, ok: "Rename" });
    if (to) savesDo({ op: "rename", id, name: to }, "Renamed.");
  } else if (b.dataset.act === "delete") {
    if (!(await ask("Delete this save", `Delete "${name}"? Its progress is no longer offered here. A copy is kept in the backups folder (save\\slot-backups) in case you change your mind.`, { ok: "Delete" }))) return;
    if (ed && ed.id === id) closeEditor();
    savesDo({ op: "delete", id }, (r) => `"${name}" was deleted.` + (r.backup ? ` Its copy: ${r.backup}.` : ""));
  }
});
document.getElementById("slot-restart").addEventListener("click", async () => {
  if (!(await keepPending())) return;
  const n = saves.slots.length + 1;
  const name = await ask("Start over", `A new save is made and used: the game starts from the beginning next time. "${slotName(saves.active)}" is kept as it is and can be used again from this list. Name of the new save:`, { input: `New game ${n}`, ok: "Start over" });
  if (!name) return;
  if (await savesDo({ op: "new", name, switch: true }, `"${name}" is in use. Press START to begin a new game.`)) closeEditor();
});
document.getElementById("slot-new").addEventListener("click", async () => {
  const name = await ask("New empty save", "A new save that starts from the beginning when you use it. The save in use stays in use. Name:", { input: `Save ${saves.slots.length + 1}`, ok: "Make it" });
  if (name) savesDo({ op: "new", name, switch: false }, `"${name}" was made. Choose Use on it to play it.`);
});
document.getElementById("slot-backup").addEventListener("click", () => {
  const id = ed ? ed.id : saves.active;
  savesDo({ op: "backup", id }, (r) => `A copy of "${slotName(id)}" was kept: ${r.backup}.`);
});
document.getElementById("slot-backups").addEventListener("click", () => send("saves-folder"));

// ---- the editor ----
const RARITY = { 10: "★1", 20: "★2", 30: "★3", 40: "★4", 50: "★5" };
const ROW_LIMIT = 400;
let ed = null;          // { id, player, catalog, charName }
let edKind = "costumes";
let pending = freshPending();
function freshPending() {
  return { costumes: new Map(), weapons: new Map(), companions: new Map(), materials: new Map(), consumables: new Map(), amounts: {} };
}
function edPendingCount() {
  return pending.costumes.size + pending.weapons.size + pending.companions.size + pending.materials.size +
    pending.consumables.size + Object.keys(pending.amounts).length;
}
async function keepPending() {
  if (!edPendingCount()) return true;
  if (await ask("Changes not applied", "The changes made in the editor are not applied yet. Drop them?", { ok: "Drop them" })) {
    pending = freshPending();
    renderEditor();
    return true;
  }
  return false;
}
function closeEditor() {
  ed = null;
  pending = freshPending();
  renderEditor();
  renderSlots();
}
async function loadEditor(id) {
  const r = await savesDo({ op: "read", id }, "");
  if (!r) return;
  ed = { id, player: r.player, catalog: r.catalog, charName: {} };
  for (const c of r.catalog.characters) ed.charName[c.id] = c.name;
  // The rarest first, then by character and name.
  const order = (a, b) => (b.rarity || 0) - (a.rarity || 0) ||
    (ed.charName[a.char] || "").localeCompare(ed.charName[b.char] || "") || a.name.localeCompare(b.name);
  for (const k of ["costumes", "weapons"]) r.catalog[k].sort(order);
  r.catalog.companions.sort((a, b) => a.name.localeCompare(b.name));
  pending = freshPending();
  renderSlots();
  renderEditor();
  document.getElementById("editor").scrollIntoView({ behavior: "smooth", block: "start" });
  // The pictures of everything the editor lists (made once from the game files).
  const keys = ["costumes", "weapons", "companions", "materials", "consumables"].flatMap((k) => r.catalog[k].map((x) => x.img));
  if (keys.some((k) => k && !picAsked.has(k))) savesMsg("Making the pictures from your game files (the first time takes a few seconds)...");
  await wantPics(keys, renderEditor);
  savesMsg("");
}
const edList = document.getElementById("ed-list");
const levelKinds = ["costumes", "weapons", "companions"];
function edItems() {
  if (!ed) return [];
  const c = ed.catalog, p = ed.player;
  const q = document.getElementById("ed-search").value.trim().toLowerCase();
  const rarity = document.getElementById("ed-rarity").value;
  const owned = document.getElementById("ed-owned").value;
  let items;
  if (edKind === "costumes") items = c.costumes.map((x) => ({ ...x, sub: ed.charName[x.char] || "", have: p.costumes[x.id] }));
  else if (edKind === "weapons") items = c.weapons.map((x) => ({ ...x, sub: "", have: p.weapons[x.id] }));
  else if (edKind === "companions") items = c.companions.map((x) => ({ ...x, sub: "", have: p.companions[x.id] }));
  else {
    const counts = edKind === "materials" ? p.materials : p.consumables;
    items = c[edKind].map((x) => ({ ...x, sub: "", have: counts[x.id] ? { count: counts[x.id] } : null }));
  }
  return items.filter((x) =>
    (!q || x.name.toLowerCase().includes(q) || x.sub.toLowerCase().includes(q) || String(x.id) === q) &&
    (!rarity || !levelKinds.includes(edKind) || edKind === "companions" || String(x.rarity) === rarity) &&
    (!owned || (owned === "yes") === !!x.have));
}
function renderEditor() {
  const editor = document.getElementById("editor");
  editor.classList.toggle("off", !ed);
  document.querySelectorAll("#ed-kinds button").forEach((b) => b.classList.toggle("on", b.dataset.value === edKind));
  document.getElementById("ed-rarity").hidden = !["costumes", "weapons"].includes(edKind);
  document.querySelector('[data-bulk="chars"]').hidden = edKind !== "costumes";
  document.querySelectorAll('[data-bulk="max"], #ed-bulk-level').forEach((el) => (el.closest(".tool").hidden = !levelKinds.includes(edKind)));
  document.querySelector('[data-bulk="own"]').textContent = levelKinds.includes(edKind) ? "Own all shown" : "Set all shown to";
  renderAmounts();
  renderPending();
  if (!ed) {
    edList.innerHTML = '<p class="more">Choose Edit on a save (one played at least once).</p>';
    document.getElementById("ed-count").textContent = "";
    document.getElementById("ed-who").textContent = "Choose Edit on a save.";
    return;
  }
  document.getElementById("ed-who").textContent = `Editing "${slotName(ed.id)}"` + (ed.player.name ? ` (${ed.player.name}, Lv ${ed.player.level})` : "") + ".";
  const items = edItems();
  const shown = items.slice(0, ROW_LIMIT);
  const map = pending[edKind];
  // A picture grid: click a picture to own it; the level or amount below it.
  let html;
  if (levelKinds.includes(edKind)) {
    html = shown.map((x) => {
      const has = map.has(x.id), lv = map.get(x.id);
      const now = x.have ? `Owned · Lv ${x.have.level}` + (x.have.lb ? ` · ${x.have.lb}✦` : "") : has ? "To own" : "Not owned";
      const sub = [x.sub, RARITY[x.rarity] || ""].filter(Boolean).join(" · ");
      return `<div class="tile${has ? " changed" : ""}${x.have ? " owned" : ""}" data-id="${x.id}" title="${escapeHtml(x.name + (sub ? "\n" + sub : ""))}">
        <button class="own frame${x.have ? " on" : has ? " new" : ""}" ${x.have ? "disabled" : ""}>${picImg(x.img, x.name)}<i></i></button>
        <span class="t-name">${escapeHtml(x.name)}</span><span class="t-sub">${escapeHtml(sub)}</span>
        <span class="t-sub now">${now}</span>
        <span class="lv"><input type="number" min="1" max="100" placeholder="Lv" value="${has && lv > 0 ? lv : ""}"><button class="tool${lv === -1 ? " on" : ""}" data-max>Max</button></span>
      </div>`;
    }).join("");
  } else {
    html = shown.map((x) => {
      const has = map.has(x.id);
      return `<div class="tile items${has ? " changed" : ""}" data-id="${x.id}" title="${escapeHtml(x.name)}">
        <span class="frame">${picImg(x.img, x.name)}</span>
        <span class="t-name">${escapeHtml(x.name)}</span>
        <span class="t-sub now">Now ${x.have ? x.have.count.toLocaleString("en-US") : "0"}</span>
        <input class="item-n" type="number" min="0" placeholder="${x.have ? x.have.count : 0}" value="${has ? map.get(x.id) : ""}"></div>`;
    }).join("");
  }
  html = `<div class="tiles">${html}</div>`;
  if (items.length > ROW_LIMIT) html += `<p class="more">Showing ${ROW_LIMIT} of ${items.length}: search to find the rest.</p>`;
  if (!items.length) html += '<p class="more">Nothing matches.</p>';
  edList.innerHTML = html;
  document.getElementById("ed-count").textContent = `${items.length} shown.`;
}
function renderAmounts() {
  document.querySelectorAll("[data-amount]").forEach((i) => {
    const k = i.dataset.amount;
    const cur = ed ? ed.player[k] : "";
    i.disabled = !ed;
    i.placeholder = ed ? String(cur) : "";
    i.value = k in pending.amounts ? pending.amounts[k] : ed ? cur : "";
    i.closest("label").classList.toggle("changed", k in pending.amounts);
  });
}
document.querySelectorAll("[data-amount]").forEach((i) => i.addEventListener("change", () => {
  if (!ed) return;
  const k = i.dataset.amount;
  const v = Math.floor(Number(i.value));
  if (i.value.trim() === "" || !Number.isFinite(v) || v === ed.player[k]) delete pending.amounts[k];
  else pending.amounts[k] = Math.min(Math.max(v, 0), 2147483647);
  renderAmounts();
  renderPending();
}));
function levelCap() { return edKind === "companions" ? 50 : 100; }
edList.addEventListener("click", (e) => {
  const row = e.target.closest("[data-id]");
  if (!row || !ed) return;
  const id = Number(row.dataset.id), map = pending[edKind];
  const item = edItems().find((x) => x.id === id);
  if (e.target.closest(".own")) {
    if (item && item.have) return;
    map.has(id) ? map.delete(id) : map.set(id, 0);
  } else if (e.target.closest("[data-max]")) {
    if (map.get(id) === -1) item && item.have ? map.delete(id) : map.set(id, 0);
    else map.set(id, -1);
  } else return;
  renderEditor();
});
edList.addEventListener("change", (e) => {
  const row = e.target.closest("[data-id]");
  if (!row || !ed) return;
  const id = Number(row.dataset.id), map = pending[edKind];
  const v = Math.floor(Number(e.target.value));
  const item = edItems().find((x) => x.id === id);
  if (e.target.classList.contains("item-n")) {
    if (e.target.value.trim() === "" || !Number.isFinite(v) || (item && item.have ? item.have.count : 0) === v) map.delete(id);
    else map.set(id, Math.min(Math.max(v, 0), 2147483647));
  } else {
    if (e.target.value.trim() === "" || !Number.isFinite(v) || v < 1) item && item.have ? map.delete(id) : map.set(id, 0);
    else map.set(id, Math.min(v, levelCap()));
  }
  renderEditor();
});
document.getElementById("ed-kinds").addEventListener("click", (e) => {
  const b = e.target.closest("button");
  if (!b) return;
  edKind = b.dataset.value;
  document.getElementById("ed-search").value = "";
  renderEditor();
});
let searchTimer = 0;
document.getElementById("ed-search").addEventListener("input", () => { clearTimeout(searchTimer); searchTimer = setTimeout(renderEditor, 150); });
document.getElementById("ed-rarity").addEventListener("change", renderEditor);
document.getElementById("ed-owned").addEventListener("change", renderEditor);
document.getElementById("ed-bulk").addEventListener("click", (e) => {
  const b = e.target.closest("button[data-bulk]");
  if (!b || !ed) return;
  const map = pending[edKind];
  if (b.dataset.bulk === "chars") {
    const have = ed.player.characters || {};
    for (const c of ed.catalog.characters) if (!(c.id in have) && !map.has(c.costume)) pending.costumes.set(c.costume, 0);
  } else if (b.dataset.bulk === "own") {
    if (levelKinds.includes(edKind)) {
      for (const x of edItems()) if (!x.have && !map.has(x.id)) map.set(x.id, 0);
    } else {
      ask("Set all shown", "The amount every item shown will have:", { input: "99", ok: "Set" }).then((v) => {
        const amount = Math.floor(Number(v));
        if (v === null || !Number.isFinite(amount) || amount < 0) return;
        for (const x of edItems()) map.set(x.id, Math.min(amount, 2147483647));
        renderEditor();
      });
      return;
    }
  } else if (b.dataset.bulk === "max") {
    for (const x of edItems()) map.set(x.id, -1);
  }
  renderEditor();
});
document.getElementById("ed-bulk-level").addEventListener("change", (e) => {
  if (!ed || !levelKinds.includes(edKind)) return;
  const v = Math.floor(Number(e.target.value));
  if (!Number.isFinite(v) || v < 1) return;
  for (const x of edItems()) pending[edKind].set(x.id, Math.min(v, levelCap()));
  e.target.value = "";
  renderEditor();
});
function renderPending() {
  const parts = [];
  const lv = (map, what) => {
    if (!map.size) return;
    const owned = ed ? ed.player[what] || {} : {};
    let add = 0, level = 0;
    for (const [id, l] of map) { if (!owned[id]) add++; if (l !== 0) level++; }
    const bits = [];
    if (add) bits.push(`${add} to own`);
    if (level) bits.push(`${level} to level`);
    parts.push(`${{ costumes: "Costumes", weapons: "Weapons", companions: "Companions" }[what]}: ${bits.join(", ")}`);
  };
  lv(pending.costumes, "costumes");
  lv(pending.weapons, "weapons");
  lv(pending.companions, "companions");
  if (pending.materials.size) parts.push(`Materials: ${pending.materials.size}`);
  if (pending.consumables.size) parts.push(`Items: ${pending.consumables.size}`);
  const names = { freeGems: "Gems", paidGems: "Paid gems", gold: "Gold" };
  for (const [k, v] of Object.entries(pending.amounts)) parts.push(`${names[k]}: ${v.toLocaleString("en-US")}`);
  document.getElementById("ed-pending").textContent = parts.length ? parts.join(" · ") : "None yet.";
  document.getElementById("ed-apply").disabled = !parts.length;
  document.getElementById("ed-discard").disabled = !parts.length;
}
document.getElementById("ed-discard").addEventListener("click", () => {
  pending = freshPending();
  renderEditor();
  savesMsg("");
});
document.getElementById("ed-apply").addEventListener("click", async () => {
  if (!ed || !edPendingCount()) return;
  const name = slotName(ed.id);
  if (!(await ask("Apply the changes", `Change "${name}": ${document.getElementById("ed-pending").textContent}. A copy of the save is kept first (save\\slot-backups).`, { ok: "Apply" }))) return;
  const list = (map) => [...map].map(([id, level]) => ({ id, level }));
  const edit = {
    costumes: list(pending.costumes), weapons: list(pending.weapons), companions: list(pending.companions),
    materials: Object.fromEntries(pending.materials), consumables: Object.fromEntries(pending.consumables),
    ...pending.amounts,
  };
  const r = await savesDo({ op: "apply", id: ed.id, edit }, (r) =>
    `Applied to "${name}": ${r.result.granted} new, ${r.result.leveled} leveled, ${r.result.items} amounts set. A copy from before: ${r.backup}.`);
  if (!r) return;
  ed.player = r.player;
  pending = freshPending();
  renderEditor();
  savesCall({ op: "list" });
});
document.querySelector('#subtabs [data-sub="saves"]').addEventListener("click", () => savesCall({ op: "list" }));
renderEditor();
savesCall({ op: "list" });

// ---- pictures ------------------------------------------------------------------------
// The server makes them from the player's own game files (nier-server --admin
// "images", internal/pcsave/images.go) into launcher\ui\game\icons, once; a key
// is "kind/name" and its picture is game/icons/<key>.png. Nothing of the game
// ships with the port.
const picHave = new Set();
const picAsked = new Set();
function picImg(key, alt = "") {
  return key && picHave.has(key)
    ? `<img src="game/icons/${escapeHtml(key)}.png" alt="${escapeHtml(alt)}" loading="lazy" draggable="false">`
    : `<span class="nopic">${escapeHtml(alt.slice(0, 1))}</span>`;
}
// Asks for the pictures not asked for yet; then() runs once they are there.
async function wantPics(keys, then) {
  const todo = [...new Set(keys.filter((k) => k && !picAsked.has(k)))];
  if (!todo.length) return;
  todo.forEach((k) => picAsked.add(k));
  const r = await savesCall({ op: "images", keys: todo });
  if (r.ok) {
    r.have.forEach((k) => picHave.add(k));
    then();
  } else {
    todo.forEach((k) => picAsked.delete(k));
  }
}

// ---- Settings › Events --------------------------------------------------------------
// Which summons and events are open: save\active-content.json, written by the
// server ("content-set"); everything is closed until the player opens it. The
// server lists the choices with their names and pictures ("content-list").
const ct = { loaded: false, banners: [], events: [], saveTimer: 0 };
const CT = {
  banners: { box: document.getElementById("ct-banners"), id: (x) => x.key, group: (x) => x.kind,
    sub: (x) => [x.kind, x.featured && x.featured.length ? "Featured: " + x.featured.join(", ") : "", x.note || ""].filter(Boolean).join(" · "),
    name: (x) => x.name || (x.featured && x.featured.length ? x.featured.join(", ") : `Summon ${x.id}`) },
  events: { box: document.getElementById("ct-events"), id: (x) => String(x.id), group: (x) => x.group,
    sub: (x) => x.group, name: (x) => x.name || `Event ${x.id}` },
};
// What a search matches: the shown name, a summon's featured characters (the
// name of a summon with no title), and the id.
CT.banners.find = (x) => [CT.banners.name(x), ...(x.featured || []), x.key, String(x.id)].join(" | ").toLowerCase();
CT.events.find = (x) => [CT.events.name(x), String(x.id)].join(" | ").toLowerCase();
function ctMsg(text, error = false) {
  const line = document.getElementById("ct-msg");
  line.textContent = text || "";
  line.classList.toggle("alert", !!error);
}
async function ctLoad() {
  if (!native) return ctMsg("Open the launcher (NieR.exe) to choose summons and events.", true);
  ctMsg("Reading the summons and events...");
  const r = await savesCall({ op: "content-list" });
  if (!r.ok) return ctMsg(r.error || "The list could not be read.", true);
  ct.loaded = true;
  ct.banners = r.banners || [];
  ct.events = r.events || [];
  for (const kind of ["banners", "events"]) {
    const sel = CT[kind].box.querySelector(".ct-group");
    const groups = [...new Set(ct[kind].map(CT[kind].group))];
    sel.innerHTML = '<option value="">Every kind</option>' + groups.map((g) => `<option>${escapeHtml(g)}</option>`).join("");
  }
  ctRender();
  const keys = [...ct.banners, ...ct.events].map((x) => x.img);
  if (keys.some((k) => k && !picAsked.has(k))) ctMsg("Making the pictures from your game files (the first time takes a few seconds)...");
  await wantPics(keys, ctRender);
  ctMsg(ctSummary());
}
function ctSummary() {
  const on = (list) => list.filter((x) => x.on).length;
  return `Open: ${on(ct.banners)} of ${ct.banners.length} summons, ${on(ct.events)} of ${ct.events.length} events.`;
}
function ctShown(kind) {
  const box = CT[kind].box;
  const q = box.querySelector(".ct-search").value.trim().toLowerCase();
  const group = box.querySelector(".ct-group").value;
  const state = box.querySelector(".ct-state").value;
  return ct[kind].filter((x) =>
    (!group || CT[kind].group(x) === group) && (!state || (state === "on") === x.on) &&
    (!q || CT[kind].find(x).includes(q)));
}
function ctRender() {
  for (const kind of ["banners", "events"]) {
    const c = CT[kind];
    const shown = ctShown(kind);
    c.box.querySelector("[data-list]").innerHTML = shown.map((x) => {
      const name = c.name(x);
      // A permanent summon (x.always): open whatever the choice, the toggle disabled.
      const always = x.always ? " (always on: one of the game's permanent summons)" : "";
      return `<button class="pic${x.on ? " on" : ""}${x.always ? " always" : ""}${x.name ? "" : " unnamed"}" data-id="${escapeHtml(c.id(x))}"${x.always ? " disabled" : ""} title="${escapeHtml(name + always + "\n" + c.sub(x))}">
        <span class="frame">${picImg(x.img, name)}<i class="state">${x.always ? "Always on" : x.on ? "Open" : "Closed"}</i></span>
        <span class="t-name">${escapeHtml(name)}</span><span class="t-sub">${escapeHtml(c.sub(x))}</span></button>`;
    }).join("") || '<p class="more">Nothing matches.</p>';
    c.box.querySelector(".ct-count").textContent =
      `${shown.length} shown · ${ct[kind].filter((x) => x.on).length} of ${ct[kind].length} open`;
  }
}
// Each change is kept at once (a moment after the last click).
function ctSave() {
  clearTimeout(ct.saveTimer);
  ctMsg("Saving...");
  ct.saveTimer = setTimeout(async () => {
    const r = await savesCall({
      op: "content-set",
      banners: ct.banners.filter((x) => x.on && !x.always).map((x) => x.key),
      events: ct.events.filter((x) => x.on).map((x) => x.id),
    });
    if (!r.ok) ctMsg(r.error || "The choice could not be saved.", true);
    else ctMsg("Saved. " + ctSummary() + (gameRunning ? " Events change the next time the game starts." : ""));
  }, 600);
}
for (const kind of ["banners", "events"]) {
  const box = CT[kind].box;
  box.querySelector("[data-list]").addEventListener("click", (e) => {
    const b = e.target.closest(".pic[data-id]");
    if (!b) return;
    const x = ct[kind].find((y) => CT[kind].id(y) === b.dataset.id);
    if (!x || x.always) return;  // a permanent summon: the server keeps it open
    x.on = !x.on;
    ctRender();
    ctSave();
  });
  box.querySelectorAll("[data-ct]").forEach((b) => b.addEventListener("click", () => {
    const shown = ctShown(kind);
    if (!shown.length) return;
    for (const x of shown) if (!x.always) x.on = b.dataset.ct === "on";
    ctRender();
    ctSave();
  }));
  let timer = 0;
  box.querySelector(".ct-search").addEventListener("input", () => { clearTimeout(timer); timer = setTimeout(ctRender, 150); });
  box.querySelectorAll("select").forEach((s) => s.addEventListener("change", ctRender));
}
// The Events page (a page of its own in the tab bar) reads the list when it first opens: go().

// #settings, #events or #controls opens that page (and a page to preview).
// #controls/battle opens that page on that screen.
{
  const [page, screen] = location.hash.slice(1).split("/");
  if (SCENES[screen]) scene = screen;
  go(["settings", "events", "controls"].includes(page) ? page : "home");
}
render();
send("load");
send("info");
