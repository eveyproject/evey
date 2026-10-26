const { withMainActivity } = require("expo/config-plugins");

// Drops the window's own background once the React root is in place: every
// screen paints the theme background itself (the Stack contentStyle), so the
// window's was one more opaque full-screen layer and the overdraw set the
// frame time on a TV box. Before super.onCreate it still serves as the launch background.
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
