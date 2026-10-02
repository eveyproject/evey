// ThemeBridge API run in the theme's frame page (/plugin-frame.html) before theme.js.
// Only evey.theme.css(rules) and evey.theme.log(level, msg) are functional in V1.
// registerComponent / observe / transition are acknowledged but not implemented
// (parent replies with an error message so authors know the boundary).

export interface ThemeBridge {
  css(rules: string): void;
  log(level: "debug" | "info" | "warn" | "error", msg: string): void;
  registerComponent(slotName: string, render: () => void): void;
  observe(stateKey: string, cb: (value: unknown) => void): void;
}

// THEME_BRIDGE_SCRIPT runs in the theme's frame page right before theme.js
// loads. It defines window.evey.theme, then signals readiness to the parent.
// The script intentionally does NOT reference any globals (document, fetch,
// localStorage) - those are unavailable in a sandbox="allow-scripts" iframe
// and should remain inaccessible to theme code.
export const THEME_BRIDGE_SCRIPT = `
(function() {
  'use strict';
  window.evey = window.evey || {};
  window.evey.theme = {
    // css(rules) - inject extra CSS into the parent document.
    // rules is a plain CSS string. The parent strips <script> tags,
    // expression() calls and javascript: URIs before injecting.
    css: function(rules) {
      if (typeof rules !== 'string') return;
      window.parent.postMessage({ type: 'theme.css', rules: rules }, '*');
    },
    // log(level, msg) - forward a log line to parent for dev-mode display.
    log: function(level, msg) {
      if (typeof msg !== 'string') return;
      var safeLevel = (level === 'debug' || level === 'info' || level === 'warn' || level === 'error') ? level : 'info';
      window.parent.postMessage({ type: 'theme.log', level: safeLevel, msg: String(msg).slice(0, 500) }, '*');
    },
    // registerComponent - not implemented in V1.
    registerComponent: function(slotName) {
      window.parent.postMessage({ type: 'theme.registerComponent', slot: slotName, error: 'not implemented' }, '*');
    },
    // observe - not implemented in V1.
    observe: function(stateKey) {
      window.parent.postMessage({ type: 'theme.observe', key: stateKey, error: 'not implemented' }, '*');
    },
  };
})();
`;
