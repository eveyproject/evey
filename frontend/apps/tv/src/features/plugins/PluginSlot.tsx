import { useCallback, useEffect, useRef, useState } from "react";
import { View, Text, StyleSheet, useTVEventHandler, useWindowDimensions } from "react-native";
import { useFocusEffect } from "expo-router";
import { useTranslation } from "react-i18next";
import { resolveSlotEntries, usePluginList, usePluginSlotEntries, type SlotEntry } from "@evey/client";
import { PluginWebView } from "@evey/client/native/PluginWebView";
import { diskBundleCache } from "@evey/client/native/pluginBundleCache";
import { useTheme } from "../../providers/ThemeProvider";
import { useServerUrl } from "../../hooks/useServerUrl";
import { homeWidgetBudget } from "../../lib/tvLayout";
import { useRemoteActivity } from "../../hooks/useRemoteActivity";
import { FONT_SIZE } from "@evey/theme";

// The host (WebView, bridge, RPC, protocol rules) is @evey/client's; what is
// this app's is below: the television's slot budget, its focus rule, and
// the fact that plugin navigation is not wired on this surface yet.
export { usePluginSlotEntries };
export type { SlotEntry } from "@evey/client";

// The height the host recommends for a slot on this surface: the home's
// widget budget, so every home widget lands the same size without knowing
// where it runs; a plugin with an opinion still resizes.
export function suggestedSlotHeight(slot: string, panelHeight: number): number | undefined {
  if (slot !== "home.section") return undefined;
  return homeWidgetBudget(panelHeight);
}

// How long the remote stays untouched before a widget animates again.
const REMOTE_QUIET_MS = 1200;
// How long after a key the page has finished scrolling, before a widget
// checks whether it is still on screen.
const SCROLL_SETTLE_MS = 350;
// How long the remote stays untouched before a widget is first started.
const BOOT_IDLE_MS = 2500;

interface PluginSlotProps {
  /** Slot name declared in the plugin manifest (e.g. "home.section"). */
  name: string;
  /** Per-slot context forwarded as the init payload `context`. */
  context?: Record<string, unknown>;
  /** When set, only render the plugin with this id matching the slot. */
  pluginId?: string;
  /** Skip these plugins (the home renders them elsewhere, at their own position). */
  excludeRegistered?: Set<string>;
}

/**
 * TV parity with web's PluginSlot (ADR 0021, phase 1). Each matching plugin
 * gets its own WebView running the plugin's frontend bundle in isolation.
 * Slots are display-only here: a WebView does not take part in D-pad focus.
 */
export function PluginSlot({ name, context, pluginId, excludeRegistered }: PluginSlotProps) {
  const { data, isError } = usePluginList();
  const matches = resolveSlotEntries(data?.plugins, name, { pluginId, excludeRegistered });

  // A failed plugin LIST must not pose as "no plugins here" - same
  // failure-shown-as-empty contract as web/mobile, same i18n key as the
  // per-module load error.
  if (isError) return <PluginSlotError />;
  if (matches.length === 0) return null;

  return (
    <>
      {matches.map((m) => (
        <PluginSlotEntry key={m.pluginId} slot={name} entry={m} context={context} />
      ))}
    </>
  );
}

// One mounted plugin. The hooks the host needs (panel, theme, server) run
// here, not in PluginSlot: a slot with no plugin must cost the screen nothing.
// TV routes are simpler than web/mobile's nav surface; only home is wired to
// plugin slots today, so plugin-triggered navigation isn't supported yet -
// dropped (no onNavigate) rather than guessing an unsafe destination.
function PluginSlotEntry({ slot, entry, context }: { slot: string; entry: SlotEntry; context?: Record<string, unknown> }) {
  const theme = useTheme();
  const serverUrl = useServerUrl();
  const { height: windowHeight } = useWindowDimensions();
  // The screen under the player stays mounted, and so did its WebView, its
  // timers and its WebGL, sharing the GPU with the video. Left, the widget is
  // hidden and its frames held, not unmounted: tearing a WebView down as the
  // next screen loads, then booting its bundle and WebGL again on the way
  // back, cost both screens their opening.
  const [screenFocused, setScreenFocused] = useState(true);
  useFocusEffect(
    useCallback(() => {
      setScreenFocused(true);
      return () => setScreenFocused(false);
    }, []),
  );
  // While the remote moves the focus the widget's animation stands still: a
  // WebGL frame it draws is one the app's own frame waits for, which froze the
  // D-pad on a modest set. It resumes once the remote has been quiet for a moment.
  const remoteActive = useRemoteActivity(REMOTE_QUIET_MS);
  // Held frames are not enough: the app draws the WebView itself, on every
  // one of its own frames, wherever the WebView sits. Scrolled off screen (the
  // focus on a row below), it is taken out of the drawing altogether - kept
  // alive, so it comes back as it was - and its box keeps the page's height.
  const box = useRef<View>(null);
  const boxHeight = useRef(0);
  const [onScreen, setOnScreen] = useState(true);
  const settleTimer = useRef<ReturnType<typeof setTimeout> | null>(null);
  useTVEventHandler(
    useCallback(() => {
      if (settleTimer.current) clearTimeout(settleTimer.current);
      settleTimer.current = setTimeout(() => {
        box.current?.measureInWindow((_x, y, _w, h) => setOnScreen(y + h > 0 && y < windowHeight));
      }, SCROLL_SETTLE_MS);
    }, [windowHeight]),
  );
  useEffect(
    () => () => {
      if (settleTimer.current) clearTimeout(settleTimer.current);
    },
    [],
  );
  // A widget's first start boots a browser engine and its bundle: measured on
  // a modest set, seconds of frozen main thread right as the home appeared,
  // the moment the viewer starts moving. It waits for the remote to rest.
  const [booted, setBooted] = useState(false);
  useEffect(() => {
    if (booted || remoteActive) return;
    const t = setTimeout(() => setBooted(true), BOOT_IDLE_MS);
    return () => clearTimeout(t);
  }, [booted, remoteActive]);
  const suggestedHeight = suggestedSlotHeight(slot, windowHeight);
  if (!booted) return <View style={{ height: suggestedHeight ?? 0 }} />;
  const shown = screenFocused && onScreen;
  const framesHeld = remoteActive || !shown;
  return (
    <View
      ref={box}
      collapsable={false}
      style={shown ? undefined : { height: boxHeight.current || suggestedHeight || 0 }}
      onLayout={(e) => {
        if (shown) boxHeight.current = e.nativeEvent.layout.height;
      }}
    >
      <View style={shown ? undefined : { display: "none" }}>
        <PluginWebView
          pluginId={entry.pluginId}
          slot={slot}
          entryPath={entry.entryPath}
          externalFetch={entry.externalFetch}
          context={context}
          surface="tv"
          serverUrl={serverUrl}
          theme={{ text: theme.colors.text, textMuted: theme.colors.textMuted, accent: theme.colors.accent, bg: theme.colors.bg }}
          suggestedHeight={suggestedHeight}
          viewportHeight={Math.round(windowHeight)}
          focusable={false}
          framesHeld={framesHeld}
          bundleCache={diskBundleCache}
        />
      </View>
    </View>
  );
}

function PluginSlotError() {
  const { t } = useTranslation();
  const theme = useTheme();
  return (
    <View style={styles.error}>
      <Text style={{ color: theme.colors.textMuted, fontSize: FONT_SIZE.xs }}>
        {t("admin.plugin.frontLoadError", "Failed to load plugin")}
      </Text>
    </View>
  );
}

const styles = StyleSheet.create({
  error: {
    paddingVertical: 8,
    paddingHorizontal: 16,
  },
});
