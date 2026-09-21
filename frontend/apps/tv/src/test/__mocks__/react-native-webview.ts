// react-native-webview ships source vite cannot parse; every screen that
// transitively renders a PluginSlot resolves here instead (same posture as
// vector-icon.ts). Renders nothing: slot behavior itself is covered by
// PluginSlot.test.tsx, whose richer per-file vi.mock takes precedence over
// this alias.
import React from "react";

export class WebView extends React.Component {
  render() {
    return null;
  }
}

export default WebView;
