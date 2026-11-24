# NieR Re[in]carnation PC port 1.3.1

Small one, two fixes.

## Fixed

* **The game was hoarding master data.** Every time you hit START, the server made the game think
  there was new game data, so it downloaded it again and kept another 7.7 MB copy around. Play long
  enough and that's gigabytes of the same file. Now it only downloads when the data actually changes
  (like when you change Events or Mods). The old copies are safe to delete, they live in
  `runtime-data\android\sdcard\Android\data\com.square_enix.android_googleplay.nierspww\cache` as
  `mst-...` files (keep the newest one).
* **Crash at 20% on Linux / Proton** (#9). The game's root check read a file that didn't open under
  Wine and took the whole thing down with it. It just gets a normal error now. The launcher on Proton
  still needs the WebView2 workaround from #9 for now.

## Updating

Close the game and the launcher, unzip `NieR-Reincarnation-PC-v1.3.1.zip` over your game folder and
say yes to replacing files. Saves and settings stay, and the first start backs up your save by itself
since the server version changed. New here? Same zip, unzip it anywhere and check the README.

Needs Windows 10/11 64-bit, a Direct3D 11 or Vulkan GPU, ~25 GB free and 8 GB of RAM.

## Still on it

* The loading screen stopping at 60% after New Game on some newer AMD CPUs (#4).
* The "Process failed" loop at 20% (#3).
* A launcher that works without WebView2 (#7), and the Mac version.

Found a bug? Settings › System › **Export logs**, then attach the zip to an issue.
