import { useState } from "react";
import { View, Text, Pressable, StyleSheet } from "react-native";
import { useTranslation } from "react-i18next";
import { formatAudioTrackLabel, type TrackPick, type V1MediaStream, trackDisplayName } from "@evey/api";
import { useTheme } from "../providers/ThemeProvider";

interface Props {
  streams: V1MediaStream[];
  pick: TrackPick;
  onChange: (pick: TrackPick) => void;
}

type Choice = { key: string; label: string; selected: boolean; onPress: () => void };

// Audio and subtitle chosen BEFORE playback, as focusable pills under the
// action row. "Auto" is the player's own resolution, so an untouched page
// plays exactly what it always did. Mirrors the mobile TrackPickChips.
export function TrackPickRow({ streams, pick, onChange }: Props) {
  const { t, i18n } = useTranslation();
  const theme = useTheme();
  const [focused, setFocused] = useState<string | null>(null);
  const audio = streams.filter((s) => s.type === "MEDIA_STREAM_TYPE_AUDIO" && s.index !== undefined);
  const subtitles = streams.filter((s) => s.type === "MEDIA_STREAM_TYPE_SUBTITLE" && s.index !== undefined);
  if (audio.length < 2 && subtitles.length === 0) return null;

  const audioChoices: Choice[] = [
    { key: "auto", label: t("media.streams.auto"), selected: pick.audio === undefined, onPress: () => onChange({ ...pick, audio: undefined }) },
    ...audio.map((s) => ({
      key: String(s.index),
      label: formatAudioTrackLabel(s, t("media.track", { number: s.index }), i18n.language),
      selected: pick.audio === s.index,
      onPress: () => onChange({ ...pick, audio: s.index }),
    })),
  ];
  const subtitleChoices: Choice[] = [
    { key: "auto", label: t("media.streams.auto"), selected: pick.subtitle === undefined, onPress: () => onChange({ ...pick, subtitle: undefined }) },
    { key: "off", label: t("media.streams.none"), selected: pick.subtitle === "off", onPress: () => onChange({ ...pick, subtitle: "off" }) },
    ...subtitles.map((s) => ({
      key: String(s.index),
      label: `${trackDisplayName(s, t("media.track", { number: s.index }), i18n.language)}${s.isForced ? ` - ${t("media.streams.forced")}` : ""}`,
      selected: pick.subtitle === s.index,
      onPress: () => onChange({ ...pick, subtitle: s.index }),
    })),
  ];

  const row = (group: string, title: string, choices: Choice[]) => (
    <View style={styles.row}>
      <Text style={[styles.title, { color: theme.colors.textMuted }]}>{title}</Text>
      {choices.map((c) => {
        const id = `${group}:${c.key}`;
        return (
          <Pressable
            key={id}
            onPress={c.onPress}
            onFocus={() => setFocused(id)}
            onBlur={() => setFocused(null)}
            accessible={true}
            accessibilityRole="button"
            accessibilityState={{ selected: c.selected }}
            accessibilityLabel={`${title}: ${c.label}`}
            style={[
              styles.pill,
              {
                backgroundColor: c.selected ? theme.colors.accent : theme.colors.surface,
                borderColor: focused === id ? "#fff" : "transparent",
              },
            ]}
          >
            <Text style={[styles.label, { color: c.selected ? theme.colors.onAccent : theme.colors.text }]}>{c.label}</Text>
          </Pressable>
        );
      })}
    </View>
  );

  return (
    <View style={styles.block} testID="track-pick-row">
      {audio.length > 1 ? row("audio", t("media.streams.audio"), audioChoices) : null}
      {subtitles.length > 0 ? row("subtitle", t("media.streams.subtitles"), subtitleChoices) : null}
    </View>
  );
}

const styles = StyleSheet.create({
  block: { marginTop: 24, gap: 10 },
  row: { flexDirection: "row", alignItems: "center", flexWrap: "wrap", gap: 10 },
  title: { fontSize: 18, fontWeight: "600", marginRight: 8 },
  pill: { paddingHorizontal: 20, paddingVertical: 10, borderRadius: 8, borderWidth: 3 },
  label: { fontSize: 17, fontWeight: "600" },
});
