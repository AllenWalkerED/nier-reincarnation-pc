# NieR Re[in]carnation PC port 1.0

First public release. Your own copy of NieR Re[in]carnation, running natively on Windows: no
emulator, no internet, with a local [Lunar Tear](https://github.com/Walter-Sparrow/lunar-tear) server doing the job
the official one used to.

**The game isn't included.** You'll need the arm64 APK (3.7.1-152), `resource_dump_android.7z` and
`20240404193219.bin.e`. The last two are on the Lunar Tear Discord: https://discord.gg/MZAf5aVkJG.
The APK, Google it.

## How to play

1. Download `NieR-Reincarnation-PC-v1.0.zip` below and unzip it anywhere.
2. Run `NieR.exe` and give it your three files. The first setup unpacks ~21 GB, so it takes a bit.
3. Hit START. The first session or two run slower while the game's code gets translated; then it's smooth.

Needs Windows 10/11 64-bit, a Direct3D 11 or Vulkan GPU, ~25 GB free, and ideally 16 GB RAM.

Tested on RTX 3060/3080/4060/5060, RX 580 and Intel integrated graphics, with long sessions from
several testers.

## Heads up

* The exes aren't signed, so SmartScreen or your antivirus might complain.
* Found a bug? Settings › System › **Export logs**, then attach the zip to an issue. No game files in
  issues, please.

The PC port is my work, built on [Lunar Tear](https://github.com/Walter-Sparrow/lunar-tear),
[win.apk](https://github.com/israel2176/win.apk) and [Bunker](https://github.com/veritr1x/bunker)'s patch
modules and server changes. Full credits are in the README.
