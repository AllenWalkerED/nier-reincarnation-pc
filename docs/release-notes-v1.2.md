# NieR Re[in]carnation PC port 1.2

Small one, but it fixes stuff people actually ran into.

## What's new

* **No more tiny picture in the corner.** Some of you got the game squished into the bottom-left
  corner instead of filling the window. Full screen now starts at full size right away, and if the
  game ever draws smaller than the window anyway, it gets stretched to fit.
* **The launcher picks your GPU for you.** First time you open it, it checks what's in your PC and
  sets things up to match: a proper graphics card gets Best quality at 60 fps, integrated graphics
  (handhelds, laptops without a dedicated GPU) get Medium at 30. If your settings pointed at a card you
  don't have, it fixes that too. You can still change everything in Settings.
* **PCs with two GPUs from the same brand** (like a Ryzen chip plus a Radeon card) now use the real
  graphics card instead of the integrated one.
* **Setup doesn't die on the touch stick anymore.** If hiding the on-screen stick fails, setup just
  keeps going and the game works fine, you'll only see the stick.

## Updating

Close the game and the launcher, unzip `NieR-Reincarnation-PC-v1.2.zip` over your game folder and say
yes to replacing files. Saves and settings stay. New here? Same zip, unzip it anywhere and check the
README.

Needs Windows 10/11 64-bit, a Direct3D 11 or Vulkan GPU, ~25 GB free and 8 GB of RAM.

A Mac version is in the works.

## Heads up

* The exes aren't signed, so SmartScreen or your antivirus might complain.
* Found a bug? Settings › System › **Export logs**, then attach the zip to an issue.
