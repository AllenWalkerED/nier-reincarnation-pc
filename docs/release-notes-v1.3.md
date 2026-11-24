# NieR Re[in]carnation PC port 1.3

Bug squashing round, mostly stuff you reported in the issues. Thanks for the logs, they helped a ton.

## Fixed

* **Picture stuck in a corner with Render resolution on** (#2). The game's quality presets ask for a
  fixed size (720p or 1080p), so on big or scaled screens it drew a small picture in the corner. Now,
  when you pick a Render resolution, every preset renders at your actual window size, and anything
  that still comes out smaller gets stretched to fill the screen. Bonus: that's native-res rendering,
  so it's way sharper on 1440p/4K (#6).
* **Crash when you minimize the game** (#4). On Vulkan with NVIDIA, minimizing killed the game inside
  the driver. Fixed.
* **"Server error" / game never connects** (#1, #5). Something else on your PC (looking at you, Razer
  Synapse) was sitting on port 8080, so the server couldn't start. The launcher now tells you exactly
  which app is hogging the port, so you can close it.
* **Long install folder broke the code translation** (#8). The temp files went past Windows' path
  limit and the game was stuck on the slow mode. They live in a short folder now.
* **Moving the game folder broke the game** (#8). It went looking for its files in the old spot. It
  finds them wherever the folder is now.
* Setup handles links a bit more gracefully, so one failed link doesn't stop the whole thing.

## Still on it

* The loading screen stopping at 60% after New Game on some newer AMD laptops (#4). Not cracked yet.
* The "Process failed" loop at 20% (#3): the server now logs every connection the game makes, so the
  next logs should show where it gets blocked.
* Linux / Steam Deck (#7) and the Mac version are in the works.

## Updating

Close the game and the launcher, unzip `NieR-Reincarnation-PC-v1.3.zip` over your game folder and say
yes to replacing files. Saves and settings stay, and the first start backs up your save by itself since
the server version changed. New here? Same zip, unzip it anywhere and check the README.

Needs Windows 10/11 64-bit, a Direct3D 11 or Vulkan GPU, ~25 GB free and 8 GB of RAM.

## Heads up

* The exes aren't signed, so SmartScreen or your antivirus might complain.
* Found a bug? Settings › System › **Export logs**, then attach the zip to an issue.
