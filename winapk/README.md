# The runtime: win.apk with this port's changes

`winapk-nier.patch` is every change this port makes to
[win.apk](https://github.com/israel2176/win.apk) (AGPL-3.0, based on Mac.apk), against commit
`640f38f06c4934664767abc365a43b3c6bd7dea3`:

```
git clone https://github.com/israel2176/win.apk.git
cd win.apk
git checkout 640f38f06c4934664767abc365a43b3c6bd7dea3
git apply --binary <this folder>/winapk-nier.patch
```

Then build it as win.apk's own README says (MSYS2 CLANG64: `cmake`, `windows/scripts/build_java.sh`).
The release's `runtime/` holds that build's `bin`, `lib`, `jre` and `clang`.

What it changes, by area:

* **arm64 guest:** signals delivered to arm64 handlers (the game's Boehm GC stops threads with them),
  signal-aware waits, guest stack bounds, ahead-of-time translation that keeps up while the game runs
  full screen.
* **bionic / linker:** `/sys` and `/proc` CPU files, per-allocation `mprotect`, inotify for Firebase,
  `<lib>.source` stubs.
* **JNI:** Android paths in strings both ways, UTF-16 strings, Firebase task callbacks across class
  loaders.
* **GL:** a real window surface presented and scaled directly, eglGetConfigAttrib details, ASTC decoded
  natively where the GPU has none (`host/gl/astc_host.*`, texture2ddecoder's decoder), external (OES)
  textures as 2D, heap buffers in the GLES bindings, FXAA at present, a frame-rate cap.
* **Video:** AVPro Video's frames through SurfaceTexture (bound as GLConsumer binds them, pooled,
  bottom-up BGRA uploads, drawn at each swap with the app's GL state kept).
* **Window and input:** full screen, keyboard and XInput mapped to touch from an input map, a controller
  mouse mode, a custom cursor.
