const { withMainApplication, withMainActivity } = require("expo/config-plugins");

// UI scale for the 10-foot UI: the app's density scaled by a percentage the
// user picks in Settings (the shared EveyMediaCaps module persists it under
// the preference below; a change restarts the app). Scaling the density
// shrinks every layout uniformly - 55 screens of hard-coded dp untouched.
//
// React Native 0.83 (Fabric) converts dp to px from THREE places, and all
// three must agree or text and boxes drift apart:
//   1. the Activity's resources (ReactSurfaceImpl.setLayoutConstraints reads
//      context.resources.displayMetrics.density for the C++ layout) - covered
//      by an override configuration in MainActivity.attachBaseContext;
//   2. the Application's DisplayMetrics object (DisplayMetricsHolder's window
//      metrics) - mutated in place by applyUiScale();
//   3. DisplayMetricsHolder's SCREEN metrics, rebuilt from the real display by
//      ReactRootView.init() (PixelUtil.toPixelFromDIP/SP read these) - mutated
//      by applyUiScale() once they exist, i.e. right after super.onCreate in
//      MainActivity, and again after a configuration change.
//
// defaultPercent: what a fresh install gets before the user touches the
// setting (the owner found the stock 100 % too large on a 1080p panel).
const PREFS = "evey_ui";
const KEY = "scale";

function applicationSnippet(defaultPercent) {
  return `
  // Evey UI scale - see plugins/withUiScale.js (the source of this block).
  fun uiScalePercent(): Int =
    getSharedPreferences("${PREFS}", MODE_PRIVATE).getInt("${KEY}", ${defaultPercent}).coerceIn(50, 150)

  fun applyUiScale() {
    val percent = uiScalePercent()
    if (percent == 100) return
    val config = resources.configuration
    val scaledDpi = config.densityDpi * percent / 100
    val scale = { dm: android.util.DisplayMetrics ->
      dm.densityDpi = scaledDpi
      dm.density = scaledDpi / 160f
      dm.scaledDensity = dm.density * config.fontScale
    }
    scale(resources.displayMetrics)
    try {
      scale(com.facebook.react.uimanager.DisplayMetricsHolder.getScreenDisplayMetrics())
      scale(com.facebook.react.uimanager.DisplayMetricsHolder.getWindowDisplayMetrics())
    } catch (e: IllegalStateException) {
      // React Native not initialised yet: its init copies the application
      // metrics scaled above; the screen metrics are scaled by the Activity
      // hook once the root view exists.
    }
  }
`;
}

const activityAttachSnippet = `
  // Evey UI scale - see plugins/withUiScale.js.
  override fun attachBaseContext(newBase: android.content.Context) {
    super.attachBaseContext(newBase)
    val percent = (newBase.applicationContext as? MainApplication)?.uiScalePercent() ?: 100
    if (percent != 100) {
      val override = android.content.res.Configuration()
      override.densityDpi = newBase.resources.configuration.densityDpi * percent / 100
      applyOverrideConfiguration(override)
    }
  }

  override fun onConfigurationChanged(newConfig: android.content.res.Configuration) {
    super.onConfigurationChanged(newConfig)
    (application as? MainApplication)?.applyUiScale()
  }
`;

module.exports = function withUiScale(config, props = {}) {
  const defaultPercent = Number.isInteger(props.defaultPercent) ? props.defaultPercent : 100;
  config = withMainApplication(config, (cfg) => {
    cfg.modResults.contents = applyUiScaleToMainApplication(cfg.modResults.contents, defaultPercent);
    return cfg;
  });
  return withMainActivity(config, (cfg) => {
    cfg.modResults.contents = applyUiScaleToMainActivity(cfg.modResults.contents);
    return cfg;
  });
};

// Both transforms are exported so the same patch can be run on an existing
// android/ project without a full prebuild (the tv android dir is not
// versioned). Idempotent: a file already carrying the block is returned as is.
function applyUiScaleToMainApplication(src, defaultPercent) {
  if (src.includes("fun applyUiScale()")) return src;
  let out = src.replace(
    /(\n\s*override fun onCreate\(\) \{\n\s*super\.onCreate\(\)\n)/,
    `$1    applyUiScale()\n`,
  );
  out = out.replace(
    /(override fun onConfigurationChanged\(newConfig: Configuration\) \{\n\s*super\.onConfigurationChanged\(newConfig\)\n)/,
    `$1    applyUiScale()\n`,
  );
  if (out.split("applyUiScale()").length !== 3) {
    throw new Error("withUiScale: MainApplication.kt does not have the expected onCreate/onConfigurationChanged shape");
  }
  const close = out.lastIndexOf("}");
  return out.slice(0, close) + applicationSnippet(defaultPercent) + out.slice(close);
}

function applyUiScaleToMainActivity(src) {
  if (src.includes("applyUiScale()")) return src;
  // Before super.onCreate: the application metrics (React Native's window
  // metrics copy them); after: the screen metrics the root view just rebuilt.
  let out = src.replace(
    /(\n\s*override fun onCreate\(savedInstanceState: Bundle\?\) \{\n)/,
    `$1    (application as? MainApplication)?.applyUiScale() // Evey UI scale, see plugins/withUiScale.js\n`,
  );
  out = out.replace(/(\n\s*super\.onCreate\(null\)\n)/, `$1    (application as? MainApplication)?.applyUiScale()\n`);
  if (out.split("applyUiScale()").length !== 3) {
    throw new Error("withUiScale: MainActivity.kt does not have the expected onCreate shape");
  }
  const close = out.lastIndexOf("}");
  return out.slice(0, close) + activityAttachSnippet + out.slice(close);
}
module.exports.applyUiScaleToMainApplication = applyUiScaleToMainApplication;
module.exports.applyUiScaleToMainActivity = applyUiScaleToMainActivity;
