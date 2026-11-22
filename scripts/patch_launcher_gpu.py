"""The launcher's renderer choices for every GPU: Vulkan (the GPU itself), with
a compatibility mode for drivers newer than ANGLE knows (auto: on for the
RTX 50 series), or software rendering on any PC. Direct3D 11 is gone: with
this game ANGLE's Direct3D 11 device is lost at the first textures."""
import sys

html, js = sys.argv[1], sys.argv[2]


def edit(path, pairs):
    s = open(path, encoding='utf-8').read()
    for old, new in pairs:
        assert s.count(old) == 1, (path, old[:70], s.count(old))
        s = s.replace(old, new)
    open(path, 'w', encoding='utf-8').write(s)


edit(html, [
    ('''            <h3 class="bar">Renderer</h3>
            <div class="choice grid" data-key="graphics.backend">
              <button data-value="vulkan">Vulkan</button><button data-value="d3d11">Direct3D 11</button>
            </div>''', '''            <h3 class="bar">Renderer</h3>
            <div class="choice grid" data-key="graphics.backend">
              <button data-value="vulkan">Vulkan</button><button data-value="swiftshader">Software</button>
            </div>
            <p class="lead">Vulkan draws on your graphics card. Software works on any PC but is slow: use it only
              if nothing shows with Vulkan.</p>
            <h3 class="bar">Vulkan compatibility</h3>
            <div class="choice grid" data-key="graphics.vkCompat">
              <button data-value="auto">Auto</button><button data-value="on">On</button><button data-value="off">Off</button>
            </div>
            <p class="lead">For the newest graphics cards and drivers (3D black while menus show).
              Auto turns it on for the GeForce RTX 50 series.</p>'''),
])

edit(js, [
    ('''  graphics: { backend: "vulkan", adapter: "nvidia", quality: "", aa: "fxaa" },''',
     '''  graphics: { backend: "vulkan", adapter: "nvidia", quality: "", aa: "fxaa", vkCompat: "auto" },'''),
    ('''    if ((settings.version || 1) < 2) settings.controls.move.radius = DEFAULTS.controls.move.radius;  // v2: a longer run drag''',
     '''    if ((settings.version || 1) < 2) settings.controls.move.radius = DEFAULTS.controls.move.radius;  // v2: a longer run drag
    if (!["vulkan", "swiftshader"].includes(settings.graphics.backend)) {  // Direct3D 11 is gone
      settings.graphics.backend = "vulkan";
      save();
    }'''),
    ('''  if (settings.graphics.aa === "fxaa") env.WINAPK_FXAA = "1";''',
     '''  if (settings.graphics.aa === "fxaa") env.WINAPK_FXAA = "1";
  env.WINAPK_VK_COMPAT = settings.graphics.vkCompat || "auto";'''),
])
print('ok')
