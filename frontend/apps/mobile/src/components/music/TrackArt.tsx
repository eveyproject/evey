import { useState } from "react";
import { View, StyleSheet } from "react-native";
import { Image } from "expo-image";
import Ionicons from "@expo/vector-icons/Ionicons";
import { useTheme } from "../../providers/ThemeProvider";

// Per-track cover art, mirroring the web app's TrackArt: try the dedicated
// track-art endpoint, fall back to a placeholder icon on 404/error. Shared
// by the music screen's tracks tab and the playlists tab.
export function TrackArt({
  trackID,
  size,
  authHeaders,
  serverUrl,
  theme,
}: {
  trackID: string;
  size: number;
  authHeaders: { Authorization: string } | undefined;
  serverUrl: string | undefined;
  theme: ReturnType<typeof useTheme>;
}) {
  const [error, setError] = useState(false);
  const uri = serverUrl ? `${serverUrl}/api/v1/music/tracks/${trackID}/art` : undefined;
  if (error || !uri) {
    return (
      <View
        style={[
          styles.placeholder,
          { width: size, height: size, borderRadius: theme.radii.sm, backgroundColor: theme.colors.surfaceRaised },
        ]}
      >
        <Ionicons name="musical-notes" size={size * 0.5} color={theme.colors.textMuted} />
      </View>
    );
  }
  return (
    <Image
      cachePolicy="memory-disk"
      source={{ uri, headers: authHeaders }}
      style={{ width: size, height: size, borderRadius: theme.radii.sm }}
      contentFit="cover"
      onError={() => setError(true)}
    />
  );
}

const styles = StyleSheet.create({
  placeholder: { justifyContent: "center", alignItems: "center" },
});
