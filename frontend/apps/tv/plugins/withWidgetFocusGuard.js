const { withMainActivity } = require("expo/config-plugins");

// Plugin widgets must never stand in the D-pad's way. Every WebView in the TV
// app is a plugin's home widget (PluginSlot), display-only by design - but an
// Android WebView is focusable by default and React Native's `focusable` prop
// only exists on plain Views (ReactViewManager), so `focusable={false}` on a
// <WebView> does nothing.
//
// Measured on the emulator (2026-09-19), pressing UP from the first poster
// row: the focus lands INSIDE the weather widget, which draws no focus ring of
// its own and hands the arrows to the web page, and the next press jumps
// straight to the header - so the Customize row sitting between the two is
// skipped, which is how "Personnaliser" became unreachable from the sofa.
//
// So the focus flag is cleared before each key is resolved, rather than once
// the widget has taken the focus (too late - it already has it).
// dispatchKeyEvent is the hook because it runs ahead of every key the window
// handles and has no observer lifetime to get wrong: registered from
// onAttachedToWindow, an OnGlobalLayoutListener on the decor view fired
// exactly once, before the widget existed. The walk stops at each widget and
// does nothing once one is cleared.
const SNIPPET = `
  // Evey: plugin widgets never stand in the D-pad's way - see plugins/withWidgetFocusGuard.js.
  override fun dispatchKeyEvent(event: android.view.KeyEvent): Boolean {
    if (event.action == android.view.KeyEvent.ACTION_DOWN) eveyClearWidgetFocus(window.decorView)
    return super.dispatchKeyEvent(event)
  }

  private fun eveyClearWidgetFocus(view: android.view.View) {
    if (view is android.webkit.WebView) {
      if (view.isFocusable) {
        view.isFocusable = false
        view.isFocusableInTouchMode = false
        view.descendantFocusability = android.view.ViewGroup.FOCUS_BLOCK_DESCENDANTS
      }
      return
    }
    if (view is android.view.ViewGroup) {
      for (i in 0 until view.childCount) eveyClearWidgetFocus(view.getChildAt(i))
    }
  }
`;

module.exports = function withWidgetFocusGuard(config) {
  return withMainActivity(config, (cfg) => {
    cfg.modResults.contents = applyWidgetFocusGuardToMainActivity(cfg.modResults.contents);
    return cfg;
  });
};

function applyWidgetFocusGuardToMainActivity(src) {
  if (src.includes("eveyClearWidgetFocus")) return src;
  const close = src.lastIndexOf("}");
  return src.slice(0, close) + SNIPPET + src.slice(close);
}
module.exports.applyWidgetFocusGuardToMainActivity = applyWidgetFocusGuardToMainActivity;
