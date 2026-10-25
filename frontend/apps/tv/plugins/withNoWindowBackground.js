const { withMainActivity } = require("expo/config-plugins");

// Drops the window's own background once the React root is in place. Every
// screen paints the theme background itself (the Stack contentStyle), so the
// window's was one more opaque full-screen layer under all of them: measured
// on a TV box with the overdraw debug view, the home stacked four of them and
// the GPU, not the CPU, set the frame time (2026-09-20).
//
// Before super.onCreate the theme background still shows as the launch
// background; it is only dropped after, when the root view covers the window.
function applyNoWindowBackground(src) {
  if (src.includes("window.setBackgroundDrawable(null)")) return src;
  const out = src.replace(
    /(\n(\s*)super\.onCreate\(null\)\n)/,
    `$1$2window.setBackgroundDrawable(null) // see plugins/withNoWindowBackground.js\n`,
  );
  if (out === src) {
    throw new Error("withNoWindowBackground: MainActivity.kt does not call super.onCreate(null)");
  }
  return out;
}

module.exports = function withNoWindowBackground(config) {
  return withMainActivity(config, (cfg) => {
    cfg.modResults.contents = applyNoWindowBackground(cfg.modResults.contents);
    return cfg;
  });
};
module.exports.applyNoWindowBackground = applyNoWindowBackground;
