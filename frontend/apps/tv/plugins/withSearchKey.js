const { withMainActivity } = require("expo/config-plugins");

// The remote's SEARCH button. React Native's TV key map (RN 0.83,
// ReactAndroidHWInputDeviceHelper) forwards select/menu/media/D-pad keys and
// NOTHING else, so KEYCODE_SEARCH never reaches JS - the app could not offer
// the one shortcut a viewer expects from that button. This forwards it as a
// plain device event the shell listens for (SEARCH_EVENT).
//
// Caveat worth keeping: on many Android TV devices the search button is
// swallowed by the system (it starts the Assistant) and the app never sees
// the key at all. Where it IS delivered, this is what carries it.
const SNIPPET = `
  // Evey: the remote's SEARCH button - see plugins/withSearchKey.js.
  override fun onKeyDown(keyCode: Int, event: android.view.KeyEvent): Boolean {
    if (keyCode == android.view.KeyEvent.KEYCODE_SEARCH) {
      // emitDeviceEvent, not getJSModule(RCTDeviceEventEmitter): under the
      // bridgeless runtime the JS-module proxy is not the delivery path and
      // the event never arrived (measured on the emulator, 2026-09-19).
      val ctx = (application as? com.facebook.react.ReactApplication)?.reactHost?.currentReactContext
      if (ctx == null) {
        android.util.Log.w("EveyTV", "remote search key: no react context")
        return true
      }
      ctx.emitDeviceEvent("eveyRemoteSearch")
      return true
    }
    return super.onKeyDown(keyCode, event)
  }
`;

module.exports = function withSearchKey(config) {
  return withMainActivity(config, (cfg) => {
    cfg.modResults.contents = applySearchKeyToMainActivity(cfg.modResults.contents);
    return cfg;
  });
};

function applySearchKeyToMainActivity(src) {
  if (src.includes("KEYCODE_SEARCH")) return src;
  const close = src.lastIndexOf("}");
  return src.slice(0, close) + SNIPPET + src.slice(close);
}
module.exports.applySearchKeyToMainActivity = applySearchKeyToMainActivity;
