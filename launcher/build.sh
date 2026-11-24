#!/usr/bin/env bash
# Builds NieR.exe (MSYS2 CLANG64): run from this folder.
set -euo pipefail
SDK=/d/tools/winapk/src/thirdparty/webview2-sdk
OUT=${OUT:-..}
windres src/nier.rc -O coff -o build/nier.res
clang++ -std=c++17 -O2 -municode -mwindows -I"$SDK/include" src/main.cpp build/nier.res \
  -o "$OUT/NieR.exe" -static -lole32 -ldwmapi -lws2_32 -lshell32 -luser32 -lgdi32 -luuid -ldxgi -ladvapi32 -liphlpapi
cp -f "$SDK/x64/WebView2Loader.dll" "$OUT/"
echo built "$OUT/NieR.exe"
